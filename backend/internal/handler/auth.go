package handler

import (
	"errors"

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
