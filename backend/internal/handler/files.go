package handler

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

func uid(c *gin.Context) uint {
	v, _ := c.Get("user_id")
	return v.(uint)
}

func username(c *gin.Context) string {
	v, _ := c.Get("username")
	return v.(string)
}

// ListFiles 文件列表，参数 parent_id
func ListFiles(c *gin.Context) {
	parentID, _ := strconv.Atoi(c.DefaultQuery("parent_id", "0"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}
	userID := uid(c)
	query := db.DB.Where("user_id = ? AND parent_id = ?", userID, parentID)
	var total int64
	query.Model(&model.File{}).Count(&total)
	var files []model.File
	query.Order("type ASC, created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&files)
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": files})
}

// Mkdir 新建文件夹
func Mkdir(c *gin.Context) {
	var req struct {
		ParentID uint   `json:"parent_id"`
		Name     string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	if err := checkNameDup(userID, req.ParentID, req.Name); err != nil {
		util.Fail(c, 409, err.Error())
		return
	}
	f := model.File{UserID: userID, ParentID: req.ParentID, Name: req.Name, Type: 0}
	if err := db.DB.Create(&f).Error; err != nil {
		util.Fail(c, 500, "创建失败")
		return
	}
	LogAction(userID, username(c), "mkdir", fmt.Sprintf("创建文件夹 %s", req.Name))
	util.OK(c, f)
}

// UploadHash 秒传校验：同 hash 已存在则直接建记录
func UploadHash(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Size     int64  `json:"size" binding:"required"`
		Hash     string `json:"hash" binding:"required"`
		ParentID uint   `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var exist model.File
	err := db.DB.Where("user_id = ? AND hash = ? AND type = 1", userID, req.Hash).
		Order("id ASC").First(&exist).Error
	if err == nil && exist.StoragePath != "" {
		// 秒传：关联同一实体
		if e := checkNameDup(userID, req.ParentID, req.Name); e != nil {
			util.Fail(c, 409, e.Error())
			return
		}
		f := model.File{
			UserID: userID, ParentID: req.ParentID, Name: req.Name,
			Type: 1, Size: req.Size, Hash: req.Hash, StoragePath: exist.StoragePath,
		}
		if err := db.DB.Create(&f).Error; err != nil {
			util.Fail(c, 500, "创建失败")
			return
		}
		updateQuota(userID, req.Size)
		LogAction(userID, username(c), "upload", fmt.Sprintf("秒传 %s", req.Name))
		util.OK(c, gin.H{"exists": true, "file_id": f.ID})
		return
	}
	util.OK(c, gin.H{"exists": false, "file_id": 0})
}

// Upload 小文件直传（multipart: file, parent_id）
func Upload(c *gin.Context) {
	userID := uid(c)
	parentID, _ := strconv.Atoi(c.PostForm("parent_id"))
	file, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, 400, "缺少文件")
		return
	}
	// 超大文件由客户端走分片，此处仅允许 <= 100MB
	if file.Size > config.Cfg.LargeFileMB*1024*1024 {
		util.Fail(c, 400, "文件过大，请使用分片上传")
		return
	}
	// 配额检查
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
		return
	}
	if u.QuotaUsed+file.Size > u.QuotaMax {
		util.Fail(c, 507, "存储空间不足")
		return
	}
	src, err := file.Open()
	if err != nil {
		util.Fail(c, 500, "读取文件失败")
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		util.Fail(c, 500, "读取文件失败")
		return
	}
	hash := util.SHA256(data)
	path := util.FilePath(userID, hash)
	if err := util.EnsureDir(util.UserDir(userID)); err != nil {
		util.Fail(c, 500, "存储目录异常")
		return
	}
	// 实体已存在（秒传命中）则跳过写盘
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			util.Fail(c, 500, "写入文件失败")
			return
		}
	}
	// 同一目录重名则自动加序号
	name := uniqueName(userID, uint(parentID), file.Filename)
	f := model.File{
		UserID: userID, ParentID: uint(parentID), Name: name,
		Type: 1, Size: file.Size, Hash: hash, StoragePath: path,
	}
	if err := db.DB.Create(&f).Error; err != nil {
		util.Fail(c, 500, "创建记录失败")
		return
	}
	updateQuota(userID, file.Size)
	LogAction(userID, username(c), "upload", fmt.Sprintf("上传 %s (%.1fMB)", name, float64(file.Size)/1024/1024))
	util.OK(c, f)
}

// UploadChunk 上传分片（multipart: file, hash, chunk_index, chunk_total）
func UploadChunk(c *gin.Context) {
	hash := c.PostForm("hash")
	index := c.PostForm("chunk_index")
	if hash == "" || index == "" {
		util.Fail(c, 400, "参数错误")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, 400, "缺少分片文件")
		return
	}
	dir := util.ChunkDir(hash)
	if err := util.EnsureDir(dir); err != nil {
		util.Fail(c, 500, "存储目录异常")
		return
	}
	if err := c.SaveUploadedFile(file, filepath.Join(dir, index)); err != nil {
		util.Fail(c, 500, "保存分片失败")
		return
	}
	util.OK(c, gin.H{"chunk_index": index})
}

// UploadMerge 合并分片，参数 hash, name, size, parent_id, chunk_total
func UploadMerge(c *gin.Context) {
	var req struct {
		Hash       string `json:"hash" binding:"required"`
		Name       string `json:"name" binding:"required"`
		Size       int64  `json:"size" binding:"required"`
		ParentID   uint   `json:"parent_id"`
		ChunkTotal int    `json:"chunk_total" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
		return
	}
	if u.QuotaUsed+req.Size > u.QuotaMax {
		util.Fail(c, 507, "存储空间不足")
		return
	}
	dir := util.ChunkDir(req.Hash)
	// 读取全部分片
	var parts []int
	entries, err := os.ReadDir(dir)
	if err != nil {
		util.Fail(c, 400, "分片不完整，请重新上传")
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n, _ := strconv.Atoi(e.Name())
		parts = append(parts, n)
	}
	sort.Ints(parts)
	if len(parts) != req.ChunkTotal {
		util.Fail(c, 400, "分片不完整，请重新上传")
		return
	}
	path := util.FilePath(userID, req.Hash)
	if err := util.EnsureDir(util.UserDir(userID)); err != nil {
		util.Fail(c, 500, "存储目录异常")
		return
	}
	final, err := os.Create(path)
	if err != nil {
		util.Fail(c, 500, "创建文件失败")
		return
	}
	defer final.Close()
	for _, p := range parts {
		b, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(p)))
		if err != nil {
			util.Fail(c, 400, "分片读取失败")
			return
		}
		if _, err := final.Write(b); err != nil {
			util.Fail(c, 500, "合并写入失败")
			return
		}
	}
	final.Sync()
	// 校验合并后大小
	if fi, err := os.Stat(path); err == nil && fi.Size() != req.Size {
		util.Fail(c, 400, "合并文件大小不一致")
		return
	}
	// 清理临时分片
	os.RemoveAll(dir)
	name := uniqueName(userID, req.ParentID, req.Name)
	f := model.File{
		UserID: userID, ParentID: req.ParentID, Name: name,
		Type: 1, Size: req.Size, Hash: req.Hash, StoragePath: path,
	}
	if err := db.DB.Create(&f).Error; err != nil {
		util.Fail(c, 500, "创建记录失败")
		return
	}
	updateQuota(userID, req.Size)
	LogAction(userID, username(c), "upload", fmt.Sprintf("分片上传 %s (%.1fMB)", name, float64(req.Size)/1024/1024))
	util.OK(c, f)
}

