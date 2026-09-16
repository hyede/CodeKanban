package websession

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code-kanban/model/tables"
	"code-kanban/utils"
)

const devinACPProtocolVersion = 1

type devinACPMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *devinACPError  `json:"error,omitempty"`
}

type devinACPError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *devinACPError) Error() string {
	if e == nil {
		return "Devin ACP error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("Devin ACP error %d", e.Code)
}

type devinACPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	ctx    context.Context

	writeMu   sync.Mutex
	pending   map[string]chan devinACPMessage
	pendingMu sync.Mutex
	events    chan devinACPMessage
	closed    chan struct{}
	closeOnce sync.Once
	seq       atomic.Uint64

	terminalMu sync.Mutex
	terminals  map[string]*devinACPTerminal

	// shell is the resolved shell command (binary + startup args) used to run
	// terminal/create commands, mirroring how interactive terminals resolve it.
	shell []string
}

type devinACPTerminal struct {
	id       string
	cmd      *exec.Cmd
	outputMu sync.Mutex
	output   bytes.Buffer
	done     chan struct{}
	exitCode *int
	exitErr  error
}

type devinACPTerminalWriter struct{ terminal *devinACPTerminal }

func (w devinACPTerminalWriter) Write(data []byte) (int, error) {
	w.terminal.outputMu.Lock()
	defer w.terminal.outputMu.Unlock()
	return w.terminal.output.Write(data)
}

func startDevinACP(ctx context.Context, path, cwd, model string) (*devinACPClient, error) {
	parts := splitCommandParts(path)
	if len(parts) == 0 {
		return nil, errors.New("Devin CLI path is empty")
	}
	args := append(parts[1:], "acp")
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	client := &devinACPClient{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		ctx:       ctx,
		pending:   make(map[string]chan devinACPMessage),
		events:    make(chan devinACPMessage, 64),
		closed:    make(chan struct{}),
		terminals: make(map[string]*devinACPTerminal),
	}
	// Drain stderr independently so diagnostic output cannot block the ACP
	// process while the session is streaming events.
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go client.readLoop()
	return client, nil
}

func (c *devinACPClient) readLoop() {
	defer close(c.events)
	defer close(c.closed)
	reader := bufio.NewReaderSize(c.stdout, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var message devinACPMessage
			if decodeErr := json.Unmarshal(line, &message); decodeErr != nil {
				continue
			}
			if key := devinACPIDKey(message.ID); key != "" && message.Method == "" {
				c.pendingMu.Lock()
				response := c.pending[key]
				delete(c.pending, key)
				c.pendingMu.Unlock()
				if response != nil {
					response <- message
					close(response)
				}
			} else {
				select {
				case c.events <- message:
				case <-c.ctx.Done():
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func devinACPIDKey(raw json.RawMessage) string {
	return strings.TrimSpace(string(raw))
}

func (c *devinACPClient) send(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (c *devinACPClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.seq.Add(1)
	rawID := json.RawMessage(strconv.FormatUint(id, 10))
	response := make(chan devinACPMessage, 1)
	key := devinACPIDKey(rawID)
	c.pendingMu.Lock()
	c.pending[key] = response
	c.pendingMu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return nil, err
	}
	select {
	case message := <-response:
		if message.Error != nil {
			return nil, message.Error
		}
		return message.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, key)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closed:
		return nil, errors.New("Devin ACP process closed")
	}
}

func (c *devinACPClient) respond(id json.RawMessage, result any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result})
}

func (c *devinACPClient) respondError(id json.RawMessage, code int, message string) error {
	return c.send(map[string]any{
		"jsonrpc": "2.0", "id": json.RawMessage(id),
		"error": map[string]any{"code": code, "message": message},
	})
}

func (c *devinACPClient) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *devinACPClient) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		_ = c.stdin.Close()
		for _, terminal := range c.terminalsSnapshot() {
			if terminal.cmd.Process != nil {
				_ = terminal.cmd.Process.Kill()
			}
		}
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	})
}

func (c *devinACPClient) terminalsSnapshot() []*devinACPTerminal {
	c.terminalMu.Lock()
	defer c.terminalMu.Unlock()
	items := make([]*devinACPTerminal, 0, len(c.terminals))
	for _, terminal := range c.terminals {
		items = append(items, terminal)
	}
	return items
}

