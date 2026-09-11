package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

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
	query := db.DB.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", userID, parentID)
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
	// L2 修复：新建文件夹时校验名称合法性（禁止路径分隔符/控制字符、限制长度）
	if err := validateName(req.Name); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
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
	// L2 修复：秒传同样校验文件名合法性
	if err := validateName(req.Name); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
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
		// L1 修复：配额增加改为原子条件更新（并发下防止突破配额），失败则回滚记录
		if !tryAddQuota(userID, req.Size) {
			db.DB.Delete(&f)
			util.Fail(c, 507, "存储空间不足")
			return
		}
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
	// L2 修复：上传文件名校验（禁止路径分隔符/控制字符、限制长度）
	if err := validateName(file.Filename); err != nil {
		util.Fail(c, 400, err.Error())
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
	// L1 修复：配额增加改为原子条件更新（并发下防止突破配额），失败则回滚记录
	if !tryAddQuota(userID, file.Size) {
		db.DB.Delete(&f)
		util.Fail(c, 507, "存储空间不足")
		return
	}
	LogAction(userID, username(c), "upload", fmt.Sprintf("上传 %s (%.1fMB)", name, float64(file.Size)/1024/1024))
	util.OK(c, f)
}

// UploadChunk 上传分片（multipart: file, hash, chunk_index, chunk_total）
func UploadChunk(c *gin.Context) {
	hash := c.PostForm("hash")
	index := c.PostForm("chunk_index")
	total := c.PostForm("chunk_total")
	if hash == "" || index == "" || total == "" {
		util.Fail(c, 400, "参数错误")
		return
	}
	idx, errIdx := strconv.Atoi(index)
	totalN, errTotal := strconv.Atoi(total)
	if errIdx != nil || errTotal != nil || idx < 0 || totalN <= 0 || idx >= totalN {
		util.Fail(c, 400, "参数错误")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		util.Fail(c, 400, "缺少分片文件")
		return
	}
	// S6 修复：限制单片大小，防止恶意超大分片耗尽磁盘
	if file.Size > config.Cfg.ChunkSize {
		util.Fail(c, 400, "分片大小超限")
		return
	}
	userID := uid(c)
	// S6 修复：分片目录按用户隔离（tmp/<userID>/<hash>），避免跨用户同 hash 分片互相覆盖/混用
	dir := util.ChunkDir(userID, hash)
	if err := util.EnsureDir(dir); err != nil {
		util.Fail(c, 500, "存储目录异常")
		return
	}
	// L16 修复：清理目录中超出本次总片数的残留分片，避免上次失败上传的旧分片干扰本次合并
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if n, err := strconv.Atoi(e.Name()); err == nil && n >= totalN {
				os.Remove(filepath.Join(dir, e.Name()))
			}
		}
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
	// L2 修复：合并上传同样校验文件名合法性
	if err := validateName(req.Name); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
		return
	}
	if u.QuotaUsed+req.Size > u.QuotaMax {
		util.Fail(c, 507, "存储空间不足")
		return
	}
	// S6 修复：分片目录按用户隔离，避免跨用户分片互相覆盖/混用
	dir := util.ChunkDir(userID, req.Hash)
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
	// L16 修复：严格校验分片索引必须为 0..ChunkTotal-1 连续，防止残留分片干扰合并
	for i := 0; i < len(parts); i++ {
		if parts[i] != i {
			util.Fail(c, 400, "分片不完整，请重新上传")
			return
		}
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
	// M12 修复：重新计算合并文件内容 SHA256 并与请求 hash 比对，防止客户端伪造 hash 命中错误秒传内容
	if got, err := hashFile(path); err != nil || got != req.Hash {
		os.Remove(path)
		util.Fail(c, 400, "文件内容校验失败")
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
	// L1 修复：配额增加改为原子条件更新，失败则回滚记录与实体
	if !tryAddQuota(userID, req.Size) {
		db.DB.Delete(&f)
		var refs int64
		db.DB.Model(&model.File{}).Where("hash = ? AND type = 1", req.Hash).Count(&refs)
		if refs == 0 {
			os.Remove(path)
		}
		util.Fail(c, 507, "存储空间不足")
		return
	}
	LogAction(userID, username(c), "upload", fmt.Sprintf("分片上传 %s (%.1fMB)", name, float64(req.Size)/1024/1024))
	util.OK(c, f)
}

// Download 下载文件流
func Download(c *gin.Context) {
	userID := uid(c)
	fid, _ := strconv.Atoi(c.Query("file_id"))
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, userID).First(&f).Error; err != nil {
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
	// L4 修复：使用 RFC 5987 百分号编码（空格为 %20），避免 url.QueryEscape 把空格变成 + 导致部分浏览器文件名错误
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(f.Name)))
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
	// L2 修复：重命名时同样校验名称合法性
	if err := validateName(req.NewName); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
	userID := uid(c)
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
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
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
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