// Download 下载文件流
func Download(c *gin.Context) {
	userID := uid(c)
	fid, _ := strconv.Atoi(c.Query("file_id"))
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ?", fid, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	if f.Type != 1 {
		util.Fail(c, 400, "文件夹不可下载")
		return
	}
	if _, err := os.Stat(f.StoragePath); err != nil {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.QueryEscape(f.Name)))
	c.File(f.StoragePath)
}

// Rename 重命名
func Rename(c *gin.Context) {
	var req struct {
		FileID  uint   `json:"file_id" binding:"required"`
		NewName string `json:"new_name" binding:"required"`
	}
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
	if err := checkNameDup(userID, f.ParentID, req.NewName); err != nil {
		util.Fail(c, 409, err.Error())
		return
	}
	f.Name = req.NewName
	if err := db.DB.Save(&f).Error; err != nil {
		util.Fail(c, 500, "重命名失败")
		return
	}
	LogAction(userID, username(c), "rename", fmt.Sprintf("重命名为 %s", req.NewName))
	util.OK(c, f)
}

// Move 移动文件到目标文件夹
func Move(c *gin.Context) {
	var req struct {
		FileID         uint `json:"file_id" binding:"required"`
		TargetParentID uint `json:"target_parent_id"`
	}
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
	if req.TargetParentID == f.ID {
		util.Fail(c, 400, "不能移动到自身")
		return
	}
	// 校验目标目录归属，并防止移动到自身子目录
	if err := checkTarget(userID, req.TargetParentID, req.FileID); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
	if err := checkNameDup(userID, req.TargetParentID, f.Name); err != nil {
		util.Fail(c, 409, err.Error())
		return
	}
	f.ParentID = req.TargetParentID
	if err := db.DB.Save(&f).Error; err != nil {
		util.Fail(c, 500, "移动失败")
		return
	}
	LogAction(userID, username(c), "move", fmt.Sprintf("移动 %s", f.Name))
	util.OK(c, f)
}

