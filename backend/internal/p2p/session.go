// session：后端作为 answerer 的一次 WebRTC 协商会话，负责一个客户端信号的
// offer/answer/ICE 处理 与 DataChannel(隧道) 的建立。
package p2p

import (
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"

	"happy-cloud/backend/config"
)

type session struct {
	s       *Service
	peerCID string // 信令对方客户端 id
	mu      sync.Mutex
	signal  func(m signalMsg) // 经信令向 peerCID 发消息（已含 To）
	pc      *webrtc.PeerConnection
	remote  *webrtc.SessionDescription
	tunnel  *Tunnel
	dead    atomic.Int32

	// DataChannel 就绪前收到的报文暂存在这里
	dcOpen bool
	buf    [][]byte
}

// 浏览器在 dc.onopen 里会立刻发送 R 握手帧，该帧可能早于本端 OnOpen 回调到达；
// 此时隧道还不能回包（pion 在未 open 的通道上 Send 会报错），因此把报文暂存，
// 等 OnOpen 后按原顺序回放。上限 8 条：握手阶段正常只有 1 条，多出来的说明异常。
const maxPendingPkts = 8

func newSession(s *Service) *session {
	return &session{s: s}
}

func (sg *session) ready() error {
	if sg.dead.Load() == 1 {
		return errClosed
	}
	return nil
}

func (sg *session) say(m signalMsg) {
	sg.mu.Lock()
	fn := sg.signal
	sg.mu.Unlock()
	if fn != nil {
		fn(m)
	}
}

// runOffer 处理收到的 offer：建立 answerer，完成 SDP/ICE 协商并等待 DataChannel。
func (sg *session) runOffer(sdp string) {
	if err := sg.ready(); err != nil {
		return
	}
	se := webrtc.SettingEngine{}
	// 强制使用 IP 形态的 host 候选（禁用 mDNS），保证局域网/容器环境可直连
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	// 容器网络边界修复：后端跑在 Docker 里时，pion 生成的 host 候选是容器内网地址
	// （如 172.18.0.5），宿主机与局域网内的浏览器都路由不到，导致 ICE 必然失败。
	// 这里把 host 候选的 IP 替换为宿主机可达地址，并把 UDP 端口固定成一段
	// 与 docker-compose 发布映射一致的端口，客户端才能主动连上来完成打洞。
	if ip := config.Cfg.AdvertiseIP; ip != "" {
		// 只给 External + AsCandidateType，即为通配规则；host 候选默认是「替换」语义，
		// 容器内网的 172.x 地址不会再对外广播。
		if err := se.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External:        []string{ip},
			AsCandidateType: webrtc.ICECandidateTypeHost,
			Networks:        []webrtc.NetworkType{webrtc.NetworkTypeUDP4},
		}); err != nil {
			log.Printf("[p2p] ICE 对外广播地址 %s 设置失败: %v", ip, err)
		} else {
			log.Printf("[p2p] ICE 候选对外广播地址: %s", ip)
		}
	}
	if lo, hi := config.Cfg.ICEPortMin, config.Cfg.ICEPortMax; lo > 0 && hi >= lo {
		if err := se.SetEphemeralUDPPortRange(uint16(lo), uint16(hi)); err != nil {
			log.Printf("[p2p] ICE 端口范围 %d-%d 设置失败: %v", lo, hi, err)
		} else {
			log.Printf("[p2p] ICE UDP 端口范围: %d-%d", lo, hi)
		}
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: localICEServers()})
	if err != nil {
		log.Printf("[p2p] 创建 PeerConnection 失败: %v", err)
		sg.say(signalMsg{T: "error", Msg: "内部错误"})
		return
	}
	sg.mu.Lock()
	if sg.pc != nil {
		_ = sg.pc.Close()
	}
	sg.pc = pc
	sg.mu.Unlock()

	pc.OnDataChannel(func(d *webrtc.DataChannel) { sg.setupDataChannel(d) })
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		ci := c.ToJSON()
		cand := &Cand{}
		if ci.SDPMid != nil {
			cand.SDPMid = *ci.SDPMid
		}
		if ci.SDPMLineIndex != nil {
			cand.SDPMLineIndex = *ci.SDPMLineIndex
		}
		cand.Candidate = ci.Candidate
		sg.say(signalMsg{T: "ice", Cand: cand})
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch state {
		case webrtc.PeerConnectionStateConnected, webrtc.PeerConnectionStateConnecting, webrtc.PeerConnectionStateNew:
		default:
			sg.markDead()
		}
	})

	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: sdp}
	if err := pc.SetRemoteDescription(offer); err != nil {
		log.Printf("[p2p] SetRemoteDescription: %v", err)
		sg.markDead()
		return
	}
	sg.remote = &offer
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		log.Printf("[p2p] CreateAnswer: %v", err)
		sg.markDead()
		return
	}
	if err := pc.SetLocalDescription(answer); err != nil {
		log.Printf("[p2p] SetLocalDescription: %v", err)
		sg.markDead()
		return
	}
	sg.say(signalMsg{T: "answer", Sdp: &SDP{Type: "answer", SDP: answer.SDP}})
}

