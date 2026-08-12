package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"regexp"
	"strings"
	"time"
)

const (
	topicHotTTL     = 5 * time.Minute
	topicMaxPerNote = 5
	topicNameMaxLen = 32
)

var topicRe = regexp.MustCompile(`#([\p{L}\p{N}_]{1,32})`)

type TopicService struct {
	repo  repo.TopicRepo
	cache *cache.RedisCache
}

func NewTopicService(r repo.TopicRepo, c *cache.RedisCache) *TopicService {
	return &TopicService{repo: r, cache: c}
}

// ExtractTopics 合并显式 topics 与内容中的 #话题，去重并限制数量
func (s *TopicService) ExtractTopics(content string, explicit []string) []string {
	seen := make(map[string]struct{}, 8)
	out := make([]string, 0, topicMaxPerNote)
	add := func(t string) {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t == "" || len(t) > topicNameMaxLen || len(out) >= topicMaxPerNote {
			return
		}
		if _, ok := seen[t]; ok {
			return
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	for _, t := range explicit {
		add(t)
	}
	for _, m := range topicRe.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	return out
}

// AttachToNote 为笔记绑定话题（计数 +1，热门榜增量更新）
func (s *TopicService) AttachToNote(ctx context.Context, noteID int64, topics []string) error {
	if len(topics) == 0 {
		return nil
	}
	idByName, err := s.repo.UpsertByName(ctx, topics)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(idByName))
	for _, id := range idByName {
		ids = append(ids, id)
	}
	if err := s.repo.AttachNoteTopics(ctx, noteID, ids); err != nil {
		return err
	}
	s.bumpHot(ctx, ids)
	return nil
}

// ReplaceTopics 编辑时全量替换（先解绑递减，再绑定新话题）
func (s *TopicService) ReplaceTopics(ctx context.Context, noteID int64, topics []string) error {
	oldIDs, err := s.repo.DetachNoteTopics(ctx, noteID)
	if err != nil {
		return err
	}
	if err := s.repo.DecrementCounts(ctx, oldIDs); err != nil {
		return err
	}
	return s.AttachToNote(ctx, noteID, topics)
}

// DetachFromNote 删除笔记时解绑并递减
func (s *TopicService) DetachFromNote(ctx context.Context, noteID int64) error {
	oldIDs, err := s.repo.DetachNoteTopics(ctx, noteID)
	if err != nil {
		return err
	}
	return s.repo.DecrementCounts(ctx, oldIDs)
}

func (s *TopicService) GetByID(ctx context.Context, id int64) (*model.Topic, error) {
	return s.repo.GetByID(ctx, id)
}

// Hot 热门话题（完整 JSON 缓存 5 分钟，写操作失效重建）
func (s *TopicService) Hot(ctx context.Context, limit int) ([]*model.Topic, error) {
	key := cache.TopicHotKey()
	if s.cache != nil {
		var cached []*model.Topic
		if err := s.cache.GetJSON(ctx, key, &cached); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}
	topics, err := s.repo.Hot(ctx, 50)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(topics) > 0 {
		_ = s.cache.SetJSON(ctx, key, topics, topicHotTTL)
	}
	if len(topics) > limit {
		topics = topics[:limit]
	}
	return topics, nil
}

func (s *TopicService) Suggest(ctx context.Context, q string, limit int) ([]*model.Topic, error) {
	return s.repo.Suggest(ctx, q, limit)
}

// bumpHot 热门榜缓存失效（下次读取重建）
func (s *TopicService) bumpHot(ctx context.Context, topicIDs []int64) {
	if s.cache == nil || len(topicIDs) == 0 {
		return
	}
	_ = s.cache.Delete(ctx, cache.TopicHotKey())
}