// Delete 删除文件/文件夹（批量）：移入回收站（软删除），可恢复
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
	var moved int64
	for _, fid := range req.FileIDs {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if err := moveToTrash(userID, uname, &f); err == nil {
			moved++
		}
	}
	util.OK(c, gin.H{"moved": moved})
}

// ListTrash 回收站文件列表，支持关键字搜索与分页
func ListTrash(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 100
	}
	userID := uid(c)
	query := db.DB.Where("user_id = ? AND is_deleted = 1", userID)
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("name LIKE ?", "%"+kw+"%")
	}
	var total int64
	query.Model(&model.File{}).Count(&total)
	var files []model.File
	query.Order("updated_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&files)
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": files})
}

// Restore 恢复回收站文件/文件夹
func Restore(c *gin.Context) {
	var req struct {
		FileIDs []uint `json:"file_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var restored int64
	for _, fid := range req.FileIDs {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 1", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if err := restoreTree(userID, &f); err == nil {
			restored++
		}
	}
	util.OK(c, gin.H{"restored": restored})
}

// DeletePermanent 彻底删除回收站文件/文件夹（物理删除）
func DeletePermanent(c *gin.Context) {
	var req struct {
		FileIDs []uint `json:"file_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	uname := username(c)
	// S1/L8 修复：批量彻底删除时过滤出"根节点"（父文件夹也在本次待删列表中的子文件会被剔除），
	// 子文件由根节点的递归处理完成，避免配额被重复扣减、purged 计数虚高
	roots := trashRoots(userID, req.FileIDs)
	var purged int64
	for _, fid := range roots {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 1", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if err := purgeTree(userID, uname, &f); err == nil {
			purged++
		}
	}
	util.OK(c, gin.H{"purged": purged})
}

// ClearTrash 清空回收站
func ClearTrash(c *gin.Context) {
	userID := uid(c)
	uname := username(c)
	var files []model.File
	db.DB.Where("user_id = ? AND is_deleted = 1", userID).Find(&files)
	// S1/L8 修复：仅对"根节点"彻底删除（父级也在回收站的子文件由根节点递归处理），
	// 避免清空回收站时对子文件二次处理导致配额重复扣减、计数虚高
	ids := make([]uint, 0, len(files))
	for i := range files {
		ids = append(ids, files[i].ID)
	}
	roots := trashRoots(userID, ids)
	var purged int64
	for _, fid := range roots {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 1", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if err := purgeTree(userID, uname, &f); err == nil {
			purged++
		}
	}
	util.OK(c, gin.H{"purged": purged})
}

// SearchFiles 全局搜索：关键字 + 类型筛选，返回最近 200 条
func SearchFiles(c *gin.Context) {
	kw := strings.TrimSpace(c.Query("keyword"))
	if kw == "" {
		util.Fail(c, 400, "请输入搜索关键字")
		return
	}
	ftype, _ := strconv.Atoi(c.DefaultQuery("type", "-1"))
	userID := uid(c)
	query := db.DB.Where("user_id = ? AND is_deleted = 0 AND name LIKE ?", userID, "%"+kw+"%")
	if ftype == 0 || ftype == 1 {
		query = query.Where("type = ?", ftype)
	}
	var files []model.File
	query.Order("type ASC, updated_at DESC").Limit(200).Find(&files)
	util.OK(c, gin.H{"items": files})
}

// RecentFiles 最近文件（仅文件，按更新时间倒序）
func RecentFiles(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 200 {
		limit = 50
	}
	userID := uid(c)
	var files []model.File
	db.DB.Where("user_id = ? AND is_deleted = 0 AND type = 1", userID).
		Order("updated_at DESC").Limit(limit).Find(&files)
	util.OK(c, gin.H{"items": files})
}

