package settings

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
)

// 全局系统设置键（与 admin 面板表单一一对应）
const (
	KeyAllowRegister = "allow_register" // 是否开放注册："true"/"false"
	KeyDefaultQuota  = "default_quota"  // 新用户默认配额（字节）
	KeyMaxUploadMB   = "max_upload_mb"  // 单文件上传上限（MB，超过走分片）

	// 访问地址配置（cloudflared 等隧道穿透场景下用于下发正确的公网地址）
	KeyPublicURL     = "public_url"      // 主页面公开网址（如 https://cloud.example.com），前端分享链接优先使用
	KeyP2PPublicHost = "p2p_public_host" // P2P 隧道对外主机（信令/STUN 下发的 host，覆盖请求 Host 推导）
)

// 缓存：把 settings 表整表载入内存，带 TTL 失效，避免每次请求都查库
var (
	mu       sync.RWMutex
	cache    = map[string]string{}
	loadedAt time.Time
	ttl      = 5 * time.Second
)

func ensureLoaded() {
	mu.RLock()
	fresh := time.Since(loadedAt) < ttl
	mu.RUnlock()
	if !fresh {
		load()
	}
}

func load() {
	rows := []model.SystemSetting{}
	if err := db.DB.Find(&rows).Error; err != nil {
		return
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	mu.Lock()
	cache = m
	loadedAt = time.Now()
	mu.Unlock()
}

// Reload 强制刷新缓存（管理员保存设置后调用）
func Reload() {
	load()
}

// Get 读取字符串设置，缺失/未初始化时返回默认值
func Get(key, def string) string {
	ensureLoaded()
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := cache[key]; ok {
		return v
	}
	return def
}

// GetBool 读取布尔设置
func GetBool(key string, def bool) bool {
	return strings.ToLower(strings.TrimSpace(Get(key, strconv.FormatBool(def)))) == "true"
}

// GetInt64 读取整数设置
func GetInt64(key string, def int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(Get(key, strconv.FormatInt(def, 10))), 10, 64)
	if err != nil {
		return def
	}
	return v
}

// All 返回当前全量设置（供管理员面板读取，已带默认值兜底）
func All() map[string]string {
	return map[string]string{
		KeyAllowRegister:  strconv.FormatBool(GetBool(KeyAllowRegister, true)),
		KeyDefaultQuota:   strconv.FormatInt(GetInt64(KeyDefaultQuota, 10*1024*1024*1024), 10),
		KeyMaxUploadMB:    strconv.FormatInt(GetInt64(KeyMaxUploadMB, 100), 10),
		KeyPublicURL:      Get(KeyPublicURL, ""),
		KeyP2PPublicHost:  Get(KeyP2PPublicHost, ""),
	}
}
