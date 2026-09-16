package websession

import (
	"context"
	"errors"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
)

func TestDevinAgentSupportsRevert(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want bool
	}{
		{
			name: "advertised",
			raw: map[string]any{
				"_meta": map[string]any{"cognition.ai/revert": true},
			},
			want: true,
		},
		{
			name: "explicit false",
			raw: map[string]any{
				"_meta": map[string]any{"cognition.ai/revert": false},
			},
			want: false,
		},
		{
			name: "missing meta key",
			raw: map[string]any{
				"_meta": map[string]any{"cognition.ai/other": true},
			},
			want: false,
		},
		{name: "no meta", raw: map[string]any{"loadSession": true}, want: false},
		{name: "non-bool value", raw: map[string]any{"_meta": map[string]any{"cognition.ai/revert": "yes"}}, want: false},
		{name: "nil", raw: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := devinAgentSupportsRevert(test.raw); got != test.want {
				t.Fatalf("devinAgentSupportsRevert(%#v) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestDevinACPInitializeParamsDeclaresRevert(t *testing.T) {
	manager := &Manager{}
	params := manager.devinACPInitializeParams()
	capabilities, ok := params["clientCapabilities"].(map[string]any)
	if !ok {
		t.Fatal("clientCapabilities missing")
	}
	meta, ok := capabilities["_meta"].(map[string]any)
	if !ok {
		t.Fatal("clientCapabilities._meta missing")
	}
	if meta["cognition.ai/revert"] != true {
		t.Fatalf("clientCapabilities._meta = %#v, want revert opt-in", meta)
	}
	// Terminal must stay undeclared so the agent uses its native tools.
	if _, declared := capabilities["terminal"]; declared {
		t.Fatal("clientCapabilities must not declare terminal")
	}
	clientInfo, ok := params["clientInfo"].(map[string]any)
	if !ok || clientInfo["name"] != "windsurf" {
		t.Fatalf("clientInfo = %#v, want windsurf", params["clientInfo"])
	}
}

func devinPromptStep(stepNumber uint32, summary string, forkNode uint32) devinRevertStep {
	node := forkNode
	return devinRevertStep{
		StepNumber:       stepNumber,
		Kind:             "prompt",
		Summary:          summary,
		ForkTargetNodeID: &node,
	}
}

func TestMatchDevinForkStep(t *testing.T) {
	target := HistoryItem{Kind: "user", Text: "fix the flaky test"}

	tests := []struct {
		name        string
		steps       []devinRevertStep
		userOrdinal int
		wantNode    uint32
		wantErr     error
	}{
		{
			name: "ordinal match",
			steps: []devinRevertStep{
				devinPromptStep(1, "hello", 10),
				devinPromptStep(2, "fix the flaky test", 20),
			},
			userOrdinal: 1,
			wantNode:    20,
		},
		{
			name: "ordinal match with empty summary",
			steps: []devinRevertStep{
				devinPromptStep(1, "hello", 10),
				devinPromptStep(2, "", 20),
			},
			userOrdinal: 1,
			wantNode:    20,
		},
		{
			name: "unique text match after local drift",
			steps: []devinRevertStep{
				devinPromptStep(1, "fix the flaky test", 10),
				devinPromptStep(2, "something else", 20),
			},
			userOrdinal: 3, // local history has extra failed messages
			wantNode:    10,
		},
		{
			name: "ordinal mismatch falls back to unique text match",
			steps: []devinRevertStep{
				devinPromptStep(1, "hello", 10),
				devinPromptStep(2, "different", 20),
				devinPromptStep(3, "fix the flaky test", 30),
			},
			userOrdinal: 1,
			wantNode:    30,
		},
		{
			name: "no prompt steps",
			steps: []devinRevertStep{
				{Kind: "questionAnswer", Summary: "q"},
			},
			userOrdinal: 0,
			wantErr:     ErrDevinForkHistoryConflict,
		},
		{
			name:        "empty steps",
			steps:       nil,
			userOrdinal: 0,
			wantErr:     ErrDevinForkHistoryConflict,
		},
		{
			name: "ordinal out of range and no text match",
			steps: []devinRevertStep{
				devinPromptStep(1, "hello", 10),
			},
			userOrdinal: 5,
			wantErr:     ErrDevinForkHistoryConflict,
		},
		{
			name: "ambiguous text match",
			steps: []devinRevertStep{
				devinPromptStep(1, "fix the flaky test", 10),
				devinPromptStep(2, "fix the flaky test", 20),
			},
			userOrdinal: 4,
			wantErr:     ErrDevinForkHistoryConflict,
		},
		{
			name: "mismatched ordinal with no other match",
			steps: []devinRevertStep{
				devinPromptStep(1, "hello", 10),
				devinPromptStep(2, "unrelated", 20),
			},
			userOrdinal: 1,
			wantErr:     ErrDevinForkHistoryConflict,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step, err := matchDevinForkStep(test.steps, target, test.userOrdinal)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("matchDevinForkStep error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchDevinForkStep error = %v", err)
			}
			if step.ForkTargetNodeID == nil || *step.ForkTargetNodeID != test.wantNode {
				t.Fatalf("matchDevinForkStep forkTargetNodeId = %v, want %d", step.ForkTargetNodeID, test.wantNode)
			}
		})
	}
}

func TestDevinStepStrongMatch(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		text    string
		want    bool
	}{
		{name: "exact", summary: "fix it", text: "fix it", want: true},
		{name: "summary is truncated prefix", summary: "fix the", text: "fix the flaky test", want: true},
		{name: "summary longer than stored text", summary: "fix the flaky test now", text: "fix the flaky test", want: true},
		{name: "whitespace normalized", summary: "fix   the\nflaky", text: "fix the flaky", want: true},
		{name: "mismatch", summary: "fix it", text: "other", want: false},
		{name: "empty summary", summary: "", text: "fix it", want: false},
		{name: "empty text", summary: "fix it", text: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := devinRevertStep{Summary: test.summary}
			if got := devinStepStrongMatch(step, test.text); got != test.want {
				t.Fatalf("devinStepStrongMatch(%q, %q) = %v, want %v", test.summary, test.text, got, test.want)
			}
		})
	}
}