// FavoriteFile 收藏/星标
func FavoriteFile(c *gin.Context) {
	var req struct {
		FileID uint `json:"file_id" binding:"required"`
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
	// L3 修复：配合 (user_id, file_id) 唯一索引使用 FirstOrCreate，并发收藏不会产生重复记录
	db.DB.FirstOrCreate(
		&model.Favorite{UserID: userID, FileID: req.FileID},
		model.Favorite{UserID: userID, FileID: req.FileID},
	)
	util.OK(c, gin.H{"favorited": true})
}

// UnfavoriteFile 取消收藏
func UnfavoriteFile(c *gin.Context) {
	var req struct {
		FileID uint `json:"file_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	db.DB.Where("user_id = ? AND file_id = ?", userID, req.FileID).Delete(&model.Favorite{})
	util.OK(c, gin.H{"favorited": false})
}

// ListFavorites 我的收藏
func ListFavorites(c *gin.Context) {
	userID := uid(c)
	var favs []model.Favorite
	db.DB.Where("user_id = ?", userID).Order("created_at DESC").Limit(500).Find(&favs)
	fids := make([]uint, 0, len(favs))
	for _, fv := range favs {
		fids = append(fids, fv.FileID)
	}
	var files []model.File
	if len(fids) > 0 {
		db.DB.Where("id IN ? AND is_deleted = 0", fids).Find(&files)
	}
	// 按收藏时间排序保持前端展示顺序
	order := make(map[uint]int)
	for i, fv := range favs {
		order[fv.FileID] = i
	}
	sort.Slice(files, func(i, j int) bool {
		oi, oki := order[files[i].ID]
		oj, okj := order[files[j].ID]
		if oki && okj {
			return oi < oj
		}
		return !oki && okj
	})
	util.OK(c, gin.H{"items": files})
}

// BatchMove 批量移动
func BatchMove(c *gin.Context) {
	var req struct {
		FileIDs        []uint `json:"file_ids" binding:"required"`
		TargetParentID uint   `json:"target_parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	var moved int64
	for _, fid := range req.FileIDs {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, userID).First(&f).Error; err != nil {
			continue
		}
		if req.TargetParentID == f.ID {
			continue
		}
		if err := checkTarget(userID, req.TargetParentID, fid); err != nil {
			continue
		}
		if err := checkNameDup(userID, req.TargetParentID, f.Name); err != nil {
			continue
		}
		f.ParentID = req.TargetParentID
		if err := db.DB.Save(&f).Error; err != nil {
			continue
		}
		moved++
	}
	util.OK(c, gin.H{"moved": moved})
}

// Preview 在线预览：以内联方式返回文件流，附带正确的 Content-Type
func Preview(c *gin.Context) {
	userID := uid(c)
	fid, _ := strconv.Atoi(c.Query("file_id"))
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	if f.Type != 1 {
		util.Fail(c, 400, "文件夹不可预览")
		return
	}
	if _, err := os.Stat(f.StoragePath); err != nil {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	ext := strings.ToLower(filepath.Ext(f.Name))
	ctype := ""
	switch ext {
	case ".png":
		ctype = "image/png"
	case ".jpg", ".jpeg":
		ctype = "image/jpeg"
	case ".gif":
		ctype = "image/gif"
	case ".webp":
		ctype = "image/webp"
	case ".bmp":
		ctype = "image/bmp"
	// M2 修复：按扩展名精确映射视频/音频 MIME，修复 webm/ogg/mov/wav/flac 等因错误类型无法播放的问题
	case ".mp4", ".m4v":
		ctype = "video/mp4"
	case ".webm":
		ctype = "video/webm"
	case ".ogg":
		ctype = "video/ogg"
	case ".mov":
		ctype = "video/quicktime"
	case ".mp3":
		ctype = "audio/mpeg"
	case ".wav":
		ctype = "audio/wav"
	case ".flac":
		ctype = "audio/flac"
	case ".m4a":
		ctype = "audio/mp4"
	case ".aac":
		ctype = "audio/aac"
	case ".pdf":
		ctype = "application/pdf"
	case ".txt", ".md", ".log", ".json", ".yaml", ".yml", ".csv":
		ctype = "text/plain; charset=utf-8"
	// S5 修复：html/htm 不再以 text/html 内联返回（存储型 XSS 风险，可窃取同域 localStorage 中的 JWT），
	// 降级为纯文本；svg 也移出内联白名单（回退到 default 的 octet-stream，强制下载）
	case ".html", ".htm":
		ctype = "text/plain; charset=utf-8"
	default:
		ctype = "application/octet-stream"
	}
	c.Header("Content-Type", ctype)
	c.Header("Content-Disposition", "inline")
	c.File(f.StoragePath)
}

// StorageOverview 存储概览：配额 + 按类别统计占用，用于前端可视化
func StorageOverview(c *gin.Context) {
	userID := uid(c)
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	var files []model.File
	// M8 修复：统计口径统一——回收站文件仍占配额（moveToTrash 不扣减配额，purgeTree 才扣减），
	// 因此概览统计全部 type=1 文件（含回收站），使分类之和与 quota_used 一致；前端 StorageCard 已标注"含回收站"
	db.DB.Where("user_id = ? AND type = 1", userID).Find(&files)
	var image, video, audio, doc, other int64
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico", ".avif":
			image += f.Size
		case ".mp4", ".webm", ".ogg", ".mov", ".m4v", ".avi", ".mkv", ".flv":
			video += f.Size
		case ".mp3", ".wav", ".flac", ".m4a", ".aac", ".opus":
			audio += f.Size
		case ".pdf", ".txt", ".md", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".json", ".yaml", ".yml", ".log":
			doc += f.Size
		default:
			other += f.Size
		}
	}
	util.OK(c, gin.H{
		"quota_max":  u.QuotaMax,
		"quota_used": u.QuotaUsed,
		"categories": gin.H{"image": image, "video": video, "audio": audio, "doc": doc, "other": other},
	})
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

// updateQuota 增减配额（增加时带配额上限条件，并发下无法突破配额，L1）
func updateQuota(userID uint, delta int64) {
	if delta >= 0 {
		db.DB.Model(&model.User{}).
			Where("id = ? AND quota_used + ? <= quota_max", userID, delta).
			UpdateColumn("quota_used", gorm.Expr("quota_used + ?", delta))
		return
	}
	db.DB.Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("quota_used", gorm.Expr("GREATEST(quota_used + ?, 0)", delta))
}

// tryAddQuota 原子增加配额，返回是否成功（配额不足返回 false，L1 防并发超配额）
func tryAddQuota(userID uint, delta int64) bool {
	if delta <= 0 {
		return true
	}
	res := db.DB.Model(&model.User{}).
		Where("id = ? AND quota_used + ? <= quota_max", userID, delta).
		UpdateColumn("quota_used", gorm.Expr("quota_used + ?", delta))
	return res.RowsAffected > 0
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

// checkNameDupTx 同目录重名检查（事务内使用，供恢复等批量操作）
func checkNameDupTx(tx *gorm.DB, userID, parentID uint, name string) error {
	var count int64
	tx.Model(&model.File{}).
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

// uniqueNameTx 重名时自动添加 (n) 后缀（事务内使用）
func uniqueNameTx(tx *gorm.DB, userID, parentID uint, name string) string {
	if checkNameDupTx(tx, userID, parentID, name) == nil {
		return name
	}
	ext := filepath.Ext(name)
	base := name[:len(name)-len(ext)]
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if checkNameDupTx(tx, userID, parentID, candidate) == nil {
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

// validateName 校验文件名/文件夹名：禁止路径分隔符、控制字符，限制长度（L2）
func validateName(name string) error {
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return fmt.Errorf("名称长度不合法（1-200 个字符）")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("名称不合法")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("名称不能包含 / 或 \\")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("名称包含非法控制字符")
		}
	}
	return nil
}

// rfc5987Encode RFC 5987 文件名编码：百分号编码，空格用 %20
// （url.QueryEscape 会把空格变成 +，导致部分浏览器下载文件名错误，L4）
func rfc5987Encode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// hashFile 流式计算文件内容 SHA256，避免大文件整体读入内存（M12）
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// trashRoots 从待处理文件 ID 列表中过滤出"根节点"：父文件夹也在回收站中的条目会被剔除，
// 子文件统一由根节点的递归处理完成，避免清空回收站/批量彻底删除时重复处理
// （S1 配额重复扣减、L8 purged 计数虚高）
func trashRoots(userID uint, fileIDs []uint) []uint {
	if len(fileIDs) <= 1 {
		return fileIDs
	}
	inList := make(map[uint]bool, len(fileIDs))
	for _, id := range fileIDs {
		inList[id] = true
	}
	roots := make([]uint, 0, len(fileIDs))
	for _, id := range fileIDs {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 1", id, userID).First(&f).Error; err != nil {
			continue
		}
		if !inList[f.ParentID] {
			roots = append(roots, id)
		}
	}
	return roots
}

// moveToTrashWithTx 递归将文件/文件夹标记为回收站（软删除，不扣配额、不删磁盘；事务内执行）
func moveToTrashWithTx(tx *gorm.DB, userID uint, uname string, f *model.File) error {
	if f.Type == 0 {
		var children []model.File
		tx.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", userID, f.ID).Find(&children)
		for i := range children {
			if err := moveToTrashWithTx(tx, userID, uname, &children[i]); err != nil {
				return err
			}
		}
	}
	// 回收站文件仍计为占用配额（配额在彻底删除 purgeTree 时统一扣减，与 StorageOverview 口径一致，M8）
	// S3 修复：移入回收站时同步撤销公共共享，防止回收站文件通过共享直链被越权下载
	f.IsDeleted = 1
	f.IsShared = 0
	if err := tx.Save(f).Error; err != nil {
		return err
	}
	LogAction(userID, uname, "delete", fmt.Sprintf("移入回收站 %s", f.Name))
	return nil
}

// moveToTrash 移入回收站（事务包裹，保证整棵树原子更新，M13）
func moveToTrash(userID uint, uname string, f *model.File) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		return moveToTrashWithTx(tx, userID, uname, f)
	})
}

