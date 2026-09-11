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
	// S4 修复：不允许对已移入回收站的文件创建分享链接
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
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
	// M5 修复：返回不带 /api 前缀的落地页路径（/share/TOKEN），与前端路由一致，避免复制到的是 API 地址
	util.OK(c, gin.H{"token": share.Token, "url": "/share/" + share.Token})
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
	// M9 修复：浏览量仅在无需密码时统计（有密码的分享由 VerifyShare 成功后计数），
	// 并用原子自增避免读-改-写竞态丢失更新
	if share.Password == "" {
		db.DB.Model(&share).UpdateColumn("views", gorm.Expr("views + 1"))
	}
	var f model.File
	// S4 修复：文件被移入回收站后，分享链接立即失效
	if err := db.DB.Where("id = ? AND is_deleted = 0", share.FileID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	passwordRequired := share.Password != ""
	data := gin.H{
		"share":             share,
		"file":              f,
		"password_required": passwordRequired,
	}
	// 文件夹分享时附带子项列表
	if f.Type == 0 {
		var children []model.File
		// S4 修复：子项列表同样过滤回收站文件
		db.DB.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", f.UserID, f.ID).
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
		// M9/L14 修复：密码校验通过后原子自增浏览量（配合 GetShare 不计数逻辑，
		// 避免密码验证前后重复 +views、避免错误重试重复计数）
		db.DB.Model(&share).UpdateColumn("views", gorm.Expr("views + 1"))
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
	if _, err := os.Stat(f.StoragePath); errors.Is(err, os.ErrNotExist) {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	// L4 修复：RFC 5987 百分号编码（空格为 %20），避免部分浏览器文件名错误
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(f.Name)))
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

// UpdateShare 更新分享（有效期/密码）；password 传空串表示清除密码；
// expire_at 传具体时间表示设置有效期，clear_expire=true 表示设为永久（M1）
func UpdateShare(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id == 0 {
		util.Fail(c, 400, "参数错误")
		return
	}
	var req struct {
		Password    *string    `json:"password"`
		ExpireAt    *time.Time `json:"expire_at"`
		ClearExpire *bool      `json:"clear_expire"`
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
	// M1 修复：区分"未传"与"显式 null"——原逻辑 `if req.ExpireAt != nil` 使 null 不更新，
	// 导致前端选"永久"（expire_at 传 null）后过期时间不变；现前端传 clear_expire=true 时清除过期时间
	if req.ClearExpire != nil && *req.ClearExpire {
		s.ExpireAt = nil
	} else if req.ExpireAt != nil {
		s.ExpireAt = req.ExpireAt
	}
	if err := db.DB.Save(&s).Error; err != nil {
		util.Fail(c, 500, "更新分享失败")
		return
	}
	util.OK(c, gin.H{"updated": true})
}
