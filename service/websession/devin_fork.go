package websession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"code-kanban/model"
	"code-kanban/model/tables"
	"code-kanban/utils"

	"gorm.io/gorm"
)

// Devin's private ACP revert extension backs session forking (the CLI's
// /fork [step]). It is only available when the client opts in via
// clientCapabilities._meta["cognition.ai/revert"] and the agent advertises
// the same key in agentCapabilities._meta.
const (
	devinRevertListStepsMethod = "_cognition.ai/revert/listSteps"
	devinRevertForkMethod      = "_cognition.ai/revert/forkFromStep"

	devinForkOperationTimeout = 3 * time.Minute
	devinForkProbeTimeout     = 20 * time.Second
)

var (
	ErrDevinForkUnsupported     = errors.New("the installed Devin CLI does not support session forking")
	ErrDevinForkSessionActive   = errors.New("stop the current run before forking the session")
	ErrDevinForkPendingInput    = errors.New("send or discard pending messages before forking the session")
	ErrDevinForkTargetNotFound  = errors.New("fork target message not found")
	ErrDevinForkHistoryConflict = errors.New("session history changed; refresh and try again")

	errDevinForkHistoryUnavailable = errors.New("forked Devin session history is unavailable")
)

// probeDevinACPSessionFork reports whether the installed Devin CLI's ACP
// agent advertises the private revert extension (_meta["cognition.ai/revert"])
// that backs session forking. Best-effort: any failure reports no support.
func (m *Manager) probeDevinACPSessionFork() bool {
	ctx, cancel := context.WithTimeout(context.Background(), devinForkProbeTimeout)
	defer cancel()
	client, err := startDevinACP(ctx, m.cfg.DevinPath, "", "")
	if err != nil {
		return false
	}
	defer client.close()
	result, err := client.request(ctx, "initialize", m.devinACPInitializeParams())
	if err != nil {
		return false
	}
	var initializeResult map[string]any
	if json.Unmarshal(result, &initializeResult) != nil {
		return false
	}
	return devinAgentSupportsRevert(initializeResult["agentCapabilities"])
}

// devinRevertStep mirrors zRevertStepInfo from the Devin ACP schema.
type devinRevertStep struct {
	StepID             string  `json:"stepId"`
	StepNumber         uint32  `json:"stepNumber"`
	Kind               string  `json:"kind"` // "prompt" | "questionAnswer"
	UserMessageID      string  `json:"userMessageId"`
	ToolCallID         string  `json:"toolCallId"`
	QuestionNodeID     *uint32 `json:"questionNodeId"`
	RevertTargetNodeID *uint32 `json:"revertTargetNodeId"`
	ForkTargetNodeID   *uint32 `json:"forkTargetNodeId"`
	Summary            string  `json:"summary"`
}

