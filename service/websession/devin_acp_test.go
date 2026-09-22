package websession

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"code-kanban/model"
	"code-kanban/model/tables"

	"go.uber.org/zap"
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

func newDevinSubAgentTestManager(t *testing.T) (*Manager, *tables.WebSessionTable) {
	t.Helper()
	cleanup := initTestDB(t)
	t.Cleanup(cleanup)
	project := seedProject(t)
	session := seedWebSessionWithAgent(t, project.ID, "Devin sub-agents", 1, AgentDevin)
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	return manager, session
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

func TestDevinACPDispatchDropsReplayedUpdates(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "devin-replay"}
	proj := newDevinRunProjection()
	sess := *session

	replayed := devinACPMessage{
		Method: "session/update",
		Params: devinACPUpdatePayload("agent_message_chunk", map[string]any{
			"content": map[string]any{"text": "old reply"},
		}),
	}
	manager.dispatchDevinACPMessage(nil, sess, run, proj, replayed, devinACPDispatchReplay)
	if events := readTextDeltaTestEvents(t, manager, session.ID); len(events) != 0 {
		t.Fatalf("replayed session/update produced %d events, want 0", len(events))
	}

	manager.dispatchDevinACPMessage(nil, sess, run, proj, replayed, devinACPDispatchLive)
	if events := readTextDeltaTestEvents(t, manager, session.ID); len(events) == 0 {
		t.Fatal("live session/update produced no events")
	}
}

func TestSessionCapabilitiesContain(t *testing.T) {
	caps := map[string]any{
		"loadSession": true,
		"sessionCapabilities": map[string]any{
			"resume": map[string]any{},
		},
	}
	if !sessionCapabilitiesContain(caps, "resume") {
		t.Fatal("sessionCapabilitiesContain should detect resume")
	}
	if sessionCapabilitiesContain(caps, "close") {
		t.Fatal("sessionCapabilitiesContain should reject missing entry")
	}
	if sessionCapabilitiesContain(map[string]any{"loadSession": true}, "resume") {
		t.Fatal("sessionCapabilitiesContain should reject missing sessionCapabilities")
	}
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

// Devin streams token-sized chunks where newlines and lone spaces arrive as
// whitespace-only updates. Dropping them merged paragraphs and ate the space
// after markdown markers like "#".
func TestDevinACPProjectionPreservesWhitespaceChunks(t *testing.T) {
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "devin-whitespace"}
	proj := newDevinRunProjection()
	sess := *session

	feed := func(sessionUpdate, text string) {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload(sessionUpdate, map[string]any{
			"content": map[string]any{"type": "text", "text": text},
		}))
	}

	// A leading whitespace chunk carries no text yet; it must not open an
	// empty assistant bubble.
	feed("agent_message_chunk", "\n")
	feed("agent_message_chunk", "  ")
	for _, chunk := range []string{"#", " ", "标题", "\n\n", "-", " ", "列表项", "\n", "done"} {
		feed("agent_message_chunk", chunk)
	}
	feed("agent_thought_chunk", "thinking ")
	feed("agent_thought_chunk", "\n\n")
	feed("agent_thought_chunk", "done")
	manager.finishDevinRun(sess, run, proj)

	events := readTextDeltaTestEvents(t, manager, session.ID)
	var text strings.Builder
	messageStarts := 0
	for _, event := range events {
		switch event.Type {
		case "msg_a_st":
			messageStarts++
		case "txt_d":
			text.WriteString(stringValue(event.Payload["txt"]))
		}
	}
	if messageStarts != 1 {
		t.Fatalf("expected exactly 1 assistant message, got %d", messageStarts)
	}
	if got, want := text.String(), "# 标题\n\n- 列表项\ndone"; got != want {
		t.Fatalf("reconstructed message text = %q, want %q", got, want)
	}
	var reasoningOut string
	for _, event := range events {
		if event.Type == "tool_end" && stringValue(event.Payload["kind"]) == "reasoning" {
			reasoningOut = stringValue(event.Payload["out"])
		}
	}
	if reasoningOut != "thinking \n\ndone" {
		t.Fatalf("reasoning output = %q, want %q", reasoningOut, "thinking \n\ndone")
	}
}

func TestDevinACPCapturedUserTextPreservesWhitespace(t *testing.T) {
	proj := newDevinRunProjection()
	proj.captureHistory = true
	manager, session := newTextDeltaTestManager(t)
	run := &activeRun{runID: "devin-capture"}
	sess := *session

	for _, chunk := range []string{"line one", "\n\n", "line two"} {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("user_message_chunk", map[string]any{
			"content": map[string]any{"type": "text", "text": chunk},
		}))
	}
	if got, want := proj.takeDevinCapturedUserText(), "line one\n\nline two"; got != want {
		t.Fatalf("captured user text = %q, want %q", got, want)
	}
}

