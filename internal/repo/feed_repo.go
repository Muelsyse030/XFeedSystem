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
	ListByTopic(ctx context.Context, topicID int64, cursor *model.FeedCursor, limit int) ([]*model.Note, error)
	GetUserTypePreference(ctx context.Context, userID int64) (map[int8]float64, error)
	GetNoteAuthorIDs(ctx context.Context, noteIDs []int64) (map[int64]int64, error)
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

// ListByTopic 话题页 feed（时间倒序键集分页）
func (r *GormFeedRepo) ListByTopic(ctx context.Context, topicID int64, cursor *model.FeedCursor, limit int) ([]*model.Note, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := r.db.WithContext(ctx).
		Model(&model.Note{}).
		Joins("JOIN note_topics nt ON nt.note_id = notes.id AND nt.topic_id = ?", topicID).
		Where("notes.status = ?", model.NoteStatusPublished)
	if cursor != nil && !cursor.PublishedAt.IsZero() {
		q = q.Where("(notes.published_at < ?) OR (notes.published_at = ? AND notes.id < ?)",
			cursor.PublishedAt, cursor.PublishedAt, cursor.ID)
	}
	var notes []*model.Note
	if err := q.Order("notes.published_at DESC").Order("notes.id DESC").Limit(limit).Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

// ListTopicsByNoteIDs 批量返回笔记的话题名（feed 卡片展示用）
func (r *GormFeedRepo) ListTopicsByNoteIDs(ctx context.Context, noteIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string)
	if len(noteIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		NoteID int64
		Name   string
	}
	if err := r.db.WithContext(ctx).
		Table("note_topics nt").
		Joins("JOIN topics t ON t.id = nt.topic_id").
		Where("nt.note_id IN ?", noteIDs).
		Order("nt.topic_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.NoteID] = append(out[row.NoteID], row.Name)
	}
	return out, nil
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

func (r *GormFeedRepo) GetNoteAuthorIDs(ctx context.Context, noteIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64)
	if len(noteIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		ID       int64
		AuthorID int64
	}
	if err := r.db.WithContext(ctx).Model(&model.Note{}).
		Select("id", "author_id").Where("id IN ?", noteIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.AuthorID
	}
	return out, nil
}