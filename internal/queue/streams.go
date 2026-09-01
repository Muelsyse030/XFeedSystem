package queue

import (
	"XFeedSystem/internal/events"
	"XFeedSystem/internal/pkg/logger"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Stream struct {
	client *redis.Client
	key    string
}

type PublishEvent struct {
	Type    string
	Payload events.Payload
}

type Consumer struct {
	client *redis.Client
	key    string
	group  string
	name   string
	batch  int64
}

type BatchMessage struct {
	ID      string
	Type    string
	Payload events.Payload
}

func NewStream(client *redis.Client) *Stream {
	return &Stream{client: client, key: events.StreamKey}
}

func (s *Stream) Publish(ctx context.Context, typ string, p events.Payload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: s.key,
		Values: map[string]any{"type": typ, "payload": string(raw)},
	}).Err()
}

func NewConsumer(client *redis.Client, group string, batch int64) *Consumer {
	if batch <= 10 {
		batch = 16
	}
	name := fmt.Sprintf("%s-%d", hostname(), os.Getpid())
	return &Consumer{client: client, group: group, name: name, key: events.StreamKey, batch: batch}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

func (c *Consumer) EnsureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, c.key, c.group, "$").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

type Handler func(ctx context.Context, typ string, p events.Payload) error

func (c *Consumer) Run(ctx context.Context, handle Handler) error {
	if err := c.EnsureGroup(ctx); err != nil {
		return err
	}
	go c.recoverLoop(ctx, handle)
	for {
		if ctx.Err() != nil {
			return nil
		}
		msgs, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Streams:  []string{c.key, ">"},
			Count:    c.batch,
			Block:    2 * time.Second,
			Consumer: c.name,
		}).Result()
		if err != nil {
			if err != redis.Nil {
				logger.Sugar.Errorf("stream %s group %s read err %v", c.key, c.group, err)
			}
			continue
		}
		for _, stream := range msgs {
			acked := make([]string, 0, len(stream.Messages))
			for _, msg := range stream.Messages {
				if c.process(ctx, msg.Values, handle) == nil {
					acked = append(acked, msg.ID)
				} else {
					break // 失败消息不 ack，连同后续未处理消息留在 pending，XAutoClaim 30s 后重试
				}
			}
			if len(acked) > 0 {
				_ = c.client.XAck(ctx, c.key, c.group, acked...)
			}
		}
	}
}

func (c *Consumer) process(ctx context.Context, values map[string]any, handle Handler) error {
	typ, _ := values["type"].(string)
	payload, _ := values["payload"].(string)
	var p events.Payload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		// 坏消息直接视为成功（ack 掉），不阻塞消费组
		logger.Sugar.Errorf("stream %s bad payload: %v", c.key, err)
		return nil
	}
	return handle(ctx, typ, p)
}

func (c *Consumer) recoverLoop(ctx context.Context, handle Handler) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.recoverPending(ctx, handle)
		}
	}
}

func (c *Consumer) recoverPending(ctx context.Context, handle Handler) {
	start := "0"
	for {
		msgs, _, err := c.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   c.key,
			Group:    c.group,
			Consumer: c.name,
			MinIdle:  30 * time.Second,
			Start:    start,
			Count:    32,
		}).Result()
		if err != nil {
			return
		}
		acked := make([]string, 0, len(msgs))
		for _, msg := range msgs {
			if c.process(ctx, msg.Values, handle) == nil {
				acked = append(acked, msg.ID)
			} else {
				break
			}
		}
		if len(acked) > 0 {
			_ = c.client.XAck(ctx, c.key, c.group, acked...)
		}
	}
}

func (s *Stream) PublishBatch(ctx context.Context, evts []PublishEvent) error {
	if len(evts) == 0 {
		return nil
	}
	pipe := s.client.Pipeline()
	for _, e := range evts {
		raw, err := json.Marshal(e.Payload)
		if err != nil {
			return err
		}
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: s.key,
			Values: map[string]any{"type": e.Type, "payload": string(raw)},
		})
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Stream) Len(ctx context.Context) (int64, error) {
	return s.client.XLen(ctx, s.key).Result()
}

type BatchHandler func(ctx context.Context, msgs []BatchMessage) error

func (c *Consumer) RunBatch(ctx context.Context, handle BatchHandler) error {
	if err := c.EnsureGroup(ctx); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		msgs, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Streams:  []string{c.key, ">"},
			Count:    c.batch,
			Block:    2 * time.Second,
			Consumer: c.name,
		}).Result()
		if err != nil {
			if err != redis.Nil {
				logger.Sugar.Errorf("stream %s group %s read err %v", c.key, c.group, err)
			}
			continue
		}
		for _, stream := range msgs {
			batch := make([]BatchMessage, 0, len(stream.Messages))
			ids := make([]string, 0, len(stream.Messages))
			for _, msg := range stream.Messages {
				typ, _ := msg.Values["type"].(string)
				payload, _ := msg.Values["payload"].(string)
				var p events.Payload
				if err := json.Unmarshal([]byte(payload), &p); err != nil {
					logger.Sugar.Errorf("stream bad payload: %v", err)
					ids = append(ids, msg.ID) // 坏消息直接 ack
					continue
				}
				batch = append(batch, BatchMessage{ID: msg.ID, Type: typ, Payload: p})
				ids = append(ids, msg.ID)
			}
			if len(batch) > 0 && handle(ctx, batch) != nil {
				// 整批失败：不 ack，留 pending 重试（幂等去重兜底）
				continue
			}
			if len(ids) > 0 {
				_ = c.client.XAck(ctx, c.key, c.group, ids...)
			}
		}
	}
}

func (s *Stream) Pending(ctx context.Context, group string) (int64, error) {
	info, err := s.client.XPending(ctx, s.key, group).Result()
	if err != nil {
		return 0, err
	}
	return info.Count, nil
}
