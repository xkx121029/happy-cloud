package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/settings"
	"happy-cloud/backend/internal/util"
)

// settingsKeys 管理员可维护的系统设置键白名单
var settingsKeys = map[string]struct{}{
	settings.KeyAllowRegister: {},
	settings.KeyDefaultQuota:  {},
	settings.KeyMaxUploadMB:   {},
	settings.KeyPublicURL:     {},
	settings.KeyP2PPublicHost: {},
}

// GetAdminSettings 读取全局系统设置
func GetAdminSettings(c *gin.Context) {
	util.OK(c, settings.All())
}

// SiteInfo 公开站点信息（无需登录）：返回主页面公开网址，供前端分享链接等使用
func SiteInfo(c *gin.Context) {
	util.OK(c, gin.H{
		"public_url": settings.Get(settings.KeyPublicURL, ""),
	})
}

// SaveAdminSettings 保存全局系统设置（仅白名单键，键值写入后强制刷新缓存）
func SaveAdminSettings(c *gin.Context) {
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var msgs []string
	now := time.Now()
	for k, v := range req {
		if _, ok := settingsKeys[k]; !ok {
			msgs = append(msgs, "非法设置项: "+k)
			continue
		}
		val := fmt.Sprintf("%v", v)
		// 值校验
		switch k {
		case settings.KeyDefaultQuota:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil || n < 0 {
				msgs = append(msgs, "默认配额需为非负整数（字节）")
				continue
			}
		case settings.KeyMaxUploadMB:
			n, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64)
			if err != nil || n <= 0 {
				msgs = append(msgs, "上传上限需为正整数（MB）")
				continue
			}
		case settings.KeyAllowRegister:
			if val != "true" && val != "false" {
				msgs = append(msgs, "开放注册需为 true/false")
				continue
			}
		case settings.KeyPublicURL:
			// 允许空（未配置）或以 http/https 开头；去尾斜杠便于前端拼接
			if val != "" && !strings.HasPrefix(val, "http://") && !strings.HasPrefix(val, "https://") {
				msgs = append(msgs, "主页面网址需以 http:// 或 https:// 开头")
				continue
			}
			val = strings.TrimRight(val, "/")
		case settings.KeyP2PPublicHost:
			// 允许空；否则应为不带协议/端口的主机名，作为 P2P 信令/STUN 下发的 host
			if val != "" {
				if strings.Contains(val, "://") || strings.Contains(val, ":") || strings.ContainsAny(val, " \t") {
					msgs = append(msgs, "P2P 隧道主机需为域名或 IP（不含协议与端口）")
					continue
				}
			}
		}
		// upsert
		var s model.SystemSetting
		if err := db.DB.Where("`key` = ?", k).First(&s).Error; err != nil {
			s = model.SystemSetting{Key: k}
		}
		s.Value = val
		s.UpdatedAt = now
		if err := db.DB.Save(&s).Error; err != nil {
			msgs = append(msgs, "保存失败: "+k)
			continue
		}
	}
	settings.Reload()
	if len(msgs) > 0 {
		util.Fail(c, 400, strings.Join(msgs, "；"))
		return
	}
	adminLog(c, "save_settings", "更新系统设置")
	util.OK(c, settings.All())
}
