package websession

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDevinACPHelpers(t *testing.T) {
	if got := devinACPIDKey(json.RawMessage(` 12 `)); got != "12" {
		t.Fatalf("devinACPIDKey() = %q", got)
	}
	root := t.TempDir()
	if !pathWithin(root, filepath.Join(root, "src")) {
		t.Fatal("pathWithin should allow descendants")
	}
	if pathWithin(root, root+"-other") {
		t.Fatal("pathWithin should reject sibling prefixes")
	}
	if got := rawStringSlice([]any{"git", "status"}); !reflect.DeepEqual(got, []string{"git", "status"}) {
		t.Fatalf("rawStringSlice() = %#v", got)
	}
}

func TestDefaultDevinSessionSettings(t *testing.T) {
	if got := defaultModel(AgentDevin, ""); got != "swe-2-high" {
		t.Fatalf("defaultModel(devin) = %q", got)
	}
	if got := defaultTitle(AgentDevin, "demo"); got != "Devin · demo" {
		t.Fatalf("defaultTitle(devin) = %q", got)
	}
	if got := defaultSessionBackend(AgentDevin); got != SessionBackendDevinACP {
		t.Fatalf("defaultSessionBackend(devin) = %q", got)
	}
}

func TestParseDevinModelCatalog(t *testing.T) {
	models, err := parseDevinModelCatalog([]byte(`{"families":[{"family_label":"SWE-2","variants":[{"model_uid":"swe-2-high","label":"SWE-2 High","max_context_tokens":262000,"max_output_tokens":128000,"cost_tier":"Free","is_new":true,"supports_images":true},{"model_uid":"swe-2-medium","label":"SWE-2 Medium","max_context_tokens":262000,"max_output_tokens":128000,"cost_tier":"Free","is_beta":true,"model_info":{"supports_images":true}},{"model_uid":"swe-2-low","label":"SWE-2 Low","cost_tier":"promotion"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 || models[0].Family != "SWE-2" || models[0].DefaultReasoningEffort != ReasoningEffortHigh || models[1].DefaultReasoningEffort != ReasoningEffortMedium {
		t.Fatalf("unexpected Devin model catalog: %#v", models)
	}
	if !models[0].SupportsImages || !models[1].SupportsImages || models[2].SupportsImages {
		t.Fatalf("unexpected supports_images parsing: %#v", models)
	}
	if !models[0].supportsImagesSet || !models[1].supportsImagesSet || models[2].supportsImagesSet {
		t.Fatalf("unexpected supports_images presence tracking: %#v", models)
	}
	if !models[0].IsNew || models[0].IsBeta || models[0].IsPromo {
		t.Fatalf("unexpected badge flags for swe-2-high: %#v", models[0])
	}
	if models[1].IsNew || !models[1].IsBeta || models[1].IsPromo {
		t.Fatalf("unexpected badge flags for swe-2-medium: %#v", models[1])
	}
	if models[2].IsNew || models[2].IsBeta || !models[2].IsPromo {
		t.Fatalf("unexpected badge flags for swe-2-low: %#v", models[2])
	}
}

func TestDevinReasoningEffortFromModel(t *testing.T) {
	tests := map[string]ReasoningEffort{
		"gpt-5-6-luna-low-priority": ReasoningEffortLow,
		"gpt-5-6-sol-high-fast":     ReasoningEffortHigh,
		"swe-2-max":                 ReasoningEffortMax,
		"adaptive":                  ReasoningEffortDefault,
	}
	for model, want := range tests {
		if got := devinReasoningEffortFromModel(model); got != want {
			t.Fatalf("devinReasoningEffortFromModel(%q) = %q, want %q", model, got, want)
		}
	}
}

func devinACPUpdatePayload(sessionUpdate string, extra map[string]any) json.RawMessage {
	update := map[string]any{"sessionUpdate": sessionUpdate}
	for key, value := range extra {
		update[key] = value
	}
	payload := map[string]any{"update": update}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestDevinToolHistoryKind(t *testing.T) {
	tests := map[string]string{
		"execute": "command_execution",
		"edit":    "file_change",
		"delete":  "file_change",
		"move":    "file_change",
		"fetch":   "web_search",
		"read":    "dynamic_tool_call",
		"think":   "dynamic_tool_call",
		"":        "dynamic_tool_call",
	}
	for acpKind, want := range tests {
		if got := devinToolHistoryKind(acpKind); got != want {
			t.Fatalf("devinToolHistoryKind(%q) = %q, want %q", acpKind, got, want)
		}
	}
}

func TestDevinACPToolOutputText(t *testing.T) {
	tests := []struct {
		name   string
		update map[string]any
		want   string
	}{
		{
			name:   "string rawOutput",
			update: map[string]any{"rawOutput": "hello world"},
			want:   "hello world",
		},
		{
			name:   "map rawOutput is JSON-encoded",
			update: map[string]any{"rawOutput": map[string]any{"lines": 3}},
			want:   `{"lines":3}`,
		},
		{
			name: "nil rawOutput falls back to content text",
			update: map[string]any{
				"content": []any{
					map[string]any{
						"type":    "content",
						"content": map[string]any{"type": "text", "text": "hi"},
					},
					map[string]any{"type": "text", "text": "ignored"},
				},
			},
			want: "hi",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := devinToolOutputText(test.update); got != test.want {
				t.Fatalf("devinToolOutputText(%#v) = %q, want %q", test.update, got, test.want)
			}
		})
	}
}

func TestDevinACPProjectionSeparatesThinkingToolsAndText(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "devin-run"}
	proj := newDevinRunProjection()
	sess := *session

	feed := func(extra map[string]any) {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload(extra["sessionUpdate"].(string), extra))
	}

	feed(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"text": "I should "}})
	feed(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"text": "look."}})
	feed(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"text": "Reading file"}})
	feed(map[string]any{
		"sessionUpdate": "tool_call",
		"toolCallId":    "call-1",
		"title":         "Read file",
		"kind":          "read",
		"rawInput":      map[string]any{"path": "a.go"},
	})
	feed(map[string]any{
		"sessionUpdate": "tool_call_update",
		"toolCallId":    "call-1",
		"status":        "completed",
		"rawOutput":     map[string]any{"lines": 3},
	})
	feed(map[string]any{"sessionUpdate": "agent_thought_chunk", "content": map[string]any{"text": "Done thinking"}})
	feed(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"text": "All good."}})
	manager.finishDevinRun(sess, run, proj)

	events := readTextDeltaTestEvents(t, manager, session.ID)

	// Two assistant messages, each with its own mid; every txt_d mid matches one.
	messageIDs := make(map[string]bool)
	for _, event := range events {
		if event.Type == "msg_a_st" {
			mid := stringValue(event.Payload["mid"])
			if mid == "" || messageIDs[mid] {
				t.Fatalf("unexpected/duplicate msg_a_st mid %q", mid)
			}
			messageIDs[mid] = true
		}
	}
	if len(messageIDs) != 2 {
		t.Fatalf("expected 2 msg_a_st events, got %d (%#v)", len(messageIDs), messageIDs)
	}
	var firstMessage, secondMessage string
	for _, event := range events {
		if event.Type != "txt_d" {
			continue
		}
		mid := stringValue(event.Payload["mid"])
		if !messageIDs[mid] {
			t.Fatalf("txt_d mid %q is not an assistant message id", mid)
		}
		if firstMessage == "" {
			firstMessage = mid
		} else if mid != firstMessage && secondMessage == "" {
			secondMessage = mid
		} else if mid != firstMessage && mid != secondMessage {
			t.Fatalf("txt_d mid %q does not match either message id", mid)
		}
	}
	if count := countEventsByType(events, "txt_end"); count != 2 {
		t.Fatalf("expected 2 txt_end events, got %d", count)
	}

	// Reconstruct each message's text from its txt_d events.
	messageText := func(mid string) string {
		var b strings.Builder
		for _, event := range events {
			if event.Type == "txt_d" && stringValue(event.Payload["mid"]) == mid {
				b.WriteString(stringValue(event.Payload["txt"]))
			}
		}
		return b.String()
	}
	// Order messages by their first txt_d occurrence.
	firstTxtDSeen := false
	for _, event := range events {
		if event.Type != "txt_d" {
			continue
		}
		mid := stringValue(event.Payload["mid"])
		if !firstTxtDSeen {
			firstMessage = mid
			firstTxtDSeen = true
		} else if mid != firstMessage {
			secondMessage = mid
			break
		}
	}
	if got := messageText(firstMessage); got != "Reading file" {
		t.Fatalf("first message text = %q, want %q", got, "Reading file")
	}
	if got := messageText(secondMessage); got != "All good." {
		t.Fatalf("second message text = %q, want %q", got, "All good.")
	}

	// Reasoning: exactly 2 tool_end reasoning events, distinct tids, non-empty out, ok, ParentID is a message id.
	reasoningEnds := 0
	reasoningTids := make(map[string]bool)
	var reasoningOuts []string
	for _, event := range events {
		if event.Type != "tool_st" && event.Type != "tool_end" {
			continue
		}
		if stringValue(event.Payload["kind"]) != "reasoning" {
			continue
		}
		out := stringValue(event.Payload["out"])
		if out == "" {
			t.Fatalf("reasoning %s has empty out: %#v", event.Type, event.Payload)
		}
		if event.Payload["ok"] != true {
			t.Fatalf("reasoning %s ok != true: %#v", event.Type, event.Payload)
		}
		parentID := event.ParentID
		if !messageIDs[parentID] {
			t.Fatalf("reasoning %s ParentID %q is not a message id", event.Type, parentID)
		}
		tid := stringValue(event.Payload["tid"])
		if tid == "" {
			t.Fatalf("reasoning %s has empty tid", event.Type)
		}
		if event.Type == "tool_end" {
			reasoningEnds++
			if reasoningTids[tid] {
				t.Fatalf("duplicate reasoning tool_end tid %q", tid)
			}
			reasoningTids[tid] = true
			reasoningOuts = append(reasoningOuts, out)
		}
	}
	if reasoningEnds != 2 {
		t.Fatalf("expected 2 reasoning tool_end events, got %d", reasoningEnds)
	}
	if len(reasoningOuts) != 2 || reasoningOuts[0] != "I should look." || reasoningOuts[1] != "Done thinking" {
		t.Fatalf("reasoning outputs = %#v, want [\"I should look.\", \"Done thinking\"]", reasoningOuts)
	}

	// Tool call-1: tool_st then tool_end with the expected shape.
	var toolStart, toolEnd *Event
	for index := range events {
		event := events[index]
		if event.Type == "tool_st" && stringValue(event.Payload["tid"]) == "call-1" {
			toolStart = &events[index]
		}
		if event.Type == "tool_end" && stringValue(event.Payload["tid"]) == "call-1" {
			toolEnd = &events[index]
		}
	}
	if toolStart == nil {
		t.Fatal("missing tool_st for call-1")
	}
	if toolEnd == nil {
		t.Fatal("missing tool_end for call-1")
	}
	if got := stringValue(toolStart.Payload["kind"]); got != "dynamic_tool_call" {
		t.Fatalf("call-1 tool_st kind = %q, want dynamic_tool_call", got)
	}
	if got := stringValue(toolStart.Payload["name"]); got != "Read file" {
		t.Fatalf("call-1 tool_st name = %q, want Read file", got)
	}
	if toolStart.Payload["in"] == nil {
		t.Fatal("call-1 tool_st missing in")
	}
	if toolStart.ParentID != firstMessage {
		t.Fatalf("call-1 tool_st ParentID = %q, want %q", toolStart.ParentID, firstMessage)
	}
	if toolEnd.Payload["ok"] != true {
		t.Fatalf("call-1 tool_end ok = %v, want true", toolEnd.Payload["ok"])
	}
	if got := stringValue(toolEnd.Payload["out"]); got != `{"lines":3}` {
		t.Fatalf("call-1 tool_end out = %q, want {\"lines\":3}", got)
	}
	if toolEnd.ParentID != firstMessage {
		t.Fatalf("call-1 tool_end ParentID = %q, want %q", toolEnd.ParentID, firstMessage)
	}

	// run_done exists and the event before it is a txt_end.
	runDoneIndex := -1
	for index, event := range events {
		if event.Type == "run_done" {
			runDoneIndex = index
		}
	}
	if runDoneIndex < 0 {
		t.Fatal("missing run_done event")
	}
	if runDoneIndex == 0 || events[runDoneIndex-1].Type != "txt_end" {
		t.Fatalf("event before run_done is %q, want txt_end", events[runDoneIndex-1].Type)
	}
}
