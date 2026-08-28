package service

const FeedCandidateLimit = -1

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
	out := make([]scoredFeedItem, 0, len(candidates))
	selected := make([]bool, len(candidates))
	authorCount := map[int64]int{}

	for {
		best := -1
		for i := range candidates {
			if selected[i] {
				continue
			}
			if best == -1 || candidates[i].Score > candidates[best].Score {
				best = i
			}
		}
		if best == -1 {
			break
		}
		item := candidates[best]
		selected[best] = true
		if item.AuthorID != 0 && item.AuthorID != p.SkipLimitAuthorID && authorCount[item.AuthorID] >= p.MaxPerAuthor {
			continue // 作者超上限：丢弃，不再参与后续轮次
		}
		out = append(out, item)
		authorCount[item.AuthorID]++

		for i := range candidates {
			if selected[i] {
				continue
			}
			if candidates[i].AuthorID == item.AuthorID {
				candidates[i].Score *= p.AuthorPenalty
			}
			if topicOverlap(topicsByNote[item.ID], topicsByNote[candidates[i].ID]) {
				candidates[i].Score *= p.TopicPenalty
			}
			if item.Type != 0 && typesByNote[candidates[i].ID] == item.Type {
				candidates[i].Score *= p.TypePenalty
			}
		}
	}
	return out
}

func topicOverlap(a, b []int64) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
