package agents

import (
	"github.com/paneacea/paneacea/internal/model"
	"reflect"
	"testing"
)

func TestResumePreservesFlagsAndReplacesIdentity(t *testing.T) {
	agent := &model.Agent{Type: "claude", Executable: "claude.exe", SessionID: "new", Arguments: []string{"--model", "sonnet", "--resume", "old", "--verbose"}}
	ex, args, err := BuildResumeCommand(agent)
	if err != nil || ex != "claude.exe" || !reflect.DeepEqual(args, []string{"--model", "sonnet", "--verbose", "--resume", "new"}) {
		t.Fatalf("%s %v %v", ex, args, err)
	}
}
func TestUnknownAndUnsupportedResume(t *testing.T) {
	for _, a := range []*model.Agent{nil, {Type: "copilot", SessionID: "s", Executable: "copilot"}, {Type: "codex", SessionID: "--all", Executable: "codex"}, {Type: "codex", SessionID: "s"}} {
		if _, _, err := BuildResumeCommand(a); err == nil {
			t.Fatalf("unsupported metadata accepted: %+v", a)
		}
	}
}
func TestAgentDetectionExactNames(t *testing.T) {
	if Detect("my-codex-helper.exe") != "" || Detect("codex.exe") != "codex" || Detect("CLAUDE.EXE") != "claude" {
		t.Fatal("incorrect process detection")
	}
}
