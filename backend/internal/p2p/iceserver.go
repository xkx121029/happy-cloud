// iceserver：内置轻量 STUN 服务器（responder），为整网提供 srflx 候选；
// 以及后端 answerer 侧使用的本地 ICE Servers。
package p2p

import (
	"errors"
	"log"
	"net"

	"github.com/pion/stun/v3"
	"github.com/pion/webrtc/v4"

	"happy-cloud/backend/config"
)

var errClosed = errors.New("session closed")

// localICEServers 后端 answerer 自己使用的 ICE Servers（指向本机内置 STUN）
func localICEServers() []webrtc.ICEServer {
	return []webrtc.ICEServer{{URLs: []string{"stun:127.0.0.1:" + stunPortOf()}}}
}

// runSTUN 启动内置 STUN Binding 响应（XOR-MAPPED-ADDRESS）。失败不致命，仅打日志。
func runSTUN() {
	addr := config.Cfg.StunAddr
	if addr == "" {
		addr = ":3478"
	}
	pc, err := net.ListenPacket("udp4", addr)
	if err != nil {
		log.Printf("[p2p] 内置 STUN 启动失败(%s): %v", addr, err)
		return
	}
	log.Printf("[p2p] 内置 STUN 监听于 %s", addr)
	buf := make([]byte, 2048)
	for {
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		respondStun(pc, buf[:n], raddr)
	}
}

// respondStun 响应一次 STUN Binding 请求（仅当请求为 BindingRequest）。
func respondStun(pc net.PacketConn, pkt []byte, raddr net.Addr) {
	var req stun.Message
	if err := stun.Decode(pkt, &req); err != nil {
		return
	}
	if req.Type != stun.BindingRequest {
		return
	}
	ua, ok := raddr.(*net.UDPAddr)
	if !ok || ua == nil {
		return
	}
	res := &stun.Message{Type: stun.BindingSuccess, TransactionID: req.TransactionID}
	if err := (&stun.XORMappedAddress{IP: ua.IP, Port: ua.Port}).AddTo(res); err != nil {
		return
	}
	res.Encode()
	_, _ = pc.WriteTo(res.Raw, raddr)
}