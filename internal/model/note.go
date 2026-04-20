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
type NoteLike struct{
	ID        int64 `gorm:"primaryKey"`
	NoteID    int64 `gorm:"not null;index:idx_note_likes_note_id;uniqueIndex:uk_note_user_like,priority:1"`
	UserID    int64 `gorm:"not null;index:idx_note_likes_user_id;uniqueIndex:uk_note_user_like,priority:2"`
	CreatedAt time.Time 
}
func (NoteLike) TableName() string {
	return "note_likes"
}

func (Note) TableName() string {
	return "notes"
}
