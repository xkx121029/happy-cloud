package p2p

import (
	"encoding/binary"
	"encoding/json"
	"strconv"
)

// fragLimit 单条 DataChannel 消息上限。由于浏览器端上限不一（Chromium 256KB、Firefox 16KB、
// Safari 更小），所有跨端传输都必须按此阈值分帧，接收端按 (t, id) 聚合重组。
const fragLimit = 16000

// SDP 值：信令中使用的会话描述
type SDP struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// Cand ICECandidate 值
type Cand struct {
	Candidate     string `json:"candidate"`
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex uint16 `json:"sdpMLineIndex"`
}

// Envelope 隧道帧信封（DataChannel 每条逻辑消息的解析头）
// t ∈ R(握手) P/p(心跳) j(JSON请求) J(JSON响应) d(下载开启) D(下载数据) U(上传分片) A(ack/取消) E(错误) C(关闭)
type Envelope struct {
	V      string            `json:"v,omitempty"`
	T      string            `json:"t"`      // 类型
	ID     uint32            `json:"id,omitempty"`  // 请求/流关联 id
	M      string            `json:"m,omitempty"`   // http 方法（j）
	P      string            `json:"p,omitempty"`   // http 路径（j）
	Q      map[string]string `json:"q,omitempty"`   // query（j）
	H      map[string]string `json:"h,omitempty"`   // headers（j）
	B      json.RawMessage   `json:"b,omitempty"`   // body（j/J 的 JSON body；R 持 token）
	S      int64             `json:"s,omitempty"`   // 大小（U 分片 size / 表示期待长度）
	C      int               `json:"c,omitempty"`   // ack/err 码 / 上传分片 idx
	Msg    string            `json:"msg,omitempty"` // 错误信息
	L      bool              `json:"l,omitempty"`   // 下载最后帧
	DL     uint64            `json:"dl,omitempty"`  // 下载文件 id
	Hash   string            `json:"hash,omitempty"` // 上传分片 hash
	IDX    int               `json:"idx,omitempty"` // 上传分片 index
	TOT    int               `json:"tot,omitempty"` // 上传总片数
	TS     int64             `json:"ts,omitempty"`  // ping/pong 时间戳
	OK     bool              `json:"ok,omitempty"`  // 握手成功
	Status int               `json:"status,omitempty"` // http 状态（J）
	F      int               `json:"f,omitempty"`     // 分片序号（重组用）
	FN     int               `json:"fn,omitempty"`    // 分片总数（重组用）
	SDP    *SDP              `json:"sdp,omitempty"`   // 信令 offer/answer
	Cand   *Cand             `json:"cand,omitempty"`  // 信令 ICE
	CID    string            `json:"cid,omitempty"`   // 信令客户端 id
	To     string            `json:"to,omitempty"`    // 信令目标（单播）
	Off    int64             `json:"off,omitempty"`   // 区间下载的起始字节偏移（>=0 时启用）
	Len    int64             `json:"len,omitempty"`   // 区间下载的长度上限（-1 = 到文件末尾，0 = 完整下载不指定）
}

// packet 单条 DataChannel 消息：header(4B lenFrag) + fragJSON + (4B lenChunk) + chunkBytes
func buildPacket(frag []byte, chunk []byte) []byte {
	buf := make([]byte, 0, 8+len(frag)+len(chunk))
	var hb [4]byte
	binary.BigEndian.PutUint32(hb[:], uint32(len(frag)))
	buf = append(buf, hb[:]...)
	buf = append(buf, frag...)
	binary.BigEndian.PutUint32(hb[:], uint32(len(chunk)))
	buf = append(buf, hb[:]...)
	buf = append(buf, chunk...)
	return buf
}

// frameEncode 将一条逻辑消息（env + bin）编码为若干 packet。
// 逻辑流统一为 [4B msgLen][msgJSON][bin]，按 fragLimit 分帧；SCTP 有序，接收端按 FN 计数聚合。
func frameEncode(env Envelope, bin []byte) [][]byte {
	msg, _ := json.Marshal(env)
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(msg)))
	stream := append(h[:], msg...)
	stream = append(stream, bin...)
	if len(stream) <= fragLimit {
		frag := Envelope{T: env.T, ID: env.ID}
		fj, _ := json.Marshal(frag)
		return [][]byte{buildPacket(fj, stream)}
	}
	n := (len(stream) + fragLimit - 1) / fragLimit
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		lo := i * fragLimit
		hi := lo + fragLimit
		if hi > len(stream) {
			hi = len(stream)
		}
		frag := Envelope{T: env.T, ID: env.ID, F: i, FN: n}
		fj, _ := json.Marshal(frag)
		out = append(out, buildPacket(fj, stream[lo:hi]))
	}
	return out
}

// fragments 按 (t,id) 聚合从 DataChannel 收到的 packet
type fragments struct {
	env    Envelope
	chunks [][]byte
	count  int
	need   int
}

// assembler 聚合器：收到 packet 后返回已完成的一条完整逻辑消息流
type assembler struct {
	partial map[string]*fragments // key: t|id
}

func newAssembler() *assembler { return &assembler{partial: map[string]*fragments{}} }

// feed 处理一个 packet，返回已聚合完成的逻辑消息流；未完成时返回 (nil, false)。
func (a *assembler) feed(pkt []byte) ([]byte, bool) {
	if len(pkt) < 8 {
		return nil, false
	}
	lj := int(binary.BigEndian.Uint32(pkt[0:4]))
	if lj < 0 || len(pkt) < 4+lj+4 {
		return nil, false
	}
	var frag Envelope
	if err := json.Unmarshal(pkt[4:4+lj], &frag); err != nil {
		return nil, false
	}
	lc := int(binary.BigEndian.Uint32(pkt[4+lj : 8+lj]))
	if len(pkt) < 8+lj+lc {
		return nil, false
	}
	chunk := pkt[8+lj : 8+lj+lc]

	// 非分片：chunk 即完整逻辑流（含 4B msgLen 前缀）
	if frag.FN == 0 {
		return chunk, true
	}

	key := frag.T + "|" + strconv.Itoa(int(frag.ID))
	f := a.partial[key]
	if f == nil {
		f = &fragments{env: frag, need: frag.FN, count: 0}
		a.partial[key] = f
	}
	f.chunks = append(f.chunks, chunk)
	f.count++
	if f.count >= f.need {
		stream := f.chunks[0]
		for _, c := range f.chunks[1:] {
			stream = append(stream, c...)
		}
		delete(a.partial, key)
		return stream, true
	}
	return nil, false
}

// parseStream 从完整逻辑流解析出信封与剩余二进制载荷
func parseStream(stream []byte) (Envelope, []byte, bool) {
	if len(stream) < 4 {
		return Envelope{}, nil, false
	}
	msgLen := int(binary.BigEndian.Uint32(stream[0:4]))
	if msgLen < 0 || len(stream) < 4+msgLen {
		return Envelope{}, nil, false
	}
	var env Envelope
	if err := json.Unmarshal(stream[4:4+msgLen], &env); err != nil {
		return Envelope{}, nil, false
	}
	return env, stream[4+msgLen:], true
}