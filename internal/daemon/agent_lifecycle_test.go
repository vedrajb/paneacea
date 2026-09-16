package daemon

import (
	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/process"
	"testing"
)

func TestAgentStopPreparationClearsLiveProcessIdentity(t *testing.T) {
	pane := &model.Pane{Status: "running", PID: 42, Agent: &model.Agent{State: "working", RootPID: 43, ProcessGeneration: "generation"}}
	prepareAgentStop(pane)
	if pane.Status != agentStoppedStatus || pane.PID != 0 {
		t.Fatalf("pane was not marked for restoration: %+v", pane)
	}
	if pane.Agent.RootPID != 0 || pane.Agent.ProcessGeneration != "" || pane.Agent.State != "idle" {
		t.Fatalf("agent identity was not cleared: %+v", pane.Agent)
	}
}

func TestAgentRunningRequiresMatchingProcessGeneration(t *testing.T) {
	agent := &model.Agent{RootPID: 42, ProcessGeneration: "generation"}
	if !agentIsRunning(agent, "generation") {
		t.Fatal("matching process generation was rejected")
	}
	for _, generation := range []string{"", "other"} {
		if agentIsRunning(agent, generation) {
			t.Fatalf("generation %q was accepted", generation)
		}
	}
	if agentIsRunning(nil, "generation") || agentIsRunning(&model.Agent{ProcessGeneration: "generation"}, "generation") {
		t.Fatal("incomplete agent identity was accepted")
	}
}

func TestAgentStopLeavesOrdinaryShellRunning(t *testing.T) {
	r, err := New(&memoryStore{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}}).(*model.State)
	w := state.Workspace(state.ActiveWorkspaceID)
	_, tab := state.Tab(w.ActiveTabID)
	id := tab.ActivePaneID
	pid := state.Panes[id].PID
	state = invoke(t, r, "agent.stop", nil).(*model.State)
	if state.Panes[id].PID != pid || state.Panes[id].Status != "running" {
		t.Fatal("agent stop changed an ordinary shell")
	}
	state = invoke(t, r, "agent.restore", nil).(*model.State)
	if state.Panes[id].PID != pid || state.Panes[id].Status != "running" {
		t.Fatal("agent restore changed an ordinary shell")
	}
}

func TestAgentStopMarksAndTerminatesRegisteredAgentPane(t *testing.T) {
	store := &memoryStore{}
	r, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	state := invoke(t, r, "workspace.create", Params{Name: "Test", RootDirectory: t.TempDir()}).(*model.State)
	state = invoke(t, r, "tab.create", Params{WorkspaceID: state.ActiveWorkspaceID, Executable: "powershell.exe", Arguments: []string{"-NoLogo", "-NoProfile"}}).(*model.State)
	w := state.Workspace(state.ActiveWorkspaceID)
	_, tab := state.Tab(w.ActiveTabID)
	id := tab.ActivePaneID
	pid := state.Panes[id].PID
	generation := process.Generation(pid)
	if generation == "" {
		t.Fatal("could not capture test process generation")
	}
	r.mu.Lock()
	r.state.Panes[id].Agent = &model.Agent{Type: "codex", State: "working", SessionID: "session", RootPID: pid, ProcessGeneration: generation, Executable: "codex.exe"}
	r.mu.Unlock()
	state = invoke(t, r, "agent.stop", nil).(*model.State)
	pane := state.Panes[id]
	if pane.Status != agentStoppedStatus || pane.PID != 0 || pane.Agent.State != "idle" {
		t.Fatalf("registered agent pane was not stopped: %+v", pane)
	}
}
