package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NoteRepo interface {
	Create(note *model.Note) (*model.Note, error)
	GetByID(ctx context.Context, id int64) (*model.Note, error)
	DeleteByID(ctx context.Context, id int64, authorID int64) error
	ListByAuthorID(ctx context.Context, authorID int64, cursor int64, limit int) ([]*model.Note, error)

	Like(ctx context.Context, noteID int64, userID int64) (bool, error)
	Unlike(ctx context.Context, noteID int64, userID int64) (bool, error)
	IsLiked(ctx context.Context, noteID int64, userID int64) (bool, error)

	Favorite(ctx context.Context, noteID, userID int64) (bool, error)
	Unfavorite(ctx context.Context, noteID, userID int64) (bool, error)
	IsFavorite(ctx context.Context, noteID, userID int64) (bool, error)
	FavoriteList(ctx context.Context, userID int64, cursor int64, limit int) ([]*model.Note, int64, error)

	CreateComment(ctx context.Context, userID, noteID int64, content string) (*model.NoteComment, error)
	GetCommentByID(ctx context.Context, commentID int64) (*model.NoteComment, error)
	ListCommentsByNoteID(ctx context.Context, noteID, cursor int64, limit int) ([]*model.NoteComment, error)
	DeleteComment(ctx context.Context, commentID int64, userID int64) error
}
type GormNoteRepo struct {
	db *gorm.DB
}

func NewGormNoteRepo(db *gorm.DB) *GormNoteRepo {
	return &GormNoteRepo{
		db: db,
	}
}
func (r *GormNoteRepo) Create(note *model.Note) (*model.Note, error) {
	return note, r.db.Create(note).Error
}

func (r *GormNoteRepo) ListByAuthorID(ctx context.Context, authorID int64, cursor int64, limit int) ([]*model.Note, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	db := r.db.WithContext(ctx).
		Where("author_id = ? AND status = ?", authorID, model.NoteStatusPublished)

	if cursor > 0 {
		db = db.Where("id < ?", cursor)
	}

	var notes []*model.Note
	err := db.Order("id DESC").
		Limit(limit).
		Find(&notes).Error
	if err != nil {
		return nil, err
	}

	return notes, nil
}

func (r *GormNoteRepo) GetByID(ctx context.Context, id int64) (*model.Note, error) {
	var note model.Note
	err := r.db.WithContext(ctx).First(&note, id).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}
func (r *GormNoteRepo) DeleteByID(ctx context.Context, id int64, authorID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Note{}).
		Where("id = ? AND author_id = ? AND status = ?", id, authorID, model.NoteStatusPublished).
		Updates(map[string]interface{}{
			"status": model.NoteStatusDeleted,
		}).Error
}
func (r *GormNoteRepo) Like(ctx context.Context, noteID, userID int64) (created bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		like := &model.NoteLike{
			NoteID: noteID,
			UserID: userID,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(like)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			created = false
			return nil
		}
		created = true
		return tx.Model(&model.Note{}).Where("id = ?", noteID).Update("like_count", gorm.Expr("like_count + 1")).Error
	})
	return created, err
}

func (r *GormNoteRepo) Unlike(ctx context.Context, noteID, userID int64) (deleted bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("note_id = ? AND user_id = ?", noteID, userID).Delete(&model.NoteLike{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			deleted = false
			return nil
		}
		deleted = true
		return tx.Model(&model.Note{}).Where("id = ?", noteID).Update("like_count", gorm.Expr("like_count - 1")).Error
	})
	return deleted, err
}

func (r *GormNoteRepo) IsLiked(ctx context.Context, noteID int64, userID int64) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Model(&model.NoteLike{}).
		Where("note_id = ? AND user_id = ?", noteID, userID).
		Count(&cnt).Error
	if err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (r *GormNoteRepo) Favorite(ctx context.Context, noteID, userID int64) (created bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		favorite := &model.NoteFavorite{
			NoteID: noteID,
			UserID: userID,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(favorite)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			created = false
			return nil
		}
		created = true
		return tx.Model(&model.Note{}).Where("id = ?", noteID).Update("favorite_count", gorm.Expr("favorite_count + 1")).Error
	})
	return created, err
}
func (r *GormNoteRepo) Unfavorite(ctx context.Context, noteID, userID int64) (deleted bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("note_id = ? AND user_id = ?", noteID, userID).Delete(&model.NoteFavorite{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			deleted = false
			return nil
		}
		deleted = true
		return tx.Model(&model.Note{}).Where("id = ?", noteID).Update("favorite_count", gorm.Expr("favorite_count - 1")).Error
	})
	return deleted, err
}
func (r *GormNoteRepo) IsFavorite(ctx context.Context, noteID, userID int64) (bool, error) {
	var cnt int64
	err := r.db.WithContext(ctx).
		Model(&model.NoteFavorite{}).
		Where("note_id = ? AND user_id = ?", noteID, userID).
		Count(&cnt).Error
	return cnt > 0, err
}

func (r *GormNoteRepo) FavoriteList(ctx context.Context, userID int64, cursor int64, limit int) ([]*model.Note, int64, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var favs []model.NoteFavorite
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	if err := q.Order("id DESC").Limit(limit).Find(&favs).Error; err != nil {
		return nil, 0, err
	}
	if len(favs) == 0 {
		return []*model.Note{}, 0, nil
	}
	nextCursor := favs[len(favs)-1].ID

	ids := make([]int64, len(favs))
	for i, f := range favs {
		ids[i] = f.NoteID
	}
	var notes []*model.Note
	if err := r.db.WithContext(ctx).
		Where("id IN ? AND status = ?", ids, model.NoteStatusPublished).
		Find(&notes).Error; err != nil {
		return nil, 0, err
	}
	byID := make(map[int64]*model.Note, len(notes))
	for _, n := range notes {
		byID[n.ID] = n
	}
	out := make([]*model.Note, 0, len(favs))
	for _, f := range favs {
		if n, ok := byID[f.NoteID]; ok {
			out = append(out, n)
		}
	}
	return out, nextCursor, nil
}
func (r *GormNoteRepo) CreateComment(ctx context.Context, userID, noteID int64, content string) (*model.NoteComment, error) {
	comment := &model.NoteComment{
		NoteID:  noteID,
		UserID:  userID,
		Content: content,
	}
	if err := r.db.WithContext(ctx).Create(comment).Error; err != nil {
		return nil, err
	}
	return comment, nil
}
func (r *GormNoteRepo) GetCommentByID(ctx context.Context, commentID int64) (*model.NoteComment, error) {
	var comment model.NoteComment
	if err := r.db.WithContext(ctx).
		Model(&model.NoteComment{}).
		First(&comment, commentID).Error; err != nil {
		return nil, err
	}
	return &comment, nil
}
func (r *GormNoteRepo) ListCommentsByNoteID(ctx context.Context, noteID, cursor int64, limit int) ([]*model.NoteComment, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := r.db.WithContext(ctx).Where("note_id = ? AND status = ?", noteID, model.NoteStatusPublished)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var comments []*model.NoteComment
	if err := q.Order("id DESC").Limit(limit).Find(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}
func (r *GormNoteRepo) DeleteComment(ctx context.Context, commentID int64, userID int64) error {
	res := r.db.WithContext(ctx).
		Model(&model.NoteComment{}).
		Where("id = ? AND user_id = ? AND status = ?", commentID, userID, model.NoteStatusPublished).
		Updates(map[string]interface{}{
			"status": model.NoteStatusDeleted,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