// Delete 删除文件/文件夹（批量）
func Delete(c *gin.Context) {
	var req struct {
		FileIDs []uint `json:"file_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	uname := username(c)
	var removed int64
	for _, fid := range req.FileIDs {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ?", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if err := deleteTree(userID, uname, &f); err == nil {
			removed++
		}
	}
	util.OK(c, gin.H{"removed": removed})
}

// Quota 空间用量
func Quota(c *gin.Context) {
	userID := uid(c)
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	util.OK(c, gin.H{"quota_max": u.QuotaMax, "quota_used": u.QuotaUsed})
}

// ---------- 内部工具 ----------

func updateQuota(userID uint, delta int64) {
	db.DB.Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("quota_used", gorm.Expr("quota_used + ?", delta))
}

// checkNameDup 同目录重名检查
func checkNameDup(userID, parentID uint, name string) error {
	var count int64
	db.DB.Model(&model.File{}).
		Where("user_id = ? AND parent_id = ? AND name = ?", userID, parentID, name).
		Count(&count)
	if count > 0 {
		return fmt.Errorf("同名文件或文件夹已存在")
	}
	return nil
}

// uniqueName 重名时自动添加 (n) 后缀
func uniqueName(userID, parentID uint, name string) string {
	if checkNameDup(userID, parentID, name) == nil {
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if checkNameDup(userID, parentID, candidate) == nil {
			return candidate
		}
	}
	return name
}

// checkTarget 校验目标目录归属且不是自身子目录
func checkTarget(userID, targetParentID, selfID uint) error {
	if targetParentID == 0 {
		return nil
	}
	var t model.File
	if err := db.DB.Where("id = ? AND user_id = ?", targetParentID, userID).First(&t).Error; err != nil {
		return fmt.Errorf("目标文件夹不存在")
	}
	if t.Type != 0 {
		return fmt.Errorf("目标必须是文件夹")
	}
	// 向上遍历检查是否把文件移动到自己的后代中
	cur := targetParentID
	for cur != 0 {
		if cur == selfID {
			return fmt.Errorf("不能移动到自身子目录")
		}
		var p model.File
		if err := db.DB.Where("id = ? AND user_id = ?", cur, userID).First(&p).Error; err != nil {
			break
		}
		cur = p.ParentID
	}
	return nil
}

// deleteTree 递归删除文件/文件夹，返回是否成功
func deleteTree(userID uint, uname string, f *model.File) error {
	// 递归删除子项
	if f.Type == 0 {
		var children []model.File
		db.DB.Where("user_id = ? AND parent_id = ?", userID, f.ID).Find(&children)
		for i := range children {
			if err := deleteTree(userID, uname, &children[i]); err != nil {
				return err
			}
		}
	}
	// 删除实体：仅当全站无其他记录引用同一 hash 时才删磁盘
	// （转送/共享等场景下多用户可能复用同一实体，需跨用户统计引用）
	if f.Type == 1 && f.StoragePath != "" {
		var refs int64
		db.DB.Model(&model.File{}).
			Where("hash = ? AND type = 1", f.Hash).
			Count(&refs)
		if refs <= 1 {
			os.Remove(f.StoragePath)
		}
		db.DB.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("quota_used", gorm.Expr("GREATEST(quota_used - ?, 0)", f.Size))
	}
	// 删除关联分享
	db.DB.Where("file_id = ?", f.ID).Delete(&model.Share{})
	if err := db.DB.Delete(f).Error; err != nil {
		return err
	}
	LogAction(userID, uname, "delete", fmt.Sprintf("删除 %s", f.Name))
	return nil
}