func (m *Manager) runDevinACPSession(ctx context.Context, run *activeRun, session tables.WebSessionTable, text string, attachments []Attachment) {
	var images []piRPCImage
	if len(attachments) > 0 {
		if known, supports := m.devinModelSupportsImages(session.Model); known && !supports {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin model %s does not support image attachments", session.Model))
			return
		}
		var err error
		images, err = m.piPromptImages(attachments)
		if err != nil {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP attachments: %w", err))
			return
		}
	}
	client, err := startDevinACP(ctx, m.cfg.DevinPath, session.Cwd, session.Model)
	if err != nil {
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("failed to start Devin ACP: %w", err))
		return
	}
	client.shell, _ = utils.ResolveShellCommand("", m.terminalShellConfig())
	run.setDevinACP(client)
	run.setCommand(client.cmd)
	defer client.close()

	initialize, err := client.request(ctx, "initialize", m.devinACPInitializeParams())
	if err != nil {
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP initialize failed: %w", err))
		return
	}
	var initializeResult map[string]any
	_ = json.Unmarshal(initialize, &initializeResult)

	proj := newDevinRunProjection()
	nativeSessionID := pointerString(session.NativeSessionID)
	if nativeSessionID == "" {
		result, requestErr := client.request(ctx, "session/new", map[string]any{
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		})
		if requestErr != nil {
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/new failed: %w", requestErr))
			return
		}
		var created struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(result, &created); err != nil || strings.TrimSpace(created.SessionID) == "" {
			m.handleRunFailure(session.ID, session, run, errors.New("Devin ACP session/new returned no sessionId"))
			return
		}
		nativeSessionID = created.SessionID
		session.NativeSessionID = &nativeSessionID
		_ = m.updateRuntimeState(context.Background(), session.ID, map[string]any{
			"native_session_id": nativeSessionID,
			"updated_at":        time.Now(),
		})
	} else {
		agentCapabilities := initializeResult["agentCapabilities"]
		resumed := false
		var resumeErr error
		if sessionCapabilitiesContain(agentCapabilities, "resume") {
			// session/resume restores context without replaying history.
			if _, requestErr := client.request(ctx, "session/resume", map[string]any{
				"sessionId":  nativeSessionID,
				"cwd":        session.Cwd,
				"mcpServers": []any{},
			}); requestErr == nil {
				resumed = true
			} else {
				resumeErr = requestErr
			}
		}
		switch {
		case resumed:
		case capabilitiesContain(agentCapabilities, "loadSession"):
			if requestErr := m.loadDevinACPSession(ctx, client, session, run, proj, nativeSessionID); requestErr != nil {
				m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/load failed: %w", requestErr))
				return
			}
		case resumeErr != nil:
			m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP session/resume failed: %w", resumeErr))
			return
		}
	}

	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		m.consumeDevinACPEvents(ctx, client, session, run, proj)
	}()

	prompt := make([]map[string]any, 0, 1+len(images))
	if strings.TrimSpace(text) != "" {
		prompt = append(prompt, map[string]any{"type": "text", "text": text})
	}
	for _, image := range images {
		prompt = append(prompt, map[string]any{"type": image.Type, "data": image.Data, "mimeType": image.MimeType})
	}
	_, err = client.request(ctx, "session/prompt", map[string]any{
		"sessionId": nativeSessionID,
		"prompt":    prompt,
	})
	if err != nil {
		if ctx.Err() != nil || run.abortRequestedSnapshot() {
			_ = client.notify("session/cancel", map[string]any{"sessionId": nativeSessionID})
			m.finishAbortedRun(session.ID, session, run)
			return
		}
		m.handleRunFailure(session.ID, session, run, fmt.Errorf("Devin ACP prompt failed: %w", err))
		return
	}
	client.close()
	<-eventsDone
	m.finishDevinRun(session, run, proj)
}

// devinACPInitializeParams builds the ACP initialize request. clientInfo
// mirrors what Devin Desktop's ACP connector sends so the agent treats this
// like a first-party session. The only declared clientCapability is the
// revert extension opt-in under _meta — elicitation/fs stay undeclared, and
// terminal is omitted like the desktop so the agent uses its native tools.
func (m *Manager) devinACPInitializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": devinACPProtocolVersion,
		"clientCapabilities": map[string]any{
			"_meta": map[string]any{
				"cognition.ai/revert": true,
			},
		},
		"clientInfo": map[string]any{
			"name":    "windsurf",
			"version": m.devinACPClientVersion(),
		},
	}
}

