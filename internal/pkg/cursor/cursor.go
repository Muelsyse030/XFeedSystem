package cursor

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"XFeedSystem/internal/model"
)

func EncodeFeedCursor(t time.Time, id int64) string {
	return fmt.Sprintf("%d_%d", t.Unix(), id)
}

func ParseFeedCursor(s string) (*model.FeedCursor, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, "_")
	if len(parts) != 2 {
		return nil, errors.New("invalid cursor")
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &model.FeedCursor{
		PublishedAt: time.Unix(ts, 0),
		ID:          id,
	}, nil
}

func BuildSummary(content string, max int) string {
	content = strings.TrimSpace(content)
	content = strings.ReplaceAll(content, "\n", " ")

	runes := []rune(content)
	if len(runes) <= max {
		return content
	}
	return string(runes[:max]) + "..."
}
