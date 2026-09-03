package main

import (
	"XFeedSystem/configs"
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/events"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/outbox"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/queue"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/service"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type counterDelta struct {
	Like     int64
	Favorite int64
	Comment  int64
}

func main() {
	logger.Init("info")
	defer logger.Sync()

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Sugar.Fatalf("加载配置失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 注意：确保 outbox_events 表已建。
	// 方式 A：跑 migrations/014_outbox.sql；
	// 方式 B：configs/db.go 的 AutoMigrate 列表里加上 &outbox.Event{}。
	db := configs.InitDB(cfg.MySQL.DSN)
	redisCache := cache.NewRedisCache(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	streamClient := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
	if err := streamClient.Ping(ctx).Err(); err != nil {
		logger.Sugar.Fatalf("redis 连接失败: %v", err)
	}

	// ---- 与 API 相同的业务组装（只不挂 HTTP） ----
	searchRepo := repo.NewSearchRepo(cfg.Meilisearch.Host, cfg.Meilisearch.APIKey, cfg.Meilisearch.Index)
	_ = searchRepo.EnsureIndex(ctx)

	outboxRepo := outbox.NewRepo(db)
	userRepo := repo.NewGormUserRepo(db, outboxRepo)
	blockService := service.NewBlockService(repo.NewGormBlockRepo(db), userRepo, redisCache)
	notifService := service.NewNotificationService(repo.NewGormNotificationRepo(db), userRepo, redisCache)
	statsService := service.NewStatsService(repo.NewGormStatsRepo(db), redisCache)
	feedRepo := repo.NewGormFeedRepo(db)
	feedService := service.NewFeedService(feedRepo, userRepo, redisCache, searchRepo, blockService, statsService)
	noteRepo := repo.NewGormNoteRepo(db, outboxRepo)
	followingFanout := service.NewFollowingFanoutService(feedRepo, userRepo, noteRepo, redisCache)

	// 计数器落库与打分周期重算只保留一份：
	// 原来它们挂在每个 API 实例上，多实例后会重复执行（计数器还会重复累加），
	// 必须收归 worker。
	statsService.StartFlusher(ctx)
	feedService.StartRescorer(ctx, 5*time.Minute)

	// ---- 后台任务 ----
	if cfg.Worker.RelayEnabled {
		relay := outbox.NewRelay(
			outbox.NewRepo(db),
			queue.NewStream(streamClient),
			cfg.Event.Relay.BatchSize,
			time.Duration(cfg.Event.Relay.IntervalMs)*time.Millisecond,
		)
		go func() {
			if err := relay.Run(ctx); err != nil {
				logger.Sugar.Errorf("relay 退出: %v", err)
			}
		}()
		logger.Sugar.Info("outbox relay 已启动")
	}

	start := func(group string, h queue.Handler) {
		c := queue.NewConsumer(streamClient, group, int64(cfg.Event.Consumer.BatchSize))
		go func() {
			if err := c.Run(ctx, h); err != nil {
				logger.Sugar.Errorf("consumer %s 退出: %v", group, err)
			}
		}()
		logger.Sugar.Infof("consumer %s 已启动", group)
	}

	startBatch := func(group string, h queue.BatchHandler) {
		c := queue.NewConsumer(streamClient, group, int64(cfg.Event.Consumer.BatchSize))
		go func() {
			if err := c.RunBatch(ctx, h); err != nil {
				logger.Sugar.Errorf("consumer %s 退出: %v", group, err)
			}
		}()
		logger.Sugar.Infof("consumer %s (batch) 已启动", group)
	}

	start(events.GroupFeed, feedHandler(feedService, followingFanout, redisCache))
	start(events.GroupSearch, searchHandler(searchRepo, noteRepo))
	startBatch(events.GroupNotify, notifyBatchHandler(notifService, userRepo, blockService))
	startEventMonitor(ctx, db, queue.NewStream(streamClient), 10*time.Second)
	startBatch(events.GroupCounter, counterBatchHandler(redisCache))

	StartCounterFlusher(ctx, redisCache, noteRepo, feedService,
		time.Duration(cfg.Event.Counter.FlushIntervalMs)*time.Millisecond,
		cfg.Event.Counter.FlushBatchSize) // 每 N ms 批量落库

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Sugar.Info("worker 收到退出信号，等待收尾...")
	cancel()
	time.Sleep(2 * time.Second)
}

// feedHandler 维护 Feed 打分 ZSET：note.created/updated 与互动事件 → 重算单条分数；
// note.deleted → 移除。ZSET 变了再失效页字节缓存。
// 为什么 feed 也要消费互动事件：打分公式里 like/favorite/comment 计数会变，
// 分数不重算 Feed 就不更新（这正是当前 API 里 invalidateNoteFeed 干的事）。
func feedHandler(feed *service.FeedService, fanout *service.FollowingFanoutService, rc *cache.RedisCache) queue.Handler {
	return func(ctx context.Context, typ string, p events.Payload) error {
		switch typ {
		case events.NoteCreated:
			if err := feed.UpsertNoteScore(ctx, p.NoteID); err != nil {
				return err
			}
			if err := fanout.HandleNoteCreated(ctx, p); err != nil {
				return err
			}
			// 候选集合变化:全局失效,让新笔记尽快出现在所有人首页
			return rc.InvalidateFeedRawAll(ctx)
		case events.NoteUpdated:
			if err := feed.UpsertNoteScore(ctx, p.NoteID); err != nil {
				return err
			}
			return fanout.HandleNoteUpdated(ctx, p)
		case events.NoteDeleted:
			if err := feed.RemoveNoteScore(ctx, p.NoteID); err != nil {
				return err
			}
			if err := fanout.HandleNoteDeleted(ctx, p); err != nil {
				return err
			}
			return rc.InvalidateFeedRawAll(ctx)
		case events.UserFollowed:
			if err := fanout.HandleUserFollowed(ctx, p); err != nil {
				return err
			}
			return rc.InvalidateFeedRawForUser(ctx, p.ActorID)
		case events.UserUnfollowed:
			if err := fanout.HandleUserUnfollowed(ctx, p); err != nil {
				return err
			}
			return rc.InvalidateFeedRawForUser(ctx, p.ActorID)
		default:
			return nil
		}
	}
}

const counterDedupTTL = 24 * time.Hour

func counterDedupKey(eventID int64) string {
	// 注意：不能用 "counter:" 前缀，否则会被 Flusher 的 SCAN counter:* 误扫（去重键会海量堆积）
	return fmt.Sprintf("cntdedup:%d", eventID)
}

// searchHandler 维护 Meilisearch 索引。正文等大字段不放进事件，
// worker 回 MySQL 读（真相源），保证索引内容永远和库一致。
func searchHandler(sr *repo.SearchRepo, nr *repo.GormNoteRepo) queue.Handler {
	return func(ctx context.Context, typ string, p events.Payload) error {
		switch typ {
		case events.NoteCreated, events.NoteUpdated:
			n, err := nr.GetByID(ctx, p.NoteID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// 笔记确实不存在 → 清理索引
					return sr.Delete(ctx, p.NoteID)
				}
				// MySQL 超时/断连等临时错误：绝不能当成 NotFound 误删索引，返回 error 等待重试
				return err
			}
			if n.Status != model.NoteStatusPublished {
				return sr.Delete(ctx, p.NoteID)
			}
			return sr.Index(ctx, &repo.NoteDocument{
				ID:          n.ID,
				Title:       n.Title,
				Content:     cursor.StripHTML(n.Content),
				AuthorID:    n.AuthorID,
				Type:        n.Type,
				PublishedAt: n.PublishedAt.Unix(),
			})
		case events.NoteDeleted:
			return sr.Delete(ctx, p.NoteID)
		default:
			return nil
		}
	}
}

func StartCounterFlusher(ctx context.Context, rc *cache.RedisCache, noteRepo *repo.GormNoteRepo, feed *service.FeedService, interval time.Duration, batch int) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := flushCounters(ctx, rc, noteRepo, feed, batch); err != nil {
					logger.Sugar.Errorf("flush counters err: %v", err)
				}
			}
		}
	}()
}

