package handler

import (
	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// GetSettings 获取当前登录用户的个性化设置
func GetSettings(c *gin.Context) {
	var s model.UserSettings
	err := db.DB.Where("user_id = ?", uid(c)).First(&s).Error
	if err != nil {
		util.OK(c, gin.H{"settings": ""})
		return
	}
	util.OK(c, gin.H{"settings": s.Settings})
}

// SaveSettings 保存（覆盖）当前登录用户的个性化设置
func SaveSettings(c *gin.Context) {
	var req struct {
		Settings string `json:"settings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	// 不做严格 JSON 校验：前端自己保证
	var s model.UserSettings
	err := db.DB.Where("user_id = ?", uid(c)).First(&s).Error
	if err != nil {
		s = model.UserSettings{UserID: uid(c), Settings: req.Settings}
		db.DB.Create(&s)
	} else {
		db.DB.Model(&s).Update("settings", req.Settings)
	}
	util.OK(c, gin.H{"ok": true})
}
