// transfer：隧道内大文件专用传输。
//   - 下载：客户端发 {t:"d",ID,dl:fileId}→服务端循环 D 帧(256KB)→客户端 A 帧 ACK 窗口背压→last 结束；A{code:2} 取消。
//   - 上传：客户端发 {t:"U",hash,idx} 分片→写 ChunkDir/userID/hash/idx→A 确认；合并走隧道内 JSON 代理 UploadMerge。
package p2p

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// FileMeta 下载元信息（首帧 D 携带，用于客户端展示文件名/大小）
type FileMeta struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

const (
	dlFrameSize = 256 * 1024 // 下载分片
	dlWindow    = 16         // ACK 令牌窗（约 4MB 在途）
)

type fileStream struct {
	id   uint32
	sem  chan struct{}
	done chan struct{}
	mu   sync.Mutex
}

func (t *Tunnel) getStream(id uint32) *fileStream {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.streams[id]
}

func (t *Tunnel) putStream(fs *fileStream) {
	t.mu.Lock()
	t.streams[fs.id] = fs
	t.mu.Unlock()
}

func (t *Tunnel) dropStream(fs *fileStream) {
	t.mu.Lock()
	if cur, ok := t.streams[fs.id]; ok && cur == fs {
		delete(t.streams, fs.id)
	}
	t.mu.Unlock()
}

// handleDownloadOpen 开启一个文件下载流（d 帧）
func (t *Tunnel) handleDownloadOpen(env Envelope) {
	if env.DL == 0 {
		t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		return
	}
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0 AND type = 1", env.DL, t.userID).First(&f).Error; err != nil {
		t.send(Envelope{T: "E", ID: env.ID, C: 404, Msg: "文件不存在"}, nil)
		return
	}
	path := util.BlobPathOf(&f)
	file, err := os.Open(path)
	if err != nil {
		t.send(Envelope{T: "E", ID: env.ID, C: 404, Msg: "文件实体缺失"}, nil)
		return
	}

	fs := &fileStream{id: env.ID, sem: make(chan struct{}, dlWindow), done: make(chan struct{})}
	t.putStream(fs)

	// 先回一个元信息帧：文件名与大小（客户端据此展示进度）
	t.send(Envelope{T: "D", ID: env.ID, S: f.Size, L: false, B: mustJSON(FileMeta{Name: f.Name, Size: f.Size})}, nil)

	go t.streamDownload(env.ID, file, fs, env.Off, env.Len)
}

func (t *Tunnel) streamDownload(id uint32, file *os.File, fs *fileStream, off, length int64) {
	defer func() {
		file.Close()
		t.send(Envelope{T: "D", ID: id, S: 0, L: true}, nil)
		t.dropStream(fs)
	}()
	buf := make([]byte, dlFrameSize)

	// 区间下载：seek 到起始位置
	if off > 0 {
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			t.send(Envelope{T: "E", ID: id, C: 416, Msg: "range not satisfiable"}, nil)
			return
		}
	}

	remaining := length
	for {
		// 背压：等待 ACK 释放令牌才继续发送
		select {
		case <-t.done:
			return
		case <-fs.done:
			return
		case fs.sem <- struct{}{}:
		}

		readSize := int64(len(buf))
		if remaining > 0 && readSize > remaining {
			readSize = remaining
		}
		n, err := file.Read(buf[:readSize])
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			t.send(Envelope{T: "D", ID: id, S: int64(n)}, chunk)
		}
		if remaining > 0 {
			remaining -= int64(n)
			if remaining <= 0 || err == io.EOF {
				return
			}
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
	}
}

// onStreamAck 下载 ACK：C=0 释放令牌；C=2 取消
func (t *Tunnel) onStreamAck(env Envelope) {
	fs := t.getStream(env.ID)
	if fs == nil {
		return
	}
	if env.C == 2 {
		select {
		case <-fs.done:
		default:
			close(fs.done)
		}
		return
	}
	select {
	case <-fs.sem:
	default:
	}
}

func (t *Tunnel) onStreamErr(env Envelope) {
	t.onStreamAck(env)
}

func (t *Tunnel) onStreamClose(env Envelope) {
	if fs := t.getStream(env.ID); fs != nil {
		select {
		case <-fs.done:
		default:
			close(fs.done)
		}
		t.dropStream(fs)
	}
}

// handleUploadChunk 写入一个上传分片（U 帧）
func (t *Tunnel) handleUploadChunk(env Envelope, bin []byte) {
	if env.Hash == "" {
		t.send(Envelope{T: "E", ID: env.ID, C: 400, Msg: "参数错误"}, nil)
		return
	}
	dir := util.ChunkDir(t.userID, env.Hash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.send(Envelope{T: "E", ID: env.ID, C: 500, Msg: "写入失败"}, nil)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(env.IDX)), bin, 0o644); err != nil {
		t.send(Envelope{T: "E", ID: env.ID, C: 500, Msg: "写入失败"}, nil)
		return
	}
	t.send(Envelope{T: "A", ID: env.ID, C: env.IDX, S: int64(len(bin))}, nil)
}