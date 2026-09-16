// signaling_client：后端作为 host 主动连接独立信令服务器，收集客户端 offer/
// ICE，并驱动对端隧道协商。断线自动按退避重连。
package p2p

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"happy-cloud/backend/config"
)

// signalMsg 后端与信令服务器间的消息（透传，不含业务）
type signalMsg struct {
	T    string `json:"t"`            // welcome/offer/answer/ice/ping/pong/error/replace/host-offline
	CID  string `json:"cid,omitempty"` // 消息源客户端 id（client→host）
	To   string `json:"to,omitempty"`  // 目标客户端 id（host→client）
	Sdp  *SDP   `json:"sdp,omitempty"`
	Cand *Cand  `json:"cand,omitempty"`
	Msg  string `json:"msg,omitempty"`
}

func (s *Service) hostLoop() {
	base := config.Cfg.SignalingURL
	if base == "" {
		if h := config.Cfg.PublicHost; h != "" {
			base = "ws://" + h
		}
	}
	if base == "" {
		log.Println("[p2p] 未配置 SIGNALING_URL/PUBLIC_HOST，后端 host 角色跳过（P2P 协商不可用于本机）")
		return
	}
	room := config.Cfg.HostRoom
	if room == "" {
		room = "happy-cloud"
	}
	wsURL := strings.TrimRight(base, "/") + "/ws?role=host&room=" + url.QueryEscape(room)
	for {
		err := s.hostSession(wsURL)
		log.Printf("[p2p] host 信令断开(%v)，3s 后重连", err)
		time.Sleep(3 * time.Second)
	}
}

// hostSession 单次 host 信令连接：注册为 room 的 host，收集 offer/ice。
func (s *Service) hostSession(wsURL string) error {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	var wmu sync.Mutex
	write := func(m signalMsg) error {
		b, _ := json.Marshal(m)
		wmu.Lock()
		defer wmu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, b)
	}

	// 心跳保活
	stopH := make(chan struct{})
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopH:
				return
			case <-t.C:
				_ = write(signalMsg{T: "ping"})
			}
		}
	}()
	defer close(stopH)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		var m signalMsg
		if err := json.Unmarshal(data, &m); err != nil {
			// welcome/非标准帧直接忽略
			continue
		}
		switch m.T {
		case "offer":
			sg := s.ensureSession(m.CID)
			sg.mu.Lock()
			sg.signal = func(sm signalMsg) {
				sm.To = sg.peerCID
				_ = write(sm)
			}
			sg.mu.Unlock()
			if m.Sdp != nil {
				go sg.runOffer(m.Sdp.SDP)
			}
		case "ice":
			if sg := s.getSession(m.CID); sg != nil {
				sg.addRemoteICE(m.Cand)
			}
		case "host-offline", "replace", "welcome":
			// host 角色通常不会收到；忽略
		}
	}
}