func flushCounters(ctx context.Context, rc *cache.RedisCache, noteRepo *repo.GormNoteRepo, feed *service.FeedService, batch int) error {
	keys, err := rc.ScanKeys(ctx, cache.CounterPrefix()+"*", 500)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	for i := 0; i < len(keys); i += batch {
		end := i + batch
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[i:end]
		vals, err := rc.DrainCounters(ctx, chunk)
		if err != nil {
			return err
		}

		like := map[int64]int64{}
		favorite := map[int64]int64{}
		comment := map[int64]int64{}
		affected := map[int64]struct{}{}
		for j, k := range chunk {
			kind, id, ok := parseCounterKey(k)
			if !ok || vals[j] == 0 {
				continue
			}
			affected[id] = struct{}{}
			switch kind {
			case "like":
				like[id] += vals[j]
			case "favorite":
				favorite[id] += vals[j]
			case "comment":
				comment[id] += vals[j]
			}
		}
		if len(like)+len(favorite)+len(comment) == 0 {
			continue
		}

		if err := noteRepo.BatchAddCounters(ctx, like, favorite, comment); err != nil {
			// 落库失败：把增量原样写回 Redis，下一轮再刷，不丢计数
			rollbackCounters(ctx, rc, like, favorite, comment)
			return err
		}

		// 计数已落库：清笔记缓存（详情显示新计数）+ 重算 Feed 分数（打分依赖计数）
		for id := range affected {
			_ = rc.Delete(ctx, cache.NoteKey(id), cache.FeedNoteKey(id), cache.NoteDetailRawKey(id))
			_ = feed.UpsertNoteScore(ctx, id)
		}
	}
	return nil
}

