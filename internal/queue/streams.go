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

type Consumer struct {
	client *redis.Client
	key    string
	group  string
	name   string
	batch  int64
}

func NewConsumer(client *redis.Client, group string) *Consumer {
	name := fmt.Sprintf("%s-%d", hostname(), os.Getpid())
	return &Consumer{client: client, group: group, name: name, key: events.StreamKey, batch: 16}
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
			for _, msg := range stream.Messages {
				if c.process(ctx, msg.Values, handle) == nil {
					c.client.XAck(ctx, c.key, c.group, msg.ID)
				}
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
		for _, msg := range msgs {
			if c.process(ctx, msg.Values, handle) == nil {
				c.client.XAck(ctx, c.key, c.group, msg.ID)
			}
		}
		if len(msgs) < 32 {
			return
		}
		start = msgs[len(msgs)-1].ID
	}
}
