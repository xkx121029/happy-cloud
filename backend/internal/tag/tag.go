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

// AddFile 为文件添加标签（校验标签归属，只能关联当前用户自己的标签）
func AddFile(fileID, userID uint, tagIDs []uint) error {
	return addFile(db.DB, fileID, userID, tagIDs)
}

// addFile 在指定 DB（dbx）上为文件添加标签，逐条校验标签归属
func addFile(dbx *gorm.DB, fileID, userID uint, tagIDs []uint) error {
	for _, tid := range tagIDs {
		// 校验标签归属：按 (id, user_id) 校验，标签必须属于当前用户，防止跨用户标签串扰
		var t model.Tag
		if err := dbx.Where("id = ? AND user_id = ?", tid, userID).First(&t).Error; err != nil {
			return fmt.Errorf("标签不存在或不属于当前用户: %w", err)
		}
		var exist model.FileTag
		if err := dbx.Where("file_id = ? AND tag_id = ?", fileID, tid).First(&exist).Error; err == nil {
			continue
		}
		if err := dbx.Create(&model.FileTag{FileID: fileID, TagID: tid}).Error; err != nil {
			return err
		}
	}
	return nil
}

// SetFileTags 在单个事务内替换文件的标签（先删旧关联，再建新关联），保证原子性
func SetFileTags(fileID, userID uint, tagIDs []uint) error {
	return db.DB.Transaction(func(tx *gorm.DB) error {
		// 删除旧标签关联
		if err := tx.Where("file_id = ?", fileID).Delete(&model.FileTag{}).Error; err != nil {
			return err
		}
		// 新增新标签关联（含归属校验）
		if len(tagIDs) > 0 {
			if err := addFile(tx, fileID, userID, tagIDs); err != nil {
				return err
			}
		}
		return nil
	})
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
