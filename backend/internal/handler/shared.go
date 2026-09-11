package handler

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// ToggleShared 共享/取消共享到公共目录，仅文件所有者可操作，仅文件（type=1）可共享
func ToggleShared(c *gin.Context) {
	var req struct {
		FileID uint `json:"file_id" binding:"required"`
		Shared bool `json:"shared"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	if f.Type != 1 {
		util.Fail(c, 400, "仅文件可以共享到公共目录")
		return
	}
	shared := 0
	if req.Shared {
		shared = 1
	}
	if err := db.DB.Model(&f).Update("is_shared", shared).Error; err != nil {
		util.Fail(c, 500, "操作失败")
		return
	}
	if req.Shared {
		LogAction(userID, username(c), "share", fmt.Sprintf("共享到公共目录 %s", f.Name))
	} else {
		LogAction(userID, username(c), "share", fmt.Sprintf("取消公共共享 %s", f.Name))
	}
	util.OK(c, gin.H{"is_shared": shared})
}

// ListShared 公共共享目录：所有登录用户可见的共享文件列表
func ListShared(c *gin.Context) {
	var files []model.File
	if err := db.DB.Where("is_shared = 1 AND type = 1 AND is_deleted = 0").
		Order("created_at DESC").Find(&files).Error; err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	// 补充上传者用户名
	type sharedItem struct {
		model.File
		Username string `json:"username"`
	}
	items := make([]sharedItem, 0, len(files))
	for _, f := range files {
		var u model.User
		uname := ""
		if err := db.DB.First(&u, f.UserID).Error; err == nil {
			uname = u.Username
		}
		items = append(items, sharedItem{File: f, Username: uname})
	}
	util.OK(c, items)
}

// DownloadShared 下载公共共享文件
func DownloadShared(c *gin.Context) {
	fid, _ := strconv.Atoi(c.Query("file_id"))
	var f model.File
	if err := db.DB.Where("id = ? AND is_shared = 1 AND type = 1", fid).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在或未共享")
		return
	}
	if _, err := os.Stat(f.StoragePath); errors.Is(err, os.ErrNotExist) {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(f.Name)))
	c.File(f.StoragePath)
}
