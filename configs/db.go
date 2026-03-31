package configs

import (
	"XFeedSystem/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := "root:123456@tcp(127.0.0.1:3308)/feed_system?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("无法连接到数据库: " + err.Error())
	}

	if err := db.AutoMigrate(&model.User{}); err != nil {
		panic("自动迁移失败: " + err.Error())
	}

	return db
}