func parseCounterKey(k string) (string, int64, bool) {
	rest := strings.TrimPrefix(k, cache.CounterPrefix())
	parts := strings.Split(rest, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return parts[0], id, true
}

func rollbackCounters(ctx context.Context, rc *cache.RedisCache, like, favorite, comment map[int64]int64) {
	for id, d := range like {
		_, _ = rc.IncrBy(ctx, cache.CounterLikeKey(id), d)
	}
	for id, d := range favorite {
		_, _ = rc.IncrBy(ctx, cache.CounterFavoriteKey(id), d)
	}
	for id, d := range comment {
		_, _ = rc.IncrBy(ctx, cache.CounterCommentKey(id), d)
	}
}

// counterBatchHandler 整批事件：先去重（SetNX Pipeline），再按 (noteID, 类型) 聚合，最后 INCRBY Pipeline。
// 1000 条 Like 只产生 1~N 条 INCRBY，而非 1000 条 INCR。
func counterBatchHandler(rc *cache.RedisCache) queue.BatchHandler {
	return func(ctx context.Context, msgs []queue.BatchMessage) error {
		if len(msgs) == 0 {
			return nil
		}
		// 1) 批量幂等去重
		dedupKeys := make([]string, len(msgs))
		for i, m := range msgs {
			dedupKeys[i] = counterDedupKey(m.Payload.EventID)
		}
		first, err := rc.SetNXMany(ctx, dedupKeys, counterDedupTTL)
		if err != nil {
			return err
		}

		// 2) 按 (noteID, 类型) 聚合增量
		deltas := map[int64]*counterDelta{}
		for i, m := range msgs {
			if !first[i] {
				continue // 重复投递，跳过
			}
			d := deltas[m.Payload.NoteID]
			if d == nil {
				d = &counterDelta{}
				deltas[m.Payload.NoteID] = d
			}
			switch m.Type {
			case events.NoteLiked:
				d.Like++
			case events.NoteUnliked:
				d.Like--
			case events.NoteFavorited:
				d.Favorite++
			case events.NoteUnfavorited:
				d.Favorite--
			case events.CommentCreated:
				if m.Payload.ParentID == 0 {
					d.Comment++ // 回复评论不计入
				}
			case events.CommentDeleted:
				if m.Payload.ParentID == 0 {
					d.Comment--
				}
			}
		}

		// 3) 聚合后 INCRBY Pipeline
		keys := make([]string, 0, len(deltas)*3)
		vals := make([]int64, 0, len(deltas)*3)
		for id, d := range deltas {
			if d.Like != 0 {
				keys = append(keys, cache.CounterLikeKey(id))
				vals = append(vals, d.Like)
			}
			if d.Favorite != 0 {
				keys = append(keys, cache.CounterFavoriteKey(id))
				vals = append(vals, d.Favorite)
			}
			if d.Comment != 0 {
				keys = append(keys, cache.CounterCommentKey(id))
				vals = append(vals, d.Comment)
			}
		}
		return rc.IncrByMany(ctx, keys, vals)
	}
}

func startEventMonitor(ctx context.Context, db *gorm.DB, stream *queue.Stream, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				var pending int64
				_ = db.Raw("SELECT COUNT(*) FROM outbox_events WHERE status = 0").Scan(&pending).Error
				streamLen, _ := stream.Len(ctx)
				feedP, _ := stream.Pending(ctx, events.GroupFeed)
				searchP, _ := stream.Pending(ctx, events.GroupSearch)
				notifyP, _ := stream.Pending(ctx, events.GroupNotify)
				counterP, _ := stream.Pending(ctx, events.GroupCounter)
				logger.Sugar.Infow("event backlog",
					"outbox_pending", pending,
					"stream_len", streamLen,
					"pending_feed", feedP,
					"pending_search", searchP,
					"pending_notify", notifyP,
					"pending_counter", counterP,
				)
			}
		}
	}()
}

