package config

import (
	"os"
	"strings"
)

// Config 全局配置，全部从环境变量读取（默认值适配 docker-compose 与本地开发）
type Config struct {
	ServerPort   string
	DBDSN        string
	JWTSecret    string
	JWTExpireDay int
	StoragePath  string
	ChunkSize    int64  // 分片大小（字节），超过大文件阈值后强制分片
	LargeFileMB  int64  // 大文件阈值（MB）
	AdminUser     string // 指定用户名注册后即为管理员
	AdminPassword string // 管理员初始密码（启动时重置 admin 为该值）
	CORSOrigins   []string
}

var Cfg = &Config{}

func Load() {
	Cfg.ServerPort = getenv("SERVER_PORT", "8080")
	Cfg.DBDSN = getenv("DB_DSN", "root:123456@tcp(127.0.0.1:3306)/happy_cloud?charset=utf8mb4&parseTime=True&loc=Local")
	Cfg.JWTSecret = getenv("JWT_SECRET", "happy-cloud-dev-secret-change-me")
	Cfg.JWTExpireDay = atoiSafe(getenv("JWT_EXPIRE_DAY", "7"))
	Cfg.StoragePath = getenv("STORAGE_PATH", "/data/happy-cloud")
	Cfg.ChunkSize = atoi64Safe(getenv("CHUNK_SIZE_MB", "10")) * 1024 * 1024
	Cfg.LargeFileMB = atoi64Safe(getenv("LARGE_FILE_MB", "100"))
	Cfg.AdminUser = getenv("ADMIN_USER", "admin")
	Cfg.AdminPassword = getenv("ADMIN_PASSWORD", "password")
	origins := getenv("CORS_ORIGINS", "*")
	Cfg.CORSOrigins = strings.Split(origins, ",")
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
