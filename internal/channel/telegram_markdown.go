package channel

import (
	"regexp"
	"strings"
)

var (
	reHeading       = regexp.MustCompile(`(?m)^#{1,6}\s+([^\n]+)`)
	reBlockquote    = regexp.MustCompile(`(?m)^>\s*(.*)$`)
	reLink          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reBoldStar      = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reBoldUnder     = regexp.MustCompile(`__(.+?)__`)
	reItalicStar    = regexp.MustCompile(`(?s)\*(.+?)\*`)
	reItalicUnder   = regexp.MustCompile(`(?s)_(.+?)_`)
	reStrike        = regexp.MustCompile(`~~(.+?)~~`)
	reListItem      = regexp.MustCompile(`(?m)^[-*]\s+`)
	reCodeBlock     = regexp.MustCompile("(?s)```(?:[\\w]*)\n?([\\s\\S]*?)```")
	reInlineCode    = regexp.MustCompile("`([^`]+?)`")
	rePreCodeTag    = regexp.MustCompile(`(?s)<pre>(.+?)</pre>`)
	reAlreadyHtml   = regexp.MustCompile(`</?[a-z][a-z0-9]*`)
)

func markdownToTelegramHTML(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	typeCodePlaceholders := make(map[string]string)
	text = reCodeBlock.ReplaceAllStringFunc(text, func(m string) string {
		codeContent := reCodeBlock.FindStringSubmatch(m)[1]
		placeholder := "<<CODE_BLOCK_" + randString(8) + ">>"
		typeCodePlaceholders[placeholder] = codeContent
		return placeholder
	})

	inlineCodePlaceholders := make(map[string]string)
	text = reInlineCode.ReplaceAllStringFunc(text, func(m string) string {
		codeContent := reInlineCode.FindStringSubmatch(m)[1]
		placeholder := "<<INLINE_CODE_" + randString(8) + ">>"
		inlineCodePlaceholders[placeholder] = codeContent
		return placeholder
	})

	text = escapeHTML(text)

	text = reHeading.ReplaceAllString(text, `<b>$1</b>`)
	text = reLink.ReplaceAllString(text, `<a href="$2">$1</a>`)
	text = reBoldStar.ReplaceAllString(text, `<b>$1</b>`)
	text = reBoldUnder.ReplaceAllString(text, `<b>$1</b>`)
	text = reStrike.ReplaceAllString(text, `<s>$1</s>`)

	text = reBlockquote.ReplaceAllStringFunc(text, func(m string) string {
		content := reBlockquote.FindStringSubmatch(m)[1]
		return "<i>" + content + "</i>"
	})

	text = reListItem.ReplaceAllString(text, "• ")

	text = reItalicStar.ReplaceAllStringFunc(text, func(m string) string {
		content := reItalicStar.FindStringSubmatch(m)[1]
		if reAlreadyHtml.MatchString(content) {
			return m
		}
		return "<i>" + content + "</i>"
	})

	text = reItalicUnder.ReplaceAllStringFunc(text, func(m string) string {
		content := reItalicUnder.FindStringSubmatch(m)[1]
		if reAlreadyHtml.MatchString(content) {
			return m
		}
		return "<i>" + content + "</i>"
	})

	for placeholder, codeContent := range inlineCodePlaceholders {
		text = strings.Replace(text, placeholder, "<code>"+escapeHTML(codeContent)+"</code>", 1)
	}

	for placeholder, codeContent := range typeCodePlaceholders {
		text = strings.Replace(text, placeholder, "<pre>"+escapeHTML(codeContent)+"</pre>", 1)
	}

	return text
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[i%len(letters)]
	}
	return string(b)
}
