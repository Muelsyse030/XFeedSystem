package repo

import (
	"XFeedSystem/internal/model"
	"context"

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
}
type GormUserRepo struct {
	db *gorm.DB
}

func NewGormUserRepo(db *gorm.DB) *GormUserRepo {
	return &GormUserRepo{
		db: db,
	}
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
