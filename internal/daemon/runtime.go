package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/paneacea/paneacea/internal/agents"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/persistence"
	"github.com/paneacea/paneacea/internal/process"
	"github.com/paneacea/paneacea/internal/terminal"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Storage interface {
	Load() (*model.State, error)
	Save(*model.State) error
}
type TerminalHistoryStorage interface {
	SaveTerminalHistory(string, persistence.TerminalHistory) error
	LoadTerminalHistory(string) (*persistence.TerminalHistory, error)
	DeleteTerminalHistory(string) error
	DeleteAllTerminalHistory() error
	DeleteTerminalHistoryExcept([]string) error
}
type Runtime struct {
	mu                sync.Mutex
	historyMu         sync.Mutex
	state             *model.State
	store             Storage
	sessions          map[string]*terminal.Session
	outputs           map[string]*outputBuffer
	unreadableHistory map[string]bool
	stoppingAgents    map[string]bool
	cancel            context.CancelFunc
	done              chan struct{}
	historyDone       chan struct{}
	viewportOffsets   map[string]int
	shutting          bool
	// agentPrompts tracks shell prompts seen after a pane's agent exited.
	agentPrompts map[string]agentPrompt
	// commandNumbered marks panes whose shell reports command numbers.
	commandNumbered map[string]bool
	// resuming holds, per pane, when an in-flight agent.resume lock expires.
	resuming map[string]time.Time
	// promptAware marks panes whose shell reports prompts; for these, busy
	// means a command was submitted and its prompt has not returned yet, so
	// background jobs (cmd &) do not count.
	promptAware map[string]bool
	// commandPending marks prompt-aware panes with a submitted command.
	commandPending map[string]bool

	systemShuttingDown func() bool
}

const agentStoppedStatus = "agent-stopped"

type Params struct {
	WorkspaceID   string            `json:"workspaceId"`
	TabID         string            `json:"tabId"`
	PaneID        string            `json:"paneId"`
	Name          string            `json:"name"`
	Title         string            `json:"title"`
	RootDirectory string            `json:"rootDirectory"`
	Orientation   string            `json:"orientation"`
	Path          []int             `json:"path"`
	Ratio         float64           `json:"ratio"`
	Index         int               `json:"index"`
	Executable    string            `json:"executable"`
	Arguments     []string          `json:"arguments"`
	Environment   map[string]string `json:"environment"`
	Data          string            `json:"data"`
	Columns       uint16            `json:"columns"`
	Rows          uint16            `json:"rows"`
	Sequence      uint64            `json:"sequence"`
	Settings      *model.Settings   `json:"settings"`
	Agent         *model.Agent      `json:"agent"`
	Offset        int               `json:"offset"`
}

