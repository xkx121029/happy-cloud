package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"happy-cloud/backend/internal/blob"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/settings"
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

// extGroup 文件类型分组，数值越小越靠前；0 表示归入"其他"
func extGroup(ext string) int {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico", ".avif":
		return 1
	case ".mp4", ".webm", ".ogg", ".mov", ".m4v", ".avi", ".mkv", ".flv":
		return 2
	case ".mp3", ".wav", ".flac", ".m4a", ".aac", ".opus":
		return 3
	case ".pdf", ".txt", ".md", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".csv", ".json", ".yaml", ".yml", ".log":
		return 4
	default:
		return 5
	}
}

// sortByFields 排序字段白名单（L1 修复：仅允许枚举值，非法回退默认，禁止用户可控字符串进入 SQL）
var sortByFields = map[string]bool{"name": true, "date": true, "size": true, "type": true}

// sortDirSQL 排序方向白名单映射：用户输入 asc/desc → 常量 SQL 关键字（L1 修复，禁止用户可控字符串进入 SQL）
var sortDirSQL = map[string]string{"asc": "ASC", "desc": "DESC"}

// extGroupCaseSQL 按扩展名分组计算排序序号的 CASE 表达式（与 extGroup 分组一致；L7 修复：GORM Order 配合使用，纯常量无用户输入）
const extGroupCaseSQL = `CASE
	WHEN LOWER(substr(name, instr(name, '.') + 1)) IN ('png','jpg','jpeg','gif','webp','svg','bmp','ico','avif') THEN 1
	WHEN LOWER(substr(name, instr(name, '.') + 1)) IN ('mp4','webm','ogg','mov','m4v','avi','mkv','flv') THEN 2
	WHEN LOWER(substr(name, instr(name, '.') + 1)) IN ('mp3','wav','flac','m4a','aac','opus') THEN 3
	WHEN LOWER(substr(name, instr(name, '.') + 1)) IN ('pdf','txt','md','doc','docx','xls','xlsx','ppt','pptx','csv','json','yaml','yml','log') THEN 4
	ELSE 5
END`