func TestDevinACPDispatchDetachedAndCaptureModes(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "devin-mode-test"}
	sess := *session

	// Detached mode drops session/update entirely.
	detachedProj := newDevinRunProjection()
	manager.dispatchDevinACPMessage(nil, sess, run, detachedProj, devinACPMessage{
		Method: "session/update",
		Params: devinACPUpdatePayload("agent_message_chunk", map[string]any{
			"content": map[string]any{"text": "replayed"},
		}),
	}, devinACPDispatchDetached)
	if events := readTextDeltaTestEvents(t, manager, session.ID); len(events) != 0 {
		t.Fatalf("detached dispatch produced %d events, want 0", len(events))
	}

	// Capture mode projects updates and buffers user_message_chunk.
	captureProj := newDevinRunProjection()
	captureProj.captureHistory = true
	capture := devinACPMessage{
		Method: "session/update",
		Params: devinACPUpdatePayload("user_message_chunk", map[string]any{
			"content": map[string]any{"text": "hello "},
		}),
	}
	manager.dispatchDevinACPMessage(nil, sess, run, captureProj, capture, devinACPDispatchCapture)
	capture.Params = devinACPUpdatePayload("user_message_chunk", map[string]any{
		"content": map[string]any{"text": "devin"},
	})
	manager.dispatchDevinACPMessage(nil, sess, run, captureProj, capture, devinACPDispatchCapture)
	if got := captureProj.takeDevinCapturedUserText(); got != "hello devin" {
		t.Fatalf("captured user text = %q, want %q", got, "hello devin")
	}
	// A live (non-capture) projection must ignore user_message_chunk.
	liveProj := newDevinRunProjection()
	manager.dispatchDevinACPMessage(nil, sess, run, liveProj, capture, devinACPDispatchLive)
	if got := liveProj.takeDevinCapturedUserText(); got != "" {
		t.Fatalf("live projection buffered user text %q", got)
	}
}

func TestEmitDevinCapturedUserMessage(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "capture-run"}
	manager.emitDevinCapturedUserMessage(*session, run, "fork me here")

	events := readTextDeltaTestEvents(t, manager, session.ID)
	if len(events) != 1 || events[0].Type != "msg_u" {
		t.Fatalf("events = %#v, want a single msg_u", events)
	}
	if got := stringValue(events[0].Payload["txt"]); got != "fork me here" {
		t.Fatalf("msg_u text = %q", got)
	}
	if events[0].RunID != "capture-run" {
		t.Fatalf("msg_u runId = %q", events[0].RunID)
	}
}

