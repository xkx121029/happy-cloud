package model

import "time"

// User 用户表
type User struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Username   string    `gorm:"size:64;uniqueIndex" json:"username"`
	Password   string    `gorm:"size:255" json:"-"` // bcrypt hash
	Email      string    `gorm:"size:128" json:"email"`
	Role       int       `gorm:"default:0" json:"role"` // 0 普通 1 管理员
	QuotaMax   int64     `gorm:"default:10737418240" json:"quota_max"`  // 配额上限字节，默认 10GB
	QuotaUsed  int64     `gorm:"default:0" json:"quota_used"`           // 已用空间
	Status     int       `gorm:"default:0" json:"status"`               // 0 正常 1 禁用
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// File 文件/文件夹表，type: 0 文件夹 1 文件
type File struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"-"`
	ParentID   uint      `gorm:"index;default:0" json:"parent_id"` // 0 表示根
	Name       string    `gorm:"size:255" json:"name"`
	Type       int       `json:"type"` // 0 文件夹 1 文件，由业务代码显式赋值
	Size       int64     `gorm:"default:0" json:"size"`
	Hash       string    `gorm:"size:64;index" json:"hash"`
	StoragePath string   `gorm:"size:512" json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Share 分享表
type Share struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	FileID    uint      `gorm:"index" json:"file_id"`
	UserID    uint      `gorm:"index" json:"-"`
	Token     string    `gorm:"size:64;uniqueIndex" json:"token"`
	Password  string    `gorm:"size:255" json:"-"` // bcrypt hash，可选
	ExpireAt  *time.Time `json:"expire_at"`
	Views     int       `gorm:"default:0" json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

// Log 操作日志表
type Log struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `json:"user_id"`
	Username  string    `gorm:"size:64" json:"username"`
	Action    string    `gorm:"size:64" json:"action"`
	Detail    string    `gorm:"size:512" json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
