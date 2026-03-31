package model

import "time"

type User struct {
	ID           int64  `gorm:"primaryKey"`
	Username     string `gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	AvatarURL    string `gorm:"size:255"`
	Bio          string `gorm:"size:255"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
type Follow struct {
	UserID    int64     `gorm:"primaryKey;column:user_id"`
	FollowID  int64     `gorm:"primaryKey;column:follow_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}