// ForkDevinSession forks the native Devin session at the selected user
// message and creates a new web session bound to the forked native session.
// The source session is never mutated.
func (m *Manager) ForkDevinSession(ctx context.Context, sessionID, itemID string) (SessionSnapshot, error) {
	sessionID = strings.TrimSpace(sessionID)
	itemID = strings.TrimSpace(itemID)
	if sessionID == "" || itemID == "" {
		return SessionSnapshot{}, ErrDevinForkTargetNotFound
	}

	dispatchLock := &m.sessionDispatchLocks[sessionRevisionLockIndex(sessionID)]
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	source, err := m.GetSession(ctx, sessionID)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if normalizeAgent(Agent(source.Agent)) != AgentDevin ||
		effectiveSessionBackend(source) != SessionBackendDevinACP {
		return SessionSnapshot{}, ErrDevinForkUnsupported
	}
	nativeSessionID := pointerString(source.NativeSessionID)
	if nativeSessionID == "" {
		return SessionSnapshot{}, ErrDevinForkUnsupported
	}
	if m.hasActiveRun(source.ID) {
		return SessionSnapshot{}, ErrDevinForkSessionActive
	}
	switch effectiveStatus(source, effectiveAssistantState(source)) {
	case StatusRunning, StatusWaitingApproval, StatusAborting:
		return SessionSnapshot{}, ErrDevinForkSessionActive
	}
	if len(m.pendingInputsDisplaySnapshot(source.ID)) > 0 {
		return SessionSnapshot{}, ErrDevinForkPendingInput
	}

	target, err := m.findHistoryItemByID(ctx, source.ID, itemID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SessionSnapshot{}, ErrDevinForkTargetNotFound
		}
		return SessionSnapshot{}, err
	}
	if target.Kind != "user" {
		return SessionSnapshot{}, ErrDevinForkTargetNotFound
	}
	userOrdinal, err := m.devinForkUserOrdinal(ctx, source.ID, target)
	if err != nil {
		return SessionSnapshot{}, err
	}

	forkCtx, cancel := context.WithTimeout(ctx, devinForkOperationTimeout)
	defer cancel()

	forkedSessionID, err := m.runDevinNativeFork(forkCtx, source, nativeSessionID, target, userOrdinal)
	if err != nil {
		return SessionSnapshot{}, err
	}

	branch, err := m.createDevinForkSession(ctx, source, forkedSessionID, target)
	if err != nil {
		return SessionSnapshot{}, err
	}
	if err := m.hydrateDevinForkHistory(forkCtx, source, branch, target); err != nil {
		_ = m.DeleteSession(context.Background(), branch.ID)
		return SessionSnapshot{}, err
	}
	return m.Snapshot(ctx, branch.ID, DefaultHistoryWindow)
}

