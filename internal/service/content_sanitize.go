package service

import (
	"regexp"
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

// 清洗后的 HTML 里 img 属性是双引号格式，直接匹配 src="..."
var imgSrcRe = regexp.MustCompile(`<img[^>]*\ssrc="([^"]+)"`)

func SanitizeRichContent(content string) string {
	return richTextPolicy.Sanitize(content)
}

// ExtractFirstImage 从清洗后的富文本中提取第一张图片 URL，取不到返回空串。
// 用于"没单独上传图片时，自动把正文第一张图作为封面"。
func ExtractFirstImage(html string) string {
	m := imgSrcRe.FindStringSubmatch(html)
	if len(m) == 2 {
		return m[1]
	}
	return ""
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