// devinAgentSupportsRevert reports whether agentCapabilities advertises the
// private revert extension via _meta["cognition.ai/revert"] === true. The
// agent only enables listSteps/forkFromStep when the client opted in during
// initialize, which devinACPInitializeParams does.
func devinAgentSupportsRevert(raw any) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	meta, ok := values["_meta"].(map[string]any)
	if !ok {
		return false
	}
	enabled, ok := meta["cognition.ai/revert"].(bool)
	return ok && enabled
}

var devinClientVersion struct {
	once    sync.Once
	version string
}

// devinACPClientVersion reports the Devin CLI version, probed once per process
// and sent as clientInfo.version in the ACP initialize handshake.
func (m *Manager) devinACPClientVersion() string {
	devinClientVersion.once.Do(func() {
		if version := detectDevinVersion(m.cfg.DevinPath); version != nil {
			devinClientVersion.version = *version
		}
	})
	return devinClientVersion.version
}

func (m *Manager) terminalShellConfig() utils.TerminalShellConfig {
	if m != nil && m.cfg.TerminalShell != nil {
		return m.cfg.TerminalShell()
	}
	return utils.TerminalShellConfig{}
}

// devinModelSupportsImages reports whether the catalog explicitly flags the
// model's image support. It only returns known=true when the catalog provided
// the supports_images flag at all — models without the flag (older CLI output
// or unexpected layouts) are treated as unknown so the agent decides.
func (m *Manager) devinModelSupportsImages(model string) (known bool, supports bool) {
	model = strings.TrimSpace(model)
	for _, info := range m.getDevinModelCatalog(false) {
		if info.Model == model && info.supportsImagesSet {
			return true, info.SupportsImages
		}
	}
	return false, false
}

func capabilitiesContain(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	value, ok := values[key]
	return ok && value != nil
}

// sessionCapabilitiesContain reports whether agentCapabilities advertises the
// named sessionCapabilities entry (e.g. "resume").
func sessionCapabilitiesContain(raw any, key string) bool {
	values, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	scoped, ok := values["sessionCapabilities"].(map[string]any)
	if !ok {
		return false
	}
	value, ok := scoped[key]
	return ok && value != nil
}

// loadDevinACPSession performs session/load while draining client.events. Per
// the ACP spec the agent replays the entire conversation as session/update
// notifications before responding; those replays are discarded here so prior
// replies are not appended to the run again. Draining during the call also
// keeps the buffered events channel from filling up and deadlocking the load.
func (m *Manager) loadDevinACPSession(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, nativeSessionID string) error {
	done := make(chan error, 1)
	go func() {
		_, err := client.request(ctx, "session/load", map[string]any{
			"sessionId":  nativeSessionID,
			"cwd":        session.Cwd,
			"mcpServers": []any{},
		})
		done <- err
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			// The response is ordered after every replayed notification on
			// the wire, so whatever is still queued is history — discard it.
			for {
				select {
				case message, ok := <-client.events:
					if !ok {
						return err
					}
					m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchReplay)
				default:
					return err
				}
			}
		case message, ok := <-client.events:
			if !ok {
				return errors.New("Devin ACP process closed")
			}
			m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchReplay)
		}
	}
}

func (m *Manager) consumeDevinACPEvents(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-client.events:
			if !ok {
				return
			}
			m.dispatchDevinACPMessage(client, session, run, proj, message, devinACPDispatchLive)
		}
	}
}

// devinACPDispatchMode controls how dispatchDevinACPMessage treats incoming
// agent→client traffic outside of an ordinary live run.
type devinACPDispatchMode int

const (
	// devinACPDispatchLive projects session/update into history and answers
	// terminal/permission requests normally.
	devinACPDispatchLive devinACPDispatchMode = iota
	// devinACPDispatchReplay drops session/update (session/load history
	// replay — codekanban already persisted those turns) but still answers
	// terminal/permission requests on the attached run.
	devinACPDispatchReplay
	// devinACPDispatchDetached drops session/update and refuses terminal and
	// permission requests: the ACP process is attached to a session only to
	// issue extension calls, so no operator exists to service requests.
	devinACPDispatchDetached
	// devinACPDispatchCapture projects session/update into history (used to
	// materialize a forked session's replay) while refusing terminal and
	// permission requests — replaying history must not execute commands or
	// block on prompts.
	devinACPDispatchCapture
)