// L16 修复：Move/BatchMove 事务内错误标记，用于事务外映射响应状态码
var (
	errFileNotFound = errors.New("文件不存在")
	errMoveToSelf   = errors.New("不能移动到自身")
	errMoveSave     = errors.New("移动失败")
	errNameDup      = errors.New("同名文件或文件夹已存在")
)

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
	// 排序参数：by=name|date|size|type；asc/desc（name/type 默认升序，date/size 默认降序）
	by := c.DefaultQuery("by", "name")
	// L1 修复：by 白名单校验（仅允许枚举值），非法回退默认，禁止用户可控字符串进入 SQL
	if !sortByFields[by] {
		by = "name"
	}
	dir := c.DefaultQuery("dir", "desc")
	// L1 修复：dir 白名单校验，非法回退默认；合法值映射为常量 SQL 关键字 ASC/DESC
	if d, ok := sortDirSQL[dir]; ok {
		dir = d
	} else {
		dir = sortDirSQL["desc"]
	}
	// 标签筛选：逗号分隔的 tag_id 列表，文件需包含所有指定标签
	tagIDsParam := c.Query("tag_ids")
	// 文件夹始终置顶（type=0 在前），再按排序字段排列
	var orderStr string
	switch by {
	case "date":
		// L1 修复：dir 已白名单映射为常量 ASC/DESC，此拼接安全（原实现直接拼接用户输入 dir）
		orderStr = "type ASC, created_at " + dir
	case "size":
		orderStr = "type ASC, size " + dir
	case "type":
		// 按扩展名类型分组（文件夹已在最前），同组内按名称字母序
		if dir == "DESC" {
			orderStr = "type ASC, ext_group ASC, name DESC"
		} else {
			orderStr = "type ASC, ext_group ASC, name ASC"
		}
	default:
		orderStr = "type ASC, name ASC"
	}
	query := db.DB.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", userID, parentID)

	// 标签筛选：文件需包含所有指定 tag_id（AND 逻辑）
	if tagIDsParam != "" {
		var ids []uint
		for _, p := range strings.Split(tagIDsParam, ",") {
			v, _ := strconv.ParseUint(strings.TrimSpace(p), 10, 32)
			if v > 0 {
				ids = append(ids, uint(v))
			}
		}
		if len(ids) > 0 {
			// 使用子查询：file_tags 必须包含所有指定标签
			// L7 修复：tag_id 改为 ? 参数绑定（原实现 %d 拼接字符串），并保证条件由 GORM 链式合并、可被后续查询沿用
			var condParts []string
			condArgs := make([]interface{}, 0, len(ids))
			for _, tid := range ids {
				condParts = append(condParts, "EXISTS (SELECT 1 FROM file_tags WHERE file_tags.file_id = files.id AND file_tags.tag_id = ?)")
				condArgs = append(condArgs, tid)
			}
			query = query.Where(strings.Join(condParts, " AND "), condArgs...)
		}
	}

	var total int64
	query.Model(&model.File{}).Count(&total)
	var files []model.File
	if by == "type" {
		// 按扩展名类型分组（文件夹已在最前），同组内按名称字母序
		// L7 修复：不再使用 query.Raw（Raw 不会合并链式 Where 累积的条件，
		// 导致按类型排序时上方标签筛选的 EXISTS 子查询被静默丢弃），
		// 改用 GORM Order 配合常量 CASE 表达式（extGroupCaseSQL，无用户输入，无注入面）；
		// 原实现：query.Raw(`SELECT * FROM files WHERE user_id = ? AND parent_id = ? AND is_deleted = 0
		//	ORDER BY type ASC, CASE ... END ASC, name `+dir+` LIMIT ? OFFSET ?`, ...).Scan(&files)
		query.Order("type ASC").
			Order(gorm.Expr(extGroupCaseSQL + " ASC")).
			Order("name " + dir).
			Offset((page - 1) * pageSize).Limit(pageSize).
			Find(&files)
	} else {
		query.Order(orderStr).
			Offset((page - 1) * pageSize).Limit(pageSize).
			Find(&files)
	}
	// 为每个文件附带标签
	type fileWithTags struct {
		model.File
		Tags []model.Tag `json:"tags"`
	}
	// L perf：批量一次性查询本页所有文件的标签，消除“每文件一次查询”的 N+1 问题
	tagMap := make(map[uint][]model.Tag, len(files))
	if len(files) > 0 {
		ids := make([]uint, 0, len(files))
		for _, f := range files {
			ids = append(ids, f.ID)
		}
		var rows []struct {
			FileID uint   `gorm:"column:file_id"`
			ID     uint   `gorm:"column:id"`
			Name   string `gorm:"column:name"`
			Color  string `gorm:"column:color"`
		}
		db.DB.Table("file_tags").
			Select("file_tags.file_id, tags.id, tags.name, tags.color").
			Joins("JOIN tags ON tags.id = file_tags.tag_id").
			Where("file_tags.file_id IN ?", ids).
			Find(&rows)
		for _, r := range rows {
			tagMap[r.FileID] = append(tagMap[r.FileID], model.Tag{ID: r.ID, Name: r.Name, Color: r.Color})
		}
	}
	items := make([]fileWithTags, 0, len(files))
	for _, f := range files {
		items = append(items, fileWithTags{File: f, Tags: tagMap[f.ID]})
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
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

// UploadHash 秒传校验：全站实体（内容寻址）已存在则直接建索引记录。
// 去重后同一内容只保留一份物理实体，因此秒传可跨用户、跨目录命中。
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
	// 统一小写：hash 是实体主键，大小写不一致会造成同一内容被当成两个实体
	req.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
	userID := uid(c)
	// L2 修复：秒传同样校验文件名合法性
	if err := validateName(req.Name); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
	if err := checkNameDup(userID, req.ParentID, req.Name); err != nil {
		util.Fail(c, 409, err.Error())
		return
	}
	// 全局实体命中：任何用户上传过同一内容即可秒传（大小必须一致，防止伪造 hash）
	if b, err := blob.Get(req.Hash); err == nil && b.Size == req.Size {
		f := model.File{
			UserID: userID, ParentID: req.ParentID, Name: req.Name,
			Type: 1, Size: req.Size, Hash: req.Hash, StoragePath: util.BlobPath(req.Hash),
		}
		if err := db.DB.Transaction(func(tx *gorm.DB) error {
			if _, _, err := blob.Acquire(tx, userID, req.Hash, req.Size); err != nil {
				return err
			}
			return tx.Create(&f).Error
		}); err != nil {
			failBlob(c, err)
			return
		}
		LogAction(userID, username(c), "upload", fmt.Sprintf("秒传 %s", req.Name))
		util.OK(c, gin.H{"exists": true, "file_id": f.ID})
		return
	}
	// 兜底：存量未迁移数据（同一用户同 hash 已有实体）仍走复用
	var exist model.File
	err := db.DB.Where("user_id = ? AND hash = ? AND type = 1", userID, req.Hash).
		Order("id ASC").First(&exist).Error
	if err == nil && exist.StoragePath != "" {
		f := model.File{
			UserID: userID, ParentID: req.ParentID, Name: req.Name,
			Type: 1, Size: req.Size, Hash: req.Hash, StoragePath: blobPathOf(&exist),
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
	// 超大文件由客户端走分片，此处仅允许 <= 上传上限（管理面板可配置，兜底取配置阈值）
	if file.Size > settings.GetInt64(settings.KeyMaxUploadMB, config.Cfg.LargeFileMB)*1024*1024 {
		util.Fail(c, 400, "文件过大，请使用分片上传")
		return
	}
	// L2 修复：上传文件名校验（禁止路径分隔符/控制字符、限制长度）
	if err := validateName(file.Filename); err != nil {
		util.Fail(c, 400, err.Error())
		return
	}
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
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
	path := util.BlobPath(hash)
	// 实体已存在（秒传命中）则跳过写盘；否则先写 .part 再改名，避免半成品实体被其他请求读到
	if !blob.Exists(hash) {
		if err := util.EnsureDir(filepath.Dir(path)); err != nil {
			util.Fail(c, 500, "存储目录异常")
			return
		}
		tmp := path + ".part"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			util.Fail(c, 500, "写入文件失败")
			return
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
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
	// 实体登记 + 索引写入同事务：实体首次入库时计费（去重后计费），失败则整体回滚
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if _, _, err := blob.Acquire(tx, userID, hash, file.Size); err != nil {
			return err
		}
		return tx.Create(&f).Error
	}); err != nil {
		cleanupUnreferenced(hash, path)
		failBlob(c, err)
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

// UploadMerge 合并分片，参数 hash, name, size, parent_id, chunk_total。
// 合并时边写边算 SHA256（不再二次全量读盘），校验通过后直接落到全局实体路径。
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
	// 统一小写：hash 是实体主键，大小写不一致会造成同一内容被当成两个实体
	req.Hash = strings.ToLower(strings.TrimSpace(req.Hash))
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
	// 全局实体已在册且物理文件在盘：无需合并，直接建索引（跨用户秒传）
	if b, err := blob.Get(req.Hash); err == nil && b.Size == req.Size && blob.Exists(req.Hash) {
		os.RemoveAll(dir)
		if err := createIndexForBlob(c, userID, req.Hash, req.Size, req.Name, req.ParentID, "分片上传"); err != nil {
			return
		}
		return
	}
	path := util.BlobPath(req.Hash)
	if err := util.EnsureDir(filepath.Dir(path)); err != nil {
		util.Fail(c, 500, "存储目录异常")
		return
	}
	// 先写 .merge 中转文件，避免半成品实体被其他请求当作完整内容读到
	tmp := path + ".merge"
	final, err := os.Create(tmp)
	if err != nil {
		util.Fail(c, 500, "创建文件失败")
		return
	}
	// 边合并边增量计算 SHA256（原实现合并后再全量读盘一次，大文件下是双倍 IO）
	h := sha256.New()
	w := io.MultiWriter(final, h)
	for _, p := range parts {
		b, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(p)))
		if err != nil {
			final.Close()
			os.Remove(tmp)
			util.Fail(c, 400, "分片读取失败")
			return
		}
		if _, err := w.Write(b); err != nil {
			final.Close()
			os.Remove(tmp)
			util.Fail(c, 500, "合并写入失败")
			return
		}
	}
	final.Sync()
	final.Close()
	// 校验合并后大小
	if fi, err := os.Stat(tmp); err != nil || fi.Size() != req.Size {
		os.Remove(tmp)
		util.Fail(c, 400, "合并文件大小不一致")
		return
	}
	// M12 修复：比对增量计算的 SHA256 与请求 hash，防止客户端伪造 hash 命中错误秒传内容
	if got := hex.EncodeToString(h.Sum(nil)); got != req.Hash {
		os.Remove(tmp)
		util.Fail(c, 400, "文件内容校验失败")
		return
	}
	// 清理临时分片
	os.RemoveAll(dir)
	if blob.Exists(req.Hash) {
		// 并发场景下其他请求已生成同一实体，直接复用
		os.Remove(tmp)
	} else if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		util.Fail(c, 500, "写入文件失败")
		return
	}
	if err := createIndexForBlob(c, userID, req.Hash, req.Size, req.Name, req.ParentID, "分片上传"); err != nil {
		return
	}
}

