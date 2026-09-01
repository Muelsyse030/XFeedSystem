package outbox

import (
	"XFeedSystem/internal/events"
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/queue"
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Event struct {
	ID          int64      `gorm:"primaryKey"`
	EventType   string     `gorm:"size:64;not null"`
	Payload     string     `gorm:"type:json;not null"`
	Status      int8       `gorm:"not null;default:0"` // 0=待发布 1=已发布
	Attempts    int        `gorm:"not null;default:0"`
	CreatedAt   time.Time  `gorm:"not null"`
	PublishedAt *time.Time `gorm:"index"`
}

func (Event) TableName() string { return "outbox_events" }

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Enqueue 在业务写完成后插入事件
func (r *Repo) Enqueue(ctx context.Context, typ string, p events.Payload) error {
	return r.EnqueueTx(ctx, r.db.WithContext(ctx), typ, p)
}

// EnqueueTx 在调用方传入的事务里插入事件，与业务写同生共死
func (r *Repo) EnqueueTx(ctx context.Context, tx *gorm.DB, typ string, p events.Payload) error {
	if p.EventID != 0 {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	evt := Event{EventType: typ, Payload: string(raw)}
	if err := tx.WithContext(ctx).Create(&evt).Error; err != nil {
		return err
	}
	p.EventID = evt.ID
	raw, _ = json.Marshal(p)
	return tx.WithContext(ctx).Model(&evt).Update("payload", string(raw)).Error
}

// Claim 领取一批待发布事件。SKIP LOCKED 保证多个 relay 实例并发领取互不冲突
func (r *Repo) Claim(ctx context.Context, batch int) ([]*Event, error) {
	var list []*Event
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", 0).
			Order("id ASC").
			Limit(batch).
			Find(&list).Error
	})
	return list, err
}

func (r *Repo) MarkPublished(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&Event{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"status": 1, "published_at": time.Now()}).Error
}

func (r *Repo) MarkAttempt(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&Event{}).
		Where("id = ?", id).
		Update("attempts", gorm.Expr("attempts + 1")).Error
}

// Relay 把 outbox 表里的待发布事件 XADD 到 Redis Streams
type Relay struct {
	repo  *Repo
	bus   *queue.Stream
	batch int
	tick  time.Duration
}

func NewRelay(r *Repo, bus *queue.Stream, batch int, tick time.Duration) *Relay {
	return &Relay{repo: r, bus: bus, batch: batch, tick: tick}
}

// Run 周期性执行：领事件 → XADD → 标记已发布
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		evts, err := r.repo.Claim(ctx, r.batch)
		if err != nil {
			logger.Sugar.Errorf("outbox claim error %v", err)
			continue
		}
		if len(evts) == 0 {
			continue
		}

		pub := make([]queue.PublishEvent, 0, len(evts))
		ok := make([]bool, len(evts))
		for i, evt := range evts {
			var p events.Payload
			if err := json.Unmarshal([]byte(evt.Payload), &p); err != nil {
				logger.Sugar.Errorf("outbox event %d payload broken: %v", evt.ID, err)
				_ = r.repo.MarkPublished(ctx, evt.ID) // 坏消息直接丢弃
				continue
			}
			p.EventID = evt.ID
			pub = append(pub, queue.PublishEvent{Type: evt.EventType, Payload: p})
			ok[i] = true
		}
		if len(pub) > 0 {
			if err := r.bus.PublishBatch(ctx, pub); err != nil {
				// 发布失败：整批重试（attempts+1）
				for i, evt := range evts {
					if ok[i] {
						_ = r.repo.MarkAttempt(ctx, evt.ID)
					}
				}
				continue
			}
		}
		for i, evt := range evts {
			if ok[i] {
				_ = r.repo.MarkPublished(ctx, evt.ID)
			}
		}
	}
}
