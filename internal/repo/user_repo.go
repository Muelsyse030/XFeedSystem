package repo

import (
	"XFeedSystem/internal/model"
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRepo interface {
	FindByUsername(username string) (*model.User, error)
	CreateUser(user *model.User) error
	CompareHashAndPassword(hash string, password string) error
	GetProfile(uid int64) (*model.User, error)
	GetByIDs(ids []int64) ([]*model.User, error)
	Followbyid(ctx context.Context, user_id int64, follow_id int64) error
	Delete(ctx context.Context, userID, followID int64) error
	Exists(ctx context.Context, userID, followID int64) (bool, error)
	GetFollowingIDs(ctx context.Context, userID int64) ([]int64, error)
	Updata(ctx context.Context, userID int64, avatarURL string, bio string) error
	ListFollowing(ctx context.Context, userID int64, cursor time.Time, limit int) ([]*model.Follow, error)
	ListFollowers(ctx context.Context, userId int64, cursor time.Time, limit int) ([]*model.Follow, error)
	SearchByUsername(ctx context.Context, keyword string, limit int) ([]*model.User, error)
}
type GormUserRepo struct {
	db *gorm.DB
}

func NewGormUserRepo(db *gorm.DB) *GormUserRepo {
	return &GormUserRepo{
		db: db,
	}
}

// SearchByUsername 按用户名前缀搜索正常状态用户（发起站内信用）
func (r *GormUserRepo) SearchByUsername(ctx context.Context, keyword string, limit int) ([]*model.User, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	var users []*model.User
	err := r.db.WithContext(ctx).
		Where("username LIKE ? AND status = 1", keyword+"%").
		Order("id ASC").
		Limit(limit).
		Find(&users).Error
	return users, err
}

func (r *GormUserRepo) FindByUsername(username string) (*model.User, error) {
	var user model.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (r *GormUserRepo) CreateUser(user *model.User) error {
	return r.db.Create(user).Error
}

func (r *GormUserRepo) CompareHashAndPassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func (r *GormUserRepo) GetProfile(uid int64) (*model.User, error) {
	var user model.User
	err := r.db.First(&user, uid).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}
func (r *GormUserRepo) GetByIDs(ids []int64) ([]*model.User, error) {
	var users []*model.User
	err := r.db.Where("id IN ?", ids).Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}
func (r *GormUserRepo) Followbyid(ctx context.Context, user_id int64, follow_id int64) error {
	follow := &model.Follow{
		UserID:   user_id,
		FollowID: follow_id,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(follow).Error
}
func (r *GormUserRepo) Delete(ctx context.Context, userID, followID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND follow_id = ?", userID, followID).
		Delete(&model.Follow{}).Error
}
func (r *GormUserRepo) Exists(ctx context.Context, userID, followID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Follow{}).
		Where("user_id = ? AND follow_id = ?", userID, followID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
func (r *GormUserRepo) GetFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&model.Follow{}).
		Where("user_id = ?", userID).
		Pluck("follow_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}
func (r *GormUserRepo) Updata(ctx context.Context, userID int64, avatarURL string, bio string) error {
	updates := make(map[string]interface{})
	if avatarURL != "" {
		updates["avatar_url"] = avatarURL
	}
	if bio != "" {
		updates["bio"] = bio
	}
	if len(updates) == 0 {
		return gorm.ErrRecordNotFound
	}

	res := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", userID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *GormUserRepo) ListFollowing(ctx context.Context, userID int64, cursor time.Time, limit int) ([]*model.Follow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if !cursor.IsZero() {
		q = q.Where("created_at < ?", cursor)
	}
	var follows []*model.Follow
	err := q.Order("created_at DESC").Limit(limit).Find(&follows).Error
	return follows, err
}

func (r *GormUserRepo) ListFollowers(ctx context.Context, userID int64, cursor time.Time, limit int) ([]*model.Follow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	q := r.db.WithContext(ctx).Where("follow_id = ?", userID)
	if !cursor.IsZero() {
		q = q.Where("created_at < ?", cursor)
	}
	var followers []*model.Follow
	err := q.Order("created_at DESC").Limit(limit).Find(&followers).Error
	return followers, err
}
