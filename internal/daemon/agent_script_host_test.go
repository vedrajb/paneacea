package daemon

import (
	"testing"

	"github.com/paneacea/paneacea/internal/model"
	"github.com/paneacea/paneacea/internal/process"
)

func TestDetectAgentKindFallsBackToCommandLineForScriptHosts(t *testing.T) {
	lines := map[uint32]string{
		2: `node C:\npm\node_modules\@openai\codex\bin\codex.js`,
		3: `node C:\work\server.js`,
		4: `node C:\npm\node_modules\@openai\codex\bin\codex.js`,
	}
	queried := map[uint32]bool{}
	commandLine := func(pid uint32) string { queried[pid] = true; return lines[pid] }
	cases := []struct {
		info process.Info
		want string
	}{
		{process.Info{PID: 1, Executable: "claude.exe"}, "claude"},
		{process.Info{PID: 2, Executable: "node.exe"}, "codex"},
		{process.Info{PID: 3, Executable: "node.exe"}, ""},
		{process.Info{PID: 4, Executable: "cmd.exe"}, ""},
	}
	for _, c := range cases {
		if got := detectAgentKind(c.info, commandLine); got != c.want {
			t.Fatalf("pid %d: got %q want %q", c.info.PID, got, c.want)
		}
	}
	if queried[1] || queried[4] {
		t.Fatal("command line must only be read for script hosts without a name match")
	}
}

func TestPaneOrphansGroupsByInheritedPaneID(t *testing.T) {
	// sh.exe (20) lost its parent bash (99); node (21) and codex (22) run under it.
	items := []process.Info{
		{PID: 10, ParentPID: 1, Executable: "bash.exe"},
		{PID: 1, Executable: "paneacea-runtime.exe"},
		{PID: 20, ParentPID: 99, Executable: "sh.exe"},
		{PID: 21, ParentPID: 20, Executable: "node.exe"},
		{PID: 22, ParentPID: 21, Executable: "codex.exe"},
		{PID: 30, ParentPID: 98, Executable: "unrelated.exe"},
		// A detached daemon started from the pane: same pane ID, not MSYS.
		{PID: 40, ParentPID: 97, Executable: "paneacea-runtime.exe"},
		{PID: 41, ParentPID: 40, Executable: "bash.exe"},
	}
	// Like MSYS sh.exe, the orphan itself exposes no pane ID; its child does.
	ids := map[uint32]string{21: "pane-a", 40: "pane-a", 41: "pane-a"}
	forked := func(pid uint32) bool { return pid == 20 }
	got := paneOrphans(items, func(pid uint32) string { return ids[pid] }, forked)
	if len(got) != 1 || len(got["pane-a"]) != 3 || got["pane-a"][0].PID != 20 {
		t.Fatalf("got %+v", got)
	}
	kinds := []string{}
	for _, p := range got["pane-a"] {
		if kind := detectAgentKind(p, func(uint32) string { return "" }); kind != "" {
			kinds = append(kinds, kind)
		}
	}
	if len(kinds) != 1 || kinds[0] != "codex" {
		t.Fatalf("kinds %v", kinds)
	}
}

func TestAgentClearedOnlyAfterAnotherCommandPrompt(t *testing.T) {
	agent := &model.Agent{Type: "claude", RootPID: 7, ProcessGeneration: "g1"}
	var state agentPrompt
	var clear bool
	// Prompts while the agent runs (e.g. emitted by the agent) never clear it.
	if state, clear = countAgentPrompt(state, agent, true, "4"); clear {
		t.Fatal("cleared while agent alive")
	}
	// Prompt after the agent exits: agent was the last command, keep icon.
	if state, clear = countAgentPrompt(state, agent, false, "5"); clear {
		t.Fatal("cleared on the agent's own exit prompt")
	}
	// Empty Enter: command number unchanged, keep icon.
	if state, clear = countAgentPrompt(state, agent, false, "5"); clear {
		t.Fatal("cleared on empty Enter")
	}
	// Another command ran (e.g. pwd), clear icon.
	if _, clear = countAgentPrompt(state, agent, false, "6"); !clear {
		t.Fatal("expected clear after another command")
	}
	// A new agent identity resets the state.
	other := &model.Agent{Type: "codex", RootPID: 8, ProcessGeneration: "g2"}
	if _, clear = countAgentPrompt(state, other, false, "6"); clear {
		t.Fatal("new agent must start fresh")
	}
}

func TestAgentClearedOnSecondPromptWithoutCommandNumbers(t *testing.T) {
	agent := &model.Agent{Type: "claude", RootPID: 7, ProcessGeneration: "g1"}
	state, clear := countAgentPrompt(agentPrompt{}, agent, false, "")
	if clear {
		t.Fatal("cleared on the agent's own exit prompt")
	}
	if _, clear = countAgentPrompt(state, agent, false, ""); !clear {
		t.Fatal("expected clear on next prompt")
	}
}

func TestMetadataCommandNumberKeepsAgentOnEmptyEnter(t *testing.T) {
	pane := &model.Pane{ID: "p1", Agent: &model.Agent{Type: "claude", RootPID: 0xFFFFFFF0, ProcessGeneration: "dead"}}
	r := &Runtime{state: &model.State{Panes: map[string]*model.Pane{"p1": pane}}}
	if r.agentPromptLocked(pane, "5") || pane.Agent == nil {
		t.Fatal("exit prompt must keep agent")
	}
	if r.agentPromptLocked(pane, "5") || pane.Agent == nil {
		t.Fatal("empty Enter must keep agent")
	}
	if !r.agentPromptLocked(pane, "6") || pane.Agent != nil {
		t.Fatal("new command must clear agent")
	}
}

func TestRunningProgramLabelShowsAgentForScriptHost(t *testing.T) {
	active := &model.Agent{Type: "codex", State: "unknown"}
	if got := runningProgramLabel("node.exe", active); got != "codex" {
		t.Fatalf("got %q", got)
	}
	if got := runningProgramLabel("node.exe", &model.Agent{Type: "codex", State: "done"}); got != "node.exe" {
		t.Fatalf("done agent: got %q", got)
	}
	if got := runningProgramLabel("node.exe", nil); got != "node.exe" {
		t.Fatalf("no agent: got %q", got)
	}
	if got := runningProgramLabel("vim.exe", active); got != "vim.exe" {
		t.Fatalf("non script host: got %q", got)
	}
}