func TestDevinAgentSupportsSubAgents(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want bool
	}{
		{
			name: "advertised",
			raw:  map[string]any{"_meta": map[string]any{"cognition.ai/subagentControl": true}},
			want: true,
		},
		{
			name: "explicit false",
			raw:  map[string]any{"_meta": map[string]any{"cognition.ai/subagentControl": false}},
			want: false,
		},
		{
			name: "missing meta key",
			raw:  map[string]any{"_meta": map[string]any{"cognition.ai/revert": true}},
			want: false,
		},
		{name: "no meta", raw: map[string]any{"loadSession": true}, want: false},
		{name: "nil", raw: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := devinAgentSupportsSubAgents(test.raw); got != test.want {
				t.Fatalf("devinAgentSupportsSubAgents(%#v) = %v, want %v", test.raw, got, test.want)
			}
		})
	}
}

func TestDevinACPInitializeParamsDeclaresSubAgents(t *testing.T) {
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
	for _, key := range []string{"cognition.ai/revert", "cognition.ai/subagentSupport", "cognition.ai/subagentControl"} {
		if meta[key] != true {
			t.Fatalf("clientCapabilities._meta[%q] = %#v, want true", key, meta[key])
		}
	}
}

func TestParseDevinSubAgentMeta(t *testing.T) {
	started := parseDevinSubAgentStartedMeta(map[string]any{
		"cognition.ai/subagent_started": map[string]any{
			"agentId":      "agent-1",
			"title":        "Explorer",
			"task":         "find things",
			"profile":      "subagent_explore",
			"depth":        1,
			"isBackground": true,
			"runId":        "run-9",
		},
	})
	if started == nil || started.AgentID != "agent-1" || started.Title != "Explorer" ||
		started.Task != "find things" || started.Profile != "subagent_explore" ||
		started.Depth != 1 || !started.IsBackground || started.RunID != "run-9" {
		t.Fatalf("unexpected started meta: %#v", started)
	}
	if parseDevinSubAgentStartedMeta(map[string]any{
		"cognition.ai/subagent_started": map[string]any{"title": "no id"},
	}) != nil {
		t.Fatal("started meta without agentId must be nil")
	}
	if parseDevinSubAgentStartedMeta(map[string]any{"other": true}) != nil {
		t.Fatal("missing started meta must be nil")
	}

	completed := parseDevinSubAgentCompletedMeta(map[string]any{
		"cognition.ai/subagent_completed": map[string]any{
			"agentId": "agent-1",
			"success": false,
			"summary": "failed hard",
		},
	})
	if completed == nil || completed.AgentID != "agent-1" || completed.Success == nil ||
		*completed.Success || completed.Summary != "failed hard" {
		t.Fatalf("unexpected completed meta: %#v", completed)
	}
	if parseDevinSubAgentCompletedMeta(map[string]any{
		"cognition.ai/subagent_completed": map[string]any{"success": true},
	}) != nil {
		t.Fatal("completed meta without agentId must be nil")
	}

	if got := parseDevinSubAgentContextMeta(map[string]any{
		"cognition.ai/subagent_context": map[string]any{"parentAgentId": "agent-1", "runId": "r"},
	}); got != "agent-1" {
		t.Fatalf("context meta = %q, want agent-1", got)
	}
	if got := parseDevinSubAgentContextMeta(map[string]any{}); got != "" {
		t.Fatalf("empty context meta = %q, want \"\"", got)
	}
}

