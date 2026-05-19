package service

import (
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/cache"
	"context"
	"time"
)

type FeedListResponse struct {
	Items      []model.FeedItem `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type FeedService struct {
	repo     *repo.GormFeedRepo
	userRepo *repo.GormUserRepo
	cache 	 *cache.RedisCache
}

func NewFeedService(r *repo.GormFeedRepo, u *repo.GormUserRepo , c *cache.RedisCache) *FeedService {
	return &FeedService{repo: r, userRepo: u, cache: c}
}

func (s *FeedService) ListForYou(ctx context.Context, cursorStr string, limit int) (*FeedListResponse, error) {
	feedCursor, err := cursor.ParseFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	notes, err := s.repo.ListForYou(ctx, feedCursor, limit)
	if err != nil {
		return nil, err
	}

	authorIDs := make([]int64, 0, len(notes))
	seen := make(map[int64]struct{}, len(notes))
	for _, note := range notes {
		if _, ok := seen[note.AuthorID]; ok {
			continue
		}
		seen[note.AuthorID] = struct{}{}
		authorIDs = append(authorIDs, note.AuthorID)
	}

	users, err := s.userRepo.GetByIDs(authorIDs)
	if err != nil {
		return nil, err
	}

	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]model.FeedItem, 0, len(notes))
	nextCursor := ""
	for _, note := range notes {
		item := model.FeedItem{
			ID:          note.ID,
			AuthorID:    note.AuthorID,
			Title:       note.Title,
			Content:     cursor.BuildSummary(note.Content, 100),
			Type:        note.Type,
			PublishedAt: note.PublishedAt,
		}
		if u, ok := userMap[note.AuthorID]; ok {
			item.Author = model.AuthorInfo{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: u.AvatarURL,
			}
		}
		items = append(items, item)
		nextCursor = cursor.EncodeFeedCursor(note.PublishedAt, note.ID)
	}
	return &FeedListResponse{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (s *FeedService) buildFeedResponse(ctx context.Context, notes []*model.Note) (*FeedListResponse, error) {
	authorIDs := make([]int64, 0, len(notes))
	seen := make(map[int64]struct{}, len(notes))

	for _, note := range notes {
		if _, ok := seen[note.AuthorID]; ok {
			continue
		}
		seen[note.AuthorID] = struct{}{}
		authorIDs = append(authorIDs, note.AuthorID)
	}

	users, err := s.userRepo.GetByIDs(authorIDs)
	if err != nil {
		return nil, err
	}

	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]model.FeedItem, 0, len(notes))
	nextCursor := ""

	for _, note := range notes {
		item := model.FeedItem{
			ID:          note.ID,
			AuthorID:    note.AuthorID,
			Title:       note.Title,
			Content:     cursor.BuildSummary(note.Content, 120),
			Type:        note.Type,
			PublishedAt: note.PublishedAt,
		}

		if u, ok := userMap[note.AuthorID]; ok {
			item.Author = model.AuthorInfo{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: u.AvatarURL,
			}
		}

		items = append(items, item)
		nextCursor = cursor.EncodeFeedCursor(note.PublishedAt, note.ID)
	}

	return &FeedListResponse{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (s *FeedService) ListFollowing(ctx context.Context, userID int64, cursorStr string, limit int) (*FeedListResponse, error) {
	feedCursor, err := cursor.ParseFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	followIDs , err := s.getFollowingIDs(ctx,userID)
	// followIDs, err := s.userRepo.GetFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(followIDs) == 0 {
		return &FeedListResponse{
			Items:      []model.FeedItem{},
			NextCursor: "",
		}, nil
	}

	notes, err := s.repo.ListFollowing(ctx, followIDs, feedCursor, limit)
	if err != nil {
		return nil, err
	}

	return s.buildFeedResponse(ctx, notes)
}
func (s *FeedService) getFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	key := cache.FollowingIDsKey(userID)
	if s.cache != nil {
		ids , err := s.cache.GetInt64Slice(ctx,key)
		if err == nil {
			return ids , nil
		}
		if err != cache.ErrRedisMiss {
			// 如果缓存不存在，则从数据库中获取
		}
	}
	ids , err := s.userRepo.GetFollowingIDs(ctx,userID)
	if err != nil {
		return nil,err
	}
	if s.cache != nil {
		_ = s.cache.SetInt64Slice(ctx,key,ids,30*time.Minute)
	}
	return ids,nil
}
