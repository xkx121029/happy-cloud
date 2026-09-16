package handler

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

type shareReq struct {
	FileID   uint       `json:"file_id" binding:"required"`
	Name     string     `json:"name"`
	ExpireAt *time.Time `json:"expire_at"`
	Password string     `json:"password"`
}

// Share 创建分享：公开目录（is_shared=1）下上传的文件默认共享；私有文件按权限生成专属链接。
func Share(c *gin.Context) {
	var req shareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	if req.ExpireAt != nil && req.ExpireAt.Before(time.Now().Add(time.Hour*24*7)) {
		util.Fail(c, 400, "有效期最短 7 天")
		return
	}
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	// 生成分享 token：32 位随机 hex
	token := fmt.Sprintf("%032x", time.Now().UnixNano())
	s := model.Share{
		Token:    token,
		UserID:   userID,
		FileID:   req.FileID,
		ExpireAt: req.ExpireAt,
		Password: req.Password,
		Views:    0,
	}
	if req.Password != "" {
		b, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			util.Fail(c, 500, "密码加密失败")
			return
		}
		s.Password = string(b)
	}
	if err := db.DB.Create(&s).Error; err != nil {
		util.Fail(c, 500, "创建分享失败")
		return
	}
	LogAction(userID, username(c), "share", fmt.Sprintf("创建分享 %s（%s）", f.Name, token[:8]))
	util.OK(c, gin.H{"token": token, "url": "/share/" + token})
}

// DeleteShare 删除分享
func DeleteShare(c *gin.Context) {
	userID := uid(c)
	var s model.Share
	if err := db.DB.Where("token = ? AND user_id = ?", c.Param("token"), userID).First(&s).Error; err != nil {
		util.Fail(c, 404, "分享不存在")
		return
	}
	db.DB.Delete(&s)
	LogAction(userID, username(c), "delete_share", fmt.Sprintf("删除分享 token=%s", s.Token[:8]))
	util.OK(c, gin.H{"ok": true})
}

// GetShare 查看分享详情（含下载列表/文件夹浏览）
func GetShare(c *gin.Context) {
	token := c.Param("token")
	password := c.Query("password")
	if password == "" {
		password = c.GetHeader("X-Share-Password")
	}
	var s model.Share
	if err := db.DB.Where("token = ?", token).First(&s).Error; err != nil {
		util.Fail(c, 404, "分享链接不存在")
		return
	}
	if s.ExpireAt != nil && time.Now().After(*s.ExpireAt) {
		util.Fail(c, 410, "分享链接已过期")
		return
	}
	if s.Password != "" && bcrypt.CompareHashAndPassword([]byte(s.Password), []byte(password)) != nil {
		util.Fail(c, 401, "密码错误")
		return
	}
	// 获取原始文件信息
	var f model.File
	if err := db.DB.Where("id = ? AND is_deleted = 0", s.FileID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	children := []model.File{}
	if f.Type == 0 {
		db.DB.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", f.UserID, f.ID).Order("type ASC, created_at DESC").Find(&children)
	}
	util.OK(c, gin.H{"share": s, "file": f, "children": children})
}

// DownloadShare 分享下载（文件夹分享时通过 file_id 指定子文件）
func DownloadShare(c *gin.Context) {
	token := c.Param("token")
	password := c.Query("password")
	if password == "" {
		password = c.GetHeader("X-Share-Password")
	}
	var share model.Share
	if err := db.DB.Where("token = ?", token).First(&share).Error; err != nil {
		util.Fail(c, 404, "分享链接不存在")
		return
	}
	if share.ExpireAt != nil && time.Now().After(*share.ExpireAt) {
		util.Fail(c, 410, "分享链接已过期")
		return
	}
	if share.Password != "" {
		if bcrypt.CompareHashAndPassword([]byte(share.Password), []byte(password)) != nil {
			util.Fail(c, 401, "密码错误")
			return
		}
	}
	var f model.File
	// S4 修复：文件被移入回收站后，分享下载立即失效
	if shareFileErr := db.DB.Where("id = ? AND is_deleted = 0", share.FileID).First(&f).Error; shareFileErr != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	if f.Type == 0 {
		// 文件夹分享：file_id 必须属于该文件夹且归属分享所有者
		fid, _ := strconv.Atoi(c.Query("file_id"))
		if fid == 0 {
			util.Fail(c, 400, "文件夹不可直接下载")
			return
		}
		// S4 修复：子文件同样过滤回收站状态
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, f.UserID).First(&f).Error; err != nil {
			util.Fail(c, 404, "文件不存在")
			return
		}
		if !isDescendant(f.UserID, uint(fid), share.FileID) {
			util.Fail(c, 403, "文件不属于该分享")
			return
		}
	}
	if f.Type != 1 {
		util.Fail(c, 400, "文件夹不可下载")
		return
	}
	path := blobPathOf(&f)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	// L4 修复：RFC 5987 百分号编码（空格为 %20），避免部分浏览器文件名错误
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(f.Name)))
	c.File(path)
}

