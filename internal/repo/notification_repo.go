package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
)

type NotificationRepo interface {
	Create(ctx context.Context, notif *model.Notification) error
	ListByUser(ctx context.Context, userID int64, cursor int64, limit int) ([]*model.Notification, error)
	MarkRead(ctx context.Context, id int64, userID int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	CountUnread(ctx context.Context, userID int64) (int64, error)
}

type GormNotificationRepo struct {
	db *gorm.DB
}

func NewGormNotificationRepo(db *gorm.DB) *GormNotificationRepo {
	return &GormNotificationRepo{db: db}
}

func (r *GormNotificationRepo) Create(ctx context.Context, notif *model.Notification) error {
	return r.db.WithContext(ctx).Create(notif).Error
}

func (r *GormNotificationRepo) ListByUser(ctx context.Context, userID int64, cursor int64, limit int) ([]*model.Notification, error) {
	if limit < 0 || limit > 50 {
		limit = 10
	}
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var list []*model.Notification
	err := q.Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (r *GormNotificationRepo) MarkRead(ctx context.Context, id int64, userID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Notification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Update("is_read", true).Error
}

func (r *GormNotificationRepo) MarkAllRead(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Update("is_read", true).Error
}

func (r *GormNotificationRepo) CountUnread(ctx context.Context, userID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count).Error
	return count, err
}
