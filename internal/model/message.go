package model

import "time"

type Message struct {
	ID              int64  `gorm:"primaryKey"`
	SenderID        int64  `gorm:"not null;index"`
	ReceiverID      int64  `gorm:"not null;index"`
	Content         string `gorm:"type:text;not null"`
	ClientMessageID string `gorm:"size:64;not null"`
	IsRead          int8   `gorm:"not null;default:0"`
	ReadAt          *time.Time
	SenderDeleted   int8      `gorm:"not null;default:0"`
	ReceiverDeleted int8      `gorm:"not null;default:0"`
	CreatedAt       time.Time `gorm:"not null"`
}

func (Message) TableName() string { return "messages" }

// ConversationRow 会话聚合查询的中间结果
type ConversationRow struct {
	PeerID    int64
	LastMsgID int64
	Unread    int64
}