// dispatchDevinACPMessage routes one agent→client message.
func (m *Manager) dispatchDevinACPMessage(client *devinACPClient, session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, message devinACPMessage, mode devinACPDispatchMode) {
	if message.Method == "session/update" {
		if mode == devinACPDispatchLive || mode == devinACPDispatchCapture {
			m.handleDevinACPUpdate(session, run, proj, message.Params)
		}
		return
	}
	if message.Method == "session/request_permission" {
		if mode == devinACPDispatchLive || mode == devinACPDispatchReplay {
			m.handleDevinPermissionRequest(client, session, run, message)
		} else if message.ID != nil {
			_ = client.respond(message.ID, map[string]any{"outcome": map[string]any{"outcome": "cancelled"}})
		}
		return
	}
	if strings.HasPrefix(message.Method, "terminal/") {
		if mode == devinACPDispatchLive || mode == devinACPDispatchReplay {
			m.handleDevinTerminalRequest(client, session, run, message)
		} else if message.ID != nil {
			_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
		}
		return
	}
	if message.ID != nil && strings.TrimSpace(message.Method) != "" {
		_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
	}
}

func (m *Manager) handleDevinTerminalRequest(client *devinACPClient, session tables.WebSessionTable, run *activeRun, message devinACPMessage) {
	var params map[string]any
	if err := json.Unmarshal(message.Params, &params); err != nil {
		_ = client.respondError(message.ID, -32602, err.Error())
		return
	}
	var result map[string]any
	var err error
	switch message.Method {
	case "terminal/create":
		result, err = client.terminalCreate(params, session.Cwd)
	case "terminal/output":
		result, err = client.terminalOutput(params, false)
	case "terminal/wait_for_exit":
		result, err = client.terminalOutput(params, true)
	case "terminal/kill":
		err = client.terminalKill(params)
		result = map[string]any{}
	case "terminal/release":
		err = client.terminalRelease(params)
		result = map[string]any{}
	default:
		_ = client.respondError(message.ID, -32601, "unsupported method: "+message.Method)
		return
	}
	if err != nil {
		_ = client.respondError(message.ID, -32603, err.Error())
		m.appendRunNote(session.ID, session, run, "warning",
			"Devin terminal request failed: "+err.Error(),
			map[string]any{"method": message.Method, "code": "devin_terminal_error"})
		return
	}
	_ = client.respond(message.ID, result)
}

// devinRunProjection tracks the Devin ACP run's open assistant message and
// thinking/tool state so the projection can emit Pi-shaped events: a single
// assistant message per turn that owns its thinking blocks and tool calls.
type devinRunProjection struct {
	mu             sync.Mutex
	messageID      string // currently open assistant message ("" = none)
	messageHasText bool
	thinkingID     string
	thinkingText   strings.Builder
	thinkingIndex  int
	lastThinkEmit  time.Time
	tools          map[string]*devinToolState // by ACP toolCallId

	// captureHistory marks the projection as materializing a session/load
	// replay (used when hydrating a forked session). In this mode
	// user_message_chunk updates are buffered into pendingUserText so the
	// caller can emit them as msg_u events at message boundaries.
	captureHistory  bool
	pendingUserText strings.Builder
}

type devinToolState struct {
	name     string
	kind     string
	input    any
	parentID string
	meta     map[string]any
}

func newDevinRunProjection() *devinRunProjection {
	return &devinRunProjection{tools: make(map[string]*devinToolState)}
}

// takeDevinCapturedUserText drains user_message_chunk text buffered during
// history capture. Empty means no user message is pending emission.
func (p *devinRunProjection) takeDevinCapturedUserText() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	text := p.pendingUserText.String()
	p.pendingUserText.Reset()
	return text
}