func TestDevinACPSubAgentLifecycle(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-subagent-run"}
	proj := newDevinRunProjection()
	sess := *session

	feed := func(sessionUpdate string, extra map[string]any) {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload(sessionUpdate, extra))
	}

	feed("tool_call", map[string]any{
		"toolCallId": "call-spawn",
		"title":      "Subagent",
		"kind":       "other",
		"_meta": map[string]any{
			"cognition.ai/subagent_started": map[string]any{
				"agentId": "agent-1",
				"title":   "Explorer",
				"task":    "find the thing",
				"profile": "subagent_explore",
			},
		},
	})

	events := readTextDeltaTestEvents(t, manager, session.ID)
	var stateEvent *Event
	for index := range events {
		if events[index].Type == "sub_agent_state" && events[index].ThreadID == "agent-1" {
			stateEvent = &events[index]
		}
	}
	if stateEvent == nil {
		t.Fatal("missing sub_agent_state event for agent-1")
	}
	if got := stringValue(stateEvent.Payload["status"]); got != string(WebSessionSubAgentRunning) {
		t.Fatalf("sub_agent_state status = %q, want %q", got, WebSessionSubAgentRunning)
	}
	if got := stringValue(stateEvent.Payload["summary"]); got != "find the thing" {
		t.Fatalf("sub_agent_state summary = %q", got)
	}
	var activityEvent *Event
	for index := range events {
		if events[index].Type == "sub_agent_activity" &&
			stringValue(events[index].Payload["agentThreadId"]) == "agent-1" {
			activityEvent = &events[index]
		}
	}
	if activityEvent == nil {
		t.Fatal("missing sub_agent_activity started marker for agent-1")
	}

	// Child-owned updates carry subagent_context and must tag the projected
	// events with the sub-agent's thread id.
	feed("agent_message_chunk", map[string]any{
		"content": map[string]any{"text": "child says hi"},
		"_meta":   map[string]any{"cognition.ai/subagent_context": map[string]any{"parentAgentId": "agent-1"}},
	})
	feed("tool_call", map[string]any{
		"toolCallId": "call-child",
		"title":      "Ran grep",
		"kind":       "execute",
		"_meta":      map[string]any{"cognition.ai/subagent_context": map[string]any{"parentAgentId": "agent-1"}},
	})

	events = readTextDeltaTestEvents(t, manager, session.ID)
	var childText, childTool *Event
	for index := range events {
		if events[index].Type == "txt_d" && events[index].ThreadID == "agent-1" {
			childText = &events[index]
		}
		if events[index].Type == "tool_st" && events[index].ThreadID == "agent-1" &&
			stringValue(events[index].Payload["tid"]) == "call-child" {
			childTool = &events[index]
		}
	}
	if childText == nil {
		t.Fatal("child agent_message_chunk was not tagged with the sub-agent thread")
	}
	if childTool == nil {
		t.Fatal("child tool_call was not tagged with the sub-agent thread")
	}

	feed("tool_call_update", map[string]any{
		"toolCallId": "call-spawn",
		"status":     "completed",
		"_meta": map[string]any{
			"cognition.ai/subagent_completed": map[string]any{
				"agentId": "agent-1",
				"success": true,
				"summary": "found it",
			},
		},
	})

	agents, err := manager.sessionSubAgents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("sessionSubAgents: %v", err)
	}
	if len(agents) != 1 || agents[0].ThreadID != "agent-1" {
		t.Fatalf("expected a single agent-1 registry row, got %#v", agents)
	}
	if agents[0].Status != WebSessionSubAgentCompleted || agents[0].Active {
		t.Fatalf("agent-1 status = %q active=%v, want completed/inactive", agents[0].Status, agents[0].Active)
	}
	if agents[0].Summary != "found it" {
		t.Fatalf("agent-1 summary = %q, want %q", agents[0].Summary, "found it")
	}
	if agents[0].LatestItemID == nil {
		t.Fatal("devin sub-agent activity must backfill latestItemId in the registry")
	}
	if agents[0].Role != "subagent_explore" || agents[0].Nickname != "Explorer" {
		t.Fatalf("agent-1 role/nickname = %q/%q", agents[0].Role, agents[0].Nickname)
	}
}

func TestDevinSubAgentInterruptedOnRunFinish(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-subagent-finish"}
	proj := newDevinRunProjection()
	sess := *session

	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("tool_call", map[string]any{
		"toolCallId": "call-bg",
		"title":      "Subagent",
		"_meta": map[string]any{
			"cognition.ai/subagent_started": map[string]any{
				"agentId":      "agent-bg",
				"title":        "Background",
				"isBackground": true,
			},
		},
	}))

	manager.finishDevinRun(sess, run, proj)

	var row tables.WebSessionSubAgentTable
	if err := model.GetDB().
		Where("web_session_id = ? AND thread_id = ?", session.ID, "agent-bg").
		First(&row).Error; err != nil {
		t.Fatalf("read sub-agent row: %v", err)
	}
	if row.Status != string(WebSessionSubAgentInterrupted) || row.IsActive {
		t.Fatalf("unfinished background sub-agent = %q active=%v, want interrupted/inactive", row.Status, row.IsActive)
	}
}

func TestDevinNestedSubAgentParenting(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-subagent-nested"}
	proj := newDevinRunProjection()
	sess := *session

	feed := func(sessionUpdate string, extra map[string]any) {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload(sessionUpdate, extra))
	}

	feed("tool_call", map[string]any{
		"toolCallId": "call-outer",
		"title":      "Subagent",
		"_meta": map[string]any{
			"cognition.ai/subagent_started": map[string]any{"agentId": "agent-outer", "title": "Outer"},
		},
	})
	feed("tool_call", map[string]any{
		"toolCallId": "call-inner",
		"title":      "Subagent",
		"_meta": map[string]any{
			"cognition.ai/subagent_context": map[string]any{"parentAgentId": "agent-outer"},
			"cognition.ai/subagent_started": map[string]any{"agentId": "agent-inner", "title": "Inner", "depth": 1},
		},
	})

	var outer, inner tables.WebSessionSubAgentTable
	if err := model.GetDB().
		Where("web_session_id = ? AND thread_id = ?", session.ID, "agent-outer").
		First(&outer).Error; err != nil {
		t.Fatalf("read outer sub-agent row: %v", err)
	}
	if err := model.GetDB().
		Where("web_session_id = ? AND thread_id = ?", session.ID, "agent-inner").
		First(&inner).Error; err != nil {
		t.Fatalf("read inner sub-agent row: %v", err)
	}
	if outer.ParentThreadID != nil {
		t.Fatalf("outer parent = %v, want nil", outer.ParentThreadID)
	}
	if inner.ParentThreadID == nil || *inner.ParentThreadID != "agent-outer" {
		t.Fatalf("inner parent = %v, want agent-outer", inner.ParentThreadID)
	}
}

