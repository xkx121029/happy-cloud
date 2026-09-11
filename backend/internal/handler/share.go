package handler

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

type shareReq struct {
	FileID   uint       `json:"file_id" binding:"required"`
	Password string     `json:"password"`
	ExpireAt *time.Time `json:"expire_at"`
}

// CreateShare 创建分享链接
func CreateShare(c *gin.Context) {
	var req shareReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ?", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	share := model.Share{
		FileID:   req.FileID,
		UserID:   userID,
		Token:    util.RandomToken(32),
		ExpireAt: req.ExpireAt,
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			util.Fail(c, 500, "服务内部错误")
			return
		}
		share.Password = string(hash)
	}
	if err := db.DB.Create(&share).Error; err != nil {
		util.Fail(c, 500, "创建分享失败")
		return
	}
	LogAction(userID, username(c), "share", fmt.Sprintf("分享 %s", f.Name))
	util.OK(c, gin.H{"token": share.Token, "url": "/api/share/" + share.Token})
}

// GetShare 访问分享信息
func GetShare(c *gin.Context) {
	token := c.Param("token")
	var share model.Share
	if err := db.DB.Where("token = ?", token).First(&share).Error; err != nil {
		util.Fail(c, 404, "分享链接不存在")
		return
	}
	if share.ExpireAt != nil && time.Now().After(*share.ExpireAt) {
		util.Fail(c, 410, "分享链接已过期")
		return
	}
	share.Views++
	db.DB.Model(&share).UpdateColumn("views", share.Views)
	var f model.File
	if err := db.DB.First(&f, share.FileID).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	passwordRequired := share.Password != ""
	data := gin.H{
		"share":            share,
		"file":             f,
		"password_required": passwordRequired,
	}
	// 文件夹分享时附带子项列表
	if f.Type == 0 {
		var children []model.File
		db.DB.Where("user_id = ? AND parent_id = ?", f.UserID, f.ID).
			Order("type ASC, created_at DESC").Find(&children)
		data["files"] = children
	}
	util.OK(c, data)
}

// VerifyShare 校验分享密码
func VerifyShare(c *gin.Context) {
	token := c.Param("token")
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var share model.Share
	if err := db.DB.Where("token = ?", token).First(&share).Error; err != nil {
		util.Fail(c, 404, "分享链接不存在")
		return
	}
	if share.Password == "" {
		util.OK(c, gin.H{"verified": true})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(share.Password), []byte(req.Password)) == nil {
		util.OK(c, gin.H{"verified": true})
		return
	}
	util.Fail(c, 401, "密码错误")
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
	if shareFileErr := db.DB.First(&f, share.FileID).Error; shareFileErr != nil {
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
		if err := db.DB.Where("id = ? AND user_id = ?", fid, f.UserID).First(&f).Error; err != nil {
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
	if _, err := os.Stat(f.StoragePath); errors.Is(err, os.ErrNotExist) {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(f.Name)))
	c.File(f.StoragePath)
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
	var total int64
	db.DB.Model(&model.Share{}).Where("user_id = ?", userID).Count(&total)
	var shares []model.Share
	db.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&shares)
	type shareItem struct {
		ID            uint       `json:"id"`
		Token         string     `json:"token"`
		FileID        uint       `json:"file_id"`
		FileName      string     `json:"file_name"`
		FileSize      int64      `json:"file_size"`
		FileType      int        `json:"file_type"`
		ExpireAt      *time.Time `json:"expire_at"`
		Views         int        `json:"views"`
		PasswordReq   bool       `json:"password_required"`
		IsExpired     bool       `json:"is_expired"`
		CreatedAt     time.Time  `json:"created_at"`
	}
	items := make([]shareItem, 0, len(shares))
	now := time.Now()
	for _, s := range shares {
		var f model.File
		fileType := 1
		fileName := ""
		var fileSize int64
		db.DB.First(&f, s.FileID)
		if f.ID != 0 {
			fileType = f.Type
			fileName = f.Name
			fileSize = f.Size
		}
		isExpired := s.ExpireAt != nil && now.After(*s.ExpireAt)
		items = append(items, shareItem{
			ID:          s.ID,
			Token:       s.Token,
			FileID:      s.FileID,
			FileName:    fileName,
			FileSize:    fileSize,
			FileType:    fileType,
			ExpireAt:    s.ExpireAt,
			Views:       s.Views,
			PasswordReq: s.Password != "",
			IsExpired:   isExpired,
			CreatedAt:   s.CreatedAt,
		})
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
}

// CancelShare 取消分享
func CancelShare(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id == 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	if err := db.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Share{}).Error; err != nil {
		util.Fail(c, 500, "取消分享失败")
		return
	}
	util.OK(c, gin.H{"cancelled": true})
}

// UpdateShare 更新分享（有效期/密码）；password 传空串表示清除密码，expire_at 传 null 表示永久
func UpdateShare(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id == 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	var req struct {
		Password *string    `json:"password"`
		ExpireAt *time.Time `json:"expire_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var s model.Share
	if err := db.DB.Where("id = ? AND user_id = ?", id, userID).First(&s).Error; err != nil {
		util.Fail(c, 404, "分享不存在")
		return
	}
	if req.Password != nil {
		if *req.Password == "" {
			s.Password = ""
		} else {
			hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
			if err != nil {
				util.Fail(c, 500, "服务内部错误")
				return
			}
			s.Password = string(hash)
		}
	}
	if req.ExpireAt != nil {
		s.ExpireAt = req.ExpireAt
	}
	if err := db.DB.Save(&s).Error; err != nil {
		util.Fail(c, 500, "更新分享失败")
		return
	}
	util.OK(c, gin.H{"updated": true})
}
