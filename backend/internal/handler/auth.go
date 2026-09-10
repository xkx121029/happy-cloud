package handler

import (
	"errors"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/middleware"
	"happy-cloud/backend/internal/model"
	"happy-cloud/backend/internal/util"
)

type authReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Email    string `json:"email"`
}

// Register 注册
func Register(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	if len(req.Username) < 2 || len(req.Username) > 32 {
		util.Fail(c, 400, "用户名长度需为 2-32 个字符")
		return
	}
	if len(req.Password) < 6 {
		util.Fail(c, 400, "密码长度至少 6 位")
		return
	}
	var count int64
	db.DB.Model(&model.User{}).Where("username = ?", req.Username).Count(&count)
	if count > 0 {
		util.Fail(c, 409, "用户名已存在")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		util.Fail(c, 500, "服务内部错误")
		return
	}
	user := model.User{
		Username: req.Username,
		Password: string(hash),
		Email:    req.Email,
		QuotaMax: 10 * 1024 * 1024 * 1024, // 默认 10GB
	}
	if req.Username == config.Cfg.AdminUser {
		user.Role = 1 // 指定管理员用户名
	}
	if err := db.DB.Create(&user).Error; err != nil {
		util.Fail(c, 500, "注册失败")
		return
	}
	token, _ := middleware.GenerateToken(user.ID, user.Username, user.Role)
	util.OK(c, gin.H{"token": token, "user": user})
}

// Login 登录，返回 JWT
func Login(c *gin.Context) {
	var req authReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	var user model.User
	if err := db.DB.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.Fail(c, 401, "用户名或密码错误")
		} else {
			util.Fail(c, 500, "服务内部错误")
		}
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)) != nil {
		util.Fail(c, 401, "用户名或密码错误")
		return
	}
	if user.Status != 0 {
		util.Fail(c, 403, "账号已被禁用")
		return
	}
	token, _ := middleware.GenerateToken(user.ID, user.Username, user.Role)
	LogAction(user.ID, user.Username, "login", "登录成功")
	util.OK(c, gin.H{"token": token, "user": user})
}

// InitAdmin 启动时确保管理员账号存在，若 ADMIN_PASSWORD 非空则强制重置密码
func InitAdmin() {
	if config.Cfg.AdminPassword == "" {
		return
	}
	var user model.User
	err := db.DB.Where("username = ?", config.Cfg.AdminUser).First(&user).Error
	hashed, _ := bcrypt.GenerateFromPassword([]byte(config.Cfg.AdminPassword), bcrypt.DefaultCost)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = model.User{
			Username: config.Cfg.AdminUser,
			Password: string(hashed),
			Role:     1,
			Status:   0,
			QuotaMax: 1024 * 1024 * 1024 * 1024,
		}
		if err := db.DB.Create(&user).Error; err != nil {
			log.Printf("[InitAdmin] 创建管理员账号失败: %v", err)
			return
		}
		log.Printf("[InitAdmin] 已创建管理员账号 %s", config.Cfg.AdminUser)
	} else if err == nil {
		db.DB.Model(&user).Update("password", string(hashed))
		log.Printf("[InitAdmin] 已重置管理员 %s 密码", config.Cfg.AdminUser)
	}
}

// ChangePassword 修改当前登录用户密码；管理员可指定 target_user_id 重置他人密码
func ChangePassword(c *gin.Context) {
	userID := uid(c)
	var me model.User
	if err := db.DB.First(&me, userID).Error; err != nil {
		util.Fail(c, 404, "账号不存在")
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password" binding:"required"`
		TargetUser  *uint  `json:"target_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, 400, "参数错误")
		return
	}
	if len(req.NewPassword) < 6 {
		util.Fail(c, 400, "新密码至少 6 位")
		return
	}

	var target model.User
	if req.TargetUser != nil {
		if me.Role != 1 {
			util.Fail(c, 403, "仅管理员可修改其他用户密码")
			return
		}
		if err := db.DB.First(&target, *req.TargetUser).Error; err != nil {
			util.Fail(c, 404, "目标用户不存在")
			return
		}
	} else {
		if err := bcrypt.CompareHashAndPassword([]byte(me.Password), []byte(req.OldPassword)); err != nil {
			util.Fail(c, 401, "原密码错误")
			return
		}
		target = me
	}

	hashed, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err := db.DB.Model(&target).Update("password", string(hashed)).Error; err != nil {
		util.Fail(c, 500, "修改失败")
		return
	}
	if target.ID == me.ID {
		LogAction(userID, username(c), "password", "修改登录密码")
	} else {
		LogAction(userID, username(c), "password", fmt.Sprintf("重置用户 %s 密码", target.Username))
	}
	util.OK(c, gin.H{"ok": true})
}

// Me 当前登录用户信息
func Me(c *gin.Context) {
	uid, _ := c.Get("user_id")
	var user model.User
	if err := db.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	util.OK(c, user)
}

// LogAction 写入操作日志
func LogAction(userID uint, username, action, detail string) {
	if db.DB == nil {
		return
	}
	db.DB.Create(&model.Log{UserID: userID, Username: username, Action: action, Detail: detail})
}
