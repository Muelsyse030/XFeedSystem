package model

import (
	"time"
)

const (
	NotifTypeLike     int8 = 1
	NotifTypeComment  int8 = 2
	NotifTypeReply    int8 = 3
	NotifTypeFollow   int8 = 4
	NotifTypeFavorite int8 = 5
)

type Notification struct {
	ID           int64     `gorm:"primaryKey"`
	UserID       int64     `gorm:"not null;index:idx_notif_user_read,priority:1"`
	ActorID      int64     `gorm:"not null;index"`
	Type         int8      `gorm:"not null;index"`
	TargetID     int64     `gorm:"not null;index"`
	TargetNoteID int64     `gorm:"not null;default:0"`
	Message      string    `gorm:"size:255;not null"`
	IsRead       bool      `gorm:"not null;default:false;index:idx_notif_user_read,priority:2"`
	CreatedAt    time.Time `gorm:"not null;index"`
}

func (Notification) TableName() string {
	return "notifications"
}
