package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationRepo interface {
	Create(ctx context.Context, notif *model.Notification) (bool, error)
	ListByUser(ctx context.Context, userID int64, cursor int64, limit int) ([]*model.Notification, error)
	MarkRead(ctx context.Context, id int64, userID int64) error
	MarkAllRead(ctx context.Context, userID int64) error
	CountUnread(ctx context.Context, userID int64) (int64, error)
	ExistsPairs(ctx context.Context, notifs []*model.Notification) (map[string]bool, error)
	BulkCreate(ctx context.Context, notifs []*model.Notification) (int64, error)
}

type GormNotificationRepo struct {
	db *gorm.DB
}

func NewGormNotificationRepo(db *gorm.DB) *GormNotificationRepo {
	return &GormNotificationRepo{db: db}
}

func (r *GormNotificationRepo) Create(ctx context.Context, notif *model.Notification) (bool, error) {
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}, {Name: "user_id"}}, DoNothing: true,
	}).Create(notif)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
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

func (r *GormNotificationRepo) ExistsPairs(ctx context.Context, notifs []*model.Notification) (map[string]bool, error) {
	out := map[string]bool{}
	if len(notifs) == 0 {
		return out, nil
	}
	eventIDs := make([]int64, 0, len(notifs))
	userIDs := make([]int64, 0, len(notifs))
	for _, n := range notifs {
		eventIDs = append(eventIDs, n.EventID)
		userIDs = append(userIDs, n.UserID)
	}
	var rows []struct {
		EventID int64
		UserID  int64
	}
	if err := r.db.WithContext(ctx).Model(&model.Notification{}).
		Where("event_id IN ? AND user_id IN ?", eventIDs, userIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[fmt.Sprintf("%d:%d", row.EventID, row.UserID)] = true
	}
	return out, nil
}

func (r *GormNotificationRepo) BulkCreate(ctx context.Context, notifs []*model.Notification) (int64, error) {
	if len(notifs) == 0 {
		return 0, nil
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "event_id"}, {Name: "user_id"}}, DoNothing: true,
	}).Create(&notifs)
	return res.RowsAffected, res.Error
}