func addNotif(rows []*model.Notification, actorID, userID int64, typ int8, targetID, targetNoteID int64, message string, eventID int64) []*model.Notification {
	if actorID == userID || userID <= 0 {
		return rows
	}
	return append(rows, &model.Notification{
		UserID: userID, ActorID: actorID, Type: typ,
		TargetID: targetID, TargetNoteID: targetNoteID,
		Message: message, EventID: eventID,
	})
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func notifyBatchHandler(ns *service.NotificationService, ur repo.UserRepo, bs *service.BlockService) queue.BatchHandler {
	return func(ctx context.Context, msgs []queue.BatchMessage) error {
		rows := make([]*model.Notification, 0, len(msgs)*2)
		mentionNames := map[string]struct{}{}
		for _, m := range msgs {
			p := m.Payload
			switch m.Type {
			case events.NoteLiked:
				rows = addNotif(rows, p.ActorID, p.AuthorID, model.NotifTypeLike, p.NoteID, p.NoteID, "赞了你的笔记", p.EventID)
			case events.NoteFavorited:
				rows = addNotif(rows, p.ActorID, p.AuthorID, model.NotifTypeFavorite, p.NoteID, p.NoteID, "收藏了你的笔记", p.EventID)
			case events.CommentCreated:
				if p.ParentID == 0 {
					rows = addNotif(rows, p.ActorID, p.AuthorID, model.NotifTypeComment, p.CommentID, p.NoteID, "评论了你的笔记", p.EventID)
				} else {
					rows = addNotif(rows, p.ActorID, p.ReplyToUserID, model.NotifTypeReplyComment, p.CommentID, p.NoteID, "回复了你的评论", p.EventID)
					if p.ReplyToUserID != p.AuthorID {
						rows = addNotif(rows, p.ActorID, p.AuthorID, model.NotifTypeComment, p.CommentID, p.NoteID, "评论了你的笔记", p.EventID)
					}
				}
				for _, name := range p.MentionNames {
					mentionNames[name] = struct{}{}
				}
			case events.UserFollowed:
				rows = addNotif(rows, p.ActorID, p.AuthorID, model.NotifTypeFollow, p.ActorID, 0, "关注了你", p.EventID)
			}
		}

		// @提及：整批只查一次用户名
		if len(mentionNames) > 0 {
			names := make([]string, 0, len(mentionNames))
			for n := range mentionNames {
				names = append(names, n)
			}
			users, err := ur.FindByUsernames(ctx, names)
			if err == nil && len(users) > 0 {
				for _, m := range msgs {
					if m.Type != events.CommentCreated || len(m.Payload.MentionNames) == 0 {
						continue
					}
					targetID := m.Payload.CommentID
					if targetID == 0 {
						targetID = m.Payload.NoteID
					}
					for _, u := range users {
						if !containsName(m.Payload.MentionNames, u.Username) || u.ID == m.Payload.ActorID {
							continue
						}
						if blocked, _ := bs.IsBlockedEitherWay(ctx, m.Payload.ActorID, u.ID); blocked {
							continue
						}
						rows = addNotif(rows, m.Payload.ActorID, u.ID, model.NotifTypeMention, targetID, m.Payload.NoteID, "@提到了你", m.Payload.EventID)
					}
				}
			}
		}
		return ns.BatchCreate(ctx, rows)
	}
}
