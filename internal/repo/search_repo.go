package repo

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/meilisearch/meilisearch-go"
)

type NoteDocument struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	AuthorID    int64  `json:"author_id"`
	Type        int8   `json:"type"`
	PublishedAt int64  `json:"published_at"`
}

type SearchRepo struct {
	client meilisearch.ServiceManager
	index  string
}

func NewSearchRepo(host, apiKey, index string) *SearchRepo {
	client := meilisearch.New(host, meilisearch.WithAPIKey(apiKey))
	return &SearchRepo{client: client, index: index}
}

func (r *SearchRepo) EnsureIndex(ctx context.Context) error {
	idx := r.client.Index(r.index)
	_, err := r.client.CreateIndex(&meilisearch.IndexConfig{
		Uid:        r.index,
		PrimaryKey: "id",
	})
	if err != nil {
		// 索引已存在时忽略错误
		_ = err
	}

	_, err = idx.UpdateSettings(&meilisearch.Settings{
		SearchableAttributes: []string{"title", "content"},
		SortableAttributes:   []string{"published_at"},
		FilterableAttributes: []string{"author_id", "type"},
	})
	return err
}

func (r *SearchRepo) Index(ctx context.Context, doc *NoteDocument) error {
	task, err := r.client.Index(r.index).AddDocuments(doc, nil)
	if err != nil {
		return err
	}
	_ = task
	return nil
}

func (r *SearchRepo) Delete(ctx context.Context, id int64) error {
	task, err := r.client.Index(r.index).DeleteDocument(strconv.FormatInt(id, 10), nil)
	if err != nil {
		return err
	}
	_ = task
	return nil
}

type SearchResult struct {
	IDs   []int64
	Total int64
}

func (r *SearchRepo) Search(ctx context.Context, keyword string, offset, limit int) (*SearchResult, error) {
	res, err := r.client.Index(r.index).Search(keyword, &meilisearch.SearchRequest{
		Offset:               int64(offset),
		Limit:                int64(limit),
		AttributesToRetrieve: []string{"id"},
	})
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(res.Hits))
	for _, hit := range res.Hits {
		raw, ok := hit["id"]
		if !ok {
			continue
		}
		var id int64
		if err := json.Unmarshal(raw, &id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return &SearchResult{
		IDs:   ids,
		Total: res.EstimatedTotalHits,
	}, nil
}
