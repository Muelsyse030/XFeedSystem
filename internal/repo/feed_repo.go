package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
)

type GormFeedRepo struct {
	db *gorm.DB
}

func NewGormFeedRepo(db *gorm.DB) *GormFeedRepo {
	return &GormFeedRepo{
		db: db,
	}
}

type FeedRepo interface {
	ListForYou(ctx context.Context, cursor *model.FeedCursor, limit int) ([]*model.Note, error)
}

func (r *GormFeedRepo) ListForYou(ctx context.Context, cursor *model.FeedCursor, limit int) ([]*model.Note, error) {
	var notes []*model.Note
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	query := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Where("status = ?", model.NoteStatusPublished)

	if cursor != nil && !cursor.PublishedAt.IsZero() {
		query = query.Where(
			"(published_at < ?) OR (published_at = ? AND id < ?)",
			cursor.PublishedAt,
			cursor.PublishedAt,
			cursor.ID,
		)
	}

	query = query.Order("published_at DESC").Order("id DESC").Limit(limit)

	if err := query.Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}
