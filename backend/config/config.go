package config

import (
	"log"
	"os"
	"strings"
)

// Config 全局配置，全部从环境变量读取（默认值适配 docker-compose 与本地开发）
type Config struct {
	ServerPort    string
	DBDSN         string
	JWTSecret     string
	JWTExpireDay  int
	StoragePath   string
	ChunkSize     int64  // 分片大小（字节），超过大文件阈值后强制分片
	LargeFileMB   int64  // 大文件阈值（MB）
	AdminUser     string // 指定用户名注册后即为管理员
	AdminPassword string // 管理员初始密码（仅首次创建 admin 时使用；未设置则随机生成并打印到日志，登录后请修改）
	CORSOrigins   []string

	// P2P 打洞配置
	P2PEnabled    bool   // 是否启用 P2P 打洞 host 角色
	StunEnabled   bool   // 是否启用内置 STUN
	StunAddr      string // STUN 监听地址（:3478）
	P2PMaxStreams int    // 每用户并发文件流上限
	SignalingURL  string // 后端 host 建立 WebRTC 隧道前连接的独立信令服务器 ws 地址
	PublicHost    string // 下发给客户端的 STUN/信令公网可达地址
	HostRoom      string // 后端 host 在信令服务器使用的 room id
	SignalingPort string // 独立信令服务器监听端口（cmd/signaling 各用）
	// 容器部署下 ICE 需要对外广播宿主机可达地址：
	// 容器自己的 172.x 地址在宿主机/局域网侧不可路由，必须替换为宿主机的局域网 IP，
	// 并把 ICE 的 UDP 端口固定成一段可发布的端口范围（与 docker-compose 的映射一致）。
	AdvertiseIP string // 对外广播的宿主机 IP（留空则不替换，适用于后端非容器运行）
	ICEPortMin  int    // ICE UDP 端口范围下限（0 表示不固定）
	ICEPortMax  int    // ICE UDP 端口范围上限
}

var Cfg = &Config{}

func Load() {
	Cfg.ServerPort = getenv("SERVER_PORT", "8080")
	Cfg.DBDSN = getenv("DB_DSN", "root:123456@tcp(127.0.0.1:3306)/happy_cloud?charset=utf8mb4&parseTime=True&loc=Local")
	// B4 修复：JWT_SECRET 不再提供默认值；未设置或长度不足 32 字节时拒绝启动，
	// 防止攻击者使用公开默认密钥离线伪造任意管理员 token。
	// 原逻辑：Cfg.JWTSecret = getenv("JWT_SECRET", "happy-cloud-dev-secret-change-me")
	Cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if len(Cfg.JWTSecret) < 32 {
		log.Fatalf("JWT_SECRET 未设置或长度不足 32 字节，拒绝启动：请设置强随机密钥（如 openssl rand -hex 32）后重试，生产环境禁止使用默认密钥")
	}
	Cfg.JWTExpireDay = atoiSafe(getenv("JWT_EXPIRE_DAY", "7"))
	Cfg.StoragePath = getenv("STORAGE_PATH", "/data/happy-cloud")
	Cfg.ChunkSize = atoi64Safe(getenv("CHUNK_SIZE_MB", "10")) * 1024 * 1024
	Cfg.LargeFileMB = atoi64Safe(getenv("LARGE_FILE_MB", "100"))
	Cfg.AdminUser = getenv("ADMIN_USER", "admin")
	// B11 修复：移除默认弱口令 "password"；未显式设置 ADMIN_PASSWORD 时由 InitAdmin 生成随机强密码。
	// 原逻辑：Cfg.AdminPassword = getenv("ADMIN_PASSWORD", "password")
	Cfg.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	origins := getenv("CORS_ORIGINS", "*")
	Cfg.CORSOrigins = strings.Split(origins, ",")

	Cfg.P2PEnabled = getenv("P2P_ENABLED", "true") != "false"
	Cfg.StunEnabled = getenv("P2P_STUN_ENABLED", "true") != "false"
	Cfg.StunAddr = getenv("P2P_STUN_ADDR", ":3478")
	Cfg.P2PMaxStreams = atoiSafe(getenv("P2P_MAX_STREAMS", "4"))
	Cfg.SignalingURL = getenv("SIGNALING_URL", "")
	Cfg.PublicHost = getenv("PUBLIC_HOST", "")
	Cfg.HostRoom = getenv("P2P_HOST_ROOM", "happy-cloud")
	Cfg.SignalingPort = getenv("SIGNALING_PORT", "8787")
	Cfg.AdvertiseIP = getenv("P2P_ADVERTISE_IP", "")
	Cfg.ICEPortMin = atoiSafe(getenv("P2P_UDP_PORT_MIN", "0"))
	Cfg.ICEPortMax = atoiSafe(getenv("P2P_UDP_PORT_MAX", "0"))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func atoi64Safe(s string) int64 {
	return int64(atoiSafe(s))
}
