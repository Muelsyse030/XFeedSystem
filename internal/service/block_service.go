package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/repo"
	"context"
	"errors"
	"time"
)

type BlockService struct {
	repo     repo.BlockRepo
	userRepo *repo.GormUserRepo
	cache    *cache.RedisCache
}

func NewBlockService(r repo.BlockRepo, ur *repo.GormUserRepo, c *cache.RedisCache) *BlockService {
	return &BlockService{repo: r, userRepo: ur, cache: c}
}

func (s *BlockService) Block(ctx context.Context, userID, blockedID int64) error {
	if userID == blockedID {
		return errors.New("不能拉黑自己")
	}
	if _, err := s.userRepo.GetProfile(blockedID); err != nil {
		return errors.New("用户不存在")
	}
	if blocked, _ := s.repo.IsBlocked(ctx, userID, blockedID); blocked {
		return errors.New("已经拉黑了该用户")
	}
	if err := s.repo.Block(ctx, userID, blockedID); err != nil {
		return err
	}
	// 拉黑时自动双向取消关注
	_ = s.userRepo.Delete(ctx, userID, blockedID)
	_ = s.userRepo.Delete(ctx, blockedID, userID)
	if s.cache != nil {
		_ = s.cache.SRem(ctx, cache.FollowingIDsKey(userID), blockedID)
		_ = s.cache.SRem(ctx, cache.FollowingIDsKey(blockedID), userID)
		_ = s.cache.SAdd(ctx, cache.BlockedIDsKey(userID), blockedID)
	}
	return nil
}

func (s *BlockService) Unblock(ctx context.Context, userID, blockedID int64) error {
	if err := s.repo.Unblock(ctx, userID, blockedID); err != nil {
		return err
	}
	if s.cache != nil {
		_ = s.cache.SRem(ctx, cache.BlockedIDsKey(userID), blockedID)
	}
	return nil
}

func (s *BlockService) GetBlockedIDs(ctx context.Context, userID int64) ([]int64, error) {
	key := cache.BlockedIDsKey(userID)
	if s.cache != nil {
		ids, err := s.cache.SMembers(ctx, key)
		if err == nil && len(ids) > 0 {
			return ids, nil
		}
	}
	ids, err := s.repo.GetBlockedIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(ids) > 0 {
		_ = s.cache.SAdd(ctx, key, ids...)
		_ = s.cache.Expire(ctx, key, 30*time.Minute)
	}
	return ids, nil
}

func (s *BlockService) IsBlockedEitherWay(ctx context.Context, a, b int64) (bool, error) {
	return s.repo.IsBlockedEitherWay(ctx, a, b)
}
