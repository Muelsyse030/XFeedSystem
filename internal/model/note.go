package model

import "time"

const (
	NoteStatusPublished = 1
	NoteStatusDeleted   = 2
)

type Note struct {
	ID          int64  `gorm:"primaryKey"`
	AuthorID    int64  `gorm:"not null;index"`
	Title       string `gorm:"size:255;not null;default:''"`
	Content     string `gorm:"type:text;not null"`
	Status      int8   `gorm:"not null;default:1"`
	Type        int8   `gorm:"not null;default:1"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt time.Time `gorm:"not null;index"`
}

func (Note) TableName() string {
	return "notes"
}