// createIndexForBlob 实体已在盘时的索引写入：登记引用（首次引用者计费）+ 建立文件索引，
// 两者同事务提交；失败则回收无引用实体。成功时直接写出响应。
func createIndexForBlob(c *gin.Context, userID uint, hash string, size int64, name string, parentID uint, action string) error {
	name = uniqueName(userID, parentID, name)
	f := model.File{
		UserID: userID, ParentID: parentID, Name: name,
		Type: 1, Size: size, Hash: hash, StoragePath: util.BlobPath(hash),
	}
	path := util.BlobPath(hash)
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if _, _, err := blob.Acquire(tx, userID, hash, size); err != nil {
			return err
		}
		return tx.Create(&f).Error
	}); err != nil {
		cleanupUnreferenced(hash, path)
		failBlob(c, err)
		return err
	}
	LogAction(userID, username(c), "upload", fmt.Sprintf("%s %s (%.1fMB)", action, name, float64(size)/1024/1024))
	util.OK(c, f)
	return nil
}

// UploadStatus 断点续传查询：返回该 hash 已成功上传的分片序号；
// 若全站实体已存在（内容寻址命中）则 exists=true，客户端可直接跳过全部分片调秒传接口。
func UploadStatus(c *gin.Context) {
	hash := strings.ToLower(strings.TrimSpace(c.Query("hash")))
	if hash == "" {
		util.Fail(c, 400, "参数错误")
		return
	}
	if b, err := blob.Get(hash); err == nil && blob.Exists(hash) {
		util.OK(c, gin.H{"exists": true, "size": b.Size, "uploaded": []int{}})
		return
	}
	dir := util.ChunkDir(uid(c), hash)
	uploaded := []int{}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if n, err := strconv.Atoi(e.Name()); err == nil {
				uploaded = append(uploaded, n)
			}
		}
	}
	sort.Ints(uploaded)
	util.OK(c, gin.H{"exists": false, "size": 0, "uploaded": uploaded})
}

