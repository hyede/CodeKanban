package websession

import (
	"strings"
	"testing"
)

func TestDeriveAutoTitleFromMessage(t *testing.T) {
	for _, tt := range []struct {
		name string
		text string
		want string
	}{
		{"release version", "发布 0.47 吧", "发布 0.47 吧"},
		{"semantic version", "发布 v0.47.1 吧。然后打标签。", "发布 v0.47.1 吧。"},
		{"decimal", "将比例调整为 0.47。保持其他配置。", "将比例调整为 0.47。"},
		{"filename", "修复 config.yaml 的配置", "修复 config.yaml 的配置"},
		{"English sentence", "Release 0.47. Then tag it.", "Release 0.47."},
		{"Unicode whitespace", "Release 0.47.\u3000Then tag it.", "Release 0.47."},
		{"terminal period", "Release 0.47.", "Release 0.47."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveAutoTitleFromMessage(tt.text); got != tt.want {
				t.Fatalf("deriveAutoTitleFromMessage(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}

	t.Run("uses first sentence from first non-empty line", func(t *testing.T) {
		title := deriveAutoTitleFromMessage("\n\n修复登录接口超时问题。顺便补一下测试。")
		if title != "修复登录接口超时问题。" {
			t.Fatalf("expected first sentence, got %q", title)
		}
	})

	t.Run("falls back to first line when no sentence boundary", func(t *testing.T) {
		title := deriveAutoTitleFromMessage("Refactor websocket session syncing without changing API")
		if title != "Refactor websocket session syncing without changing API" {
			t.Fatalf("unexpected title %q", title)
		}
	})

	t.Run("collapses whitespace and truncates long titles", func(t *testing.T) {
		title := deriveAutoTitleFromMessage("  this   title    should   be   compacted  " + strings.Repeat("x", 80))
		if !strings.HasPrefix(title, "this title should be compacted ") {
			t.Fatalf("expected compacted whitespace, got %q", title)
		}
		if len([]rune(title)) != maxAutoTitleRunes {
			t.Fatalf("expected %d runes, got %d", maxAutoTitleRunes, len([]rune(title)))
		}
		if !strings.HasSuffix(title, "...") {
			t.Fatalf("expected truncated suffix, got %q", title)
		}
	})
}
