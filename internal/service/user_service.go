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
var ErrBlockedByTarget = errors.New("无法关注：你已拉黑对方或被对方拉黑")

const userCacheTTL = 1 * time.Hour

type UserService struct {
	repo     repo.UserRepo
	cache    *cache.RedisCache
	notifSvc *NotificationService
	block    *BlockService
}

type FollowUserItem struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	AvatarURL  string `json:"avatar_url"`
	Bio        string `json:"bio"`
	IsFollowed bool   `json:"is_followed"`
}

type FollowListResponse struct {
	List       []*FollowUserItem `json:"list"`
	NextCursor string            `json:"next_cursor"`
}

func NewUserService(r repo.UserRepo, c *cache.RedisCache, ns *NotificationService, b *BlockService) *UserService {
	return &UserService{repo: r, cache: c, notifSvc: ns, block: b}
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
	if s.cache != nil {
		var user model.User
		if err := s.cache.GetJSON(context.Background(), cache.UserKey(uid), &user); err == nil {
			return &user, nil
		}
	}
	user, err := s.repo.GetProfile(uid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, errors.New("获取用户信息失败")
	}
	if s.cache != nil {
		user.PasswordHash = ""
		_ = s.cache.SetJSON(context.Background(), cache.UserKey(uid), user, userCacheTTL)
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
	if s.block != nil {
		if blocked, _ := s.block.IsBlockedEitherWay(ctx, userID, followID); blocked {
			return ErrBlockedByTarget
		}
	}
	if err := s.repo.Followbyid(ctx, userID, followID); err != nil {
		return errors.New("关注失败")
	}
	if s.cache != nil {
		key := cache.FollowingIDsKey(userID)
		_ = s.cache.SAdd(ctx, key, followID)
		safeGo(func() {
			_ = s.cache.InvalidateFeedEngineForUser(context.Background(), userID)
			_ = s.cache.InvalidateFeedRawForUser(context.Background(), userID)
		})
	}
	if s.notifSvc != nil {
		safeGo(func() {
			s.notifSvc.Create(context.Background(), userID, followID,
				model.NotifTypeFollow, followID, 0, "关注了你")
		})
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
		return errors.New("取消关注失败")
	}
	if s.cache != nil {
		key := cache.FollowingIDsKey(userID)
		_ = s.cache.SRem(ctx, key, followID)
		safeGo(func() {
			_ = s.cache.InvalidateFeedEngineForUser(context.Background(), userID)
			_ = s.cache.InvalidateFeedRawForUser(context.Background(), userID)
		})
	}
	return nil
}
func (s *UserService) Isfollow(ctx context.Context, userID, followID int64) (bool, error) {
	key := cache.FollowingIDsKey(userID)
	if s.cache != nil {
		isfollow, err := s.cache.SIsMember(ctx, key, followID)
		if err == nil {
			return isfollow, nil
		}
	}
	isfollow, err := s.repo.Exists(ctx, userID, followID)
	if err != nil {
		return false, errors.New("判断错误")
	}
	if isfollow && s.cache != nil {
		ids, err := s.repo.GetFollowingIDs(ctx, userID)
		if err == nil && len(ids) > 0 {
			_ = s.cache.SAdd(ctx, key, ids...)
			_ = s.cache.Expire(ctx, key, 30*time.Minute)
		}
	}
	return isfollow, nil
}
func (s *UserService) Updata(ctx context.Context, userID int64, avatarURL string, bio string) error {
	if userID <= 0 {
		return errors.New("用户ID不能为空")
	}
	if avatarURL == "" && bio == "" {
		return errors.New("头像和简介不能同时为空")
	}
	if err := s.repo.Updata(ctx, userID, avatarURL, bio); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.Delete(ctx, cache.UserKey(userID))
		_ = s.cache.Delete(ctx, cache.UserProfileRawKey(userID))
	}
	return nil
}

func parseTimeCursor(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

func formatTimeCursor(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func (s *UserService) ListFollowing(ctx context.Context, userID int64, cursorStr string, limit int, currentUserID int64) (*FollowListResponse, error) {
	cursor, err := parseTimeCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	follows, err := s.repo.ListFollowing(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(follows) == 0 {
		return &FollowListResponse{List: []*FollowUserItem{}, NextCursor: ""}, nil
	}

	ids := make([]int64, len(follows))
	for i, follow := range follows {
		ids[i] = follow.FollowID
	}

	users, _ := s.repo.GetByIDs(ids)
	userMap := make(map[int64]*model.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	followedSet := make(map[int64]bool)
	if currentUserID > 0 {
		myFollows, _ := s.repo.GetFollowingIDs(ctx, currentUserID)
		for _, fid := range myFollows {
			followedSet[fid] = true
		}
	}

	items := make([]*FollowUserItem, 0, len(follows))
	for _, f := range follows {
		u, ok := userMap[f.FollowID]
		if !ok {
			continue
		}
		items = append(items, &FollowUserItem{
			ID:         u.ID,
			Username:   u.Username,
			AvatarURL:  u.AvatarURL,
			Bio:        u.Bio,
			IsFollowed: followedSet[u.ID],
		})
	}

	nextCursor := formatTimeCursor(follows[len(follows)-1].CreatedAt)
	return &FollowListResponse{List: items, NextCursor: nextCursor}, nil
}

func (s *UserService) ListFollowers(ctx context.Context, userID int64, cursorStr string, limit int, currentUserID int64) (*FollowListResponse, error) {
	cursor, err := parseTimeCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	follows, err := s.repo.ListFollowers(ctx, userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(follows) == 0 {
		return &FollowListResponse{List: []*FollowUserItem{}, NextCursor: ""}, nil
	}

	ids := make([]int64, len(follows))
	for i, follow := range follows {
		ids[i] = follow.UserID
	}

	users, _ := s.repo.GetByIDs(ids)
	userMap := make(map[int64]*model.User)
	for _, u := range users {
		userMap[u.ID] = u
	}

	followedSet := make(map[int64]bool)
	if currentUserID > 0 {
		myFollows, _ := s.repo.GetFollowingIDs(ctx, currentUserID)
		for _, fid := range myFollows {
			followedSet[fid] = true
		}
	}

	items := make([]*FollowUserItem, 0, len(follows))
	for _, f := range follows {
		u, ok := userMap[f.UserID]
		if !ok {
			continue
		}
		items = append(items, &FollowUserItem{
			ID:         u.ID,
			Username:   u.Username,
			AvatarURL:  u.AvatarURL,
			Bio:        u.Bio,
			IsFollowed: followedSet[u.ID],
		})
	}

	nextCursor := formatTimeCursor(follows[len(follows)-1].CreatedAt)
	return &FollowListResponse{List: items, NextCursor: nextCursor}, nil
}