func TestDevinSessionModeID(t *testing.T) {
	cases := []struct {
		workflowMode    string
		permissionLevel string
		want            string
	}{
		{"default", "default", "accept-edits"},
		{"default", "elevated", "smart"},
		{"default", "yolo", "bypass"},
		{"plan", "default", "plan"},
		{"plan", "elevated", "plan"},
		{"plan", "yolo", "plan"},
		{"", "", "smart"},
	}
	for _, tc := range cases {
		session := tables.WebSessionTable{
			WorkflowMode:    tc.workflowMode,
			PermissionLevel: tc.permissionLevel,
		}
		if got := devinSessionModeID(session); got != tc.want {
			t.Fatalf("devinSessionModeID(%q, %q) = %q, want %q", tc.workflowMode, tc.permissionLevel, got, tc.want)
		}
	}
}

func TestParseDevinSessionModes(t *testing.T) {
	modes := parseDevinSessionModes(json.RawMessage(`{
		"sessionId": "abc",
		"modes": {
			"currentModeId": "accept-edits",
			"availableModes": [
				{"id": "accept-edits", "name": "Code"},
				{"id": "smart", "name": "Smart"},
				{"id": "ask", "name": "Ask"},
				{"id": "plan", "name": "Plan"},
				{"id": "bypass", "name": "Bypass Permissions"}
			]
		}
	}`))
	if modes == nil {
		t.Fatal("expected modes")
	}
	if modes.CurrentModeID != "accept-edits" {
		t.Fatalf("current mode = %q", modes.CurrentModeID)
	}
	for _, id := range []string{"accept-edits", "smart", "ask", "plan", "bypass"} {
		if !modes.AvailableModeIDs[id] {
			t.Fatalf("expected available mode %q", id)
		}
	}
	if modes.AvailableModeIDs["yolo"] {
		t.Fatal("unexpected available mode yolo")
	}
	if got := parseDevinSessionModes(json.RawMessage(`{"sessionId":"abc"}`)); got != nil {
		t.Fatalf("expected nil modes, got %#v", got)
	}
	if got := parseDevinSessionModes(nil); got != nil {
		t.Fatalf("expected nil modes, got %#v", got)
	}
}

func TestDevinACPUsageUpdateProjectsSessionStats(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-usage"}
	proj := newDevinRunProjection()
	sess := *session

	feed := func(extra map[string]any) {
		manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("usage_update", extra))
	}

	feed(map[string]any{"used": 11849, "size": 262000, "_meta": map[string]any{
		"cognition.ai/inputTokens": 11764, "cognition.ai/outputTokens": 85,
	}})
	// The agent repeats each report tagged with the owning context; the
	// duplicate must not double count.
	feed(map[string]any{"used": 11849, "size": 262000, "_meta": map[string]any{
		"cognition.ai/inputTokens": 11764, "cognition.ai/outputTokens": 85,
		"cognition.ai/subagent_context": map[string]any{"parentAgentId": "root"},
	}})
	feed(map[string]any{"used": 11926, "size": 262000, "_meta": map[string]any{
		"cognition.ai/inputTokens":      11899,
		"cognition.ai/outputTokens":     27,
		"cognition.ai/cachedReadTokens": 11776,
	}})

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.TotalInputTokens != 23663 || record.TotalCachedInputTokens != 11776 || record.TotalOutputTokens != 112 {
		t.Fatalf("unexpected totals: in=%d cin=%d out=%d",
			record.TotalInputTokens, record.TotalCachedInputTokens, record.TotalOutputTokens)
	}
	if record.LatestTokenCountTotalTokens != 11926 || record.LatestTokenCountUpdatedAt == nil {
		t.Fatalf("unexpected latest token count: total=%d at=%v",
			record.LatestTokenCountTotalTokens, record.LatestTokenCountUpdatedAt)
	}
	if record.LatestTokenCountInputTokens != 11899 || record.LatestTokenCountCachedInputTokens != 11776 ||
		record.LatestTokenCountOutputTokens != 27 {
		t.Fatalf("unexpected latest token count parts: %#v", record)
	}
	if record.SessionContextWindowTokens != 262000 || record.SessionContextWindowObservedAt == nil {
		t.Fatalf("unexpected context window: %d at=%v",
			record.SessionContextWindowTokens, record.SessionContextWindowObservedAt)
	}

	summary := manager.mapSessionSummary(record)
	if summary.ContextEstimateMode != ContextEstimateModeLatestTokenCount {
		t.Fatalf("context estimate mode = %q, want %q", summary.ContextEstimateMode, ContextEstimateModeLatestTokenCount)
	}
	if summary.ContextEstimate.UsedTokens != 11926 {
		t.Fatalf("context estimate used = %d, want 11926", summary.ContextEstimate.UsedTokens)
	}
	if summary.ContextWindowTokens == nil || *summary.ContextWindowTokens != 262000 {
		t.Fatalf("context window tokens = %v, want 262000", summary.ContextWindowTokens)
	}
	if summary.ContextWindowSource != ContextWindowSourceSessionUsage {
		t.Fatalf("context window source = %q, want %q", summary.ContextWindowSource, ContextWindowSourceSessionUsage)
	}

	var usageEvents int
	for _, event := range readTextDeltaTestEvents(t, manager, session.ID) {
		if event.Type != "usage" {
			continue
		}
		usageEvents++
		if int64(numberValue(event.Payload["cwt"])) != 262000 {
			t.Fatalf("usage event cwt = %v, want 262000", event.Payload["cwt"])
		}
	}
	if usageEvents != 2 {
		t.Fatalf("expected 2 usage events after dedup, got %d", usageEvents)
	}
}

