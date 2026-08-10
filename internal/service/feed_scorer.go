package service

import (
	"XFeedSystem/internal/model"
	"math"
	"sort"
	"time"
)

const (
	PoolSize = 200 // 候选池大小
	// 权重常量，方便调参
	WeightLike     = 3.0
	WeightFavorite = 5.0
	WeightComment  = 4.0
	FollowBoost    = 1.5
	TypePrefBoost  = 1.0 // 类型偏好基础倍率，实际会叠加
)

// scoredNote 打分后的笔记
type scoredNote struct {
	Note  *model.Note
	Score float64
}

func computeScore(note *model.Note, now time.Time, followingSet map[int64]bool, typePref map[int8]float64) float64 {
	interaction := float64(note.LikeCount)*WeightLike + float64(note.FavoriteCount)*WeightFavorite + float64(note.CommentCount)*WeightComment

	hours := now.Sub(note.PublishedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	decay := 1.0 / (1.0 + math.Sqrt(hours))

	score := interaction * decay

	if followingSet != nil && followingSet[note.AuthorID] {
		score *= FollowBoost
	}

	if typePref != nil {
		if boost, ok := typePref[note.Type]; ok {
			score *= (1.0 + boost*TypePrefBoost)
		}
	}

	return score
}

func scoreAndSort(notes []*model.Note, now time.Time, followingSet map[int64]bool, typePref map[int8]float64) []scoredNote {
	result := make([]scoredNote, 0, len(notes))
	for _, n := range notes {
		result = append(result, scoredNote{
			Note:  n,
			Score: computeScore(n, now, followingSet, typePref),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].Note.ID > result[j].Note.ID
	})
	return result
}