// restoreNode 恢复单个节点（不递归子项），恢复前自动处理同目录重名（M3）
func restoreNode(tx *gorm.DB, userID uint, f *model.File) error {
	// M3 修复：恢复前检查同目录重名，重名时自动追加序号，避免破坏"同目录名称唯一"假设
	if checkNameDupTx(tx, userID, f.ParentID, f.Name) != nil {
		f.Name = uniqueNameTx(tx, userID, f.ParentID, f.Name)
	}
	f.IsDeleted = 0
	if err := tx.Save(f).Error; err != nil {
		return err
	}
	return nil
}

// restoreTreeWithTx 递归恢复回收站文件/文件夹（事务内执行）
func restoreTreeWithTx(tx *gorm.DB, userID uint, f *model.File) error {
	// S2 修复：先向上检查祖先链，若父文件夹仍在回收站则一并恢复（仅恢复祖先节点本身），
	// 避免子文件 is_deleted=0 但父目录不可达导致"幽灵文件"（从所有列表消失、无法操作、仍占配额）
	var ancestors []*model.File
	cur := f
	for cur.ParentID != 0 {
		var p model.File
		if err := tx.Where("id = ? AND user_id = ? AND is_deleted = 1", cur.ParentID, userID).First(&p).Error; err != nil {
			break
		}
		ancestors = append(ancestors, &p)
		cur = &p
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		if err := restoreNode(tx, userID, ancestors[i]); err != nil {
			return err
		}
	}
	if f.Type == 0 {
		var children []model.File
		tx.Where("user_id = ? AND parent_id = ? AND is_deleted = 1", userID, f.ID).Find(&children)
		for i := range children {
			if err := restoreTreeWithTx(tx, userID, &children[i]); err != nil {
				return err
			}
		}
	}
	return restoreNode(tx, userID, f)
}

