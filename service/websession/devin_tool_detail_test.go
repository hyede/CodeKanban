package websession

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDevinToolDetailsRemainAvailableAcrossMessageBoundaries(t *testing.T) {
	for _, kind := range []string{"execute", "read", "edit"} {
		t.Run(kind, func(t *testing.T) {
			manager, session := newDevinSubAgentTestManager(t)
			run := &activeRun{runID: "devin-tool-details", sessionID: session.ID, agent: AgentDevin}
			manager.runs[session.ID] = run
			proj := newDevinRunProjection()
			output := strings.Repeat("full tool output ", 200)
			for index := 1; index <= 3; index++ {
				toolID := fmt.Sprintf("call-%d", index)
				manager.handleDevinACPUpdate(*session, run, proj, devinACPUpdatePayload("tool_call", map[string]any{
					"toolCallId": toolID,
					"title":      "Tool",
					"kind":       kind,
					"rawInput":   map[string]any{"command": "echo test", "path": "src/main.go"},
				}))
				manager.handleDevinACPUpdate(*session, run, proj, devinACPUpdatePayload("tool_call_update", map[string]any{
					"toolCallId": toolID,
					"status":     "completed",
					"rawOutput":  output,
				}))
			}

			for index := 1; index <= 3; index++ {
				toolID := fmt.Sprintf("call-%d", index)
				for _, key := range []string{toolID, commandExecutionGroupID(toolID)} {
					detail, err := manager.GetCommandExecutionGroup(context.Background(), session.ID, key)
					if err != nil {
						t.Fatalf("GetCommandExecutionGroup(%q): %v", key, err)
					}
					if len(detail.Items) != 1 || detail.Items[0].ToolID != toolID {
						t.Fatalf("details for %q lost the original tool: %#v", key, detail.Items)
					}
					item := detail.Items[0]
					if item.Output != output || item.Status != "done" {
						t.Fatalf("details for %q lost the full output or status: %#v", key, item)
					}
					if stringValue(decodeRawObject(item.Input)["path"]) != "src/main.go" {
						t.Fatalf("details for %q lost the input: %#v", key, item.Input)
					}
				}
			}
		})
	}
}
