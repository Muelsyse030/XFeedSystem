package repo

import (
	"XFeedSystem/internal/model"
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BlockRepo interface {
	Block(ctx context.Context, userID, blockedID int64) error
	Unblock(ctx context.Context, userID, blockedID int64) error
	IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error)
	GetBlockedIDs(ctx context.Context, userID int64) ([]int64, error)
	// IsBlockedEitherWay 检查任意方向是否拉黑（双向不可见）
	IsBlockedEitherWay(ctx context.Context, a, b int64) (bool, error)
	FilterBlockedAuthors(ctx context.Context, userID int64, authorIDs []int64) ([]int64, error)
}

type GormBlockRepo struct {
	db *gorm.DB
}

func NewGormBlockRepo(db *gorm.DB) *GormBlockRepo {
	return &GormBlockRepo{db: db}
}

func (r *GormBlockRepo) Block(ctx context.Context, userID int64, blockedID int64) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.Block{UserID: userID, BlockedID: blockedID}).Error
}

func (r *GormBlockRepo) Unblock(ctx context.Context, userID, blockedID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND blocked_id = ?", userID, blockedID).
		Delete(&model.Block{}).Error
}

func (r *GormBlockRepo) IsBlocked(ctx context.Context, userID, blockedID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Block{}).
		Where("user_id = ? AND blocked_id = ?", userID, blockedID).
		Count(&count).Error
	return count > 0, err
}

func (r *GormBlockRepo) GetBlockedIDs(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&model.Block{}).
		Pluck("blocked_id", &ids).Error
	return ids, err
}

func (r *GormBlockRepo) IsBlockedEitherWay(ctx context.Context, a, b int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Block{}).
		Where("(user_id = ? AND blocked_id = ?) OR (user_id = ? AND blocked_id = ?)", a, b, b, a).
		Count(&count).Error
	return count > 0, err
}

func (r *GormBlockRepo) FilterBlockedAuthors(ctx context.Context, userID int64, authorIDs []int64) ([]int64, error) {
	if len(authorIDs) == 0 {
		return nil, nil
	}
	type blockRow struct {
		UserID    int64
		BlockedID int64
	}
	var rows []blockRow
	err := r.db.WithContext(ctx).
		Model(&model.Block{}).
		Where("(user_id = ? AND blocked_id IN ?) OR (blocked_id = ? AND user_id IN ?)",
			userID, authorIDs, userID, authorIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	set := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if row.UserID == userID {
			set[row.BlockedID] = struct{}{}
		}
		if row.BlockedID == userID {
			set[row.UserID] = struct{}{}
		}
	}
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out, nil
}