// FileDetail 文件详情：索引信息 + 全局实体信息（引用数、真实大小、计费归属、是否去重共享），
// 让"服务器只存一份、删除只删索引"对用户可见。
func FileDetail(c *gin.Context) {
	userID := uid(c)
	fid, _ := strconv.Atoi(c.Query("file_id"))
	var f model.File
	if err := db.DB.Where("id = ? AND user_id = ?", fid, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	detail := gin.H{
		"file":         f,
		"ref_count":    0,
		"blob_size":    f.Size,
		"billed_self":  false,
		"on_disk":      false,
		"dedup":        false,
		"storage_path": blobPathOf(&f),
		"migrated":     false,
	}
	if f.Type == 1 && f.Hash != "" {
		if b, err := blob.Get(f.Hash); err == nil {
			detail["ref_count"] = b.RefCount
			detail["blob_size"] = b.Size
			detail["billed_self"] = b.BilledUserID == userID
			detail["on_disk"] = blob.Exists(f.Hash)
			detail["dedup"] = b.RefCount > 1
			detail["migrated"] = true
		}
	}
	util.OK(c, detail)
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
	path := blobPathOf(&f)
	if _, err := os.Stat(path); err != nil {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	// L4 修复：使用 RFC 5987 百分号编码（空格为 %20），避免 url.QueryEscape 把空格变成 + 导致部分浏览器文件名错误
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", rfc5987Encode(f.Name)))
	c.File(path)
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
	// L16 修复：校验与写入放入同一事务（原 checkTarget 向上遍历与 Save 非原子，
	// 并发 A→B、B→A 可各自通过检查后写入，造成目录树成环）
	var f model.File
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
			return errFileNotFound
		}
		if req.TargetParentID == f.ID {
			return errMoveToSelf
		}
		// 校验目标目录归属，并防止移动到自身子目录（事务内执行）
		if err := checkTargetTx(tx, userID, req.TargetParentID, req.FileID); err != nil {
			return err
		}
		if err := checkNameDupTx(tx, userID, req.TargetParentID, f.Name); err != nil {
			return err
		}
		f.ParentID = req.TargetParentID
		if err := tx.Save(&f).Error; err != nil {
			return errMoveSave
		}
		return nil
	}); err != nil {
		switch {
		case errors.Is(err, errFileNotFound):
			util.Fail(c, 404, err.Error())
		case errors.Is(err, errMoveToSelf):
			util.Fail(c, 400, err.Error())
		case errors.Is(err, errMoveSave):
			util.Fail(c, 500, err.Error())
		case errors.Is(err, errNameDup):
			util.Fail(c, 409, err.Error())
		default:
			// checkTargetTx 的业务校验错误（目标不存在/非文件夹/自身子目录/层级异常）
			util.Fail(c, 400, err.Error())
		}
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
		// L16 修复：每项"校验 + 写入"放入独立事务，缩小 TOCTOU 窗口（原 checkTarget 检查与 Save 非原子）
		ok := false
		err := db.DB.Transaction(func(tx *gorm.DB) error {
			var f model.File
			if err := tx.Where("id = ? AND user_id = ? AND is_deleted = 0", fid, userID).First(&f).Error; err != nil {
				return nil // 文件不存在，跳过
			}
			if req.TargetParentID == f.ID {
				return nil // 不能移动到自身，跳过
			}
			// 校验目标目录归属，并防止移动到自身子目录（事务内执行）
			if err := checkTargetTx(tx, userID, req.TargetParentID, fid); err != nil {
				return nil // 目标非法，跳过
			}
			if err := checkNameDupTx(tx, userID, req.TargetParentID, f.Name); err != nil {
				return nil // 同目录重名，跳过
			}
			f.ParentID = req.TargetParentID
			if err := tx.Save(&f).Error; err != nil {
				return err
			}
			ok = true
			return nil
		})
		if err == nil && ok {
			moved++
		}
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
	path := blobPathOf(&f)
	if _, err := os.Stat(path); err != nil {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	ext := strings.ToLower(filepath.Ext(f.Name))
	ctype := ""
	switch ext {
	case ".png":
		ctype = "image/png"
	case ".jpg", ".jpeg", ".jfif":
		ctype = "image/jpeg"
	case ".gif":
		ctype = "image/gif"
	case ".webp":
		ctype = "image/webp"
	case ".bmp":
		ctype = "image/bmp"
	case ".ico":
		ctype = "image/x-icon"
	case ".apng":
		ctype = "image/apng"
	case ".avif":
		ctype = "image/avif"
	// M2 修复：按扩展名精确映射视频/音频 MIME，修复 webm/ogg/mov/wav/flac 等因错误类型无法播放的问题
	case ".mp4", ".m4v":
		ctype = "video/mp4"
	case ".webm":
		ctype = "video/webm"
	case ".ogg", ".ogv":
		ctype = "video/ogg"
	case ".mov":
		ctype = "video/quicktime"
	case ".mkv":
		ctype = "video/x-matroska"
	case ".3gp":
		ctype = "video/3gpp"
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
	case ".opus", ".oga":
		ctype = "audio/ogg"
	case ".weba":
		ctype = "audio/webm"
	case ".aif", ".aiff":
		ctype = "audio/aiff"
	case ".pdf":
		ctype = "application/pdf"
	case ".txt", ".md", ".log", ".json", ".yaml", ".yml", ".csv":
		ctype = "text/plain; charset=utf-8"
	// S5 修复：html/htm 不再以 text/html 内联返回（存储型 XSS 风险，可窃取同域 localStorage 中的 JWT），
	// 降级为纯文本；svg 也移出内联白名单（回退到 default 的 octet-stream，强制下载）
	case ".html", ".htm":
		ctype = "text/plain; charset=utf-8"
	default:
		// 代码/配置/字幕等纯文本类一律按纯文本内联，供前端文本预览
		if isTextExt(ext) {
			ctype = "text/plain; charset=utf-8"
		} else {
			ctype = "application/octet-stream"
		}
	}
	c.Header("Content-Type", ctype)
	c.Header("Content-Disposition", "inline")
	// 预览走 Range 分片读取（c.File 内部由 http.ServeFile 处理），允许拖动进度条与边下边播
	c.Header("Accept-Ranges", "bytes")
	// token 可能经 URL 传递，禁止 Referer 外泄到第三方
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "private, max-age=0")
	c.File(path)
}