func New(store Storage) (*Runtime, error) {
	state, err := store.Load()
	if err != nil {
		return nil, err
	}
	if state.Version != 1 {
		return nil, fmt.Errorf("unsupported state version %d", state.Version)
	}
	if state.Settings.DefaultShell.Executable == "" {
		state.Settings.DefaultShell = firstAvailableProfile()
	}
	r := &Runtime{state: state, store: store, sessions: map[string]*terminal.Session{}, outputs: map[string]*outputBuffer{}, unreadableHistory: map[string]bool{}, stoppingAgents: map[string]bool{}, done: make(chan struct{}), historyDone: make(chan struct{}), viewportOffsets: map[string]int{}}
	r.systemShuttingDown = systemShuttingDown
	if historyStore, ok := store.(TerminalHistoryStorage); ok {
		paneIDs := make([]string, 0, len(state.Panes))
		for id := range state.Panes {
			paneIDs = append(paneIDs, id)
		}
		if state.Settings.TerminalHistoryLines == 0 {
			if err = historyStore.DeleteAllTerminalHistory(); err != nil {
				log.Printf("clear terminal history while disabled: %v", err)
			}
		} else if err = historyStore.DeleteTerminalHistoryExcept(paneIDs); err != nil {
			log.Printf("prune orphaned terminal history: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.mu.Lock()
	for _, p := range state.Panes {
		// Busy is recomputed by the monitor for the relaunched shell.
		p.Busy = false
		// Agents saved before resumeCommand existed get it filled in.
		if p.Agent != nil {
			p.Agent.ResumeCommand = agents.ResumeCommand(p.Agent.Type)
		}
	}
	for _, p := range state.Panes {
		if err = r.launch(p); err != nil {
			p.Status = "error"
			p.Error = err.Error()
		}
	}
	state.Revision++
	err = store.Save(state)
	r.mu.Unlock()
	if err != nil {
		for _, s := range r.sessions {
			s.Close()
		}
		cancel()
		return nil, err
	}
	go r.monitor(ctx)
	go r.checkpointLoop(ctx)
	return r, nil
}
func (r *Runtime) Close() {
	r.mu.Lock()
	r.shutting = true
	r.mu.Unlock()
	r.cancel()
	<-r.done
	<-r.historyDone
	r.mu.Lock()
	sessions := make([]*terminal.Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	r.mu.Unlock()
	for _, s := range sessions {
		s.Close()
	}
	// Sessions drain their final output on close, so the last checkpoint follows session shutdown.
	r.checkpointHistory(true)
}

func (r *Runtime) checkpointLoop(ctx context.Context) {
	defer close(r.historyDone)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkpointHistory(false)
		}
	}
}

func (r *Runtime) checkpointHistory(force bool) {
	historyStore, ok := r.store.(TerminalHistoryStorage)
	if !ok {
		return
	}
	r.mu.Lock()
	// Keep pane deletion serialized with checkpoint writes after releasing the runtime lock.
	r.historyMu.Lock()
	lines := r.state.Settings.TerminalHistoryLines
	if lines == 0 {
		r.unreadableHistory = map[string]bool{}
	}
	outputs := make(map[string]*outputBuffer, len(r.outputs))
	for id, output := range r.outputs {
		if r.state.Panes[id] != nil && !r.unreadableHistory[id] {
			outputs[id] = output
		}
	}
	r.mu.Unlock()
	var failures = map[string]error{}
	if lines == 0 {
		if err := historyStore.DeleteAllTerminalHistory(); err != nil {
			failures[""] = err
		}
	} else {
		for id, output := range outputs {
			if !force && !output.needsSave() {
				continue
			}
			data, columns, rows, generation, err := output.persistedSnapshot(lines, persistence.MaxTerminalHistoryBytes)
			if err == nil {
				err = historyStore.SaveTerminalHistory(id, persistence.TerminalHistory{Version: 1, Columns: columns, Rows: rows, Data: data})
			}
			if err != nil {
				failures[id] = err
				continue
			}
			output.markSaved(generation)
		}
	}
	r.historyMu.Unlock()
	if len(failures) == 0 {
		r.clearHistoryWarnings()
		return
	}
	r.mu.Lock()
	changed := false
	for id, err := range failures {
		if id == "" {
			log.Printf("delete saved terminal history: %v", err)
			continue
		}
		if pane := r.state.Panes[id]; pane != nil {
			warning := fmt.Sprintf("terminal history save failed: %v", err)
			if pane.Error != warning {
				pane.Error = warning
				changed = true
			}
		}
	}
	if changed {
		r.state.Revision++
		if err := r.store.Save(r.state); err != nil {
			log.Printf("save terminal history warning state: %v", err)
		}
	}
	r.mu.Unlock()
}

func (r *Runtime) clearHistoryWarnings() {
	r.mu.Lock()
	changed := false
	for _, pane := range r.state.Panes {
		if strings.HasPrefix(pane.Error, "terminal history save failed:") {
			pane.Error = ""
			changed = true
		}
	}
	if changed {
		r.state.Revision++
		if err := r.store.Save(r.state); err != nil {
			log.Printf("clear terminal history warning state: %v", err)
		}
	}
	r.mu.Unlock()
}

func (r *Runtime) deleteHistoryLocked(paneID string) {
	if historyStore, ok := r.store.(TerminalHistoryStorage); ok {
		r.historyMu.Lock()
		if err := historyStore.DeleteTerminalHistory(paneID); err != nil {
			log.Printf("delete terminal history for pane %s: %v", paneID, err)
		}
		r.historyMu.Unlock()
	}
	delete(r.viewportOffsets, paneID)
}

func (r *Runtime) applyHistoryLimitLocked() {
	historyStore, ok := r.store.(TerminalHistoryStorage)
	if ok {
		r.historyMu.Lock()
		defer r.historyMu.Unlock()
	}
	if r.state.Settings.TerminalHistoryLines == 0 {
		r.unreadableHistory = map[string]bool{}
	}
	for _, output := range r.outputs {
		output.applyHistoryLimit(min(max(r.state.Settings.Scrollback, r.state.Settings.TerminalHistoryLines), 25000))
	}
	if !ok || r.state.Settings.TerminalHistoryLines == 0 {
		return
	}
	for id, output := range r.outputs {
		if r.state.Panes[id] == nil || r.unreadableHistory[id] {
			continue
		}
		data, columns, rows, generation, err := output.persistedSnapshot(r.state.Settings.TerminalHistoryLines, persistence.MaxTerminalHistoryBytes)
		if err == nil {
			err = historyStore.SaveTerminalHistory(id, persistence.TerminalHistory{Version: 1, Columns: columns, Rows: rows, Data: data})
		}
		if err != nil {
			log.Printf("trim terminal history for pane %s: %v", id, err)
			if pane := r.state.Panes[id]; pane != nil {
				pane.Error = fmt.Sprintf("terminal history save failed: %v", err)
			}
			continue
		}
		output.markSaved(generation)
	}
}
func clone(s *model.State) *model.State {
	data, _ := json.Marshal(s)
	var result model.State
	_ = json.Unmarshal(data, &result)
	return &result
}
func directory(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("root directory is required")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root must be a directory")
	}
	return path, nil
}
func (r *Runtime) launch(p *model.Pane) error {
	output := newOutput()
	columns, rows := int(p.Columns), int(p.Rows)
	if columns == 0 {
		columns = 120
	}
	if rows == 0 {
		rows = 30
	}
	historyLines := min(max(r.state.Settings.Scrollback, r.state.Settings.TerminalHistoryLines), 25000)
	output.emulator = vt.NewEmulator(columns, rows)
	output.modes = map[ansi.Mode]bool{}
	output.emulator.SetScrollbackSize(historyLines)
	output.emulator.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { output.modes[mode] = true }, DisableMode: func(mode ansi.Mode) { output.modes[mode] = false }})
	// Saved terminal output is not replayed on launch; restored panes start a fresh shell in their
	// saved working directory, and Git Bash keeps its per-pane command history (HISTFILE) below.
	historyWarning := ""
	ready := make(chan *terminal.Session, 1)
	go func() {
		session := <-ready
		buffer := make([]byte, 4096)
		for {
			n, err := output.emulator.Read(buffer)
			if n > 0 && session != nil {
				if session.Write(buffer[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	started := false
	defer func() {
		if !started {
			ready <- nil
			_ = output.emulator.InputPipe().(io.Closer).Close()
		}
	}()
	r.outputs[p.ID] = output
	environment := map[string]string{}
	for k, v := range p.Environment {
		environment[k] = v
	}
	address, err := ipc.Address()
	if err != nil {
		return err
	}
	environment["PANEACEA_PIPE"] = address
	environment["PANEACEA_PANE_ID"] = p.ID
	environment["PANEACEA_WORKSPACE_ID"] = p.WorkspaceID
	executable := p.Executable
	arguments := append([]string(nil), p.Arguments...)
	arguments = shellIntegration(executable, arguments, environment)
	arguments, bashSetup := configureBashStartup(executable, arguments, environment)
	if bashSetup && environment["HISTFILE"] == "" {
		if historyStore, ok := r.store.(interface{ BashHistoryPath(string) (string, error) }); ok {
			historyPath, pathErr := historyStore.BashHistoryPath(p.ID)
			if pathErr != nil {
				return pathErr
			}
			if pathErr = os.MkdirAll(filepath.Dir(historyPath), 0700); pathErr != nil {
				return fmt.Errorf("create Bash history directory: %w", pathErr)
			}
			environment["HISTFILE"], pathErr = bashHistoryEnvironment(historyPath)
			if pathErr != nil {
				return pathErr
			}
		}
	}
	workingDirectory := p.CurrentWorkingDirectory
	if _, err = directory(workingDirectory); err != nil {
		workingDirectory = p.InitialWorkingDirectory
	}
	var metadataMu sync.Mutex
	metadata := ""
	emitOutput := func(data string) {
		if data == "" {
			return
		}
		output.append([]byte(data))
		metadataMu.Lock()
		metadata += data
		values, remainder := consumeMetadata(metadata)
		metadata = remainder
		for _, value := range values {
			r.metadata(p.ID, value)
		}
		metadataMu.Unlock()
	}
	session, err := terminal.NewSession(terminal.Options{Executable: executable, Arguments: arguments, Environment: environment, WorkingDirectory: workingDirectory, Columns: p.Columns, Rows: p.Rows,
		OnOutput: func(data string) {
			emitOutput(data)
		},
		OnExit: func(exitErr error) {
			_ = output.emulator.InputPipe().(io.Closer).Close()
			output.exit()
			r.mu.Lock()
			defer r.mu.Unlock()
			// Windows shutdown can end shells before the runtime; preserve their panes for the next startup.
			if r.shutting || r.outputs[p.ID] != output || r.systemShuttingDown() {
				return
			}
			if r.stoppingAgents[p.ID] {
				delete(r.stoppingAgents, p.ID)
				return
			}
			if r.state.Panes[p.ID] != nil {
				r.removePane(p.ID)
				r.state.Revision++
				if saveErr := r.store.Save(r.state); saveErr != nil {
					log.Printf("save terminal exit state: %v", saveErr)
				} else {
					r.deleteHistoryLocked(p.ID)
				}
			}
		},
	})
	if err != nil {
		output.exit()
		return err
	}
	r.sessions[p.ID] = session
	ready <- session
	started = true
	p.PID = session.PID()
	p.RunningProgram = executable
	p.RuntimeTerminalID = p.ID
	p.Status = "running"
	p.Error = historyWarning
	return nil
}
func consumeMetadata(metadata string) (values []string, remainder string) {
	for {
		start := strings.Index(metadata, "\x1b]")
		if start < 0 {
			if strings.HasSuffix(metadata, "\x1b") {
				return values, "\x1b"
			}
			return values, ""
		}
		metadata = metadata[start:]
		bel := strings.IndexByte(metadata[2:], '\a')
		st := strings.Index(metadata[2:], "\x1b\\")
		if bel < 0 && st < 0 {
			if len(metadata) > 8192 {
				return values, ""
			}
			return values, metadata
		}
		end := -1
		terminatorLength := 0
		if bel >= 0 && (st < 0 || bel < st) {
			end = bel + 2
			terminatorLength = 1
		} else {
			end = st + 2
			terminatorLength = 2
		}
		if end+terminatorLength > 8192 {
			metadata = metadata[end+terminatorLength:]
			continue
		}
		values = append(values, metadata[2:end])
		metadata = metadata[end+terminatorLength:]
	}
}
func (r *Runtime) metadata(id, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.state.Panes[id]
	if p == nil {
		return
	}
	changed := false
	if strings.HasPrefix(value, "0;") || strings.HasPrefix(value, "2;") {
		title := strings.TrimSpace(value[2:])
		if len(title) > 256 {
			title = title[:256]
		}
		if p.Title != title {
			p.Title = title
			_, t := r.state.Tab(p.TabID)
			if t != nil && t.TitleMode != "manual" {
				t.Title = title
			}
			changed = true
		}
	}
	// Prompt events drop the agent indicator once a command other than the
	// agent has run. Shells with Paneacea integration report a command number
	// (unchanged on empty Enter); others fall back to counting OSC 7 prompts.
	if command, ok := strings.CutPrefix(value, commandNumberPrefix); ok {
		if r.commandNumbered == nil {
			r.commandNumbered = map[string]bool{}
		}
		r.commandNumbered[id] = true
		if command == "" {
			command = "0"
		}
		changed = r.promptLocked(p) || changed
		changed = r.agentPromptLocked(p, command) || changed
	} else if strings.HasPrefix(value, "7;") && !r.commandNumbered[id] {
		changed = r.promptLocked(p) || changed
		changed = r.agentPromptLocked(p, "") || changed
	}
	if strings.HasPrefix(value, "7;") {
		uri, err := url.Parse(value[2:])
		if err == nil && uri.Scheme == "file" && (uri.Host == "" || uri.Host == "localhost" || strings.EqualFold(uri.Host, hostName())) {
			path := strings.TrimPrefix(uri.Path, "/")
			if root, err := directory(path); err == nil && root != p.CurrentWorkingDirectory {
				p.CurrentWorkingDirectory = root
				changed = true
			}
		}
	}
	if changed {
		r.state.Revision++
		if err := r.store.Save(r.state); err != nil {
			p.Error = err.Error()
		}
	}
}
func hostName() string { name, _ := os.Hostname(); return name }

// newDetectedAgent builds the agent record for a process found by the monitor.
func newDetectedAgent(kind string, pid uint32, generation string) *model.Agent {
	return &model.Agent{Type: kind, State: "unknown", RootPID: pid, ProcessGeneration: generation, ResumeCommand: agents.ResumeCommand(kind)}
}

// resumeLockDuration bounds how long a pane rejects repeated agent.resume
// calls when the resumed agent is never detected (e.g. picker quit at once).
const resumeLockDuration = 10 * time.Second

// resumeAgent takes a per-pane lock so repeated clicks cannot type the command
// twice; the lock is released on failure, when the monitor detects the resumed
// agent, or after resumeLockDuration.
func (r *Runtime) resumeAgent(paneID string) error {
	r.mu.Lock()
	if time.Now().Before(r.resuming[paneID]) {
		r.mu.Unlock()
		return fmt.Errorf("resume already in progress")
	}
	if r.resuming == nil {
		r.resuming = map[string]time.Time{}
	}
	r.resuming[paneID] = time.Now().Add(resumeLockDuration)
	r.mu.Unlock()
	if err := r.typeResumeCommand(paneID); err != nil {
		r.mu.Lock()
		delete(r.resuming, paneID)
		r.mu.Unlock()
		return err
	}
	return nil
}

// typeResumeCommand types the last agent's resume command (its session picker)
// into the pane shell. It refuses while the agent or any other command is running.
func (r *Runtime) typeResumeCommand(paneID string) error {
	r.mu.Lock()
	session := r.sessions[paneID]
	pane := r.state.Panes[paneID]
	var agent model.Agent
	var panePID uint32
	var executable string
	running := false
	if pane != nil {
		if pane.Agent != nil {
			agent = *pane.Agent
		}
		panePID, executable, running = pane.PID, pane.Executable, pane.Status == "running"
	}
	r.mu.Unlock()
	if session == nil || pane == nil || !running {
		return fmt.Errorf("terminal not found")
	}
	if agent.Type == "" {
		return fmt.Errorf("no agent has run in this pane")
	}
	command := agents.ResumeCommand(agent.Type)
	if command == "" {
		return fmt.Errorf("%s has no resume session picker", agent.Type)
	}
	if agent.RootPID != 0 && process.Generation(agent.RootPID) == agent.ProcessGeneration {
		return fmt.Errorf("%s is still running", agent.Type)
	}
	r.mu.Lock()
	promptAware, pending := r.promptAware[paneID], r.commandPending[paneID]
	r.mu.Unlock()
	if promptAware {
		if pending {
			return fmt.Errorf("pane is busy running a command")
		}
	} else {
		items, err := process.Snapshot()
		if err != nil {
			return err
		}
		if busy := foregroundCommand(items, panePID, paneOrphans(items, cachedPaneID, msysForked)[paneID]); busy != "" {
			return fmt.Errorf("pane is busy running %s", busy)
		}
	}
	if err := session.Write([]byte(clearLineInput(executable) + command + "\r")); err != nil {
		return err
	}
	r.commandSubmitted(paneID)
	return nil
}

// commandSubmitted marks a prompt-aware pane busy after Enter was sent, until
// the shell prints its next prompt (see promptLocked).
func (r *Runtime) commandSubmitted(paneID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pane := r.state.Panes[paneID]
	if pane == nil || !r.promptAware[paneID] || r.commandPending[paneID] {
		return
	}
	r.commandPending[paneID] = true
	if !pane.Busy {
		pane.Busy = true
		r.state.Revision++
		if err := r.store.Save(r.state); err != nil {
			log.Printf("save pane busy state: %v", err)
		}
	}
}

// promptLocked records a shell prompt: the pane is prompt-aware and idle.
// Background jobs started with & return to the prompt, so they are not busy.
func (r *Runtime) promptLocked(p *model.Pane) bool {
	if r.promptAware == nil {
		r.promptAware = map[string]bool{}
		r.commandPending = map[string]bool{}
	}
	r.promptAware[p.ID] = true
	delete(r.commandPending, p.ID)
	if !p.Busy {
		return false
	}
	p.Busy = false
	return true
}

// paneShells are processes that make up an idle pane (the shell itself, Git
// Bash's inner bash and console hosts).
var paneShells = map[string]bool{
	"bash.exe": true, "pwsh.exe": true, "powershell.exe": true, "cmd.exe": true,
	"conhost.exe": true, "openconsole.exe": true,
}

// foregroundCommand returns the first non-shell process under the pane or among
// its orphaned processes (e.g. Git Bash script shims), or "" when the pane is
// idle at its prompt.
func foregroundCommand(items []process.Info, panePID uint32, orphans []process.Info) string {
	for _, p := range append(process.Descendants(items, panePID), orphans...) {
		if !paneShells[strings.ToLower(p.Executable)] {
			return p.Executable
		}
	}
	return ""
}

// clearLineInput returns the keystroke that discards partially typed input at
// the shell prompt so the resume command is not appended to it.
func clearLineInput(executable string) string {
	switch strings.ToLower(filepath.Base(executable)) {
	case "bash.exe":
		return "\x15" // readline unix-line-discard (Ctrl+U)
	case "pwsh.exe", "powershell.exe", "cmd.exe":
		return "\x1b" // Escape clears the PSReadLine/cmd input line
	}
	return ""
}

// commandNumberPrefix is the private OSC emitted by the bash/PowerShell prompt
// hooks with the shell's command number.
const commandNumberPrefix = "1999;paneacea;command="

// agentPromptLocked handles a prompt for pane p and clears its agent when
// another command has run. command is "" when the shell has no command numbers.
func (r *Runtime) agentPromptLocked(p *model.Pane, command string) bool {
	if p.Agent == nil || p.Agent.RootPID == 0 {
		return false
	}
	alive := process.Generation(p.Agent.RootPID) == p.Agent.ProcessGeneration
	if r.agentPrompts == nil {
		r.agentPrompts = map[string]agentPrompt{}
	}
	next, clear := countAgentPrompt(r.agentPrompts[p.ID], p.Agent, alive, command)
	if !clear {
		r.agentPrompts[p.ID] = next
		return false
	}
	p.Agent = nil
	delete(r.agentPrompts, p.ID)
	return true
}

// agentPrompt tracks prompts for one agent process identity.
type agentPrompt struct {
	rootPID    uint32
	generation string
	exited     bool
	command    string
}

// countAgentPrompt records a shell prompt and reports whether the agent should
// be cleared. The first prompt after the agent exits ends the agent command, so
// the indicator stays. A later prompt clears it when the command number changed
// (empty Enter keeps it), or always when command numbers are unavailable ("").
func countAgentPrompt(prev agentPrompt, agent *model.Agent, alive bool, command string) (agentPrompt, bool) {
	if prev.rootPID != agent.RootPID || prev.generation != agent.ProcessGeneration {
		prev = agentPrompt{rootPID: agent.RootPID, generation: agent.ProcessGeneration}
	}
	if alive {
		return prev, false
	}
	if !prev.exited {
		prev.exited = true
		prev.command = command
		return prev, false
	}
	return prev, command == "" || command != prev.command
}
func (r *Runtime) Call(ctx context.Context, method string, params json.RawMessage) (any, error) {
	var p Params
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
	}
	if method == "profiles.list" {
		return Profiles(), nil
	}
	if method == "terminal.attach" || method == "terminal.requestSnapshot" || method == "terminal.read" {
		r.mu.Lock()
		output := r.outputs[p.PaneID]
		r.mu.Unlock()
		if output == nil {
			return nil, fmt.Errorf("terminal not found")
		}
		if method == "terminal.attach" {
			r.mu.Lock()
			viewportOffset := r.viewportOffsets[p.PaneID]
			r.mu.Unlock()
			return output.attachSnapshot(viewportOffset), nil
		}
		if method == "terminal.requestSnapshot" {
			return r.withViewport(p.PaneID, output.snapshot()), nil
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		value, err := output.read(ctx, p.Sequence)
		if errors.Is(err, context.DeadlineExceeded) {
			return r.withViewport(p.PaneID, ipc.Output{Sequence: p.Sequence}), nil
		}
		return r.withViewport(p.PaneID, value), err
	}
	if method == "terminal.viewport.set" {
		if p.Offset < 0 {
			return nil, fmt.Errorf("viewport offset cannot be negative")
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		output := r.outputs[p.PaneID]
		if output == nil {
			return nil, fmt.Errorf("terminal not found")
		}
		r.viewportOffsets[p.PaneID] = min(p.Offset, output.scrollbackLines())
		return struct{}{}, nil
	}
	if method == "terminal.detach" {
		return struct{}{}, nil
	}
	if method == "pane.sendInput" || method == "terminal.resize" {
		r.mu.Lock()
		session := r.sessions[p.PaneID]
		pane := r.state.Panes[p.PaneID]
		r.mu.Unlock()
		if session == nil || pane == nil {
			return nil, fmt.Errorf("terminal not found")
		}
		if method == "pane.sendInput" {
			if len(p.Data) > 65536 {
				return nil, fmt.Errorf("input exceeds 64 KiB")
			}
			if err := session.Write([]byte(p.Data)); err != nil {
				return nil, err
			}
			if strings.ContainsAny(p.Data, "\r\n") {
				r.commandSubmitted(p.PaneID)
			}
			return struct{}{}, nil
		}
		if err := session.Resize(p.Columns, p.Rows); err != nil {
			return nil, err
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if pane = r.state.Panes[p.PaneID]; pane != nil {
			if output := r.outputs[p.PaneID]; output != nil {
				output.resize(int(p.Columns), int(p.Rows))
			}
			pane.Columns = p.Columns
			pane.Rows = p.Rows
			return struct{}{}, r.store.Save(r.state)
		}
		return struct{}{}, nil
	}
	if method == "agent.stop" {
		return r.stopAgents()
	}
	if method == "agent.resume" {
		return struct{}{}, r.resumeAgent(p.PaneID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if method == "state.get" || method == "workspace.list" {
		return clone(r.state), nil
	}
	if method == "agent.list" {
		result := map[string]*model.Agent{}
		for id, pane := range r.state.Panes {
			if pane.Agent != nil {
				copy := *pane.Agent
				result[id] = &copy
			}
		}
		return result, nil
	}
	if method == "agent.get" || method == "agent.status" {
		pane := r.state.Panes[p.PaneID]
		if pane == nil {
			return nil, fmt.Errorf("pane not found")
		}
		if pane.Agent == nil {
			return nil, nil
		}
		copy := *pane.Agent
		return copy, nil
	}
	before := clone(r.state)
	oldSessions := map[string]*terminal.Session{}
	oldOutputs := map[string]*outputBuffer{}
	for id, output := range r.outputs {
		oldOutputs[id] = output
	}
	for id, s := range r.sessions {
		oldSessions[id] = s
	}
	if err := r.mutate(method, p); err != nil {
		r.rollback(before, oldSessions)
		r.outputs = oldOutputs
		return nil, err
	}
	r.state.Revision++
	if err := r.store.Save(r.state); err != nil {
		r.rollback(before, oldSessions)
		r.outputs = oldOutputs
		return nil, fmt.Errorf("persist state: %w", err)
	}
	for id := range before.Panes {
		if r.state.Panes[id] == nil {
			r.deleteHistoryLocked(id)
		}
	}
	if before.Settings.TerminalHistoryLines != r.state.Settings.TerminalHistoryLines || before.Settings.Scrollback != r.state.Settings.Scrollback {
		if r.state.Settings.TerminalHistoryLines == 0 {
			for _, output := range r.outputs {
				output.applyHistoryLimit(min(r.state.Settings.Scrollback, 25000))
			}
			if historyStore, ok := r.store.(TerminalHistoryStorage); ok {
				r.historyMu.Lock()
				if err := historyStore.DeleteAllTerminalHistory(); err != nil {
					log.Printf("clear saved terminal history: %v", err)
					for _, pane := range r.state.Panes {
						pane.Error = fmt.Sprintf("terminal history could not be cleared: %v", err)
					}
				}
				r.historyMu.Unlock()
			}
		} else {
			r.applyHistoryLimitLocked()
		}
	}
	for id, s := range oldSessions {
		if r.state.Panes[id] == nil || r.sessions[id] != s {
			if r.state.Panes[id] == nil {
				delete(r.sessions, id)
			}
			go s.Close()
		}
	}
	for id := range r.outputs {
		if r.state.Panes[id] == nil {
			r.outputs[id].exit()
			delete(r.outputs, id)
		}
	}
	return clone(r.state), nil
}

func agentIsRunning(agent *model.Agent, generation string) bool {
	return agent != nil && agent.RootPID != 0 && agent.ProcessGeneration != "" && generation != "" && agent.ProcessGeneration == generation
}

func prepareAgentStop(p *model.Pane) {
	p.Status = agentStoppedStatus
	p.PID = 0
	if p.Agent != nil {
		p.Agent.RootPID = 0
		p.Agent.ProcessGeneration = ""
		p.Agent.State = "idle"
	}
}

func (r *Runtime) stopAgents() (any, error) {
	r.mu.Lock()
	if r.stoppingAgents == nil {
		r.stoppingAgents = map[string]bool{}
	}
	before := clone(r.state)
	sessions := map[string]*terminal.Session{}
	for id, pane := range r.state.Panes {
		if pane.Agent == nil || pane.Status != "running" {
			continue
		}
		if !agentIsRunning(pane.Agent, process.Generation(pane.Agent.RootPID)) {
			continue
		}
		if session := r.sessions[id]; session != nil {
			r.stoppingAgents[id] = true
			sessions[id] = session
			prepareAgentStop(pane)
		}
	}
	if len(sessions) == 0 {
		result := clone(r.state)
		r.mu.Unlock()
		return result, nil
	}
	r.state.Revision++
	err := r.store.Save(r.state)
	if err != nil {
		for id := range sessions {
			delete(r.stoppingAgents, id)
		}
		r.state = before
		r.mu.Unlock()
		return nil, fmt.Errorf("persist agent shutdown: %w", err)
	}
	r.mu.Unlock()
	for _, session := range sessions {
		_ = session.Close()
	}
	r.mu.Lock()
	result := clone(r.state)
	r.mu.Unlock()
	return result, nil
}

func (r *Runtime) rollback(before *model.State, old map[string]*terminal.Session) {
	for id, s := range r.sessions {
		if old[id] != s {
			go s.Close()
			delete(r.outputs, id)
		}
	}
	r.sessions = old
	r.state = before
}
func (r *Runtime) newPane(w *model.Workspace, t *model.Tab, p Params) (*model.Pane, error) {
	profile := r.state.Settings.DefaultShell
	if p.Executable != "" {
		profile.Executable = p.Executable
		profile.ID = "custom"
		profile.Arguments = p.Arguments
	}
	pane := &model.Pane{ID: model.ID(), WorkspaceID: w.ID, TabID: t.ID, ProfileID: profile.ID, Executable: profile.Executable, Arguments: append([]string{}, profile.Arguments...), Environment: p.Environment, InitialWorkingDirectory: w.RootDirectory, CurrentWorkingDirectory: w.RootDirectory, Columns: 120, Rows: 30, Title: profile.Name}
	if pane.Environment == nil {
		pane.Environment = map[string]string{}
	}
	if err := r.launch(pane); err != nil {
		return nil, err
	}
	r.state.Panes[pane.ID] = pane
	return pane, nil
}

func (r *Runtime) withViewport(paneID string, output ipc.Output) ipc.Output {
	r.mu.Lock()
	output.ViewportOffset = r.viewportOffsets[paneID]
	r.mu.Unlock()
	return output
}

func validTerminalHistoryLines(lines int) bool {
	switch lines {
	case 0, 500, 2000, 5000, 10000, 25000:
		return true
	default:
		return false
	}
}

func (r *Runtime) mutate(method string, p Params) error {
	w := r.state.Workspace(p.WorkspaceID)
	_, t := r.state.Tab(p.TabID)
	pane := r.state.Panes[p.PaneID]
	switch method {
	case "workspace.create":
		root, err := directory(p.RootDirectory)
		if err != nil {
			return err
		}
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("workspace name is required")
		}
		w = &model.Workspace{ID: model.ID(), Name: p.Name, RootDirectory: root, Tabs: []*model.Tab{}, CreatedAt: time.Now(), LastOpenedAt: time.Now()}
		r.state.Workspaces = append(r.state.Workspaces, w)
		r.state.ActiveWorkspaceID = w.ID
	case "workspace.rename", "workspace.setRoot", "workspace.switch", "workspace.close":
		if w == nil {
			return fmt.Errorf("workspace not found")
		}
		switch method {
		case "workspace.rename":
			if strings.TrimSpace(p.Name) == "" {
				return fmt.Errorf("name is required")
			}
			w.Name = p.Name
		case "workspace.setRoot":
			root, err := directory(p.RootDirectory)
			if err != nil {
				return err
			}
			w.RootDirectory = root
		case "workspace.switch":
			r.state.ActiveWorkspaceID = w.ID
			w.LastOpenedAt = time.Now()
		case "workspace.close":
			for _, tab := range w.Tabs {
				for _, id := range tab.RootLayoutNode.Leaves() {
					delete(r.state.Panes, id)
				}
			}
			for i, v := range r.state.Workspaces {
				if v.ID == w.ID {
					r.state.Workspaces = append(r.state.Workspaces[:i], r.state.Workspaces[i+1:]...)
					break
				}
			}
			if r.state.ActiveWorkspaceID == w.ID {
				r.state.ActiveWorkspaceID = ""
				if len(r.state.Workspaces) > 0 {
					r.state.ActiveWorkspaceID = r.state.Workspaces[0].ID
				}
			}
		}
	case "tab.create":
		if w == nil {
			return fmt.Errorf("workspace not found")
		}
		t = &model.Tab{ID: model.ID(), WorkspaceID: w.ID, Title: "Terminal", TitleMode: "automatic", CreatedAt: time.Now(), SortOrder: len(w.Tabs)}
		pane, err := r.newPane(w, t, p)
		if err != nil {
			return err
		}
		t.RootLayoutNode = &model.Layout{PaneID: pane.ID}
		t.ActivePaneID = pane.ID
		w.Tabs = append(w.Tabs, t)
		w.ActiveTabID = t.ID
	case "tab.rename", "tab.focus", "tab.move", "tab.close":
		if t == nil {
			return fmt.Errorf("tab not found")
		}
		w = r.state.Workspace(t.WorkspaceID)
		switch method {
		case "tab.rename":
			if strings.TrimSpace(p.Title) == "" {
				return fmt.Errorf("title is required")
			}
			t.Title = p.Title
			t.TitleMode = "manual"
		case "tab.focus":
			w.ActiveTabID = t.ID
			r.state.ActiveWorkspaceID = w.ID
		case "tab.move":
			if p.Index < 0 || p.Index >= len(w.Tabs) {
				return fmt.Errorf("invalid tab index")
			}
			for i, v := range w.Tabs {
				if v.ID == t.ID {
					w.Tabs = append(w.Tabs[:i], w.Tabs[i+1:]...)
					break
				}
			}
			w.Tabs = append(w.Tabs, nil)
			copy(w.Tabs[p.Index+1:], w.Tabs[p.Index:])
			w.Tabs[p.Index] = t
			for i, v := range w.Tabs {
				v.SortOrder = i
			}
		case "tab.close":
			r.closeTab(w, t)
		}
	case "pane.create", "pane.split":
		if pane == nil {
			return fmt.Errorf("pane not found")
		}
		if p.Orientation != "vertical" && p.Orientation != "horizontal" {
			return fmt.Errorf("invalid orientation")
		}
		w, t = r.state.Tab(pane.TabID)
		node := t.RootLayoutNode.Find(pane.ID)
		if node == nil {
			return fmt.Errorf("pane absent from layout")
		}
		created, err := r.newPane(w, t, p)
		if err != nil {
			return err
		}
		*node = model.Layout{Orientation: p.Orientation, Ratio: 0.5, First: &model.Layout{PaneID: pane.ID}, Second: &model.Layout{PaneID: created.ID}}
		t.ActivePaneID = created.ID
	case "pane.focus", "pane.close", "pane.restart", "agent.register":
		if pane == nil {
			return fmt.Errorf("pane not found")
		}
		w, t = r.state.Tab(pane.TabID)
		switch method {
		case "pane.focus":
			t.ActivePaneID = pane.ID
			w.ActiveTabID = t.ID
			r.state.ActiveWorkspaceID = w.ID
		case "pane.close":
			delete(r.state.Panes, pane.ID)
			t.RootLayoutNode = t.RootLayoutNode.Remove(pane.ID)
			if t.RootLayoutNode == nil {
				r.closeTab(w, t)
			} else if t.ActivePaneID == pane.ID {
				t.ActivePaneID = t.RootLayoutNode.Leaves()[0]
			}
		case "pane.restart":
			if pane.Status == "running" {
				return fmt.Errorf("close the running command before restarting")
			}
			pane.Agent = nil
			if err := r.launch(pane); err != nil {
				return err
			}
		case "agent.register":
			return r.register(pane, p.Agent)
		}
	case "pane.resize":
		if t == nil {
			return fmt.Errorf("tab not found")
		}
		if !model.ValidRatio(p.Ratio) {
			return fmt.Errorf("ratio must be between 0.1 and 0.9")
		}
		node, err := t.RootLayoutNode.At(p.Path)
		if err != nil {
			return err
		}
		node.Ratio = p.Ratio
	case "settings.set":
		if p.Settings == nil {
			return fmt.Errorf("settings required")
		}
		if p.Settings.Scrollback < 0 || p.Settings.Scrollback > 100000 || p.Settings.FontSize < 8 || p.Settings.FontSize > 32 {
			return fmt.Errorf("invalid terminal settings")
		}
		if p.Settings.DefaultShell.Executable == "" {
			return fmt.Errorf("default shell required")
		}
		if !validTerminalHistoryLines(p.Settings.TerminalHistoryLines) {
			return fmt.Errorf("terminal history lines must be 0, 500, 2000, 5000, 10000, or 25000")
		}
		r.state.Settings = *p.Settings
	default:
		return fmt.Errorf("unknown method %q", method)
	}
	return nil
}
func (r *Runtime) closeTab(w *model.Workspace, t *model.Tab) {
	for _, id := range t.RootLayoutNode.Leaves() {
		delete(r.state.Panes, id)
	}
	for i, v := range w.Tabs {
		if v.ID == t.ID {
			w.Tabs = append(w.Tabs[:i], w.Tabs[i+1:]...)
			break
		}
	}
	if w.ActiveTabID == t.ID {
		w.ActiveTabID = ""
		if len(w.Tabs) > 0 {
			w.ActiveTabID = w.Tabs[0].ID
		}
	}
}
func (r *Runtime) removePane(id string) {
	pane := r.state.Panes[id]
	if pane == nil {
		return
	}
	delete(r.state.Panes, id)
	delete(r.sessions, id)
	delete(r.agentPrompts, id)
	delete(r.commandNumbered, id)
	delete(r.resuming, id)
	delete(r.promptAware, id)
	delete(r.commandPending, id)
	delete(r.unreadableHistory, id)
	delete(r.viewportOffsets, id)
	w, tab := r.state.Tab(pane.TabID)
	if tab == nil {
		return
	}
	tab.RootLayoutNode = tab.RootLayoutNode.Remove(id)
	if tab.RootLayoutNode == nil {
		r.closeTab(w, tab)
		return
	}
	if tab.ActivePaneID == id {
		tab.ActivePaneID = tab.RootLayoutNode.Leaves()[0]
	}
}
func (r *Runtime) register(pane *model.Pane, agent *model.Agent) error {
	if agent == nil || agent.RootPID == 0 || !agents.ValidState(agent.State) {
		return fmt.Errorf("agent root PID and valid state required")
	}
	items, err := process.Snapshot()
	if err != nil {
		return err
	}
	owned := agent.RootPID == pane.PID
	for _, p := range process.Descendants(items, pane.PID) {
		if p.PID == agent.RootPID {
			owned = true
		}
	}
	if !owned {
		// Processes orphaned by Git Bash fork/exec still carry the pane ID they
		// inherited from the pane shell.
		if id, ok := process.EnvironmentVariable(agent.RootPID, paneIDVariable); ok && id == pane.ID {
			owned = true
		}
	}
	if !owned {
		return fmt.Errorf("agent is not owned by this pane")
	}
	generation := process.Generation(agent.RootPID)
	if generation == "" || generation != agent.ProcessGeneration {
		return fmt.Errorf("agent process generation mismatch")
	}
	if pane.Agent != nil && pane.Agent.RootPID != 0 && process.Generation(pane.Agent.RootPID) == pane.Agent.ProcessGeneration && pane.Agent.RootPID != agent.RootPID {
		return fmt.Errorf("nested agent cannot replace root identity")
	}
	if agents.Detect(agent.Executable) != agent.Type {
		// Script-hosted agents (node.exe, python.exe, ...) are verified against
		// the real command line of the registered root process.
		if !agents.IsScriptHost(agent.Executable) || agents.DetectCommandLine(process.CommandLine(agent.RootPID)) != agent.Type {
			return fmt.Errorf("agent executable does not match adapter")
		}
	}
	// Never trust a caller-supplied resume command.
	agent.ResumeCommand = agents.ResumeCommand(agent.Type)
	pane.Agent = agent
	return nil
}
// paneIDVariable is set on every pane shell and inherited by its children.
const paneIDVariable = "PANEACEA_PANE_ID"

// cachedPaneID reads the inherited pane ID of a process, cached per process generation.
func cachedPaneID(pid uint32) string {
	return process.CachedEnvironmentVariable(pid, process.Generation(pid), paneIDVariable)
}

// msysForked reports whether an orphan is an MSYS program, i.e. a Git Bash
// fork/exec artifact rather than a detached daemon started from the pane.
func msysForked(pid uint32) bool {
	return process.IsMSYSImage(process.ImagePath(pid))
}

// paneOrphans maps pane IDs to processes cut off from the pane's process tree
// (their parent exited, e.g. Git Bash running an npm shell shim via fork/exec),
// plus their descendants. Ownership comes from the inherited PANEACEA_PANE_ID.
// Only MSYS orphans are considered, so detached daemons launched from a pane
// (servers, a nested runtime) never count as the pane's agent or command.
func paneOrphans(items []process.Info, paneID func(pid uint32) string, forked func(pid uint32) bool) map[string][]process.Info {
	result := map[string][]process.Info{}
	for _, o := range process.Orphans(items) {
		if !forked(o.PID) {
			continue
		}
		id := paneID(o.PID)
		// MSYS processes (sh.exe) keep their environment internally; the pane ID
		// is only visible in the Windows environment of their direct children.
		for _, child := range items {
			if id != "" {
				break
			}
			if child.ParentPID == o.PID {
				id = paneID(child.PID)
			}
		}
		if id == "" {
			continue
		}
		result[id] = append(result[id], o)
		result[id] = append(result[id], process.Descendants(items, o.PID)...)
	}
	return result
}

// cachedCommandLine reads a process command line, cached per process generation.
func cachedCommandLine(pid uint32) string {
	return process.CachedCommandLine(pid, process.Generation(pid))
}

// detectAgentKind detects an agent by image name, falling back to the command
// line for script hosts such as node.exe or python.exe.
func detectAgentKind(p process.Info, commandLine func(pid uint32) string) string {
	if kind := agents.Detect(p.Executable); kind != "" {
		return kind
	}
	if !agents.IsScriptHost(p.Executable) {
		return ""
	}
	return agents.DetectCommandLine(commandLine(p.PID))
}

// runningProgramLabel shows the agent type instead of a bare script host
// (e.g. "codex" instead of "node.exe") while an agent is active in the pane.
func runningProgramLabel(program string, agent *model.Agent) string {
	if agent != nil && agent.Type != "" && agent.State != "done" && agents.IsScriptHost(program) {
		return agent.Type
	}
	return program
}

func (r *Runtime) monitor(ctx context.Context) {
	defer close(r.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := process.Snapshot()
			if err != nil {
				continue
			}
			process.PruneCommandLines(items)
			orphans := paneOrphans(items, cachedPaneID, msysForked)
			r.mu.Lock()
			changed := false
			for _, pane := range r.state.Panes {
				if pane.Status != "running" {
					continue
				}
				rawProgram := process.RunningProgram(items, pane.PID, pane.Executable)
				busy := foregroundCommand(items, pane.PID, orphans[pane.ID]) != ""
				if r.promptAware[pane.ID] {
					busy = r.commandPending[pane.ID]
				}
				if pane.Busy != busy {
					pane.Busy = busy
					changed = true
				}
				program := runningProgramLabel(rawProgram, pane.Agent)
				if pane.RunningProgram != program {
					pane.RunningProgram = program
					changed = true
				}
				if pane.Agent != nil && pane.Agent.RootPID != 0 && process.Generation(pane.Agent.RootPID) == pane.Agent.ProcessGeneration {
					continue
				}
				detected := false
				candidates := process.Descendants(items, pane.PID)
				for _, p := range items {
					if p.PID == pane.PID {
						candidates = append([]process.Info{p}, candidates...)
						break
					}
				}
				candidates = append(candidates, orphans[pane.ID]...)
				for _, p := range candidates {
					kind := detectAgentKind(p, cachedCommandLine)
					if kind == "" {
						continue
					}
					generation := process.Generation(p.PID)
					if generation == "" {
						continue
					}
					pane.Agent = newDetectedAgent(kind, p.PID, generation)
					// A (resumed) agent is running; release the resume lock.
					delete(r.resuming, pane.ID)
					changed = true
					detected = true
					break
				}
				if !detected && pane.Agent != nil && pane.Agent.State != "done" {
					pane.Agent.State = "done"
					changed = true
				}
				// Re-label with the agent state decided on this tick.
				if label := runningProgramLabel(rawProgram, pane.Agent); pane.RunningProgram != label {
					pane.RunningProgram = label
					changed = true
				}
			}
			if changed {
				r.state.Revision++
				_ = r.store.Save(r.state)
			}
			r.mu.Unlock()
		}
	}
}
