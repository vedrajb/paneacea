package daemon

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/paneacea/paneacea/internal/ipc"

	"github.com/paneacea/paneacea/internal/agents"
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/process"
)

func TestResumeCommandsOpenSessionPicker(t *testing.T) {
	for kind, want := range map[string]string{
		"codex": "codex resume", "claude": "claude --resume", "pi": "pi --resume", "cursor": "cursor-agent --resume",
		"gemini": "", "opencode": "", "": "",
	} {
		if got := agents.ResumeCommand(kind); got != want {
			t.Fatalf("%s: got %q want %q", kind, got, want)
		}
	}
}

func TestForegroundCommandIgnoresShellProcesses(t *testing.T) {
	idle := []process.Info{
		{PID: 10, ParentPID: 1, Executable: "bash.exe"},
		{PID: 11, ParentPID: 10, Executable: "bash.exe"},
		{PID: 12, ParentPID: 1, Executable: "conhost.exe"},
	}
	if got := foregroundCommand(idle, 10, nil); got != "" {
		t.Fatalf("idle pane reported busy: %q", got)
	}
	busy := append(idle, process.Info{PID: 13, ParentPID: 11, Executable: "vim.exe"})
	if got := foregroundCommand(busy, 10, nil); got != "vim.exe" {
		t.Fatalf("got %q", got)
	}
	// Orphaned Git Bash script shim processes also count as running commands.
	if got := foregroundCommand(idle, 10, []process.Info{{PID: 20, ParentPID: 99, Executable: "sh.exe"}}); got != "sh.exe" {
		t.Fatalf("orphan: got %q", got)
	}
}

func TestFailedResumeReleasesLock(t *testing.T) {
	r := &Runtime{state: &model.State{Panes: map[string]*model.Pane{}}}
	for i := 0; i < 2; i++ {
		if err := r.resumeAgent("missing"); err == nil || !strings.Contains(err.Error(), "terminal not found") {
			t.Fatalf("attempt %d: got %v", i, err)
		}
	}
}

func TestAgentsCarryRuntimeResumeCommand(t *testing.T) {
	if got := newDetectedAgent("codex", 1, "g"); got.ResumeCommand != "codex resume" || got.State != "unknown" {
		t.Fatalf("got %+v", got)
	}
	if got := newDetectedAgent("gemini", 1, "g"); got.ResumeCommand != "" {
		t.Fatalf("gemini: got %q", got.ResumeCommand)
	}
}

func TestClearLineInputMatchesShell(t *testing.T) {
	if clearLineInput(`C:\Program Files\Git\bin\bash.exe`) != "\x15" || clearLineInput("pwsh.exe") != "\x1b" || clearLineInput("cmd.exe") != "\x1b" || clearLineInput("nu.exe") != "" {
		t.Fatal("unexpected clear-line input")
	}
}

func TestAgentResumeTypesPickerCommandOnlyWhenPaneIsIdle(t *testing.T) {
	r, err := New(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: t.TempDir()}).(*model.State)
	// PATH points nowhere so the typed resume command cannot start a real agent.
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "cmd.exe", Arguments: []string{}, Environment: map[string]string{"PATH": `C:\paneacea-missing`}}).(*model.State)
	_, tab := state.Tab(state.Workspace(state.ActiveWorkspaceID).ActiveTabID)
	id := tab.ActivePaneID
	pid := state.Panes[id].PID
	setAgent := func(agent *model.Agent) {
		r.mu.Lock()
		r.state.Panes[id].Agent = agent
		r.mu.Unlock()
	}
	resume := func() error {
		_, err := r.Call(context.Background(), "agent.resume", []byte(`{"paneId":"`+id+`"}`))
		return err
	}
	expectError := func(want string) {
		t.Helper()
		if err := resume(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("want %q, got %v", want, err)
		}
	}

	expectError("no agent has run")
	setAgent(&model.Agent{Type: "gemini", State: "done"})
	expectError("has no resume session picker")
	setAgent(&model.Agent{Type: "codex", State: "unknown", RootPID: pid, ProcessGeneration: process.Generation(pid)})
	expectError("codex is still running")

	setAgent(&model.Agent{Type: "codex", State: "done"})
	if err := resume(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(string(invoke(t, r, "terminal.requestSnapshot", Params{PaneID: id}).(ipc.Output).Data), "codex resume") {
		if time.Now().After(deadline) {
			t.Fatal("resume command was not typed into the pane")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Repeated clicks are rejected while the resume is in flight.
	expectError("resume already in progress")
	// Detecting the resumed agent releases the lock (simulated here).
	r.mu.Lock()
	delete(r.resuming, id)
	r.mu.Unlock()

	// A foreground command (ping) makes the pane busy.
	invoke(t, r, "pane.sendInput", Params{PaneID: id, Data: `C:\Windows\System32\PING.EXE -n 30 127.0.0.1` + "\r"})
	deadline = time.Now().Add(15 * time.Second)
	for {
		err := resume()
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "busy running ping.exe") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("busy pane was not rejected: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
