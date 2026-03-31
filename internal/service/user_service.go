package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	repo repo.UserRepo
}

func NewUserService(r repo.UserRepo) *UserService {
	return &UserService{repo: r}
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
	return nil
}
func (s *UserService) Isfollow(ctx context.Context, userID, followID int64) (bool, error) {
	isfollow, err := s.repo.Exists(ctx, userID, followID)
	if err != nil {
		return false, errors.New("判断错误")
	}
	return isfollow, nil
}
