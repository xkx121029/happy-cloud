package handler

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"happy-cloud/backend/internal/blob"
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
	// 文件需校验实体存在；文件夹（type=0）无需实体，直接整棵子树转送
	if f.Type == 1 {
		if _, err := os.Stat(blobPathOf(&f)); err != nil {
			util.Fail(c, 404, "文件实体缺失")
			return
		}
	}
	transfer := model.Transfer{
		SenderID:   userID,
		SenderName: username(c),
		ReceiverID: req.ReceiverID,
		FileID:     f.ID,
		FileName:   f.Name,
		FileSize:   f.Size,
		Type:       f.Type,
		Status:     0,
	}
	if err := db.DB.Create(&transfer).Error; err != nil {
		util.Fail(c, 500, "发送失败")
		return
	}
	kind := "文件"
	if f.Type == 0 {
		kind = "文件夹"
	}
	notify := model.Notification{
		UserID:     req.ReceiverID,
		Type:       "transfer",
		Title:      "收到" + kind,
		Content:    fmt.Sprintf("%s 向你发送了%s「%s」", username(c), kind, f.Name),
		TransferID: transfer.ID,
	}
	db.DB.Create(&notify)
	LogAction(userID, username(c), "transfer", fmt.Sprintf("发送%s %s 给 %s", kind, f.Name, receiver.Username))
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

// AcceptTransfer 接受转送：转存到我的网盘根目录（复用全局实体，同一内容全站只存一份）
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
	if src.Type == 1 {
		srcPath := blobPathOf(&src)
		if _, err := os.Stat(srcPath); err != nil {
			util.Fail(c, 404, "原文件实体缺失，可能已被发送方删除")
			return
		}
	}
	var u model.User
	if err := db.DB.First(&u, userID).Error; err != nil || u.Status != 0 {
		util.Fail(c, 403, "账号不可用")
		return
	}
	name := uniqueName(userID, 0, t.FileName)
	f := model.File{
		UserID: userID, ParentID: 0, Name: name,
		Type: src.Type, Size: src.Size, Hash: src.Hash, StoragePath: srcPathOf(&src),
	}
	// 实体登记 + 索引写入 + 转送状态 + 通知已读全部同事务（Bug14 修复）：
	// 原实现状态更新与通知已读在事务外，若 Save(&t) 失败，接收方文件已建立而转送仍为
	// pending，接收方可重复接受同一文件产生重复索引；现并入事务保证原子性，
	// 并用 status=0 条件更新拦截并发重复接受
	if err := db.DB.Transaction(func(tx *gorm.DB) error {
		if src.Type == 1 {
			if _, _, err := blob.Acquire(tx, userID, src.Hash, t.FileSize); err != nil {
				return err
			}
		}
		if err := tx.Create(&f).Error; err != nil {
			return err
		}
		// 文件夹：递归复制整棵子树到接收方名下（新节点复用全局实体，不重复占物理空间）
		if src.Type == 0 {
			if err := copyTree(tx, userID, src.UserID, src.ID, f.ID, 0); err != nil {
				return err
			}
		}
		res := tx.Model(&model.Transfer{}).
			Where("id = ? AND status = 0", t.ID).Update("status", 1)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errors.New("该转送已处理")
		}
		// 关联通知标记已读
		return tx.Model(&model.Notification{}).
			Where("user_id = ? AND transfer_id = ?", userID, t.ID).
			Update("is_read", 1).Error
	}); err != nil {
		if errors.Is(err, blob.ErrQuotaExceeded) {
			util.Fail(c, 507, "存储空间不足，无法接收该文件夹")
			return
		}
		util.Fail(c, 500, "转存失败")
		return
	}
	LogAction(userID, username(c), "transfer", fmt.Sprintf("接受%s %s", kindOf(src.Type), t.FileName))
	util.OK(c, f)
}

// srcPathOf 取索引实体路径（文件夹为空串）
func srcPathOf(f *model.File) string {
	if f.Type != 1 {
		return ""
	}
	return blobPathOf(f)
}

// kindOf 类型文案
func kindOf(t int) string {
	if t == 0 {
		return "文件夹"
	}
	return "文件"
}

// copyTree 在事务内递归复制 srcID 的整棵子树到目标父目录 destParentID（接收方转存文件夹用）。
// 文件节点复用全局实体（blob.Acquire 登记引用），文件夹节点新建空目录并递归。
// depth 上限 64 层，防止异常循环引用导致无限递归。
func copyTree(tx *gorm.DB, receiverID, srcUserID, srcID, destParentID uint, depth int) error {
	if depth > 64 {
		return errors.New("目录层级过深")
	}
	var children []model.File
	if err := tx.Where("user_id = ? AND parent_id = ? AND is_deleted = 0", srcUserID, srcID).
		Order("type ASC, name ASC").Find(&children).Error; err != nil {
		return err
	}
	for i := range children {
		ch := children[i]
		name := uniqueName(receiverID, destParentID, ch.Name)
		nf := model.File{
			UserID: receiverID, ParentID: destParentID, Name: name,
			Type: ch.Type, Size: ch.Size, Hash: ch.Hash, StoragePath: srcPathOf(&ch),
		}
		if ch.Type == 1 {
			if _, _, err := blob.Acquire(tx, receiverID, ch.Hash, ch.Size); err != nil {
				return err
			}
		}
		if err := tx.Create(&nf).Error; err != nil {
			return err
		}
		if ch.Type == 0 {
			if err := copyTree(tx, receiverID, srcUserID, ch.ID, nf.ID, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
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
