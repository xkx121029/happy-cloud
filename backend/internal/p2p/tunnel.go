// tunnel：一条已鉴权的 WebRTC DataChannel 隧道。负责握手鉴权(R 帧)、心跳、
// 应用层分帧/聚合分发，并将隧道内请求路由到 JSON 代理或专用传输处理。
package p2p

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/pion/webrtc/v4"

	"happy-cloud/backend/internal/middleware"
)

type Tunnel struct {
	s        *Service
	sess     *session
	dc       *webrtc.DataChannel
	sendMu   sync.Mutex
	done     chan struct{}
	asm      *assembler
	userID   uint
	username string
	token    string
	auth     bool

	mu      sync.Mutex
	streams map[uint32]*fileStream // 下载流（按 ID）
}

func newTunnel(sg *session) *Tunnel {
	return &Tunnel{
		s:       sg.s,
		sess:    sg,
		done:    make(chan struct{}),
		asm:     newAssembler(),
		streams: map[uint32]*fileStream{},
	}
}

func (t *Tunnel) run(dc *webrtc.DataChannel) {
	t.dc = dc
	log.Printf("[p2p] 隧道已建立，等待鉴权")
}

func (t *Tunnel) closed() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

func (t *Tunnel) send(env Envelope, bin []byte) {
	if t.closed() || t.dc == nil {
		return
	}
	for _, p := range frameEncode(env, bin) {
		if t.closed() {
			return
		}
		t.sendMu.Lock()
		err := t.dc.Send(p)
		t.sendMu.Unlock()
		if err != nil {
			log.Printf("[p2p] 隧道发送失败: %v", err)
			t.close()
			return
		}
	}
}

func (t *Tunnel) close() {
	select {
	case <-t.done:
		return
	default:
		close(t.done)
	}
	if t.auth {
		t.s.removeTunnel(t.userID, t)
	}
}

// handlePkt 处理来自 DataChannel 的单个 packet，聚合后分发逻辑消息
func (t *Tunnel) handlePkt(pkt []byte) {
	if t.closed() {
		return
	}
	stream, ok := t.asm.feed(pkt)
	if !ok {
		return
	}
	env, bin, ok := parseStream(stream)
	if !ok {
		return
	}

	if !t.auth {
		t.doAuthorize(env)
		return
	}
	t.dispatch(env, bin)
}

// doAuthorize 握手 R 帧：校验 Bearer token，成功则登记用户隧道
func (t *Tunnel) doAuthorize(env Envelope) {
	log.Printf("[p2p] 收到隧道首帧 t=%q id=%d", env.T, env.ID)
	if env.T != "R" {
		t.send(Envelope{T: "E", ID: env.ID, C: 401, Msg: "未鉴权"}, nil)
		t.close()
		return
	}
	var tok string
	if err := json.Unmarshal(env.B, &tok); err != nil || tok == "" {
		log.Printf("[p2p] 隧道握手帧缺少 token: %v", err)
		t.send(Envelope{T: "E", ID: env.ID, C: 401, Msg: "未登录或登录已过期"}, nil)
		t.close()
		return
	}
	claims, err := middleware.ParseToken(tok)
	if err != nil {
		log.Printf("[p2p] 隧道 token 校验失败: %v", err)
		t.send(Envelope{T: "E", ID: env.ID, C: 401, Msg: "未登录或登录已过期"}, nil)
		t.close()
		return
	}
	t.auth = true
	t.userID = claims.UserID
	t.username = claims.Username
	t.token = tok
	t.s.addTunnel(t)
	log.Printf("[p2p] 隧道鉴权通过 user=%s(id=%d)", t.username, t.userID)
	t.send(Envelope{T: "R", ID: env.ID, OK: true}, nil)
}

func (t *Tunnel) dispatch(env Envelope, bin []byte) {
	switch env.T {
	case "j":
		log.Printf("[p2p] 隧道 JSON 请求 %s %s", env.M, env.P)
		t.handleJSON(env)
	case "d": // 开启文件下载
		log.Printf("[p2p] 隧道下载请求 file=%d", env.DL)
		if t.sess.dead.Load() == 0 {
			t.handleDownloadOpen(env)
		}
	case "U": // 上传分片
		if env.IDX == 0 {
			log.Printf("[p2p] 隧道上传开始 hash=%s", env.Hash)
		}
		t.handleUploadChunk(env, bin)
	case "A": // 下载 ACK / 取消
		t.onStreamAck(env)
	case "E":
		t.onStreamErr(env)
	case "C": // 关闭流
		t.onStreamClose(env)
	case "P": // ping
		t.send(Envelope{T: "p", ID: env.ID, TS: env.TS}, nil)
	}
}