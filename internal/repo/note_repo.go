package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"gorm.io/gorm/clause"
	"gorm.io/gorm"
)

type NoteRepo interface {
	Create(note *model.Note) (*model.Note, error)
	GetByID(ctx context.Context, id int64) (*model.Note, error)
	DeleteByID(ctx context.Context, id int64, authorID int64) error
	ListByAuthorID(ctx context.Context, authorID int64, cursor int64, limit int) ([]*model.Note, error)

	Like(ctx context.Context, noteID int64, userID int64) (bool, error)
	Unlike(ctx context.Context, noteID int64, userID int64) (bool, error)
	IsLiked(ctx context.Context, noteID int64, userID int64) (bool, error)
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
func (r *GormNoteRepo) Like(ctx context.Context,noteID,userID int64) (created bool ,err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		like := &model.NoteLike{
			NoteID : noteID,
			UserID : userID,
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(like)
		if res.Error != nil{
			return res.Error
		}
		if res.RowsAffected == 0{
			created = false
			return nil
		}
		created = true
		return tx.Model(&model.Note{}).Where("id = ?",noteID).Update("like_count",gorm.Expr("like_count + 1")).Error
	})
	return created , err
}

func (r *GormNoteRepo) Unlike(ctx context.Context,noteID,userID int64) (deleted bool ,err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("note_id = ? AND user_id = ?",noteID,userID).Delete(&model.NoteLike{})
		if res.Error != nil{
			return res.Error
		}
		if res.RowsAffected == 0{
			deleted = false
			return nil
		}
		deleted = true
		return tx.Model(&model.Note{}).Where("id = ?",noteID).Update("like_count",gorm.Expr("like_count - 1")).Error
	})
	return deleted , err
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