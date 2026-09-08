package websession

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxAutoTitleRunes = 64

func deriveAutoTitleFromMessage(text string) string {
	line := firstNonEmptyLine(text)
	if line == "" {
		return ""
	}

	title := firstSentence(line)
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return ""
	}

	return truncateRunes(title, maxAutoTitleRunes)
}

func firstNonEmptyLine(value string) string {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func firstSentence(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	for index, r := range value {
		// Keep periods inside versions, filenames, and other tokens intact.
		if r == '.' && index+1 < len(value) {
			next, _ := utf8.DecodeRuneInString(value[index+1:])
			if !unicode.IsSpace(next) {
				continue
			}
		}
		if isSentenceBoundary(r) {
			return strings.TrimSpace(value[:index+utf8.RuneLen(r)])
		}
	}

	return value
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '。', '！', '？', '；':
		return true
	default:
		return false
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	const suffix = "..."
	suffixRunes := []rune(suffix)
	if limit <= len(suffixRunes) {
		return string(runes[:limit])
	}

	return string(runes[:limit-len(suffixRunes)]) + suffix
}
