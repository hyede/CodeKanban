package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Simulate ACP mode acknowledgements while recording the real wire payloads.
type devinApprovalModeWriter struct {
	client   *devinACPClient
	messages []devinACPMessage
}

func (w *devinApprovalModeWriter) Close() error { return nil }

func (w *devinApprovalModeWriter) Write(data []byte) (int, error) {
	var message devinACPMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return 0, err
	}
	w.messages = append(w.messages, message)
	if message.Method != "" {
		if message.Method != "session/set_mode" {
			return 0, fmt.Errorf("unexpected ACP method %q", message.Method)
		}
		w.client.pendingMu.Lock()
		key := devinACPIDKey(message.ID)
		response := w.client.pending[key]
		delete(w.client.pending, key)
		w.client.pendingMu.Unlock()
		if response == nil {
			return 0, fmt.Errorf("missing response channel for %s", key)
		}
		response <- devinACPMessage{ID: message.ID, Result: json.RawMessage(`{}`)}
	}
	return len(data), nil
}

func devinOrdinaryApprovalMessage(id string) devinACPMessage {
	return devinACPMessage{
		ID:     json.RawMessage(id),
		Params: json.RawMessage(`{"toolCall":{"toolCallId":"tool-1","kind":"execute"},"options":[{"optionId":"allow_once","kind":"allow_once"},{"optionId":"reject_once","kind":"reject_once"}]}`),
	}
}

func TestDevinApprovalCanBecomeFullyAutomatic(t *testing.T) {
	for _, plan := range []bool{false, true} {
		t.Run(fmt.Sprintf("plan=%v", plan), func(t *testing.T) {
			manager, session := newDevinSubAgentTestManager(t)
			ctx := context.Background()
			if _, err := manager.UpdatePermissionLevel(ctx, session.ID, PermissionLevelDefault); err != nil {
				t.Fatal(err)
			}
			workflow, nativeMode := WorkflowModeDefault, "accept-edits"
			permissionRequest := devinOrdinaryApprovalMessage("77")
			wantOption := "allow_once"
			if plan {
				workflow, nativeMode = WorkflowModePlan, "plan"
				permissionRequest = devinPlanExitPermissionMessage()
				wantOption = "plan_bypass"
			}
			if _, err := manager.UpdateWorkflowMode(ctx, session.ID, workflow); err != nil {
				t.Fatal(err)
			}
			snapshot, err := manager.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			writer := &devinApprovalModeWriter{}
			client := &devinACPClient{stdin: writer, closed: make(chan struct{}), pending: make(map[string]chan devinACPMessage)}
			writer.client = client
			client.setSessionModes("native-1", &devinACPSessionModes{
				CurrentModeID:    nativeMode,
				AvailableModeIDs: map[string]bool{"accept-edits": true, "smart": true, "bypass": true, "plan": true},
			}, true)
			run := &activeRun{runID: "devin-approve-yolo", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin, done: make(chan struct{})}
			run.setDevinACP(client)
			manager.mu.Lock()
			manager.runs[session.ID] = run
			manager.mu.Unlock()
			manager.handleDevinPermissionRequest(client, snapshot, run, nil, permissionRequest)
			if _, ok := run.pendingApprovalRequest(); !ok {
				t.Fatal("expected the original approval to be pending")
			}
			if _, err := manager.UpdatePermissionLevel(ctx, session.ID, PermissionLevelYolo); err != nil {
				t.Fatalf("change permission: %v", err)
			}
			if err := manager.respondToApproval(session.ID, "approve"); err != nil {
				t.Fatalf("continue approval: %v", err)
			}
			if _, ok := run.pendingApprovalRequest(); ok {
				t.Fatal("the original approval must be cleared")
			}
			modeCalls, approvalReplies := 0, 0
			for _, message := range writer.messages {
				if message.Method == "session/set_mode" {
					modeCalls++
					params := decodeRawObject(message.Params)
					if stringValue(params["modeId"]) != "bypass" || stringValue(params["sessionId"]) != "native-1" {
						t.Fatalf("unexpected mode change: %s", message.Params)
					}
				} else {
					approvalReplies++
					outcome := decodeRawObject(decodeRawObject(message.Result)["outcome"])
					if string(message.ID) != "77" || stringValue(outcome["optionId"]) != wantOption {
						t.Fatalf("unexpected approval response: %s %s", message.ID, message.Result)
					}
				}
			}
			if modeCalls != 1 || approvalReplies != 1 {
				t.Fatalf("got %d mode changes and %d approval replies", modeCalls, approvalReplies)
			}
			record, err := manager.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if effectivePermissionLevel(record) != PermissionLevelYolo || effectiveWorkflowMode(record) != WorkflowModeDefault || record.AssistantState != string(AssistantStateWorking) {
				t.Fatalf("session did not resume in fully automatic mode: %#v", record)
			}
			// The consumer still passes the old run snapshot to later requests.
			manager.handleDevinPermissionRequest(client, snapshot, run, nil, devinOrdinaryApprovalMessage("88"))
			if _, ok := run.pendingApprovalRequest(); ok {
				t.Fatal("a later request must use the current fully automatic permission")
			}
			last := writer.messages[len(writer.messages)-1]
			outcome := decodeRawObject(decodeRawObject(last.Result)["outcome"])
			if string(last.ID) != "88" || stringValue(outcome["optionId"]) != "allow_once" {
				t.Fatalf("later request was not automatically approved: %#v", last)
			}
		})
	}
}

func TestDevinPermissionRequestHonorsPermissionDowngrade(t *testing.T) {
	manager, session := newDevinSubAgentTestManager(t)
	snapshot := *session
	snapshot.PermissionLevel = string(PermissionLevelYolo)
	if _, err := manager.UpdatePermissionLevel(context.Background(), session.ID, PermissionLevelDefault); err != nil {
		t.Fatal(err)
	}
	client, stdin := newDevinTestClient()
	run := &activeRun{runID: "devin-downgrade", sessionID: session.ID, backend: SessionBackendDevinACP, agent: AgentDevin}
	manager.handleDevinPermissionRequest(client, snapshot, run, nil, devinOrdinaryApprovalMessage("99"))
	if _, ok := run.pendingApprovalRequest(); !ok || stdin.Len() != 0 {
		t.Fatal("a stale fully automatic snapshot must not approve after permissions are lowered")
	}
}
