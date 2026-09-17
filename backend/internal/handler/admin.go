package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/blob"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// ListUsers 用户列表（分页 + 关键字）
func ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := db.DB.Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	var total int64
	query.Count(&total)
	var users []model.User
	query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users)
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": users})
}

// UpdateUser 更新用户（status 禁用/启用、quota_max 配额）
func UpdateUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Status   *int   `json:"status"`
		QuotaMax *int64 `json:"quota_max"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var u model.User
	if err := db.DB.First(&u, id).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	updates := map[string]any{}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if req.QuotaMax != nil {
		updates["quota_max"] = *req.QuotaMax
	}
	if len(updates) > 0 {
		if err := db.DB.Model(&u).Updates(updates).Error; err != nil {
			util.Fail(c, 500, "更新失败")
			return
		}
	}
	// M10 修复：Updates 不写回结构体，重新查询后再返回，避免响应中携带更新前的旧数据
	db.DB.First(&u, id)
	adminLog(c, "update_user", fmt.Sprintf("修改用户 %s", u.Username))
	util.OK(c, u)
}

// DeleteUser 删除用户（连同其全部文件）
func DeleteUser(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	adminID, _ := c.Get("user_id")
	if uint(id) == adminID {
		util.Fail(c, 400, "不能删除自己")
		return
	}
	var u model.User
	if err := db.DB.First(&u, id).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	// Bug13 修复：删除用户前清理其关联记录（收藏、标签、文件-标签），避免残留孤儿关联数据；
	// FileTag 无 user_id 字段，按本用户文件/标签归属清理
	var tagIDs []uint
	db.DB.Model(&model.Tag{}).Where("user_id = ?", u.ID).Pluck("id", &tagIDs)
	var fileIDs []uint
	db.DB.Model(&model.File{}).Where("user_id = ?", u.ID).Pluck("id", &fileIDs)
	db.DB.Where("user_id = ?", u.ID).Delete(&model.Favorite{})
	db.DB.Where("user_id = ?", u.ID).Delete(&model.Tag{})
	db.DB.Where("tag_id IN ?", tagIDs).Delete(&model.FileTag{})
	db.DB.Where("file_id IN ?", fileIDs).Delete(&model.FileTag{})

	// 递归删除该用户所有文件（含磁盘实体）
	var roots []model.File
	db.DB.Where("user_id = ? AND parent_id = 0", u.ID).Find(&roots)
	// Bug13 修复：收集 purgeTree 返回的错误并汇总记录，
	// 原实现忽略返回值，子文件删除/实体释放失败被吞，可能残留孤儿索引与磁盘实体
	var purgeErrs []string
	for i := range roots {
		if err := purgeTree(u.ID, u.Username, &roots[i]); err != nil {
			purgeErrs = append(purgeErrs, fmt.Sprintf("文件 #%d: %v", roots[i].ID, err))
		}
	}
	if len(purgeErrs) > 0 {
		adminLog(c, "delete_user_purge_error", "purge 部分失败 "+strings.Join(purgeErrs, "; "))
	}
	db.DB.Where("user_id = ?", u.ID).Delete(&model.Share{})
	db.DB.Delete(&u)
	adminLog(c, "delete_user", fmt.Sprintf("删除用户 %s", u.Username))
	util.OK(c, gin.H{"deleted": u.ID})
}

// AdminListFiles 全站文件检索
func AdminListFiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := db.DB.Model(&model.File{}).Where("type = 1")
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	query.Count(&total)
	var files []model.File
	query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&files)
	// 附带归属用户名
	type item struct {
		model.File
		Username string `json:"username"`
	}
	items := make([]item, 0, len(files))
	for _, f := range files {
		var u model.User
		db.DB.First(&u, f.UserID)
		items = append(items, item{File: f, Username: u.Username})
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
}

// AdminDeleteFile 强制删除任意文件（含磁盘实体）
func AdminDeleteFile(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var f model.File
	if err := db.DB.First(&f, id).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	ownerID := f.UserID
	// L5 修复：purgeTree 日志应记录文件所有者的用户名，而非文件名
	var owner model.User
	uname := ""
	if err := db.DB.First(&owner, ownerID).Error; err == nil {
		uname = owner.Username
	}
	purgeTree(ownerID, uname, &f)
	adminLog(c, "delete_file", fmt.Sprintf("强制删除文件 #%d %s", id, f.Name))
	util.OK(c, gin.H{"deleted": id})
}

// Stats 系统统计
func Stats(c *gin.Context) {
	var userCount, fileCount, folderCount int64
	db.DB.Model(&model.User{}).Count(&userCount)
	db.DB.Model(&model.File{}).Where("type = 1").Count(&fileCount)
	db.DB.Model(&model.File{}).Where("type = 0").Count(&folderCount)
	var storageUsed struct {
		Total int64
	}
	// M8 决策：回收站文件仍占配额，storage_used 统计全部文件（含回收站）
	db.DB.Model(&model.File{}).Where("type = 1").Select("COALESCE(SUM(size),0) AS total").Scan(&storageUsed)
	var todayUploads int64
	db.DB.Model(&model.File{}).Where("type = 1 AND DATE(created_at) = CURDATE()").Count(&todayUploads)
	// 在线用户数：最近 15 分钟内有登录记录的用户（简化统计）
	var onlineUsers int64
	db.DB.Model(&model.Log{}).
		Where("action = ? AND created_at > DATE_SUB(NOW(), INTERVAL 15 MINUTE)", "login").
		Distinct("user_id").Count(&onlineUsers)

	// dedup：去重统计（物理占用 vs 逻辑占用）
	dedup := gin.H{"physical_used": int64(0), "logical_used": storageUsed.Total, "dedup_saved": int64(0)}
	if st, err := blob.Stats(); err == nil {
		dedup["physical_used"] = st.PhysicalBytes
		dedup["logical_used"] = st.LogicalBytes
		dedup["dedup_saved"] = st.SavedBytes
		dedup["blob_count"] = st.BlobCount
		dedup["ref_count_sum"] = st.RefCountSum
		dedup["trash_only_count"] = st.TrashOnlyCount
	}
	util.OK(c, gin.H{
		"user_count":    userCount,
		"file_count":    fileCount,
		"folder_count":  folderCount,
		"storage_used":  storageUsed.Total,
		"today_uploads": todayUploads,
		"online_users":  onlineUsers,
		"dedup":         dedup,
	})
}

// AdminStorageIndex 存储索引概览（物理实体统计 + 去重节省）
func AdminStorageIndex(c *gin.Context) {
	st, err := blob.Stats()
	if err != nil {
		util.Fail(c, 500, "存储统计失败: "+err.Error())
		return
	}
	util.OK(c, st)
}

// AdminListBlobs 管理端实体列表（分页 + hash 关键字）
func AdminListBlobs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	total, items, err := blob.List(page, pageSize, keyword)
	if err != nil {
		util.Fail(c, 500, "查询失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
}

// AdminBlobRefs 实体被哪些文件索引引用
func AdminBlobRefs(c *gin.Context) {
	hash := c.Param("hash")
	if hash == "" {
		util.Fail(c, 400, "hash 为空")
		return
	}
	items, err := blob.Refs(hash)
	if err != nil {
		util.Fail(c, 500, "查询失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"hash": hash, "refs": items})
}

// AdminStorageVerify 校验/修复存储索引
func AdminStorageVerify(c *gin.Context) {
	var req struct {
		Apply     bool `json:"apply"`
		GCOrphans bool `json:"gc_orphans"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Apply = false
		req.GCOrphans = false
	}
	rep, err := blob.Verify(req.Apply, req.GCOrphans)
	if err != nil {
		util.Fail(c, 500, "校验失败: "+err.Error())
		return
	}
	adminLog(c, "storage_verify", fmt.Sprintf("apply=%v gc_orphans=%v", req.Apply, req.GCOrphans))
	util.OK(c, rep)
}

// ListLogs 操作日志
func ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	db.DB.Model(&model.Log{}).Count(&total)
	var logs []model.Log
	db.DB.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs)
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": logs})
}

func adminLog(c *gin.Context, action, detail string) {
	uid, _ := c.Get("user_id")
	uname, _ := c.Get("username")
	LogAction(uid.(uint), uname.(string), action, detail)
}