func seedDevinHistoryItem(t *testing.T, sessionID string, kind string, orderIndex int64, text string) *tables.WebSessionItemTable {
	t.Helper()
	item := &tables.WebSessionItemTable{
		WebSessionID: sessionID,
		OrderIndex:   orderIndex,
		ItemKind:     kind,
		ItemType:     kind + "_message",
		Text:         text,
	}
	item.Init()
	if err := model.GetDB().Create(item).Error; err != nil {
		t.Fatalf("seed history item failed: %v", err)
	}
	return item
}

func TestDevinForkUserOrdinal(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	seedDevinHistoryItem(t, session.ID, "user", 1, "first")
	seedDevinHistoryItem(t, session.ID, "assistant", 2, "reply")
	targetRow := seedDevinHistoryItem(t, session.ID, "user", 3, "second")
	seedDevinHistoryItem(t, session.ID, "user", 4, "third")

	target, err := manager.findHistoryItemByID(context.Background(), session.ID, targetRow.ID)
	if err != nil {
		t.Fatalf("findHistoryItemByID: %v", err)
	}
	ordinal, err := manager.devinForkUserOrdinal(context.Background(), session.ID, target)
	if err != nil {
		t.Fatalf("devinForkUserOrdinal: %v", err)
	}
	if ordinal != 1 {
		t.Fatalf("devinForkUserOrdinal = %d, want 1", ordinal)
	}
}

func TestForkDevinSessionValidation(t *testing.T) {
	t.Run("non-devin agent rejected", func(t *testing.T) {
		manager, session := newTextDeltaTestManager(t) // codex session
		_, err := manager.ForkDevinSession(context.Background(), session.ID, "item-1")
		if !errors.Is(err, ErrDevinForkUnsupported) {
			t.Fatalf("ForkDevinSession error = %v, want %v", err, ErrDevinForkUnsupported)
		}
	})

	t.Run("missing native session rejected", func(t *testing.T) {
		cleanup := initTestDB(t)
		t.Cleanup(cleanup)
		project := seedProject(t)
		session := seedWebSessionWithAgent(t, project.ID, "Devin", 1, AgentDevin)
		manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		_, err = manager.ForkDevinSession(context.Background(), session.ID, "item-1")
		if !errors.Is(err, ErrDevinForkUnsupported) {
			t.Fatalf("ForkDevinSession error = %v, want %v", err, ErrDevinForkUnsupported)
		}
	})

	t.Run("unknown item rejected", func(t *testing.T) {
		cleanup := initTestDB(t)
		t.Cleanup(cleanup)
		project := seedProject(t)
		session := seedWebSessionWithAgent(t, project.ID, "Devin", 1, AgentDevin)
		nativeID := "devin-native-1"
		if err := model.GetDB().Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(map[string]any{
				"native_session_id": nativeID,
				"backend":           string(SessionBackendDevinACP),
			}).Error; err != nil {
			t.Fatalf("set native_session_id: %v", err)
		}
		manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		_, err = manager.ForkDevinSession(context.Background(), session.ID, "missing-item")
		if !errors.Is(err, ErrDevinForkTargetNotFound) {
			t.Fatalf("ForkDevinSession error = %v, want %v", err, ErrDevinForkTargetNotFound)
		}
	})

	t.Run("non-user item rejected", func(t *testing.T) {
		cleanup := initTestDB(t)
		t.Cleanup(cleanup)
		project := seedProject(t)
		session := seedWebSessionWithAgent(t, project.ID, "Devin", 1, AgentDevin)
		nativeID := "devin-native-2"
		if err := model.GetDB().Model(&tables.WebSessionTable{}).
			Where("id = ?", session.ID).
			Updates(map[string]any{
				"native_session_id": nativeID,
				"backend":           string(SessionBackendDevinACP),
			}).Error; err != nil {
			t.Fatalf("set native_session_id: %v", err)
		}
		assistant := seedDevinHistoryItem(t, session.ID, "assistant", 1, "reply")
		manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		_, err = manager.ForkDevinSession(context.Background(), session.ID, assistant.ID)
		if !errors.Is(err, ErrDevinForkTargetNotFound) {
			t.Fatalf("ForkDevinSession error = %v, want %v", err, ErrDevinForkTargetNotFound)
		}
	})
}