// devinACPSessionUpdateKind extracts update.sessionUpdate from a
// session/update notification payload without projecting it.
func devinACPSessionUpdateKind(raw json.RawMessage) string {
	var payload struct {
		Update struct {
			SessionUpdate string `json:"sessionUpdate"`
		} `json:"update"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return payload.Update.SessionUpdate
}

// devinToolHistoryKind maps the raw ACP tool kind to a history kind the UI
// understands. Unknown kinds fall back to dynamic_tool_call like Pi.
func devinToolHistoryKind(acpKind string) string {
	switch strings.ToLower(strings.TrimSpace(acpKind)) {
	case "execute":
		return "command_execution"
	case "edit", "delete", "move":
		return "file_change"
	case "fetch":
		return "web_search"
	default:
		return "dynamic_tool_call"
	}
}

// devinToolOutputText extracts a displayable tool output from a tool_call_update
// payload. A string rawOutput is used directly; a non-nil non-string value is
// JSON-encoded; otherwise the content blocks (type=="content") are joined.
func devinToolOutputText(update map[string]any) string {
	kind := devinToolHistoryKind(stringValue(update["kind"]))
	switch typed := update["rawOutput"].(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return truncateToolOutput(kind, typed)
		}
	case nil:
		// fall through to the content-block fallback below
	default:
		if encoded, err := json.Marshal(typed); err == nil && len(encoded) > 0 {
			return truncateToolOutput(kind, string(encoded))
		}
	}
	parts := make([]string, 0)
	if items, ok := update["content"].([]any); ok {
		for _, item := range items {
			entry := decodeRawObject(item)
			if !strings.EqualFold(stringValue(entry["type"]), "content") {
				continue
			}
			inner := decodeRawObject(entry["content"])
			if text := strings.TrimSpace(stringValue(inner["text"])); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return truncateToolOutput(kind, strings.Join(parts, "\n"))
}

// ensureDevinMessage opens an assistant message if none is open and returns its
// id. It mirrors startPiAssistantMessage: msg_a_st + run.setAssistantMessageID.
func (m *Manager) ensureDevinMessage(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) string {
	proj.mu.Lock()
	if proj.messageID != "" {
		id := proj.messageID
		proj.mu.Unlock()
		return id
	}
	id := utils.NewID()
	proj.messageID = id
	proj.messageHasText = false
	proj.mu.Unlock()
	run.setAssistantMessageID(id)
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "msg_a_st", RunID: run.runID, ParentID: id,
		Timestamp: time.Now(), Payload: map[string]any{"mid": id},
	})
	return id
}

// finishDevinThinking emits the final reasoning tool_end for an open thinking
// block and resets the thinking state. Text or a tool call ends thinking.
func (m *Manager) finishDevinThinking(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	proj.mu.Lock()
	thinkingID := proj.thinkingID
	if thinkingID == "" {
		proj.mu.Unlock()
		return
	}
	text := proj.thinkingText.String()
	messageID := proj.messageID
	proj.thinkingID = ""
	proj.thinkingText.Reset()
	proj.lastThinkEmit = time.Time{}
	proj.mu.Unlock()
	if messageID == "" {
		messageID = run.assistantMessageIDSnapshot()
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: messageID,
		Timestamp: time.Now(), Payload: map[string]any{
			"tid": thinkingID, "name": "Reasoning", "kind": "reasoning",
			"out": truncateToolOutput("reasoning", text), "ok": true,
		},
	})
}

// closeDevinMessage finishes any open thinking and emits txt_end when the open
// message produced text, then clears the message so the next turn opens a new
// bubble (mirrors Pi: text + tool calls belong to one message).
func (m *Manager) closeDevinMessage(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	m.finishDevinThinking(session, run, proj)
	proj.mu.Lock()
	messageID := proj.messageID
	hasText := proj.messageHasText
	proj.messageID = ""
	proj.messageHasText = false
	proj.mu.Unlock()
	if messageID == "" || !hasText {
		return
	}
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
		ID: utils.NewID(), Type: "txt_end", RunID: run.runID, ParentID: messageID,
		Timestamp: time.Now(), Payload: map[string]any{"mid": messageID},
	})
}

func (m *Manager) handleDevinACPUpdate(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection, raw json.RawMessage) {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return
	}
	update, _ := payload["update"].(map[string]any)
	kind := stringValue(update["sessionUpdate"])
	switch kind {
	case "user_message_chunk":
		// Only history capture consumes replayed user messages; live runs
		// already recorded the user's message when it was sent.
		if !proj.captureHistory {
			return
		}
		content, _ := update["content"].(map[string]any)
		text := firstNonEmpty(stringValue(content["text"]), stringValue(update["text"]))
		proj.mu.Lock()
		proj.pendingUserText.WriteString(text)
		proj.mu.Unlock()
	case "agent_thought_chunk":
		content, _ := update["content"].(map[string]any)
		text := firstNonEmpty(stringValue(content["text"]), stringValue(update["text"]))
		if strings.TrimSpace(text) == "" {
			return
		}
		messageID := m.ensureDevinMessage(session, run, proj)
		proj.mu.Lock()
		if proj.thinkingID == "" {
			proj.thinkingIndex++
			proj.thinkingID = fmt.Sprintf("devin-thinking:%s:%s:%d", run.runID, messageID, proj.thinkingIndex)
			proj.thinkingText.Reset()
			proj.lastThinkEmit = time.Time{}
		}
		proj.thinkingText.WriteString(text)
		thinkingID := proj.thinkingID
		snapshot := proj.thinkingText.String()
		now := time.Now()
		emit := proj.lastThinkEmit.IsZero() || now.Sub(proj.lastThinkEmit) >= piToolProgressInterval
		if emit {
			proj.lastThinkEmit = now
		}
		proj.mu.Unlock()
		if !emit {
			return
		}
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: messageID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": thinkingID, "name": "Reasoning", "kind": "reasoning",
				"out": truncateToolOutput("reasoning", snapshot), "ok": true,
			},
		})
	case "agent_message_chunk":
		content, _ := update["content"].(map[string]any)
		text := firstNonEmpty(stringValue(content["text"]), stringValue(update["text"]))
		if strings.TrimSpace(text) == "" {
			return
		}
		m.finishDevinThinking(session, run, proj)
		messageID := m.ensureDevinMessage(session, run, proj)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "txt_d", RunID: run.runID, ParentID: messageID,
			Timestamp: time.Now(), Payload: map[string]any{"mid": messageID, "txt": text},
		})
		run.markAssistantDeltaSeen(messageID)
		proj.mu.Lock()
		proj.messageHasText = true
		proj.mu.Unlock()
	case "tool_call":
		m.finishDevinThinking(session, run, proj)
		messageID := m.ensureDevinMessage(session, run, proj)
		toolID := firstNonEmpty(stringValue(update["toolCallId"]), utils.NewID())
		title := firstNonEmpty(stringValue(update["title"]), "Tool")
		rawKind := stringValue(update["kind"])
		mappedKind := devinToolHistoryKind(rawKind)
		tool := &devinToolState{
			name:     title,
			kind:     mappedKind,
			input:    update["rawInput"],
			parentID: messageID,
			meta:     map[string]any{"acpKind": rawKind, "title": title},
		}
		proj.mu.Lock()
		proj.tools[toolID] = tool
		meta := tool.meta
		proj.mu.Unlock()
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_st", RunID: run.runID, ParentID: messageID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": toolID, "name": title, "kind": mappedKind,
				"in": update["rawInput"], "meta": meta, "ok": true,
			},
		})
		m.closeDevinMessage(session, run, proj)
	case "tool_call_update":
		toolID := firstNonEmpty(stringValue(update["toolCallId"]), utils.NewID())
		status := strings.ToLower(strings.TrimSpace(stringValue(update["status"])))
		proj.mu.Lock()
		tool := proj.tools[toolID]
		if tool == nil {
			parentID := proj.messageID
			if parentID == "" {
				parentID = run.assistantMessageIDSnapshot()
			}
			tool = &devinToolState{parentID: parentID, meta: map[string]any{}}
			proj.tools[toolID] = tool
		}
		if title := strings.TrimSpace(stringValue(update["title"])); title != "" {
			tool.name = title
			tool.meta["title"] = title
		}
		if rawKind := strings.TrimSpace(stringValue(update["kind"])); rawKind != "" {
			tool.meta["acpKind"] = rawKind
			tool.kind = devinToolHistoryKind(rawKind)
		}
		if rawInput, ok := update["rawInput"]; ok && rawInput != nil {
			tool.input = rawInput
		}
		name := firstNonEmpty(tool.name, "Tool")
		mappedKind := firstNonEmpty(tool.kind, devinToolHistoryKind(stringValue(tool.meta["acpKind"])), "dynamic_tool_call")
		input := tool.input
		parentID := tool.parentID
		meta := cloneMap(tool.meta)
		proj.mu.Unlock()
		if status != "completed" && status != "failed" && status != "cancelled" {
			return
		}
		output := devinToolOutputText(update)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "tool_end", RunID: run.runID, ParentID: parentID,
			Timestamp: time.Now(), Payload: map[string]any{
				"tid": toolID, "name": name, "kind": mappedKind,
				"in": input, "out": output, "ok": status == "completed",
				"status": stringValue(update["status"]), "meta": meta,
			},
		})
	case "plan":
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{
			ID: utils.NewID(), Type: "plan", RunID: run.runID, ParentID: run.assistantMessageIDSnapshot(),
			Timestamp: time.Now(), Payload: update,
		})
	}
}

func (m *Manager) handleDevinPermissionRequest(client *devinACPClient, session tables.WebSessionTable, run *activeRun, message devinACPMessage) {
	var params map[string]any
	_ = json.Unmarshal(message.Params, &params)
	options, _ := params["options"].([]any)
	if effectivePermissionLevel(session) == PermissionLevelDefault {
		now := time.Now()
		request := &pendingServerRequest{
			RawID:       append(json.RawMessage(nil), message.ID...),
			Kind:        pendingServerRequestCommandApproval,
			Prompt:      firstNonEmpty(stringValue(params["reason"]), "Devin is waiting for permission to continue."),
			RequestedAt: &now,
			Permissions: params,
		}
		run.setPendingServerRequest(request)
		m.pauseActiveCallTimeout(run)
		_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{ID: utils.NewID(), Type: "approval_req", RunID: run.runID, ParentID: run.assistantMessageID, Timestamp: now, Payload: map[string]any{"kind": string(request.Kind), "prompt": request.Prompt}})
		_ = m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{"updated_at": now}, AssistantStateWaitingApproval, now))
		m.broadcastSessionSummary(context.Background(), session.ID)
		return
	}
	selected := ""
	for _, raw := range options {
		option, _ := raw.(map[string]any)
		kind := stringValue(option["kind"])
		if effectivePermissionLevel(session) == PermissionLevelDefault && strings.Contains(kind, "allow") {
			continue
		}
		if strings.Contains(kind, "allow") {
			selected = stringValue(option["optionId"])
			break
		}
	}
	outcome := map[string]any{"outcome": "cancelled"}
	if selected != "" {
		outcome = map[string]any{"outcome": "selected", "optionId": selected}
	}
	_ = client.respond(message.ID, map[string]any{"outcome": outcome})
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{ID: utils.NewID(), Type: "approval_res", RunID: run.runID, ParentID: run.assistantMessageID, Timestamp: time.Now(), Payload: map[string]any{"act": selected}})
}

func devinPermissionResponsePayload(action string, request *pendingServerRequest) any {
	if action == "reject" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}
	optionID := ""
	if request != nil {
		if options, ok := request.Permissions["options"].([]any); ok {
			for _, raw := range options {
				option, _ := raw.(map[string]any)
				if strings.Contains(stringValue(option["kind"]), "allow") {
					optionID = stringValue(option["optionId"])
					break
				}
			}
		}
	}
	if optionID == "" {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
	}
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}

func (m *Manager) finishDevinRun(session tables.WebSessionTable, run *activeRun, proj *devinRunProjection) {
	m.closeDevinMessage(session, run, proj)
	finalStatus, finalAssistantState := m.completedRunState(context.Background(), session, run)
	now := time.Now()
	_, _ = m.appendAndBroadcast(context.Background(), session.ID, session, Event{ID: utils.NewID(), Type: "run_done", RunID: run.runID, Timestamp: now, Payload: map[string]any{"ok": true, "st": string(finalStatus)}})
	_ = m.updateRuntimeState(context.Background(), session.ID, applyAssistantStateUpdates(map[string]any{"status": string(finalStatus), "updated_at": now, "auto_retry_attempt": 0, "auto_retry_next_at": nil, "auto_retry_last_error_code": nil}, finalAssistantState, now))
	m.cancelAutoRetryTimer(session.ID)
	m.broadcastSessionSummary(context.Background(), session.ID)
	run.syncSourceAfterRun = true
}

func (c *devinACPClient) terminalCreate(params map[string]any, root string) (map[string]any, error) {
	command := strings.TrimSpace(stringValue(params["command"]))
	if command == "" {
		return nil, errors.New("terminal command is empty")
	}
	cwd := filepath.Clean(firstNonEmpty(stringValue(params["cwd"]), root))
	if !pathWithin(root, cwd) {
		return nil, errors.New("terminal cwd is outside the session workspace")
	}
	args := rawStringSlice(params["args"])
	cmd := devinShellCommand(c.ctx, c.shell, command, args)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	terminalID := utils.NewID()
	terminal := &devinACPTerminal{id: terminalID, cmd: cmd, done: make(chan struct{})}
	writer := devinACPTerminalWriter{terminal: terminal}
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c.terminalMu.Lock()
	c.terminals[terminalID] = terminal
	c.terminalMu.Unlock()
	go func() {
		err := cmd.Wait()
		terminal.outputMu.Lock()
		terminal.exitErr = err
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			terminal.exitCode = &code
		}
		terminal.outputMu.Unlock()
		close(terminal.done)
	}()
	return map[string]any{"terminalId": terminalID}, nil
}

func (c *devinACPClient) terminalOutput(params map[string]any, wait bool) (map[string]any, error) {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	terminal := c.terminals[id]
	c.terminalMu.Unlock()
	if terminal == nil {
		return nil, errors.New("terminal not found")
	}
	if wait {
		select {
		case <-terminal.done:
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		}
	}
	terminal.outputMu.Lock()
	defer terminal.outputMu.Unlock()
	result := map[string]any{"output": terminal.output.String(), "truncated": false}
	if terminal.exitCode != nil {
		result["exitStatus"] = map[string]any{"exitCode": *terminal.exitCode, "signal": nil}
	}
	return result, nil
}

func (c *devinACPClient) terminalKill(params map[string]any) error {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	terminal := c.terminals[id]
	c.terminalMu.Unlock()
	if terminal == nil {
		return errors.New("terminal not found")
	}
	if terminal.cmd.Process != nil {
		return terminal.cmd.Process.Kill()
	}
	return nil
}

func (c *devinACPClient) terminalRelease(params map[string]any) error {
	id := strings.TrimSpace(stringValue(params["terminalId"]))
	c.terminalMu.Lock()
	delete(c.terminals, id)
	c.terminalMu.Unlock()
	return nil
}

// devinShellCommand runs the ACP terminal command through the resolved shell so
// command lines that arrive as a single string (e.g. "git status && git log")
// work, as do plain (command, args) pairs. shell is the resolved shell command
// (binary + startup args) from utils.ResolveShellCommand; when empty it falls
// back to cmd /c on Windows and sh -c elsewhere.
func devinShellCommand(ctx context.Context, shell []string, command string, args []string) *exec.Cmd {
	line := strings.TrimSpace(command)
	for _, arg := range args {
		line += " " + devinShellQuoteArg(arg)
	}
	if len(shell) == 0 {
		if runtime.GOOS == "windows" {
			return exec.CommandContext(ctx, "cmd", "/c", line)
		}
		return exec.CommandContext(ctx, "sh", "-c", line)
	}
	shellArgs := append([]string(nil), shell[1:]...)
	switch name := strings.ToLower(filepath.Base(shell[0])); {
	case strings.HasPrefix(name, "cmd"):
		shellArgs = append(shellArgs, "/c", line)
	case strings.Contains(name, "powershell") || strings.Contains(name, "pwsh"):
		shellArgs = append(shellArgs, "-NoProfile", "-Command", line)
	default:
		shellArgs = append(shellArgs, "-c", line)
	}
	return exec.CommandContext(ctx, shell[0], shellArgs...)
}

func devinShellQuoteArg(arg string) string {
	if runtime.GOOS == "windows" {
		if arg == "" {
			return `""`
		}
		if !strings.ContainsAny(arg, " \t&|<>\"^%") {
			return arg
		}
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}
	return `'` + strings.ReplaceAll(arg, `'`, `'\''`) + `'`
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) && !filepath.IsAbs(rel)
}

func rawStringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, stringValue(item))
	}
	return result
}
