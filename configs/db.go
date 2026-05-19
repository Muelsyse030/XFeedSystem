package configs

import (
	"XFeedSystem/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) *gorm.DB {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("无法连接到数据库: " + err.Error())
	}

	if err := db.AutoMigrate(&model.User{}); err != nil {
		panic("自动迁移失败: " + err.Error())
	}

	return db
}
