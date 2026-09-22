package websession

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

type claudeHookAnswerFile struct {
	Answers            map[string]string `json:"answers,omitempty"`
	PermissionDecision string            `json:"permissionDecision,omitempty"`
}

type claudePreToolUseRequest struct {
	SessionID string         `json:"session_id"`
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	Cwd       string         `json:"cwd"`
}

type claudePreToolUseResponse struct {
	HookSpecificOutput claudePreToolUseHookSpecificOutput `json:"hookSpecificOutput"`
}

type claudePreToolUseHookSpecificOutput struct {
	HookEventName            string         `json:"hookEventName"`
	PermissionDecision       string         `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string         `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]any `json:"updatedInput,omitempty"`
}

func claudeAskUserUpdatedInput(
	toolInput map[string]any,
	questions []toolRequestQuestion,
	answers map[string][]string,
) map[string]any {
	updated := cloneMap(toolInput)
	if updated == nil {
		updated = map[string]any{}
	}
	answerMap := make(map[string]string)
	for _, question := range questions {
		questionID := strings.TrimSpace(question.ID)
		values := answers[questionID]
		if len(values) == 0 {
			continue
		}
		key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header, questionID))
		if key == "" {
			continue
		}
		answerMap[key] = strings.Join(values, ", ")
	}
	updated["answers"] = answerMap
	return updated
}

func (m *Manager) writeClaudeHookAnswer(
	sessionID string,
	toolUseID string,
	payload claudeHookAnswerFile,
) error {
	if m.store == nil {
		return fmt.Errorf("store is not configured")
	}
	if err := os.MkdirAll(m.store.claudeHookDir(sessionID), 0o755); err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return os.WriteFile(m.store.claudeHookAnswerPath(sessionID, toolUseID), encoded, 0o644)
}

func (m *Manager) writeClaudeHookAnswerForSession(
	session tables.WebSessionTable,
	toolUseID string,
	payload claudeHookAnswerFile,
) error {
	if err := m.writeClaudeHookAnswer(session.ID, toolUseID, payload); err != nil {
		return err
	}
	nativeSessionID := ""
	if session.NativeSessionID != nil {
		nativeSessionID = strings.TrimSpace(*session.NativeSessionID)
	}
	if nativeSessionID == "" || nativeSessionID == strings.TrimSpace(session.ID) {
		return nil
	}
	return m.writeClaudeHookAnswer(nativeSessionID, toolUseID, payload)
}

func (m *Manager) readClaudeHookAnswer(sessionID, toolUseID string) (claudeHookAnswerFile, error) {
	data, err := os.ReadFile(m.store.claudeHookAnswerPath(sessionID, toolUseID))
	if err != nil {
		return claudeHookAnswerFile{}, err
	}
	var payload claudeHookAnswerFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return claudeHookAnswerFile{}, err
	}
	return payload, nil
}

func (m *Manager) deleteClaudeHookAnswer(sessionID, toolUseID string) {
	if m.store == nil {
		return
	}
	_ = os.Remove(m.store.claudeHookAnswerPath(sessionID, toolUseID))
}

func (m *Manager) deleteClaudeHookAnswerForSession(session tables.WebSessionTable, toolUseID string) {
	m.deleteClaudeHookAnswer(session.ID, toolUseID)
	if session.NativeSessionID == nil {
		return
	}
	nativeSessionID := strings.TrimSpace(*session.NativeSessionID)
	if nativeSessionID == "" || nativeSessionID == strings.TrimSpace(session.ID) {
		return
	}
	m.deleteClaudeHookAnswer(nativeSessionID, toolUseID)
}

