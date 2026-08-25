package model

import "time"

type FeedHide struct {
	ID        int64 `gorm:"primaryKey"`
	UserID    int64 `gorm:"not null;index"`
	NoteID    int64 `gorm:"not null"`
	Type      int8  `gorm:"not null"`
	CreatedAt time.Time
}

func (FeedHide) TableName() string { return "feed_hides" }