// isTextExt 判断扩展名是否属于可安全按纯文本内联的代码/配置/字幕类
func isTextExt(ext string) bool {
	if ext == "" {
		return false
	}
	switch ext {
	case ".jsonl", ".ndjson", ".jsonc", ".geojson",
		".toml", ".ini", ".conf", ".cfg", ".properties", ".env", ".editorconfig",
		".tsv", ".xml", ".plist",
		".css", ".scss", ".less", ".sass", ".styl",
		".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".vue", ".svelte", ".astro",
		".go", ".py", ".rb", ".php", ".java", ".kt", ".kts", ".scala", ".groovy",
		".rs", ".c", ".h", ".cpp", ".cc", ".cxx", ".hpp", ".cs", ".swift", ".m", ".mm",
		".dart", ".lua", ".r", ".pl", ".pm", ".ex", ".exs", ".erl", ".hs", ".clj",
		".sh", ".bash", ".zsh", ".fish", ".bat", ".cmd", ".ps1",
		".sql", ".graphql", ".gql", ".proto", ".thrift",
		".diff", ".patch", ".srt", ".vtt", ".ass", ".ssa",
		".tex", ".rst", ".adoc", ".org", ".textile",
		".gradle", ".cmake", ".mk", ".lock",
		".pem", ".crt", ".key", ".pub":
		return true
	}
	return false
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
		"dedup":      dedupStats(userID, files),
	})
}

