package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrUserNotFound = errors.New("user not found")

type UserService struct {
	repo repo.UserRepo
	cache *cache.RedisCache
}

func NewUserService(r repo.UserRepo , c *cache.RedisCache) *UserService {
	return &UserService{repo: r, cache: c}
}

func (s *UserService) Register(username, password, confirmPassword string) error {
	if username == "" || password == "" || confirmPassword == "" {
		return errors.New("用户名或者密码不能为空")
	}
	if password != confirmPassword {
		return errors.New("确认密码不一致")
	}
	existingUser, err := s.repo.FindByUsername(username)
	if err == nil && existingUser != nil {
		return errors.New("用户名已存在")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}
	user := model.User{
		Username:     username,
		PasswordHash: string(hash),
	}
	if err := s.repo.CreateUser(&user); err != nil {
		return errors.New("注册失败")
	}
	return nil
}
func (s *UserService) Login(username string, password string) (*model.User, error) {
	if username == "" || password == "" {
		return nil, errors.New("用户名或者密码不能为空")
	}
	user, err := s.repo.FindByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	if err := s.repo.CompareHashAndPassword(user.PasswordHash, password); err != nil {
		return nil, errors.New("密码错误")
	}
	return user, nil
}

func (s *UserService) GetProfile(uid int64) (*model.User, error) {
	user, err := s.repo.GetProfile(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, errors.New("获取用户信息失败")
	}
	return user, nil
}

func (s *UserService) Follow(ctx context.Context, userID int64, followID int64) error {
	if userID == followID {
		return errors.New("不能关注自己")
	}
	if _, err := s.repo.GetProfile(followID); err != nil {
		return errors.New("用户不存在")
	}
	if err := s.repo.Followbyid(ctx, userID, followID); err != nil {
		return errors.New("关注失败")
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx,cache.FollowingIDsKey(userID))
	}
	return nil
}
func (s *UserService) Unfollow(ctx context.Context, userID int64, followID int64) error {
	if userID == followID {
		return errors.New("不能取消关注自己")
	}
	if _, err := s.repo.GetProfile(followID); err != nil {
		return errors.New("用户不存在")
	}
	if err := s.repo.Delete(ctx, userID, followID); err != nil {
		return errors.New("关注失败")
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx,cache.FollowingIDsKey(userID))
	}
	return nil
}
func (s *UserService) Isfollow(ctx context.Context, userID, followID int64) (bool, error) {
	key := cache.FollowingIDsKey(userID)
	if s.cache != nil {
		if ids , err := s.cache.GetInt64Slice(ctx,key); err == nil{
			for _,id := range ids{
				if id == followID{
					return true , nil
				}
			}
			return false , nil
		}
	}
	isfollow, err := s.repo.Exists(ctx, userID, followID)
	if err != nil {
		return false, errors.New("判断错误")
	}
	if s.cache != nil {
		if ids,err := s.repo.GetFollowingIDs(ctx,userID); err == nil{
			_ = s.cache.SetInt64Slice(ctx,key,ids,30*time.Minute)
		}
	}
	return isfollow, nil
}
func (s *UserService) Updata(ctx context.Context,userID int64,avatarURL string,bio string) error {
	if userID <= 0 {
		return errors.New("用户ID不能为空")
	}
	if avatarURL == "" && bio == "" {
		return errors.New("头像和简介不能同时为空")
	}
	return s.repo.Updata(ctx,userID,avatarURL,bio)
}
