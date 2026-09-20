package p2p

import (
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/settings"
	"happy-cloud/backend/internal/util"
)

// Service 聚合后端 P2P 相关能力：内置 STUN、host 信令参与协商、隧道注册、客户端配置下发。
type Service struct {
	engine   *gin.Engine
	mu       sync.Mutex
	sessions map[string]*session // key: 信令 peerCID
	tunnels  map[uint]*Tunnel    // key: 握手鉴权后的 userID
}

func NewService(engine *gin.Engine) *Service {
	return &Service{
		engine:   engine,
		sessions: map[string]*session{},
		tunnels:  map[uint]*Tunnel{},
	}
}

// Start 启动内置 STUN 与 host 信令协商（由 main.go 在 P2P 启用时调用）
func (s *Service) Start() {
	if !config.Cfg.P2PEnabled {
		return
	}
	if config.Cfg.StunEnabled {
		go runSTUN()
	}
	go s.hostLoop()
}

func (s *Service) addTunnel(t *Tunnel) {
	s.mu.Lock()
	var old *Tunnel
	if prev, ok := s.tunnels[t.userID]; ok && prev != t {
		old = prev
	}
	s.tunnels[t.userID] = t
	s.mu.Unlock()
	if old != nil {
		// 同一用户重复建链（页面刷新、手动重连）：旧隧道会让位给新隧道，
		// 但必须连它的 PeerConnection 一起关掉——否则旧页面收不到任何通知，
		// 会一直以为自己「P2P 直连」却再也收不到回包（表现为下载/上传全部超时）。
		old.close()
		if old.sess != nil {
			old.sess.markDead()
		}
	}
}

func (s *Service) removeTunnel(userID uint, t *Tunnel) {
	s.mu.Lock()
	if cur, ok := s.tunnels[userID]; ok && cur == t {
		delete(s.tunnels, userID)
	}
	s.mu.Unlock()
}

func (s *Service) getSession(cid string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[cid]
}

func (s *Service) ensureSession(cid string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sg, ok := s.sessions[cid]; ok {
		if sg.ready() == nil {
			return sg
		}
		delete(s.sessions, cid)
	}
	sg := newSession(s)
	sg.peerCID = cid
	s.sessions[cid] = sg
	return sg
}

func (s *Service) dropSession(sg *session) {
	if sg == nil {
		return
	}
	s.mu.Lock()
	if cur, ok := s.sessions[sg.peerCID]; ok && cur == sg {
		delete(s.sessions, sg.peerCID)
	}
	s.mu.Unlock()
}

// clientBaseHost 客户端可访问的对外主机（公网或局域网）。
// 优先级：环境变量 PUBLIC_HOST > 当前请求 Host 直推。
func (s *Service) clientBaseHost(c *gin.Context) string {
	if h := config.Cfg.PublicHost; h != "" {
		return h
	}
	h := c.Request.Host
	if i := strings.Index(h, ":"); i >= 0 {
		h = h[:i]
	}
	return h
}

func stunPortOf() string {
	addr := config.Cfg.StunAddr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i+1:]
	}
	return "3478"
}

// signalURL 构造客户端可连的信令地址。
// 优先使用管理员配置的完整地址（隧道穿透时必须为 wss://，否则 HTTPS 页面会被混合内容策略拦截），
// 未配置时按当前请求 Host 推导（局域网直连场景）。
func (s *Service) signalURL(c *gin.Context) string {
	base := settings.Get(settings.KeyP2PWSURL, "")
	if base == "" {
		base = "ws://" + s.clientBaseHost(c) + ":" + config.Cfg.SignalingPort + "/ws"
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + "role=client&room=" + config.Cfg.HostRoom
}

// iceServers 构造 ICE 服务器列表。
// 默认使用公共 STUN（隧道不支持 UDP，自建 STUN 无法从浏览器直连）；
// 管理员配置后以其为准；两者皆空时才退回内置 STUN 地址。
func (s *Service) iceServers(c *gin.Context) []gin.H {
	stun := settings.Get(settings.KeyP2PStunURL, settings.DefaultStunURL)
	if stun == "" {
		stun = "stun:" + s.clientBaseHost(c) + ":" + stunPortOf()
	}
	return []gin.H{{"urls": []string{stun}}}
}

// Config GET /api/p2p/config（Auth）：下发客户端所需的信令地址与 ICE 服务器
func (s *Service) Config(c *gin.Context) {
	util.OK(c, gin.H{
		"p2p":        config.Cfg.P2PEnabled,
		"ws":         s.signalURL(c),
		"room":       config.Cfg.HostRoom,
		"iceServers": s.iceServers(c),
	})
}