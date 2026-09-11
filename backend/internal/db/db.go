package db

import (
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/model"
)

var DB *gorm.DB

// Init 初始化 MySQL 连接并自动建表
func Init() error {
	dsn := config.Cfg.DBDSN
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}
	return DB.AutoMigrate(&model.User{}, &model.File{}, &model.Favorite{}, &model.Share{}, &model.Log{}, &model.Transfer{}, &model.Notification{}, &model.UserSettings{})
}
