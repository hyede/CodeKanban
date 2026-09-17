package websession

import (
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
