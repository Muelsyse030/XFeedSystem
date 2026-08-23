package model

import "time"

const (
	NoteStatusPublished = 1
	NoteStatusDeleted   = 2
)

type Note struct {
	ID            int64  `gorm:"primaryKey"`
	AuthorID      int64  `gorm:"not null;index"`
	Title         string `gorm:"size:255;not null;default:''"`
	Content       string `gorm:"type:text;not null"`
	Images        string `gorm:"type:json"`
	Status        int8   `gorm:"not null;default:1"`
	Type          int8   `gorm:"not null;default:1"`
	LikeCount     int64  `gorm:"not null;default:0"`
	FavoriteCount int64  `gorm:"not null;default:0"`
	CommentCount  int64  `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PublishedAt   time.Time `gorm:"not null;index"`
	VideoURL string `gorm:"type:text" json:"video_url"`
}

type NoteLike struct {
	ID        int64 `gorm:"primaryKey"`
	NoteID    int64 `gorm:"not null;index:idx_note_likes_note_id;uniqueIndex:uk_note_user_like,priority:1"`
	UserID    int64 `gorm:"not null;index:idx_note_likes_user_id;uniqueIndex:uk_note_user_like,priority:2"`
	CreatedAt time.Time
}

type NoteFavorite struct {
	ID        int64 `gorm:"primaryKey"`
	NoteID    int64 `gorm:"not null;index:idx_note_favorites_note_id;uniqueIndex:uk_note_user_favorite,priority:1"`
	UserID    int64 `gorm:"not null;index:idx_note_favorites_user_id;uniqueIndex:uk_note_user_favorite,priority:2"`
	CreatedAt time.Time
}
type NoteComment struct {
	ID            int64  `gorm:"primaryKey"`
	NoteID        int64  `gorm:"not null;index:idx_note_comments_note_id_id,priority:1"`
	UserID        int64  `gorm:"not null;index:idx_note_comments_user_id"`
	ParentID      int64  `gorm:"not null;default:0;index:idx_note_comments_parent_id"`
	ReplyToUserID int64  `gorm:"not null;default:0;index:idx_note_comments_reply_to_user_id"`
	Content       string `gorm:"type:text;not null"`
	Status        int16  `gorm:"not null;default:1"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (NoteLike) TableName() string {
	return "note_likes"
}

func (NoteFavorite) TableName() string {
	return "note_favorites"
}

func (Note) TableName() string {
	return "notes"
}
func (NoteComment) TableName() string {
	return "note_comments"
}