// restoreTree 恢复回收站文件/文件夹（事务包裹，M13）
func restoreTree(userID uint, f *model.File) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		return restoreTreeWithTx(tx, userID, f)
	})
}

// purgeTreeWithTx 递归彻底删除文件/文件夹（物理删除）并扣减配额（事务内执行）
func purgeTreeWithTx(tx *gorm.DB, userID uint, uname string, f *model.File) error {
	// S1 修复：确认记录仍存在，避免对已被递归删除的节点再次处理导致配额重复扣减
	var exist model.File
	if err := tx.Where("id = ? AND user_id = ?", f.ID, userID).First(&exist).Error; err != nil {
		return nil
	}
	// 递归删除子项
	if f.Type == 0 {
		var children []model.File
		tx.Where("user_id = ? AND parent_id = ? AND is_deleted = 1", userID, f.ID).Find(&children)
		for i := range children {
			if err := purgeTreeWithTx(tx, userID, uname, &children[i]); err != nil {
				return err
			}
		}
	}
	// 删除实体：仅当全站无其他记录引用同一 hash 时才删磁盘
	// （转送/共享等场景下多用户可能复用同一实体，需跨用户统计引用）
	if f.Type == 1 && f.StoragePath != "" {
		var refs int64
		tx.Model(&model.File{}).
			Where("hash = ? AND type = 1", f.Hash).
			Count(&refs)
		if refs <= 1 {
			os.Remove(f.StoragePath)
		}
		tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("quota_used", gorm.Expr("GREATEST(quota_used - ?, 0)", f.Size))
	}
	// 清理收藏与分享关联
	tx.Where("file_id = ?", f.ID).Delete(&model.Favorite{})
	tx.Where("file_id = ?", f.ID).Delete(&model.Share{})
	if err := tx.Delete(f).Error; err != nil {
		return err
	}
	LogAction(userID, uname, "purge", fmt.Sprintf("彻底删除 %s", f.Name))
	return nil
}

// purgeTree 彻底删除（事务包裹，整棵树原子更新，M13；存在性检查防重复扣减，S1）
func purgeTree(userID uint, uname string, f *model.File) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		return purgeTreeWithTx(tx, userID, uname, f)
	})
}