// isDescendant 判断 fileID 是否为 ancestorID 的后代
func isDescendant(userID, fileID, ancestorID uint) bool {
	cur := fileID
	for cur != 0 {
		if cur == ancestorID {
			return true
		}
		var p model.File
		if err := db.DB.Where("id = ? AND user_id = ?", cur, userID).First(&p).Error; err != nil {
			return false
		}
		cur = p.ParentID
	}
	return false
}

// ListMyShares 我的分享列表（分页）
func ListMyShares(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}
	userID := uid(c)
	query := db.DB.Where("user_id = ?", userID)
	var total int64
	query.Model(&model.Share{}).Count(&total)
	var shares []model.Share
	query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&shares)
	items := make([]gin.H, 0, len(shares))
	for _, s := range shares {
		var f model.File
		if err := db.DB.Where("id = ? AND is_deleted = 0", s.FileID).First(&f).Error; err != nil {
			continue
		}
		items = append(items, gin.H{"share": s, "file": f})
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
}

// SharePreview 公开页面预览：展示分享信息和文件内容（HTML/文本/图片等）
func SharePreview(c *gin.Context) {
	token := c.Param("token")
	var s model.Share
	if err := db.DB.Where("token = ?", token).First(&s).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "message": "分享链接不存在"})
		return
	}
	if s.ExpireAt != nil && time.Now().After(*s.ExpireAt) {
		c.JSON(410, gin.H{"code": 410, "message": "分享链接已过期"})
		return
	}
	// 仅统计浏览，不做任何写数据库操作
	if err := db.DB.Model(&s).UpdateColumn("views", gorm.Expr("views + 1")).Error; err != nil {
		c.JSON(500, gin.H{"code": 500, "message": "系统错误"})
		return
	}
	var f model.File
	if err := db.DB.Where("id = ? AND is_deleted = 0", s.FileID).First(&f).Error; err != nil {
		c.JSON(404, gin.H{"code": 404, "message": "文件不存在"})
		return
	}
	c.JSON(200, gin.H{"share": s, "file": f, "token": token})
}

// MarkShared 标记为公共目录（is_shared=1），使文件进入 public 目录供他人访问
func MarkShared(c *gin.Context) {
	userID := uid(c)
	var req struct {
		FileID uint `json:"file_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	f.IsShared = 1
	if err := db.DB.Save(&f).Error; err != nil {
		util.Fail(c, 500, "设置失败")
		return
	}
	LogAction(userID, username(c), "mark_shared", fmt.Sprintf("标记为公共 %s", f.Name))
	util.OK(c, gin.H{"shared": true})
}

// UnmarkShared 取消公共目录标记
func UnmarkShared(c *gin.Context) {
	userID := uid(c)
	var req struct {
		FileID uint `json:"file_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	f.IsShared = 0
	if err := db.DB.Save(&f).Error; err != nil {
		util.Fail(c, 500, "设置失败")
		return
	}
	LogAction(userID, username(c), "unmark_shared", fmt.Sprintf("取消公共 %s", f.Name))
	util.OK(c, gin.H{"shared": false})
}
