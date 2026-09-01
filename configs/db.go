package configs

import (
	"XFeedSystem/internal/outbox"
	"time"

	"XFeedSystem/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) *gorm.DB {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		PrepareStmt: true, // 消融实验：预编译语句，减少 GORM 反射与 MySQL SQL 解析
	})
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
		&model.NoteStats{},
		&outbox.Event{},
	); err != nil {
		panic("自动迁移失败: " + err.Error())
	}

	return db
}
