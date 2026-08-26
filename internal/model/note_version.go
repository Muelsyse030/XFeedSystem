package model

import "time"

type NoteVersion struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	NoteID        int64     `gorm:"not null;index:idx_note_version_note,priority:1" json:"note_id"`
	AuthorID      int64     `gorm:"not null" json:"author_id"`
	Title         string    `gorm:"size:255;not null" json:"title"`
	Content       string    `gorm:"type:text;not null" json:"content"`
	Images        string    `gorm:"type:json" json:"images"`
	VideoURL      string    `gorm:"type:text" json:"video_url"`
	Type          int8      `gorm:"not null;default:1" json:"type"`
	ContentFormat int8      `gorm:"not null;default:1" json:"content_format"`
	CreatedAt     time.Time `json:"created_at"`
}

func (NoteVersion) TableName() string { return "note_versions" }
