package websession

import (
	"testing"
	"time"

	"code-kanban/model/tables"
)

func TestPiHistoryRowsEqualIgnoresSourceIdentityAdoption(t *testing.T) {
	timestamp := time.UnixMilli(1_000)
	left := tables.WebSessionItemTable{
		OrderIndex:   1,
		LastEventSeq: 7,
		ItemKind:     "assistant",
		ItemType:     "agent_message",
		Text:         "same timeline content",
		Done:         true,
		Timestamp:    &timestamp,
		ObservedAt:   &timestamp,
	}
	right := left
	right.WebTurnID = nilIfEmptyHistory("turn-row")
	right.SourceThreadID = nilIfEmptyHistory("native-session")
	right.SourceTurnID = nilIfEmptyHistory("native-turn")
	right.SourceItemID = nilIfEmptyHistory("native-item")
	right.Role = "assistant"
	right.Status = "completed"

	if !piHistoryRowsEqual(left, right) {
		t.Fatal("source identity adoption should not invalidate an unchanged timeline")
	}
	right.Text = "changed timeline content"
	if piHistoryRowsEqual(left, right) {
		t.Fatal("timeline content changes must advance the history epoch")
	}
}

func TestPiFailureItemTextSurfacesUnderlyingError(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		errorMsg string
		want     string
	}{
		{
			name:     "empty text keeps fallback prefix with cause",
			text:     "",
			errorMsg: "Selected Command Code model does not support image content in user messages",
			want:     "Pi assistant run failed: Selected Command Code model does not support image content in user messages",
		},
		{
			name:     "existing text gains the cause",
			text:     "partial answer",
			errorMsg: "upstream quota exhausted",
			want:     "partial answer\nupstream quota exhausted",
		},
		{
			name:     "duplicated cause is not appended twice",
			text:     "Pi assistant run failed: upstream quota exhausted",
			errorMsg: "upstream quota exhausted",
			want:     "Pi assistant run failed: upstream quota exhausted",
		},
		{
			name:     "no error message keeps legacy fallback",
			text:     "",
			errorMsg: "   ",
			want:     "Pi assistant run failed",
		},
		{
			name:     "no error message keeps existing text",
			text:     "aborted mid stream",
			errorMsg: "",
			want:     "aborted mid stream",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := piFailureItemText(tc.text, tc.errorMsg); got != tc.want {
				t.Fatalf("piFailureItemText(%q, %q) = %q, want %q", tc.text, tc.errorMsg, got, tc.want)
			}
		})
	}
}
