package configs

import (
	"time"

	"XFeedSystem/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) *gorm.DB {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("无法连接到数据库: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("获取底层sql.DB失败:" + err.Error())
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err := db.AutoMigrate(
		&model.User{},
		&model.Note{},
		&model.Follow{},
		&model.NoteLike{},
		&model.NoteFavorite{},
		&model.NoteComment{},
		&model.Notification{},
		&model.Block{},
		&model.Topic{},
		&model.NoteTopic{},
	); err != nil {
		panic("自动迁移失败: " + err.Error())
	}

	return db
}
