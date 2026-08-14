package model

import (
	"time"
)

type NoteStats struct {
	NoteID      int64 `gorm:"primaryKey"`
	Impressions int64 `gorm:"not null;default:0"`
	Reads       int64 `gorm:"not null;default:0"`
	UpdatedAt   time.Time
}

func (NoteStats) TableName() string {
	return "note_stats"
}
