package handler

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/tag"
	"happy-cloud/backend/internal/util"
)

// ListTags 列出当前用户所有标签
func ListTags(c *gin.Context) {
	userID := uid(c)
	tags, err := tag.List(userID)
	if err != nil {
		util.Fail(c, 500, "查询标签失败")
		return
	}
	util.OK(c, tags)
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

// CreateTag 创建标签
func CreateTag(c *gin.Context) {
	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		util.Fail(c, 400, "名称不能为空")
		return
	}
	if len(req.Name) > 64 {
		util.Fail(c, 400, "名称不能超过 64 个字符")
		return
	}
	userID := uid(c)
	color := req.Color
	if color == "" {
		color = "#6366f1"
	}
	id, err := tag.EnsureExists(userID, req.Name, color)
	if err != nil {
		util.Fail(c, 500, "创建失败")
		return
	}
	var t model.Tag
	db.DB.Where("id = ?", id).First(&t)
	LogAction(userID, username(c), "create_tag", fmt.Sprintf("创建标签 %s", t.Name))
	util.OK(c, t)
}

// DeleteTag 删除标签及其关联
func DeleteTag(c *gin.Context) {
	userID := uid(c)
	tagID, _ := strconv.Atoi(c.Param("id"))
	if tagID <= 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	if err := tag.DeleteTag(userID, uint(tagID)); err != nil {
		util.Fail(c, 500, "删除失败")
		return
	}
	LogAction(userID, username(c), "delete_tag", fmt.Sprintf("删除标签 id=%d", tagID))
	util.OK(c, gin.H{"deleted": true})
}

// SetFileTags 批量设置文件标签（先删旧关联，再建新关联）
func SetFileTags(c *gin.Context) {
	var req struct {
		FileID uint   `json:"file_id" binding:"required"`
		TagIDs []uint `json:"tag_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.FileID == 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	// 校验文件归属
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	// 单事务内删除旧标签关联并新增新标签关联（含标签归属校验），保证原子性
	if err := tag.SetFileTags(req.FileID, userID, req.TagIDs); err != nil {
		util.Fail(c, 500, "更新标签失败")
		return
	}
	LogAction(userID, username(c), "set_tags", fmt.Sprintf("文件 %s 设置标签 ids=%v", f.Name, req.TagIDs))
	util.OK(c, gin.H{"ok": true})
}

// GetFileTags 获取单个文件的标签
func GetFileTags(c *gin.Context) {
	fileID, _ := strconv.Atoi(c.Param("id"))
	if fileID <= 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	// 校验文件归属，防止越权读取他人文件的标签（IDOR 防护）
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", uint(fileID), userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	tags, err := tag.GetTags(uint(fileID))
	if err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	util.OK(c, tags)
}

// SearchTags 搜索当前用户的标签名称
func SearchTags(c *gin.Context) {
	keyword := c.DefaultQuery("q", "")
	userID := uid(c)
	tags, err := tag.SearchByName(userID, keyword)
	if err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	util.OK(c, tags)
}
