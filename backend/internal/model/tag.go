package model

import "time"

// Tag 用户标签，同一用户下名称唯一
type Tag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;uniqueIndex:idx_user_name" json:"-"`
	Name      string    `gorm:"size:64;uniqueIndex:idx_user_name" json:"name"`
	Color     string    `gorm:"size:32;default:'#6366f1'" json:"color"` // CSS 颜色
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FileTag 文件-标签关联（多对多）
type FileTag struct {
	FileID uint `gorm:"primaryKey;index" json:"file_id"`
	TagID  uint `gorm:"primaryKey;index" json:"tag_id"`
}