func TestDevinACPUsageUpdateSubAgentContext(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-usage-sub"}
	proj := newDevinRunProjection()
	sess := *session

	tokens := map[string]any{"cognition.ai/inputTokens": 90, "cognition.ai/outputTokens": 10}
	// The tagged copy may arrive before its bare sibling; either order must
	// count the report once and fold it into the session totals.
	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("usage_update", map[string]any{
		"used": 100, "size": 262000,
		"_meta": map[string]any{
			"cognition.ai/inputTokens":      90,
			"cognition.ai/outputTokens":     10,
			"cognition.ai/subagent_context": map[string]any{"parentAgentId": "agent-1"},
		},
	}))
	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("usage_update", map[string]any{
		"used": 100, "size": 262000, "_meta": tokens,
	}))

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.TotalInputTokens != 90 || record.TotalOutputTokens != 10 {
		t.Fatalf("context-tagged usage must count once in session totals: %#v", record)
	}
}

func TestDevinACPPromptUsageFallback(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-usage-fallback"}
	proj := newDevinRunProjection()
	sess := *session

	manager.applyDevinPromptUsageFallback(sess, run, proj, json.RawMessage(
		`{"stopReason":"end_turn","usage":{"totalTokens":11783,"inputTokens":11751,"outputTokens":32,"cachedReadTokens":10176}}`))

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.TotalInputTokens != 11751 || record.TotalCachedInputTokens != 10176 || record.TotalOutputTokens != 32 {
		t.Fatalf("unexpected fallback totals: %#v", record)
	}
	if record.LatestTokenCountTotalTokens != 11783 {
		t.Fatalf("fallback latest token count = %d, want 11783", record.LatestTokenCountTotalTokens)
	}

	// A run that already streamed usage_update must not recount the response.
	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("usage_update", map[string]any{
		"used": 20000, "size": 262000,
		"_meta": map[string]any{"cognition.ai/inputTokens": 19900, "cognition.ai/outputTokens": 100},
	}))
	manager.applyDevinPromptUsageFallback(sess, run, proj, json.RawMessage(
		`{"stopReason":"end_turn","usage":{"totalTokens":20000,"inputTokens":19900,"outputTokens":100}}`))

	record, err = manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.TotalInputTokens != 31651 || record.TotalOutputTokens != 132 {
		t.Fatalf("prompt usage counted twice: %#v", record)
	}
}

func TestDevinCompactionNotification(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-compact"}
	proj := newDevinRunProjection()
	sess := *session

	if err := manager.updateRuntimeState(context.Background(), session.ID, map[string]any{
		"total_input_tokens":        50000,
		"total_cached_input_tokens": 30000,
		"total_output_tokens":       2000,
	}); err != nil {
		t.Fatalf("seed totals: %v", err)
	}

	manager.dispatchDevinACPMessage(nil, sess, run, proj, devinACPMessage{
		Method: "_cognition.ai/compaction",
		Params: json.RawMessage(`{"status":"started","sessionId":"native-1"}`),
	}, devinACPDispatchLive)
	manager.dispatchDevinACPMessage(nil, sess, run, proj, devinACPMessage{
		Method: "_cognition.ai/compaction",
		Params: json.RawMessage(`{"status":"completed","summary":"Compacted 3 messages","sessionId":"native-1"}`),
	}, devinACPDispatchLive)

	var startEvent, endEvent *Event
	events := readTextDeltaTestEvents(t, manager, session.ID)
	for index := range events {
		event := &events[index]
		if stringValue(event.Payload["kind"]) != "context_compaction" {
			continue
		}
		if event.Type == "tool_st" {
			startEvent = event
		}
		if event.Type == "tool_end" {
			endEvent = event
		}
	}
	if startEvent == nil || endEvent == nil {
		t.Fatal("expected context_compaction tool_st and tool_end events")
	}
	if startEvent.Payload["tid"] != endEvent.Payload["tid"] {
		t.Fatalf("compaction events use different tids: %v vs %v", startEvent.Payload["tid"], endEvent.Payload["tid"])
	}
	if endEvent.Payload["ok"] != true || stringValue(endEvent.Payload["out"]) != "Compacted 3 messages" {
		t.Fatalf("unexpected compaction tool_end payload: %#v", endEvent.Payload)
	}

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.LastContextCompactionAt == nil {
		t.Fatal("expected last_context_compaction_at after completed compaction")
	}
	if record.ContextBaselineInputTokens != 50000 || record.ContextBaselineOutputTokens != 2000 {
		t.Fatalf("unexpected context baseline: %#v", record)
	}
}

