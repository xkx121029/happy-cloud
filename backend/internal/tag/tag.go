package tag

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/model"
)

// EnsureExists 保证 tag 在用户命名空间内存在，重复返回已有 ID
func EnsureExists(userID uint, name string, color string) (uint, error) {
	var t model.Tag
	err := db.DB.Where("user_id = ? AND name = ?", userID, name).First(&t).Error
	if err == nil {
		return t.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	t = model.Tag{UserID: userID, Name: name, Color: color}
	if err := db.DB.Create(&t).Error; err != nil {
		return 0, fmt.Errorf("创建标签失败: %w", err)
	}
	return t.ID, nil
}

// List 列出用户的所有标签
func List(userID uint) ([]model.Tag, error) {
	var tags []model.Tag
	if err := db.DB.Where("user_id = ?", userID).Order("name ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// AddFile 为文件添加标签
func AddFile(fileID uint, tagIDs []uint) error {
	for _, tid := range tagIDs {
		var exist model.FileTag
		if err := db.DB.Where("file_id = ? AND tag_id = ?", fileID, tid).First(&exist).Error; err == nil {
			continue
		}
		if err := db.DB.Create(&model.FileTag{FileID: fileID, TagID: tid}).Error; err != nil {
			return err
		}
	}
	return nil
}

// RemoveFile 移除文件的所有标签关联
func RemoveFile(fileID uint) error {
	return db.DB.Where("file_id = ?", fileID).Delete(&model.FileTag{}).Error
}

// GetTags 获取文件关联的标签
func GetTags(fileID uint) ([]model.Tag, error) {
	var tags []model.Tag
	err := db.DB.Table("file_tags").
		Joins("JOIN tags ON tags.id = file_tags.tag_id").
		Where("file_tags.file_id = ?", fileID).
		Find(&tags).Error
	return tags, err
}

// DeleteTag 删除标签及其所有关联（不删文件）
func DeleteTag(userID, tagID uint) error {
	if err := db.DB.Where("id = ? AND user_id = ?", tagID, userID).Delete(&model.Tag{}).Error; err != nil {
		return err
	}
	return db.DB.Where("tag_id = ?", tagID).Delete(&model.FileTag{}).Error
}

// SearchByName 按名称搜索用户的标签
func SearchByName(userID uint, keyword string) ([]model.Tag, error) {
	var tags []model.Tag
	if err := db.DB.Where("user_id = ? AND name LIKE ?", userID, "%"+keyword+"%").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}
