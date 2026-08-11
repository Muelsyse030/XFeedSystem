package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"time"

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
	ListFollowing(ctx context.Context, followIDs []int64, cursor *model.FeedCursor, limit int) ([]*model.Note, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*model.Note, error)
	ListRecent(ctx context.Context, limit int) ([]*model.Note, error)
	GetUserTypePreference(ctx context.Context, userID int64) (map[int8]float64, error)
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
func (r *GormFeedRepo) ListFollowing(ctx context.Context, followIDs []int64, cursor *model.FeedCursor, limit int) ([]*model.Note, error) {
	if limit < 0 || limit > 50 {
		limit = 10
	}
	if len(followIDs) == 0 {
		return []*model.Note{}, nil
	}

	db := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Where("status = ?", model.NoteStatusPublished).
		Where("author_id IN ?", followIDs)
	if cursor != nil {
		db = db.Where(
			"(published_at < ?) OR (published_at = ? AND id < ?)",
			cursor.PublishedAt, cursor.PublishedAt, cursor.ID,
		)
	}
	var notes []*model.Note
	err := db.Order("published_at DESC").
		Order("id DESC").
		Limit(limit).
		Find(&notes).Error
	if err != nil {
		return nil, err
	}

	return notes, nil
}

func (r *GormFeedRepo) GetByIDs(ctx context.Context, ids []int64) ([]*model.Note, error) {
	var notes []*model.Note
	err := r.db.WithContext(ctx).
		Select("id", "author_id", "title", "content", "images", "type", "published_at").
		Where("id IN ? AND status = ?", ids, model.NoteStatusPublished).
		Order("published_at DESC").
		Find(&notes).Error
	return notes, err
}

func (r *GormFeedRepo) ListRecent(ctx context.Context, limit int) ([]*model.Note, error) {
	var notes []*model.Note
	err := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Select("id", "author_id", "title", "type", "like_count", "favorite_count", "comment_count", "published_at").
		Where("status = ?", model.NoteStatusPublished).
		Order("published_at DESC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}

// ListSince 返回最近 since 时间点之后发布的笔记（时间窗口池，替代固定 200 条硬上限）
func (r *GormFeedRepo) ListSince(ctx context.Context, since time.Time, limit int) ([]*model.Note, error) {
	var notes []*model.Note
	err := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Select("id", "author_id", "title", "type", "like_count", "favorite_count", "comment_count", "published_at").
		Where("status = ? AND published_at >= ?", model.NoteStatusPublished, since).
		Order("published_at DESC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}

// ListAllPublished 返回全部已发布笔记（feed 引擎重建打分 ZSET 用，只查打分所需列）
func (r *GormFeedRepo) ListAllPublished(ctx context.Context) ([]*model.Note, error) {
	var notes []*model.Note
	err := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Select("id", "author_id", "title", "type", "like_count", "favorite_count", "comment_count", "published_at").
		Where("status = ?", model.NoteStatusPublished).
		Find(&notes).Error
	return notes, err
}

func (r *GormFeedRepo) GetUserTypePreference(ctx context.Context, userID int64) (map[int8]float64, error) {
	type countResult struct {
		Type  int8
		Count int64
	}
	var results []countResult

	err := r.db.WithContext(ctx).Raw(`
        SELECT n.type, COUNT(*) as count FROM (
            (SELECT note_id FROM note_likes WHERE user_id = ? LIMIT 100)
            UNION ALL
            (SELECT note_id FROM note_favorites WHERE user_id = ? LIMIT 100)
            UNION ALL
            (SELECT note_id FROM note_comments WHERE user_id = ? LIMIT 100)
        ) AS interactions
        JOIN notes n ON n.id = interactions.note_id
        GROUP BY n.type
    `, userID, userID, userID).Scan(&results).Error

	if err != nil || len(results) == 0 {
		return nil, err
	}

	var total float64
	for _, r := range results {
		total += float64(r.Count)
	}
	pref := make(map[int8]float64, len(results))
	for _, r := range results {
		pref[r.Type] = float64(r.Count) / total
	}
	return pref, nil
}
