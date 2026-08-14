package service

import (
	"XFeedSystem/internal/model"
	"math"
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

// func computeScore(note *model.Note, now time.Time, followingSet map[int64]bool, typePref map[int8]float64) float64 {
// 	interaction := float64(note.LikeCount)*WeightLike + float64(note.FavoriteCount)*WeightFavorite + float64(note.CommentCount)*WeightComment

// 	hours := now.Sub(note.PublishedAt).Hours()
// 	if hours < 0 {
// 		hours = 0
// 	}
// 	decay := 1.0 / (1.0 + math.Sqrt(hours))

// 	score := interaction * decay

// 	if followingSet != nil && followingSet[note.AuthorID] {
// 		score *= FollowBoost
// 	}

// 	if typePref != nil {
// 		if boost, ok := typePref[note.Type]; ok {
// 			score *= (1.0 + boost*TypePrefBoost)
// 		}
// 	}

// 	return score
// }



func computeScore(note *model.Note , now time.Time , followingSet map[int64]bool , typePref map[int8]float64 , stats map[int64]*model.NoteStats) float64 {
	interaction := float64(note.LikeCount) * WeightLike + float64(note.FavoriteCount) * WeightFavorite + float64(note.CommentCount)*WeightComment
	hours := now.Sub(note.PublishedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	decay := 1.0/(1.0 + math.Sqrt(hours))

	score := interaction * decay

	if st := stats[note.ID]; st != nil {
		score *= ctrBoost(st.Reads, st.Impressions)
	}

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

func ctrBoost(reads, impressions int64) float64 {
	const alpha,beta,weight = 10.0 , 100.0 , 1.0
	if impressions <= 0 {
		return 1.0
	}
	ctr := (float64(reads) + alpha) / (float64(impressions) + beta)
	return 1.0 + weight * ctr
}