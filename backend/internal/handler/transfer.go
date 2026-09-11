package handler

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

// ---------- 用户搜索（私发选择接收者） ----------

// SearchUsers 按用户名/邮箱模糊搜索用户（排除自己）
func SearchUsers(c *gin.Context) {
	keyword := c.Query("keyword")
	userID := uid(c)
	query := db.DB.Model(&model.User{}).
		Where("id <> ? AND status = 0", userID)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", like, like)
	}
	var users []model.User
	if err := query.Order("username ASC").Limit(20).Find(&users).Error; err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	items := make([]gin.H, 0, len(users))
	for _, u := range users {
		items = append(items, gin.H{"id": u.ID, "username": u.Username, "email": u.Email})
	}
	util.OK(c, items)
}

// ---------- 文件转送（私聊发送） ----------

// SendTransfer 发送文件给指定用户，接收方生成待处理通知
func SendTransfer(c *gin.Context) {
	var req struct {
		ReceiverID uint `json:"receiver_id" binding:"required"`
		FileID     uint `json:"file_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	userID := uid(c)
	if req.ReceiverID == userID {
		util.Fail(c, 400, "不能发送给自己")
		return
	}
	var receiver model.User
	if err := db.DB.First(&receiver, req.ReceiverID).Error; err != nil {
		util.Fail(c, 404, "接收用户不存在")
		return
	}
	if receiver.Status != 0 {
		util.Fail(c, 403, "接收用户已被禁用")
		return
	}
	var f model.File
	// M4 修复：不允许发送回收站中的文件
	if err := db.DB.Where("id = ? AND user_id = ? AND is_deleted = 0", req.FileID, userID).First(&f).Error; err != nil {
		util.Fail(c, 404, "文件不存在")
		return
	}
	if f.Type != 1 {
		util.Fail(c, 400, "仅文件可以发送")
		return
	}
	if _, err := os.Stat(f.StoragePath); err != nil {
		util.Fail(c, 404, "文件实体缺失")
		return
	}
	transfer := model.Transfer{
		SenderID:   userID,
		SenderName: username(c),
		ReceiverID: req.ReceiverID,
		FileID:     f.ID,
		FileName:   f.Name,
		FileSize:   f.Size,
		Status:     0,
	}
	if err := db.DB.Create(&transfer).Error; err != nil {
		util.Fail(c, 500, "发送失败")
		return
	}
	notify := model.Notification{
		UserID:     req.ReceiverID,
		Type:       "transfer",
		Title:      "收到文件",
		Content:    fmt.Sprintf("%s 向你发送了文件「%s」（%.1fMB）", username(c), f.Name, float64(f.Size)/1024/1024),
		TransferID: transfer.ID,
	}
	db.DB.Create(&notify)
	LogAction(userID, username(c), "transfer", fmt.Sprintf("发送文件 %s 给 %s", f.Name, receiver.Username))
	util.OK(c, transfer)
}

// IncomingTransfers 我收到的转送列表
func IncomingTransfers(c *gin.Context) {
	userID := uid(c)
	var list []model.Transfer
	if err := db.DB.Where("receiver_id = ?", userID).
		Order("created_at DESC").Limit(50).Find(&list).Error; err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	util.OK(c, list)
}

// AcceptTransfer 接受转送：转存到我的网盘根目录（复用实体，不重复存盘）
func AcceptTransfer(c *gin.Context) {
	userID := uid(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var t model.Transfer
	if err := db.DB.Where("id = ? AND receiver_id = ?", id, userID).First(&t).Error; err != nil {
		util.Fail(c, 404, "转送记录不存在")
		return
	}
	if t.Status != 0 {
		util.Fail(c, 400, "该转送已处理")
		return
	}
	var src model.File
	// M4 修复：接收时同样校验原文件未被软删除，防止接收回收站文件（实体仍在磁盘）
	if err := db.DB.Where("id = ? AND is_deleted = 0", t.FileID).First(&src).Error; err != nil {
		util.Fail(c, 404, "原文件不存在，可能已被发送方删除")
		return
	}
	if _, err := os.Stat(src.StoragePath); err != nil {
		util.Fail(c, 404, "原文件实体缺失，可能已被发送方删除")
		return
	}
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
		return
	}
	if u.QuotaUsed+t.FileSize > u.QuotaMax {
		util.Fail(c, 507, "存储空间不足，无法接收该文件")
		return
	}
	name := uniqueName(userID, 0, t.FileName)
	f := model.File{
		UserID: userID, ParentID: 0, Name: name,
		Type: 1, Size: t.FileSize, Hash: src.Hash, StoragePath: src.StoragePath,
	}
	if err := db.DB.Create(&f).Error; err != nil {
		util.Fail(c, 500, "转存失败")
		return
	}
	// L1 修复：配额增加改为原子条件更新（并发下防止突破配额），失败则回滚记录
	if !tryAddQuota(userID, t.FileSize) {
		db.DB.Delete(&f)
		util.Fail(c, 507, "存储空间不足，无法接收该文件")
		return
	}
	t.Status = 1
	db.DB.Save(&t)
	// 关联通知标记已读
	db.DB.Model(&model.Notification{}).
		Where("user_id = ? AND transfer_id = ?", userID, t.ID).
		Update("is_read", 1)
	LogAction(userID, username(c), "transfer", fmt.Sprintf("接受文件 %s", t.FileName))
	util.OK(c, f)
}

// RejectTransfer 拒绝转送
func RejectTransfer(c *gin.Context) {
	userID := uid(c)
	id, _ := strconv.Atoi(c.Param("id"))
	var t model.Transfer
	if err := db.DB.Where("id = ? AND receiver_id = ?", id, userID).First(&t).Error; err != nil {
		util.Fail(c, 404, "转送记录不存在")
		return
	}
	if t.Status != 0 {
		util.Fail(c, 400, "该转送已处理")
		return
	}
	t.Status = 2
	if err := db.DB.Save(&t).Error; err != nil {
		util.Fail(c, 500, "操作失败")
		return
	}
	db.DB.Model(&model.Notification{}).
		Where("user_id = ? AND transfer_id = ?", userID, t.ID).
		Update("is_read", 1)
	LogAction(userID, username(c), "transfer", fmt.Sprintf("拒绝文件 %s", t.FileName))
	util.OK(c, t)
}

// ---------- 通知 ----------

// ListNotifications 通知列表（附转送状态，供前端渲染接受/拒绝按钮）
func ListNotifications(c *gin.Context) {
	userID := uid(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	var total int64
	db.DB.Model(&model.Notification{}).Where("user_id = ?", userID).Count(&total)
	var list []model.Notification
	if err := db.DB.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error; err != nil {
		util.Fail(c, 500, "查询失败")
		return
	}
	// 补充转送状态
	type notifyItem struct {
		model.Notification
		TransferStatus int `json:"transfer_status"` // -1 无关联转送
	}
	items := make([]notifyItem, 0, len(list))
	for _, n := range list {
		status := -1
		if n.Type == "transfer" && n.TransferID > 0 {
			var t model.Transfer
			if err := db.DB.First(&t, n.TransferID).Error; err == nil {
				status = t.Status
			}
		}
		items = append(items, notifyItem{Notification: n, TransferStatus: status})
	}
	util.OK(c, gin.H{"total": total, "page": page, "page_size": pageSize, "items": items})
}

// ReadNotification 标记单条通知已读
func ReadNotification(c *gin.Context) {
	userID := uid(c)
	id, _ := strconv.Atoi(c.Param("id"))
	res := db.DB.Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", 1)
	if res.RowsAffected == 0 {
		util.Fail(c, 404, "通知不存在")
		return
	}
	util.OK(c, gin.H{"ok": true})
}

// UnreadNotifications 未读通知数
func UnreadNotifications(c *gin.Context) {
	userID := uid(c)
	var count int64
	db.DB.Model(&model.Notification{}).
		Where("user_id = ? AND is_read = 0", userID).Count(&count)
	util.OK(c, gin.H{"count": count})
}
