package model

import "time"

type Block struct {
	UserID    int64     `gorm:"primaryKey;column:user_id"`
	BlockedID int64     `gorm:"primaryKey;column:blocked_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (Block) TableName() string {
	return "blocks"
}
