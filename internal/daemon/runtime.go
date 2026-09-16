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
	"github.com/paneacea/paneacea/internal/process"
	"github.com/paneacea/paneacea/internal/terminal"
	"io"
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
type Runtime struct {
	mu             sync.Mutex
	state          *model.State
	store          Storage
	sessions       map[string]*terminal.Session
	outputs        map[string]*outputBuffer
	stoppingAgents map[string]bool
	cancel         context.CancelFunc
	done           chan struct{}
	shutting       bool
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
	r := &Runtime{state: state, store: store, sessions: map[string]*terminal.Session{}, outputs: map[string]*outputBuffer{}, stoppingAgents: map[string]bool{}, done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.mu.Lock()
	for _, p := range state.Panes {
		if p.Status == "exited" {
			o := newOutput()
			o.exit()
			r.outputs[p.ID] = o
			continue
		}
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
	return r, nil
}
func (r *Runtime) Close() {
	r.cancel()
	<-r.done
	r.mu.Lock()
	r.shutting = true
	sessions := make([]*terminal.Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	r.mu.Unlock()
	for _, s := range sessions {
		s.Close()
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
	output.emulator = vt.NewEmulator(columns, rows)
	output.modes = map[ansi.Mode]bool{}
	output.emulator.SetScrollbackSize(min(r.state.Settings.Scrollback, 2000))
	output.emulator.SetCallbacks(vt.Callbacks{EnableMode: func(mode ansi.Mode) { output.modes[mode] = true }, DisableMode: func(mode ansi.Mode) { output.modes[mode] = false }})
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
	if p.Status == agentStoppedStatus {
		if p.Agent == nil || p.Agent.SessionID == "" {
			return fmt.Errorf("agent session metadata is unavailable")
		}
		ex, args, e := agents.BuildResumeCommand(p.Agent)
		if e != nil {
			return e
		}
		executable = ex
		arguments = args
	} else if p.Agent != nil && p.Agent.SessionID != "" {
		if ex, args, e := agents.BuildResumeCommand(p.Agent); e == nil {
			executable = ex
			arguments = args
		}
	}
	arguments = shellIntegration(executable, arguments, environment)
	workingDirectory := p.CurrentWorkingDirectory
	if _, err = directory(workingDirectory); err != nil {
		workingDirectory = p.InitialWorkingDirectory
	}
	var metadataMu sync.Mutex
	metadata := ""
	session, err := terminal.NewSession(terminal.Options{Executable: executable, Arguments: arguments, Environment: environment, WorkingDirectory: workingDirectory, Columns: p.Columns, Rows: p.Rows,
		OnOutput: func(data string) {
			output.append([]byte(data))
			metadataMu.Lock()
			metadata += data
			for {
				start := strings.Index(metadata, "\x1b]")
				if start < 0 {
					metadata = ""
					break
				}
				metadata = metadata[start:]
				end := strings.IndexAny(metadata[2:], "\a\x1b")
				if end < 0 {
					if len(metadata) > 8192 {
						metadata = ""
					}
					break
				}
				end += 2
				value := metadata[2:end]
				metadata = metadata[end+1:]
				r.metadata(p.ID, value)
			}
			metadataMu.Unlock()
		},
		OnExit: func(exitErr error) {
			_ = output.emulator.InputPipe().(io.Closer).Close()
			output.exit()
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.shutting || r.outputs[p.ID] != output {
				return
			}
			if r.stoppingAgents[p.ID] {
				delete(r.stoppingAgents, p.ID)
				return
			}
			if current := r.state.Panes[p.ID]; current != nil {
				current.Status = "exited"
				current.PID = 0
				if current.Agent != nil {
					current.Agent.State = "done"
				}
				if exitErr != nil {
					current.Error = exitErr.Error()
				}
				r.state.Revision++
				if err := r.store.Save(r.state); err != nil {
					current.Error = err.Error()
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
	if p.Agent != nil && p.Agent.SessionID != "" && agents.Detect(executable) == p.Agent.Type {
		p.Agent.RootPID = p.PID
		p.Agent.ProcessGeneration = process.Generation(p.PID)
		p.Agent.State = "unknown"
	}
	p.RuntimeTerminalID = p.ID
	p.Status = "running"
	p.Error = ""
	return nil
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
		if method == "terminal.attach" || method == "terminal.requestSnapshot" {
		return output.snapshot(), nil
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		value, err := output.read(ctx, p.Sequence)
		if errors.Is(err, context.DeadlineExceeded) {
			return ipc.Output{Sequence: p.Sequence}, nil
		}
		return value, err
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
			return struct{}{}, session.Write([]byte(p.Data))
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
	if method == "agent.restore" {
		return r.restoreAgents()
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

func (r *Runtime) restoreAgents() (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := clone(r.state)
	oldSessions := map[string]*terminal.Session{}
	oldOutputs := map[string]*outputBuffer{}
	for id, session := range r.sessions {
		oldSessions[id] = session
	}
	for id, output := range r.outputs {
		oldOutputs[id] = output
	}
	changed := false
	for _, pane := range r.state.Panes {
		if pane.Status != agentStoppedStatus || pane.Agent == nil || pane.Agent.SessionID == "" {
			continue
		}
		if r.stoppingAgents[pane.ID] {
			continue
		}
		if err := r.launch(pane); err != nil {
			pane.Status = "error"
			pane.Error = err.Error()
		} else {
			pane.Error = ""
		}
		changed = true
	}
	if changed {
		r.state.Revision++
		if err := r.store.Save(r.state); err != nil {
			for id, session := range r.sessions {
				if oldSessions[id] != session {
					go session.Close()
				}
			}
			for id, output := range r.outputs {
				if oldOutputs[id] != output {
					output.exit()
				}
			}
			r.sessions = oldSessions
			r.outputs = oldOutputs
			r.state = before
			return nil, fmt.Errorf("persist agent restoration: %w", err)
		}
	}
	return clone(r.state), nil
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
	case "pane.focus", "pane.close", "pane.restart", "agent.resume", "agent.register":
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
		case "pane.restart", "agent.resume":
			if pane.Status == "running" && method == "pane.restart" {
				return fmt.Errorf("close the running command before restarting or resuming")
			}
			if method == "agent.resume" {
				if pane.Agent != nil && pane.Agent.RootPID != 0 && process.Generation(pane.Agent.RootPID) == pane.Agent.ProcessGeneration {
					return fmt.Errorf("agent session is already running")
				}
				if _, _, err := agents.BuildResumeCommand(pane.Agent); err != nil {
					return err
				}
			} else {
				pane.Agent = nil
			}
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
func (r *Runtime) register(pane *model.Pane, agent *model.Agent) error {
	if agent == nil || agent.SessionID == "" || agent.RootPID == 0 || !agents.ValidState(agent.State) {
		return fmt.Errorf("agent session, root PID, and valid state required")
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
		return fmt.Errorf("agent executable does not match adapter")
	}
	pane.Agent = agent
	return nil
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
			r.mu.Lock()
			changed := false
			for _, pane := range r.state.Panes {
				if pane.Status != "running" {
					continue
				}
				program := process.RunningProgram(items, pane.PID, pane.Executable)
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
				for _, p := range candidates {
					kind := agents.Detect(p.Executable)
					if kind == "" {
						continue
					}
					generation := process.Generation(p.PID)
					if generation == "" {
						continue
					}
					pane.Agent = &model.Agent{Type: kind, State: "unknown", RootPID: p.PID, ProcessGeneration: generation}
					changed = true
					detected = true
					break
				}
				if !detected && pane.Agent != nil && pane.Agent.State != "done" {
					pane.Agent.State = "done"
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
