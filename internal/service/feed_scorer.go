package service

import (
	"XFeedSystem/internal/model"
	"math"
	"time"
)

const (
	// 权重常量，方便调参
	WeightLike     = 3.0
	WeightFavorite = 5.0
	WeightComment  = 4.0
	FollowBoost    = 1.5
	TypePrefBoost  = 1.0 // 类型偏好基础倍率，实际会叠加
	DecayFreezeHours = 24.0 //24小时冻结衰减
)

// scoredNote 打分后的笔记
type scoredNote struct {
	Note  *model.Note
	Score float64
}

func decayFactor(hours float64) float64 {
	if hours < 0 {
		hours = 0
	}
	if hours > DecayFreezeHours {
		hours = DecayFreezeHours
	}
	return 1.0 / (1.0 + math.Sqrt(hours))
}

func baseScore(note *model.Note , now time.Time , stats *model.NoteStats) float64 {
	interaction := float64(note.LikeCount) * WeightLike +
	float64(note.FavoriteCount) * WeightFavorite +
	float64(note.CommentCount) * WeightComment

	score := interaction * decayFactor(now.Sub(note.PublishedAt).Hours())

	if stats != nil {
		score *= ctrBoost(stats.Reads,stats.Impressions)
	}
	return score
}
func personalizedScore(base float64 , authorID int64 , noteType int8 ,followingSet map[int64]bool , typePref map[int8]float64) float64 {
	s := base
	if followingSet != nil && followingSet[authorID] {
		s *= FollowBoost
	}
	if typePref != nil {
		if boost, ok := typePref[noteType]; ok {
			s *= (1.0 + boost * TypePrefBoost)
		}
	}
	return s
}

func ctrBoost(reads, impressions int64) float64 {
	const alpha, beta, weight = 10.0, 100.0, 1.0
	if impressions <= 0 {
		return 1.0
	}
	ctr := (float64(reads) + alpha) / (float64(impressions) + beta)
	return 1.0 + weight*ctr
}