// Devin sends an extra "completed" notification with an empty summary when the
// history snapshot is dumped, long before the real summary arrives. That ping
// must keep the compaction card open so both notifications render as one card.
func TestDevinCompactionEmptySummaryCompletedStaysOpen(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-compact-empty"}
	proj := newDevinRunProjection()
	sess := *session

	manager.dispatchDevinACPMessage(nil, sess, run, proj, devinACPMessage{
		Method: "_cognition.ai/compaction",
		Params: json.RawMessage(`{"status":"completed","sessionId":"native-1"}`),
	}, devinACPDispatchLive)

	compactionEvents := func() []Event {
		var out []Event
		events := readTextDeltaTestEvents(t, manager, session.ID)
		for _, event := range events {
			if stringValue(event.Payload["kind"]) == "context_compaction" {
				out = append(out, event)
			}
		}
		return out
	}

	events := compactionEvents()
	if len(events) != 1 || events[0].Type != "tool_st" {
		t.Fatalf("empty-summary completed should emit one tool_st, got %#v", events)
	}
	startTid := events[0].Payload["tid"]

	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.LastContextCompactionAt != nil {
		t.Fatal("progress ping must not mark compaction completed")
	}

	manager.dispatchDevinACPMessage(nil, sess, run, proj, devinACPMessage{
		Method: "_cognition.ai/compaction",
		Params: json.RawMessage(`{"status":"completed","summary":"Compacted 3 messages","sessionId":"native-1"}`),
	}, devinACPDispatchLive)

	events = compactionEvents()
	if len(events) != 2 || events[1].Type != "tool_end" {
		t.Fatalf("expected one tool_st + one tool_end, got %#v", events)
	}
	if events[1].Payload["tid"] != startTid {
		t.Fatalf("completion reused a different tid: %v vs %v", events[1].Payload["tid"], startTid)
	}
	if stringValue(events[1].Payload["out"]) != "Compacted 3 messages" {
		t.Fatalf("unexpected tool_end payload: %#v", events[1].Payload)
	}
}

type devinTestWriteCloser struct{ bytes.Buffer }

func (w *devinTestWriteCloser) Close() error { return nil }

func newDevinTestClient() (*devinACPClient, *devinTestWriteCloser) {
	stdin := &devinTestWriteCloser{}
	return &devinACPClient{stdin: stdin, closed: make(chan struct{})}, stdin
}

func devinPlanExitPermissionMessage() devinACPMessage {
	params, _ := json.Marshal(map[string]any{
		"sessionId": "native-1",
		"toolCall":  map[string]any{"toolCallId": "call-exit"},
		"options": []any{
			map[string]any{"optionId": "plan_accept_edits", "name": "Yes, implement plan and accept edits", "kind": "allow_once"},
			map[string]any{"optionId": "plan_bypass", "name": "Yes, implement plan and bypass permissions", "kind": "allow_always"},
			map[string]any{"optionId": "reject_once", "name": "No, plan needs changes", "kind": "reject_once"},
		},
	})
	return devinACPMessage{ID: json.RawMessage(`77`), Params: params}
}

func TestDevinPermissionIsPlanExit(t *testing.T) {
	if !devinPermissionIsPlanExit(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call-1"},
		"options": []any{
			map[string]any{"optionId": "plan_accept_edits", "kind": "allow_once"},
			map[string]any{"optionId": "reject_once", "kind": "reject_once"},
		},
	}) {
		t.Fatal("plan_ option ids should mark a plan exit request")
	}
	if !devinPermissionIsPlanExit(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call-2", "kind": "switch_mode"},
	}) {
		t.Fatal("switch_mode tool calls should mark a plan exit request")
	}
	if devinPermissionIsPlanExit(map[string]any{
		"toolCall": map[string]any{"toolCallId": "call-3", "kind": "execute"},
		"options": []any{
			map[string]any{"optionId": "allow_once", "kind": "allow_once"},
			map[string]any{"optionId": "reject_once", "kind": "reject_once"},
		},
	}) {
		t.Fatal("ordinary permission request must not be classified as plan exit")
	}
}