func (m *Manager) ensureClaudeHookServer() (string, error) {
	m.claudeHookOnce.Do(func() {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			m.claudeHookErr = err
			return
		}
		m.claudeHookToken = utils.NewID()
		mux := http.NewServeMux()
		mux.HandleFunc("/claude-hooks/pre-tool-use", m.handleClaudePreToolUseHook)
		server := &http.Server{Handler: mux}
		m.claudeHookServer = server
		m.claudeHookBaseURL = "http://" + listener.Addr().String()
		m.claudeHookSettingsPath = filepath.Join(m.store.rootDir, "claude-hook-settings.json")

		settings := m.claudeHookSettings()
		encoded, err := json.Marshal(settings)
		if err != nil {
			_ = listener.Close()
			m.claudeHookErr = err
			return
		}
		if err := os.WriteFile(m.claudeHookSettingsPath, encoded, 0o644); err != nil {
			_ = listener.Close()
			m.claudeHookErr = err
			return
		}
		go func() {
			_ = server.Serve(listener)
		}()
	})
	if m.claudeHookErr != nil {
		return "", m.claudeHookErr
	}
	return m.claudeHookSettingsPath, nil
}

func (m *Manager) claudeHookSettings() map[string]any {
	hookURL := codeKanbanClaudeHookURL(m.claudeHookBaseURL)
	return map[string]any{
		"allowedHttpHookUrls": []string{
			hookURL,
		},
		"hooks": map[string]any{
			"PreToolUse": codeKanbanClaudeHookEntries(m.claudeHookBaseURL, m.claudeHookToken),
		},
	}
}

func codeKanbanClaudeHookURL(baseURL string) string {
	return baseURL + "/claude-hooks/pre-tool-use"
}

func codeKanbanClaudeHookEntries(baseURL, token string) []map[string]any {
	hookURL := codeKanbanClaudeHookURL(baseURL)
	return []map[string]any{
		{
			"matcher": "AskUserQuestion",
			"hooks": []map[string]any{
				{
					"type": "http",
					"url":  hookURL,
					"headers": map[string]any{
						"Authorization": "Bearer " + token,
					},
				},
			},
		},
		{
			"matcher": "ExitPlanMode",
			"hooks": []map[string]any{
				{
					"type": "http",
					"url":  hookURL,
					"headers": map[string]any{
						"Authorization": "Bearer " + token,
					},
				},
			},
		},
	}
}

func (m *Manager) handleClaudePreToolUseHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); token == "" || token != m.claudeHookToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()

	var request claudePreToolUseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	toolName := strings.TrimSpace(request.ToolName)
	if toolName != "AskUserQuestion" && toolName != "ExitPlanMode" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if m.shouldBypassClaudeHook(request.SessionID, request.Cwd) {
		// The live --permission-prompt-tool stdio bridge owns this decision.
		// An HTTP hook response here would turn the request into the legacy
		// deferred/resume flow before Claude can emit control_request.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response := claudePreToolUseResponse{
		HookSpecificOutput: claudePreToolUseHookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "defer",
		},
	}
	if answerFile, err := m.readClaudeHookAnswer(strings.TrimSpace(request.SessionID), strings.TrimSpace(request.ToolUseID)); err == nil {
		if toolName == "ExitPlanMode" && strings.TrimSpace(answerFile.PermissionDecision) != "" {
			response.HookSpecificOutput.PermissionDecision = strings.TrimSpace(answerFile.PermissionDecision)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		if toolName != "AskUserQuestion" || len(answerFile.Answers) == 0 {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		answers := make(map[string][]string, len(answerFile.Answers))
		questions := decodeToolQuestions(request.ToolInput["questions"])
		for _, question := range questions {
			key := strings.TrimSpace(firstNonEmpty(question.Question, question.Header))
			if key == "" {
				continue
			}
			value := strings.TrimSpace(answerFile.Answers[key])
			if value == "" {
				continue
			}
			answers[firstNonEmpty(question.ID, question.Question, question.Header)] = []string{value}
		}
		response.HookSpecificOutput.PermissionDecision = "allow"
		response.HookSpecificOutput.UpdatedInput = claudeAskUserUpdatedInput(request.ToolInput, questions, answers)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
