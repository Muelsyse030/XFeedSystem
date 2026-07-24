package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
)

type AdminRepo interface {
	ListUsers(ctx context.Context, cursor int64, limit int, keyword string) ([]*model.User, error)
	CountUsers(ctx context.Context, keyword string) (int64, error)
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	UpdateUserStatus(ctx context.Context, id int64, status int8) error
	UpdateUserRole(ctx context.Context, id int64, role int8) error
	DeleteUser(ctx context.Context, id int64) error
	DeleteNoteByID(ctx context.Context, id int64) error
	DeleteCommentByID(ctx context.Context, id int64) error
	CountNotes(ctx context.Context) (int64, error)
	CountComments(ctx context.Context) (int64, error)
}

type GormAdminRepo struct {
	db *gorm.DB
}

func NewGormAdminRepo(db *gorm.DB) *GormAdminRepo {
	return &GormAdminRepo{db: db}
}

func (r *GormAdminRepo) ListUsers(ctx context.Context, cursor int64, limit int, keyword string) ([]*model.User, error) {
	if limit > 50 || limit < 0 {
		limit = 20
	}
	q := r.db.WithContext(ctx).Model(&model.User{})
	if keyword != "" {
		q = q.Where("username LIKE ?", "%"+keyword+"%")
	}
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var users []*model.User
	err := q.Order("id DESC").Limit(limit).Find(&users).Error
	return users, err
}

func (r *GormAdminRepo) CountUsers(ctx context.Context, keyword string) (int64, error) {
	q := r.db.WithContext(ctx).Model(&model.User{})
	if keyword != "" {
		q = q.Where("username LIKE ?", "%"+keyword+"%")
	}
	var count int64
	err := q.Count(&count).Error
	return count, err
}

func (r *GormAdminRepo) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).First(&user, id).Error
	return &user, err
}

func (r *GormAdminRepo) UpdateUserStatus(ctx context.Context, id int64, status int8) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("status", status).Error
}

func (r *GormAdminRepo) UpdateUserRole(ctx context.Context, id int64, role int8) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("role", role).Error
}

func (r *GormAdminRepo) DeleteUser(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx.Where("user_id = ?", id).Delete(&model.Follow{})
		tx.Where("follow_id = ?", id).Delete(&model.Follow{})
		tx.Where("user_id = ? OR blocked_id = ?", id, id).Delete(&model.Block{})
		tx.Where("user_id = ?", id).Delete(&model.NoteLike{})
		tx.Where("user_id = ?", id).Delete(&model.NoteFavorite{})
		tx.Where("user_id = ?", id).Delete(&model.NoteComment{})
		tx.Where("author_id = ?", id).Delete(&model.Note{})
		return tx.Delete(&model.User{}, id).Error
	})
}

func (r *GormAdminRepo) DeleteNoteByID(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Note{}).
		Where("id = ?", id).
		Update("status", model.NoteStatusDeleted).Error
}

func (r *GormAdminRepo) DeleteCommentByID(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cm model.NoteComment
		if err := tx.Where("id = ? AND status = ?", id, model.NoteStatusPublished).First(&cm).Error; err != nil {
			return err
		}
		if err := tx.Model(&cm).Update("status", model.NoteStatusDeleted).Error; err != nil {
			return err
		}
		if cm.ParentID == 0 {
			return tx.Model(&model.Note{}).Where("id = ?", cm.NoteID).
				Update("comment_count", gorm.Expr("GREATEST(comment_count - 1, 0)")).Error
		}
		return nil
	})
}

func (r *GormAdminRepo) CountNotes(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Note{}).Where("status = ?", model.NoteStatusPublished).Count(&count).Error
	return count, err
}

func (r *GormAdminRepo) CountComments(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.NoteComment{}).Where("status = ?", model.NoteStatusPublished).Count(&count).Error
	return count, err
}