func TestDevinPlanExitPermissionRequest(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	sess := *session
	run := &activeRun{runID: "devin-plan", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin}

	manager.handleDevinPermissionRequest(&devinACPClient{}, sess, run, devinPlanExitPermissionMessage())

	pending, ok := run.pendingApprovalRequest()
	if !ok {
		t.Fatal("expected a pending approval request")
	}
	if pending.Kind != pendingServerRequestPlanApproval {
		t.Fatalf("pending kind = %q, want %q", pending.Kind, pendingServerRequestPlanApproval)
	}
	if pending.ItemID != "call-exit" {
		t.Fatalf("pending item id = %q, want call-exit", pending.ItemID)
	}
	if !run.completedPlanToolSeen() {
		t.Fatal("exit-plan request must mark the completed plan tool")
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.AssistantState != string(AssistantStateWaitingPlanApproval) {
		t.Fatalf("assistant state = %q, want %q", record.AssistantState, AssistantStateWaitingPlanApproval)
	}
	var approvalReq *Event
	for i, event := range readTextDeltaTestEvents(t, manager, session.ID) {
		if event.Type == "approval_req" {
			approvalReq = &readTextDeltaTestEvents(t, manager, session.ID)[i]
		}
	}
	if approvalReq == nil || stringValue(approvalReq.Payload["kind"]) != string(pendingServerRequestPlanApproval) {
		t.Fatalf("expected plan_approval approval_req event, got %#v", approvalReq)
	}
}

func TestDevinOrdinaryPermissionRequest(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	sess := *session
	run := &activeRun{runID: "devin-cmd", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin}
	params, _ := json.Marshal(map[string]any{
		"sessionId": "native-1",
		"toolCall":  map[string]any{"toolCallId": "call-1", "kind": "execute", "title": "Run command"},
		"options": []any{
			map[string]any{"optionId": "allow_once", "kind": "allow_once"},
			map[string]any{"optionId": "reject_once", "kind": "reject_once"},
		},
	})

	client, _ := newDevinTestClient()
	// With the agent-side smart mode applied, elevated sessions surface the
	// request instead of falling back to client-side auto-approve.
	client.setSessionModes("native-1", &devinACPSessionModes{
		CurrentModeID:    "smart",
		AvailableModeIDs: map[string]bool{"smart": true},
	}, true)
	manager.handleDevinPermissionRequest(client, sess, run, devinACPMessage{ID: json.RawMessage(`12`), Params: params})

	pending, ok := run.pendingApprovalRequest()
	if !ok || pending.Kind != pendingServerRequestCommandApproval {
		t.Fatalf("expected command approval pending request, got %#v", pending)
	}
	if run.completedPlanToolSeen() {
		t.Fatal("ordinary permission request must not mark the plan tool")
	}
	record, err := manager.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if record.AssistantState != string(AssistantStateWaitingApproval) {
		t.Fatalf("assistant state = %q, want %q", record.AssistantState, AssistantStateWaitingApproval)
	}
}

func TestDevinPermissionResponsePayloadPlanExit(t *testing.T) {
	request := &pendingServerRequest{
		Kind: pendingServerRequestPlanApproval,
		Permissions: map[string]any{
			"options": []any{
				map[string]any{"optionId": "plan_accept_edits", "kind": "allow_once"},
				map[string]any{"optionId": "plan_bypass", "kind": "allow_always"},
				map[string]any{"optionId": "reject_once", "kind": "reject_once"},
			},
		},
	}
	assertOption := func(session tables.WebSessionTable, want string) {
		t.Helper()
		payload, _ := devinPermissionResponsePayload("approve", request, session).(map[string]any)
		outcome := decodeRawObject(payload["outcome"])
		if got := stringValue(outcome["optionId"]); got != want {
			t.Fatalf("approve optionId = %q, want %q", got, want)
		}
	}
	assertOption(tables.WebSessionTable{PermissionLevel: string(PermissionLevelDefault)}, "plan_accept_edits")
	assertOption(tables.WebSessionTable{PermissionLevel: string(PermissionLevelElevated)}, "plan_accept_edits")
	assertOption(tables.WebSessionTable{PermissionLevel: string(PermissionLevelYolo)}, "plan_bypass")

	payload, _ := devinPermissionResponsePayload("reject", request, tables.WebSessionTable{}).(map[string]any)
	if got := stringValue(decodeRawObject(payload["outcome"])["outcome"]); got != "cancelled" {
		t.Fatalf("reject outcome = %q, want cancelled", got)
	}
}

func TestDevinSwitchModeToolCallProjectsPlanCard(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	run := &activeRun{runID: "devin-plan-card", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin}
	proj := newDevinRunProjection()
	sess := *session

	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("tool_call", map[string]any{
		"toolCallId": "call-exit",
		"title":      "Exit plan mode",
		"kind":       "switch_mode",
		"rawInput":   map[string]any{"modeId": "accept-edits", "plan": "# Plan\n\n1. Ship it"},
		"locations":  []any{map[string]any{"path": "/home/user/.devin/plans/plan-1.md"}},
	}))
	if !run.completedPlanToolSeen() {
		t.Fatal("switch_mode tool call must mark the completed plan tool")
	}
	// The follow-up tool_call_update after the permission resolves must keep
	// the plan text instead of overwriting it with an empty output.
	manager.handleDevinACPUpdate(sess, run, proj, devinACPUpdatePayload("tool_call_update", map[string]any{
		"toolCallId": "call-exit",
		"status":     "completed",
	}))

	var planEnd *Event
	events := readTextDeltaTestEvents(t, manager, session.ID)
	for i, event := range events {
		if event.Type == "tool_end" && stringValue(event.Payload["tid"]) == "call-exit" {
			planEnd = &events[i]
		}
	}
	if planEnd == nil {
		t.Fatalf("expected a tool_end for the plan card, got %#v", events)
	}
	if stringValue(planEnd.Payload["kind"]) != "plan" || stringValue(planEnd.Payload["name"]) != "Plan" {
		t.Fatalf("plan card must use kind plan / name Plan, got %#v", planEnd.Payload)
	}
	if stringValue(planEnd.Payload["out"]) != "# Plan\n\n1. Ship it" {
		t.Fatalf("plan card lost the plan text: %#v", planEnd.Payload)
	}
	if meta := decodeRawObject(planEnd.Payload["meta"]); stringValue(meta["path"]) != "/home/user/.devin/plans/plan-1.md" {
		t.Fatalf("plan card missing plan path: %#v", meta)
	}
}

