package service

import (
	"strings"
	"unicode/utf8"

	"XFeedSystem/internal/pkg/cursor"

	"github.com/microcosm-cc/bluemonday"
)

const (
	// 纯文本
	ContentFormatPlain = 1
	// 富文本（HTML，白名单清洗后入库）
	ContentFormatRich = 2
	// 富文本正文最大字符数，防止单条笔记拖垮 DB/索引/渲染
	MaxRichContentLen = 50000
)

var richTextPolicy = bluemonday.UGCPolicy()

func SanitizeRichContent(content string) string {
	return richTextPolicy.Sanitize(content)
}

func NormalizeContent(content string, format int) (string, int8) {
	if format == ContentFormatRich {
		return SanitizeRichContent(content), ContentFormatRich
	}
	return content, ContentFormatPlain
}

func ValidateNoteContent(title, content string, format int8) error {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(cursor.StripHTML(content)) == "" {
		return ErrEmptyNoteContent
	}
	if format == ContentFormatRich && utf8.RuneCountInString(content) > MaxRichContentLen {
		return ErrContentTooLong
	}
	return nil
}