// devinForkUserOrdinal returns how many user messages precede the target in
// the local history — the expected index of the matching native prompt step.
func (m *Manager) devinForkUserOrdinal(ctx context.Context, sessionID string, target HistoryItem) (int, error) {
	db := model.GetDB()
	if db == nil {
		return 0, model.ErrDBNotInitialized
	}
	var count int64
	if err := db.WithContext(ctx).
		Model(&tables.WebSessionItemTable{}).
		Where(
			"web_session_id = ? AND item_kind = ? AND order_index < ?",
			sessionID, "user", target.OrderIndex,
		).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

// runDevinNativeFork performs the extension handshake on a dedicated ACP
// process: initialize, listSteps (attaching the source session only if the
// extension requires it), then forkFromStep. It returns the forked native
// session id.
func (m *Manager) runDevinNativeFork(
	ctx context.Context,
	source tables.WebSessionTable,
	nativeSessionID string,
	target HistoryItem,
	userOrdinal int,
) (string, error) {
	client, err := startDevinACP(ctx, m.cfg.DevinPath, source.Cwd, source.Model)
	if err != nil {
		return "", fmt.Errorf("failed to start Devin ACP: %w", err)
	}
	defer client.close()

	initialize, err := client.request(ctx, "initialize", m.devinACPInitializeParams())
	if err != nil {
		return "", fmt.Errorf("Devin ACP initialize failed: %w", err)
	}
	var initializeResult map[string]any
	_ = json.Unmarshal(initialize, &initializeResult)
	agentCapabilities := initializeResult["agentCapabilities"]
	if !devinAgentSupportsRevert(agentCapabilities) {
		return "", ErrDevinForkUnsupported
	}

	steps, err := m.listDevinRevertSteps(ctx, client, nativeSessionID)
	if err != nil {
		// The extension may require the session to be attached to this ACP
		// process before it can enumerate steps.
		if attachErr := m.attachDevinACPSession(ctx, client, source, nativeSessionID, agentCapabilities); attachErr != nil {
			return "", fmt.Errorf("Devin ACP listSteps failed: %w", err)
		}
		steps, err = m.listDevinRevertSteps(ctx, client, nativeSessionID)
		if err != nil {
			return "", fmt.Errorf("Devin ACP listSteps failed: %w", err)
		}
	}

	step, err := matchDevinForkStep(steps, target, userOrdinal)
	if err != nil {
		return "", err
	}
	if step.ForkTargetNodeID == nil {
		return "", ErrDevinForkTargetNotFound
	}

	result, err := client.request(ctx, devinRevertForkMethod, map[string]any{
		"sessionId":    nativeSessionID,
		"targetNodeId": *step.ForkTargetNodeID,
	})
	if err != nil {
		return "", fmt.Errorf("Devin ACP fork failed: %w", err)
	}
	var forked struct {
		ForkedSessionID string `json:"forkedSessionId"`
	}
	if err := json.Unmarshal(result, &forked); err != nil || strings.TrimSpace(forked.ForkedSessionID) == "" {
		return "", errors.New("Devin ACP fork returned no forkedSessionId")
	}
	return strings.TrimSpace(forked.ForkedSessionID), nil
}

func (m *Manager) listDevinRevertSteps(ctx context.Context, client *devinACPClient, nativeSessionID string) ([]devinRevertStep, error) {
	result, err := client.request(ctx, devinRevertListStepsMethod, map[string]any{
		"sessionId": nativeSessionID,
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Steps []devinRevertStep `json:"steps"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, err
	}
	return response.Steps, nil
}

// attachDevinACPSession attaches an existing native session to a fork-scoped
// ACP process. session/resume is preferred (no history replay); session/load
// falls back while its replayed updates are drained and dropped.
func (m *Manager) attachDevinACPSession(
	ctx context.Context,
	client *devinACPClient,
	session tables.WebSessionTable,
	nativeSessionID string,
	agentCapabilities any,
) error {
	if sessionCapabilitiesContain(agentCapabilities, "resume") {
		if _, err := client.request(ctx, "session/resume", map[string]any{
			"sessionId":  nativeSessionID,
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		}); err == nil {
			return nil
		}
	}
	if !capabilitiesContain(agentCapabilities, "loadSession") {
		return errors.New("Devin ACP agent cannot attach an existing session")
	}
	done := make(chan error, 1)
	go func() {
		_, err := client.request(ctx, "session/load", map[string]any{
			"sessionId":  nativeSessionID,
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		})
		done <- err
	}()
	run := &activeRun{sessionID: session.ID, runID: "devin-attach"}
	proj := newDevinRunProjection()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			for {
				select {
				case message, ok := <-client.events:
					if !ok {
						return err
					}
					m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchDetached)
				default:
					return err
				}
			}
		case message, ok := <-client.events:
			if !ok {
				return errors.New("Devin ACP process closed")
			}
			m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchDetached)
		}
	}
}

// matchDevinForkStep resolves the native prompt step for the selected
// history item. The ordinal position among kind=="prompt" steps is checked
// first; a unique text match is the fallback because local history can drift
// from native steps (e.g. prompts that failed after msg_u was persisted).
func matchDevinForkStep(steps []devinRevertStep, target HistoryItem, userOrdinal int) (devinRevertStep, error) {
	prompts := make([]devinRevertStep, 0, len(steps))
	for _, step := range steps {
		if step.Kind == "prompt" {
			prompts = append(prompts, step)
		}
	}
	if len(prompts) == 0 {
		return devinRevertStep{}, ErrDevinForkHistoryConflict
	}
	if userOrdinal >= 0 && userOrdinal < len(prompts) {
		candidate := prompts[userOrdinal]
		if devinStepStrongMatch(candidate, target.Text) || strings.TrimSpace(candidate.Summary) == "" {
			return candidate, nil
		}
	}
	matched := -1
	for index, step := range prompts {
		if !devinStepStrongMatch(step, target.Text) {
			continue
		}
		if matched >= 0 {
			return devinRevertStep{}, ErrDevinForkHistoryConflict
		}
		matched = index
	}
	if matched < 0 {
		return devinRevertStep{}, ErrDevinForkHistoryConflict
	}
	return prompts[matched], nil
}

// devinStepStrongMatch compares a step summary against the stored message
// text. Summaries may be truncated, so a prefix relation in either direction
// counts; both sides must be non-empty.
func devinStepStrongMatch(step devinRevertStep, text string) bool {
	summary := normalizeDevinStepText(step.Summary)
	normalized := normalizeDevinStepText(text)
	if summary == "" || normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, summary) || strings.HasPrefix(summary, normalized)
}

func normalizeDevinStepText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// createDevinForkSession creates the branch web session that wraps the
// forked native session, inheriting the source session's settings.
func (m *Manager) createDevinForkSession(
	ctx context.Context,
	source tables.WebSessionTable,
	forkedSessionID string,
	target HistoryItem,
) (tables.WebSessionTable, error) {
	title := deriveAutoTitleFromMessage(target.Text)
	if title == "" {
		title = source.Title
	}
	worktreeID := ""
	if source.WorktreeID != nil {
		worktreeID = strings.TrimSpace(*source.WorktreeID)
	}
	summary, err := m.CreateSession(ctx, CreateParams{
		ProjectID:                         source.ProjectID,
		WorktreeID:                        worktreeID,
		Agent:                             AgentDevin,
		Backend:                           SessionBackendDevinACP,
		Model:                             source.Model,
		ReasoningEffort:                   ReasoningEffort(source.ReasoningEffort),
		WorkflowMode:                      effectiveWorkflowMode(source),
		PermissionLevel:                   effectivePermissionLevel(source),
		ActiveCallTimeoutEnabled:          source.ActiveCallTimeoutEnabled,
		ContextWindowSetting:              ptr(source.ContextWindowSetting),
		AutoRetryEnabled:                  source.AutoRetryEnabled,
		AutoRetryPolicyMode:               ptr(normalizeAutoRetryPolicyMode(AutoRetryPolicyMode(source.AutoRetryPolicyMode))),
		AutoRetryScope:                    ptr(normalizeAutoRetryScope(AutoRetryScope(source.AutoRetryScope))),
		AutoRetryPreset:                   ptr(normalizeAutoRetryPreset(AutoRetryPreset(source.AutoRetryPreset))),
		AutoRetryMaxAttempts:              ptr(normalizeAutoRetryMaxAttempts(source.AutoRetryMaxAttempts)),
		AutoRetryDispatchPendingOnFailure: ptr(source.AutoRetryDispatchPendingOnFailure),
		Title:                             title,
	})
	if err != nil {
		return tables.WebSessionTable{}, err
	}
	if err := m.updateRuntimeState(ctx, summary.ID, map[string]any{
		"native_session_id": forkedSessionID,
		"updated_at":        time.Now(),
	}); err != nil {
		_ = m.DeleteSession(context.Background(), summary.ID)
		return tables.WebSessionTable{}, err
	}
	return m.GetSession(ctx, summary.ID)
}

// hydrateDevinForkHistory materializes the branch's history. The native
// replay of the forked session is the authoritative source; when it cannot
// be captured, the local prefix through the fork point is copied instead so
// the branch never opens empty.
func (m *Manager) hydrateDevinForkHistory(
	ctx context.Context,
	source tables.WebSessionTable,
	branch tables.WebSessionTable,
	target HistoryItem,
) error {
	userCount, err := m.captureDevinForkedHistory(ctx, branch)
	if err == nil && userCount > 0 {
		return nil
	}
	if copyErr := m.copyDevinForkHistoryPrefix(ctx, source, branch, target.OrderIndex); copyErr != nil {
		if err != nil {
			return err
		}
		return fmt.Errorf("forked Devin session replayed no user messages and history copy failed: %w", copyErr)
	}
	return nil
}

// captureDevinForkedHistory loads the forked native session on a dedicated
// ACP process and projects the replayed session/update notifications into
// the branch's history. It returns the number of captured user messages.
func (m *Manager) captureDevinForkedHistory(ctx context.Context, branch tables.WebSessionTable) (int, error) {
	nativeSessionID := pointerString(branch.NativeSessionID)
	if nativeSessionID == "" {
		return 0, errDevinForkHistoryUnavailable
	}
	client, err := startDevinACP(ctx, m.cfg.DevinPath, branch.Cwd, branch.Model)
	if err != nil {
		return 0, fmt.Errorf("failed to start Devin ACP: %w", err)
	}
	defer client.close()

	initialize, err := client.request(ctx, "initialize", m.devinACPInitializeParams())
	if err != nil {
		return 0, fmt.Errorf("Devin ACP initialize failed: %w", err)
	}
	var initializeResult map[string]any
	_ = json.Unmarshal(initialize, &initializeResult)
	agentCapabilities := initializeResult["agentCapabilities"]
	if !capabilitiesContain(agentCapabilities, "loadSession") {
		return 0, errDevinForkHistoryUnavailable
	}

	proj := newDevinRunProjection()
	proj.captureHistory = true
	// Each replayed turn gets its own run id so captured items group the same
	// way live runs do. currentRun is swapped at each user-message boundary.
	currentRun := &activeRun{sessionID: branch.ID, runID: utils.NewID()}
	userCount := 0
	flushUser := func() {
		text := proj.takeDevinCapturedUserText()
		if strings.TrimSpace(text) == "" {
			return
		}
		m.closeDevinMessage(branch, currentRun, proj)
		currentRun = &activeRun{sessionID: branch.ID, runID: utils.NewID()}
		userCount++
		m.emitDevinCapturedUserMessage(branch, currentRun, text)
	}
	processMessage := func(message devinACPMessage) {
		if message.Method == "session/update" && devinACPSessionUpdateKind(message.Params) != "user_message_chunk" {
			flushUser()
		}
		m.dispatchDevinACPMessage(client, branch, currentRun, proj, message, devinACPDispatchCapture)
	}
	finish := func() {
		flushUser()
		m.closeDevinMessage(branch, currentRun, proj)
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.request(ctx, "session/load", map[string]any{
			"sessionId":  nativeSessionID,
			"cwd":        branch.Cwd,
			"mcpServers": []any{},
		})
		done <- err
	}()
	for {
		select {
		case <-ctx.Done():
			return userCount, ctx.Err()
		case err := <-done:
			// Every replayed notification precedes the response on the wire;
			// drain whatever remains queued, then finish.
			for {
				select {
				case message, ok := <-client.events:
					if !ok {
						finish()
						return userCount, err
					}
					processMessage(message)
				default:
					finish()
					return userCount, err
				}
			}
		case message, ok := <-client.events:
			if !ok {
				finish()
				return userCount, errors.New("Devin ACP process closed")
			}
			processMessage(message)
		}
	}
}

// emitDevinCapturedUserMessage appends a replayed user message as a msg_u
// event, mirroring the shape sendMessageInternal persists for live sends.
func (m *Manager) emitDevinCapturedUserMessage(session tables.WebSessionTable, run *activeRun, text string) {
	messageID := utils.NewID()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID:        utils.NewID(),
		Type:      "msg_u",
		RunID:     run.runID,
		ParentID:  messageID,
		Timestamp: time.Now(),
		Payload: map[string]any{
			"mid": messageID,
			"txt": text,
		},
	})
}

// copyDevinForkHistoryPrefix copies the source history through the selected
// message (inclusive) into the branch. Used when the forked native session's
// replay cannot be captured.
func (m *Manager) copyDevinForkHistoryPrefix(
	ctx context.Context,
	source tables.WebSessionTable,
	branch tables.WebSessionTable,
	targetOrder int64,
) error {
	db := model.GetDB()
	if db == nil {
		return model.ErrDBNotInitialized
	}
	var sourceItems []tables.WebSessionItemTable
	if err := db.WithContext(ctx).
		Where("web_session_id = ? AND order_index <= ?", source.ID, targetOrder).
		Order("order_index ASC").
		Find(&sourceItems).Error; err != nil {
		return err
	}

	itemRows := make([]tables.WebSessionItemTable, 0, len(sourceItems))
	for _, sourceItem := range sourceItems {
		row := sourceItem
		row.ID = ""
		row.CreatedAt = time.Time{}
		row.UpdatedAt = time.Time{}
		row.DeletedAt = gorm.DeletedAt{}
		row.WebSessionID = branch.ID
		row.WebTurnID = nil
		row.Init()
		itemRows = append(itemRows, row)
	}

	return m.replaceSessionHistoryCache(ctx, branch, nil, itemRows, map[string]any{
		"turn_count": 0,
		"item_count": len(itemRows),
		"sync_state": SyncStateFresh,
		"updated_at": time.Now(),
	})
}