func TestDevinResolvePlanApprovalForSendApprovesOnDefaultMode(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	record := *session
	record.WorkflowMode = string(WorkflowModeDefault)
	run := &activeRun{runID: "devin-plan-send", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin, done: make(chan struct{})}
	client, stdin := newDevinTestClient()
	run.setDevinACP(client)
	run.setPendingServerRequest(&pendingServerRequest{
		RawID: json.RawMessage(`77`),
		Kind:  pendingServerRequestPlanApproval,
		Permissions: map[string]any{
			"options": []any{
				map[string]any{"optionId": "plan_accept_edits", "kind": "allow_once"},
				map[string]any{"optionId": "plan_bypass", "kind": "allow_always"},
				map[string]any{"optionId": "reject_once", "kind": "reject_once"},
			},
		},
	})
	run.markCompletedPlanTool()
	manager.mu.Lock()
	manager.runs[session.ID] = run
	manager.mu.Unlock()

	handled, err := manager.resolveDevinPlanApprovalForSend(context.Background(), session.ID, record, "Implement the plan.", nil, nil)
	if err != nil || !handled {
		t.Fatalf("resolveDevinPlanApprovalForSend = handled %v, err %v", handled, err)
	}
	if _, ok := run.pendingApprovalRequest(); ok {
		t.Fatal("pending request must be cleared after approval")
	}
	if run.completedPlanToolSeen() {
		t.Fatal("completed plan tool marker must be cleared after approval")
	}
	var response struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdin.Bytes()), &response); err != nil {
		t.Fatalf("invalid response written to ACP stdin: %v", err)
	}
	outcome := decodeRawObject(decodeRawObject(response.Result)["outcome"])
	if stringValue(outcome["outcome"]) != "selected" || stringValue(outcome["optionId"]) != "plan_accept_edits" {
		t.Fatalf("unexpected permission outcome: %s", response.Result)
	}

	events := readTextDeltaTestEvents(t, manager, session.ID)
	var sawApprovalRes, sawUserMessage bool
	for _, event := range events {
		if event.Type == "approval_res" && stringValue(event.Payload["act"]) == "approve" {
			sawApprovalRes = true
		}
		if event.Type == "msg_u" && stringValue(event.Payload["txt"]) == "Implement the plan." {
			sawUserMessage = true
		}
	}
	if !sawApprovalRes || !sawUserMessage {
		t.Fatalf("expected approval_res and msg_u events, got %#v", events)
	}
	if queued := manager.pendingInputsSnapshot(session.ID); len(queued) != 0 {
		t.Fatalf("approved plan must not queue the message, got %#v", queued)
	}
}

func TestDevinResolvePlanApprovalForSendQueuesFeedbackInPlanMode(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	record := *session
	record.WorkflowMode = string(WorkflowModePlan)
	run := &activeRun{runID: "devin-plan-feedback", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin, done: make(chan struct{})}
	client, stdin := newDevinTestClient()
	run.setDevinACP(client)
	run.setPendingServerRequest(&pendingServerRequest{
		RawID: json.RawMessage(`77`),
		Kind:  pendingServerRequestPlanApproval,
		Permissions: map[string]any{
			"options": []any{
				map[string]any{"optionId": "plan_accept_edits", "kind": "allow_once"},
				map[string]any{"optionId": "reject_once", "kind": "reject_once"},
			},
		},
	})
	run.markCompletedPlanTool()
	manager.mu.Lock()
	manager.runs[session.ID] = run
	manager.mu.Unlock()

	handled, err := manager.resolveDevinPlanApprovalForSend(context.Background(), session.ID, record, "Please also handle retries", nil, nil)
	if err != nil || !handled {
		t.Fatalf("resolveDevinPlanApprovalForSend = handled %v, err %v", handled, err)
	}
	outcome := decodeRawObject(decodeRawObject(mustUnmarshalDevinResponse(t, stdin.Bytes()))["outcome"])
	if stringValue(outcome["outcome"]) != "cancelled" {
		t.Fatalf("feedback while still in plan mode must decline the exit, got %s", stdin.Bytes())
	}
	queued := manager.pendingInputsSnapshot(session.ID)
	if len(queued) != 1 || queued[0].Text != "Please also handle retries" {
		t.Fatalf("expected the message to be queued for the next turn, got %#v", queued)
	}
}

func mustUnmarshalDevinResponse(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var response struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &response); err != nil {
		t.Fatalf("invalid response written to ACP stdin: %v", err)
	}
	return decodeRawObject(response.Result)
}
