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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

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
	feedService := service.NewFeedService(repo.NewGormFeedRepo(db), userRepo, redisCache, searchRepo, blockService, statsService)
	noteRepo := repo.NewGormNoteRepo(db, outboxRepo)

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
			cfg.Worker.Batch,
			time.Duration(cfg.Worker.PollIntervalMs)*time.Millisecond,
		)
		go func() {
			if err := relay.Run(ctx); err != nil {
				logger.Sugar.Errorf("relay 退出: %v", err)
			}
		}()
		logger.Sugar.Info("outbox relay 已启动")
	}

	start := func(group string, h queue.Handler) {
		c := queue.NewConsumer(streamClient, group)
		go func() {
			if err := c.Run(ctx, h); err != nil {
				logger.Sugar.Errorf("consumer %s 退出: %v", group, err)
			}
		}()
		logger.Sugar.Infof("consumer %s 已启动", group)
	}

	start(events.GroupFeed, feedHandler(feedService, redisCache))
	start(events.GroupSearch, searchHandler(searchRepo, noteRepo))
	start(events.GroupNotify, notifyHandler(notifService, userRepo, blockService))

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
func feedHandler(feed *service.FeedService, rc *cache.RedisCache) queue.Handler {
	return func(ctx context.Context, typ string, p events.Payload) error {
		switch typ {
		case events.NoteCreated, events.NoteUpdated,
			events.NoteLiked, events.NoteUnliked,
			events.NoteFavorited, events.NoteUnfavorited,
			events.CommentCreated, events.CommentDeleted:
			if err := feed.UpsertNoteScore(ctx, p.NoteID); err != nil {
				return err
			}
		case events.NoteDeleted:
			if err := feed.RemoveNoteScore(ctx, p.NoteID); err != nil {
				return err
			}
		case events.UserFollowed, events.UserUnfollowed:
			return rc.InvalidateFeedRawForUser(ctx, p.ActorID)
		default:
			return nil
		}
		return rc.InvalidateFeedRawAll(ctx)
	}
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

// notifyHandler 创建站内通知并维护未读数。
// at-least-once 语义下同一事件可能被投递两次，所以先用 event_id 去重：
// SET NX 成功说明第一次见，失败说明之前已处理过。
func notifyHandler(ns *service.NotificationService, ur repo.UserRepo, bs *service.BlockService) queue.Handler {
	return func(ctx context.Context, typ string, p events.Payload) error {
		switch typ {
		case events.NoteLiked:
			return ns.Create(ctx, p.ActorID, p.AuthorID, model.NotifTypeLike, p.NoteID, p.NoteID, "赞了你的笔记", p.EventID)
		case events.NoteFavorited:
			return ns.Create(ctx, p.ActorID, p.AuthorID, model.NotifTypeFavorite, p.NoteID, p.NoteID, "收藏了你的笔记", p.EventID)
		case events.CommentCreated:
			if p.ParentID == 0 {
				if err := ns.Create(ctx, p.ActorID, p.AuthorID, model.NotifTypeComment, p.CommentID, p.NoteID, "评论了你的笔记", p.EventID); err != nil {
					return err
				}
			} else {
				if err := ns.Create(ctx, p.ActorID, p.ReplyToUserID, model.NotifTypeReplyComment, p.CommentID, p.NoteID, "回复了你的评论", p.EventID); err != nil {
					return err
				}
				if p.ReplyToUserID != p.AuthorID {
					if err := ns.Create(ctx, p.ActorID, p.AuthorID, model.NotifTypeComment, p.CommentID, p.NoteID, "评论了你的笔记", p.EventID); err != nil {
						return err
					}
				}
			}
			return createMentions(ctx, ns, ur, bs, p)
		case events.UserFollowed:
			return ns.Create(ctx, p.ActorID, p.AuthorID, model.NotifTypeFollow, p.ActorID, 0, "关注了你", p.EventID)
		}
		return nil
	}
}

// createMentions 复刻原 NoteService.notifyMentions 的逻辑：
// 用户名 → 查库 → 过滤自己/拉黑 → 建通知。
func createMentions(ctx context.Context, ns *service.NotificationService, ur repo.UserRepo, bs *service.BlockService, p events.Payload) error {
	if len(p.MentionNames) == 0 {
		return nil
	}
	users, err := ur.FindByUsernames(ctx, p.MentionNames)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}
	targetID := p.CommentID
	if targetID == 0 {
		targetID = p.NoteID
	}
	for _, u := range users {
		if u.ID == p.ActorID {
			continue
		}
		if bs != nil {
			blocked, err := bs.IsBlockedEitherWay(ctx, p.ActorID, u.ID)
			if err != nil {
				return err
			}
			if blocked {
				continue
			}
		}
		if err := ns.Create(ctx, p.ActorID, u.ID, model.NotifTypeMention, targetID, p.NoteID, "@提到了你", p.EventID); err != nil {
			return err
		}
	}
	return nil
}
