package service

import "sort"

const FeedCandidateLimit = 1000

// DiversityParams 控制多样性强度，全部可调。
type DiversityParams struct {
	MaxPerAuthor      int     // 同一作者最多出现几条
	AuthorPenalty     float64 // 已选作者的其余候选，每被撞一次乘一次衰减
	TopicPenalty      float64 // 与已选内容同话题的候选衰减
	TypePenalty       float64 // 与已选内容同类型的候选衰减
	SkipLimitAuthorID int64   // 该作者不参与上限限制（自己的笔记不限量）
}

func DefaultDiversityParams() DiversityParams {
	return DiversityParams{
		MaxPerAuthor:  2,
		AuthorPenalty: 0.35,
		TopicPenalty:  0.7,
		TypePenalty:   0.85,
	}
}

func diverseRank(candidates []scoredFeedItem, topicsByNote map[int64][]int64, typesByNote map[int64]int8, p DiversityParams) []scoredFeedItem {
	if len(candidates) <= 1 {
		return candidates
	}

	// 1. 统计池内出现次数（各一遍 O(N)，不再每选一条就全量扫）
	authorCount := make(map[int64]int, len(candidates))
	topicCount := make(map[int64]int)
	typeCount := make(map[int8]int)
	for i := range candidates {
		it := &candidates[i]
		if it.AuthorID != 0 {
			authorCount[it.AuthorID]++
		}
		for _, tid := range topicsByNote[it.ID] {
			topicCount[tid]++
		}
		if it.Type != 0 {
			typeCount[it.Type]++
		}
	}

	// 2. 预计算作者惩罚幂次，避免循环里反复 math.Pow
	authorPow := make([]float64, p.MaxPerAuthor+1)
	for i := 0; i <= p.MaxPerAuthor; i++ {
		authorPow[i] = powf(p.AuthorPenalty, i)
	}

	// 3. 每个候选一次性算好最终分
	for i := range candidates {
		it := &candidates[i]
		penalty := 1.0
		if it.AuthorID != 0 && it.AuthorID != p.SkipLimitAuthorID {
			c := authorCount[it.AuthorID]
			if c > p.MaxPerAuthor {
				c = p.MaxPerAuthor
			}
			penalty *= authorPow[c]
		}
		for _, tid := range topicsByNote[it.ID] {
			penalty *= powf(p.TopicPenalty, topicCount[tid])
		}
		if it.Type != 0 {
			penalty *= powf(p.TypePenalty, typeCount[it.Type])
		}
		it.Score *= penalty
	}

	// 4. 一次排序（分数降序，同分 id 降序，与调用方原 sort 规则一致）
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].ID > candidates[j].ID
	})

	// 5. 输出时应用作者上限（自己的笔记不限量）
	out := make([]scoredFeedItem, 0, len(candidates))
	selected := make(map[int64]int, len(candidates))
	for _, it := range candidates {
		if it.AuthorID != 0 && it.AuthorID != p.SkipLimitAuthorID && selected[it.AuthorID] >= p.MaxPerAuthor {
			continue
		}
		out = append(out, it)
		selected[it.AuthorID]++
	}
	return out
}

func powf(base float64, exp int) float64 {
	r := 1.0
	for i := 0; i < exp; i++ {
		r *= base
	}
	return r
}
