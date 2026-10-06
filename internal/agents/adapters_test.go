package agents

import "testing"

func TestAgentDetectionExactNames(t *testing.T) {
	if Detect("my-codex-helper.exe") != "" || Detect("codex.exe") != "codex" || Detect("CLAUDE.EXE") != "claude" {
		t.Fatal("incorrect process detection")
	}
}
func TestAgentDetectionExtendedNames(t *testing.T) {
	for executable, want := range map[string]string{
		"C:\\x\\gemini.exe": "gemini",
		"aider.exe":         "aider",
		"kiro-cli":          "kiro",
		"qchat.exe":         "amazonq",
		"PI.EXE":            "pi",
	} {
		if got := Detect(executable); got != want {
			t.Fatalf("%s: got %q want %q", executable, got, want)
		}
	}
	for _, executable := range []string{"gemini-helper.exe", "pip.exe"} {
		if got := Detect(executable); got != "" {
			t.Fatalf("%s: got %q", executable, got)
		}
	}
}