// addRemoteICE 将客户端 trickle 的 ICE 候选喂给本会话
func (sg *session) addRemoteICE(cand *Cand) {
	if cand == nil {
		return
	}
	sg.mu.Lock()
	pc := sg.pc
	sg.mu.Unlock()
	if pc == nil {
		return
	}
	idx := cand.SDPMLineIndex
	_ = pc.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:     cand.Candidate,
		SDPMid:        &cand.SDPMid,
		SDPMLineIndex: &idx,
	})
}

// setupDataChannel 隧道建立：OnOpen 后进入隧道剥离/分发循环
func (sg *session) setupDataChannel(d *webrtc.DataChannel) {
	d.OnMessage(func(msg webrtc.DataChannelMessage) {
		sg.mu.Lock()
		if !sg.dcOpen {
			// 通道还没 open：暂存，等 OnOpen 后回放（需要拷贝，pion 会复用底层缓冲）
			if len(sg.buf) < maxPendingPkts {
				sg.buf = append(sg.buf, append([]byte(nil), msg.Data...))
				sg.mu.Unlock()
				log.Printf("[p2p] 暂存隧道包：等待 DataChannel 就绪（%d 字节）", len(msg.Data))
				return
			}
			sg.mu.Unlock()
			log.Printf("[p2p] 丢弃隧道包：暂存队列已满（%d 字节）", len(msg.Data))
			return
		}
		t := sg.tunnel
		sg.mu.Unlock()
		if t == nil {
			return
		}
		t.handlePkt(msg.Data)
	})
	d.OnOpen(func() {
		t := newTunnel(sg)
		sg.mu.Lock()
		sg.tunnel = t
		sg.dcOpen = true
		buf := sg.buf
		sg.buf = nil
		sg.mu.Unlock()
		t.run(d)
		// 回放暂存的报文，顺序与到达顺序一致
		for _, p := range buf {
			t.handlePkt(p)
		}
	})
	d.OnClose(func() {
		sg.markDead()
	})
}

func (sg *session) markDead() {
	if !sg.dead.CompareAndSwap(0, 1) {
		return
	}
	sg.s.dropSession(sg)
	sg.mu.Lock()
	if sg.tunnel != nil {
		sg.tunnel.close() // close 内部负责从 svc.tunnels 移除
	}
	if sg.pc != nil {
		_ = sg.pc.Close()
	}
	sg.mu.Unlock()
}

// handleSignal 处理来自信令服务器的 solo 消息（offer/ice）
func (sg *session) handleSignal(data []byte) {
	var m signalMsg
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	switch m.T {
	case "offer":
		if m.Sdp != nil {
			go sg.runOffer(m.Sdp.SDP)
		}
	case "ice":
		sg.addRemoteICE(m.Cand)
	}
}