// dedupStats 去重统计：同一内容（hash）在全站只存一份，本用户的逻辑占用与去重后实际占用之差即节省量
func dedupStats(userID uint, files []model.File) gin.H {
	logical := int64(0)
	sizes := map[string]int64{} // 每个唯一内容只计一次
	for _, f := range files {
		logical += f.Size
		if _, ok := sizes[f.Hash]; !ok {
			sizes[f.Hash] = f.Size
		}
	}
	hashes := make([]string, 0, len(sizes))
	for h := range sizes {
		if h != "" {
			hashes = append(hashes, h)
		}
	}
	blobSizes := map[string]int64{}
	if len(hashes) > 0 {
		var bs []model.Blob
		db.DB.Select("hash, size").Where("hash IN ?", hashes).Find(&bs)
		for _, b := range bs {
			blobSizes[b.Hash] = b.Size
		}
	}
	physical := int64(0)
	for h, sz := range sizes {
		if s, ok := blobSizes[h]; ok {
			physical += s
		} else {
			physical += sz // 未迁移存量按其自身大小计入
		}
	}
	saved := logical - physical
	if saved < 0 {
		saved = 0
	}
	var charged int64
	db.DB.Model(&model.Blob{}).Where("billed_user_id = ?", userID).Count(&charged)
	return gin.H{
		"file_count":     len(files),
		"unique_count":   len(sizes),
		"logical_bytes":  logical,
		"physical_bytes": physical,
		"saved_bytes":    saved,
		"charged_count":  charged,
	}
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
		// L16 修复：返回哨兵错误 errNameDup，供 Move 事务外映射 409 状态码（消息与原来一致）
		return errNameDup
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

// checkTargetTx 校验目标目录归属且不是自身子目录（事务内使用，供 Move/BatchMove 缩小 TOCTOU 窗口，L16）。
// L3 修复：目标目录必须可见（is_deleted=0），禁止移动到回收站内的文件夹，
// 否则会产生 is_deleted=0 的子文件挂在 is_deleted=1 的父目录下的"幽灵文件"（无法释放配额）。
// L16 修复：祖先向上遍历加深度上限（64 层），目录树成环时超限即视为非法目标，防止死循环。
func checkTargetTx(tx *gorm.DB, userID, targetParentID, selfID uint) error {
	if targetParentID == 0 {
		return nil
	}
	var t model.File
	if err := tx.Where("id = ? AND user_id = ? AND is_deleted = 0", targetParentID, userID).First(&t).Error; err != nil {
		return fmt.Errorf("目标文件夹不存在")
	}
	if t.Type != 0 {
		return fmt.Errorf("目标必须是文件夹")
	}
	// 向上遍历检查是否把文件移动到自己的后代中（最多 64 层，L16 防成环死循环）
	const maxDepth = 64
	cur := targetParentID
	for depth := 0; cur != 0 && depth < maxDepth; depth++ {
		if cur == selfID {
			return fmt.Errorf("不能移动到自身子目录")
		}
		var p model.File
		if err := tx.Where("id = ? AND user_id = ?", cur, userID).First(&p).Error; err != nil {
			return nil // 祖先链断裂（孤儿节点），视为无环
		}
		cur = p.ParentID
	}
	if cur != 0 {
		// L16 修复：达到深度上限仍未遍历到根，说明目录树成环或层级过深，视为非法目标
		return fmt.Errorf("目标目录层级异常")
	}
	return nil
}

// checkTarget 校验目标目录归属且不是自身子目录（L3：目标必须可见 is_deleted=0；L16：深度上限防成环死循环）
func checkTarget(userID, targetParentID, selfID uint) error {
	return checkTargetTx(db.DB, userID, targetParentID, selfID)
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

// blobPathOf 返回索引对应的实体物理路径：优先全局内容寻址路径（实体已存在时），
// 否则回退到记录里的 StoragePath（仅未迁移的存量数据会走这里）。
func blobPathOf(f *model.File) string {
	if f == nil {
		return ""
	}
	if f.Hash != "" {
		p := util.BlobPath(f.Hash)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return f.StoragePath
}

// failBlob 实体登记或索引写入失败时的统一响应（配额不足 → 507，其余 → 500）
func failBlob(c *gin.Context, err error) {
	if errors.Is(err, blob.ErrQuotaExceeded) {
		util.Fail(c, 507, "存储空间不足")
		return
	}
	util.Fail(c, 500, "创建记录失败")
}

// cleanupUnreferenced 事务失败后的兜底清理：实体尚未登记（无人引用）时删除刚写入的物理文件，
// 避免留下无索引的孤儿实体；若已被其他请求抢先登记则保留。
func cleanupUnreferenced(hash, path string) {
	if hash == "" {
		return
	}
	if _, err := blob.Get(hash); err == nil {
		return
	}
	os.Remove(path)
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

// purgeTreeWithTx 递归彻底删除文件/文件夹（物理删除）并释放实体引用（事务内执行）。
// "用户删除只删除索引，不删除源文件"：物理实体由 blob.Release 统一裁决——
// 全站仍有任何索引（含他人、含回收站）引用则保留；引用归零才回收实体并退还配额。
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
	fType, fHash, fSize, fPath := f.Type, f.Hash, f.Size, f.StoragePath
	// 清理收藏与分享关联
	tx.Where("file_id = ?", f.ID).Delete(&model.Favorite{})
	tx.Where("file_id = ?", f.ID).Delete(&model.Share{})
	if err := tx.Delete(f).Error; err != nil {
		return err
	}
	// 索引已删除后再释放实体引用（userHasHash 需要看到删除后的状态）
	if fType == 1 {
		if err := blob.Release(tx, userID, fHash, fSize, fPath); err != nil {
			return err
		}
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

// FileAncestors 目录祖先链：从 folder_id 向上回溯到根，返回从根到当前文件夹的路径数组。
// folder_id=0（根目录）返回空数组。用于 URL 直达/刷新时重建面包屑。
func FileAncestors(c *gin.Context) {
	userID := uid(c)
	folderID, _ := strconv.ParseUint(c.DefaultQuery("folder_id", "0"), 10, 32)
	var path []model.File
	cur := uint(folderID)
	for cur != 0 {
		var f model.File
		if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", cur, userID).First(&f).Error; err != nil {
			// 目录不存在或不属于该用户 → 视为根目录，避免泄露他人路径
			path = nil
			break
		}
		path = append([]model.File{f}, path...) // 根在前
		cur = f.ParentID
	}
	items := make([]gin.H, 0, len(path))
	for _, f := range path {
		items = append(items, gin.H{"id": f.ID, "name": f.Name, "parent_id": f.ParentID})
	}
	util.OK(c, items)
}
