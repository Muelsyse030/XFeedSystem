package model

import "time"

// Topic 话题
type Topic struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:64;uniqueIndex;not null"`
	NoteCount int64     `json:"note_count" gorm:"not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
}

// NoteTopic 笔记-话题关联（一篇笔记最多 5 个话题，由业务层保证）
type NoteTopic struct {
	NoteID    int64 `gorm:"primaryKey;index:idx_nt_topic"`
	TopicID   int64 `gorm:"primaryKey"`
	CreatedAt time.Time
}

type TopicFollow struct {
	ID        int64 `gorm:"primaryKey"`
	UserID    int64 `gorm:"not null"`
	TopicID   int64 `gorm:"not null"`
	CreatedAt time.Time
}

func (TopicFollow) TableName() string { return "topic_follows" }
