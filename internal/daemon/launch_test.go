package daemon

import "testing"

func TestShellIntegrationSetsTerminalColorEnvironment(t *testing.T) {
	environment := map[string]string{}
	shellIntegration("bash.exe", nil, environment)
	if environment["TERM"] != "xterm-256color" {
		t.Fatalf("TERM = %q", environment["TERM"])
	}
	if environment["COLORTERM"] != "truecolor" {
		t.Fatalf("COLORTERM = %q", environment["COLORTERM"])
	}
}

func TestShellIntegrationPreservesTerminalColorEnvironment(t *testing.T) {
	environment := map[string]string{
		"term":      "custom-terminal",
		"COLORTERM": "custom-color",
	}
	shellIntegration("bash.exe", nil, environment)
	if environment["term"] != "custom-terminal" {
		t.Fatalf("term = %q", environment["term"])
	}
	if environment["COLORTERM"] != "custom-color" {
		t.Fatalf("COLORTERM = %q", environment["COLORTERM"])
	}
}

func TestShellIntegrationReplacesDumbTerminalEnvironment(t *testing.T) {
	environment := map[string]string{
		"TERM":      "dumb",
		"COLORTERM": "",
	}
	shellIntegration("bash.exe", nil, environment)
	if environment["TERM"] != "xterm-256color" {
		t.Fatalf("TERM = %q", environment["TERM"])
	}
	if environment["COLORTERM"] != "truecolor" {
		t.Fatalf("COLORTERM = %q", environment["COLORTERM"])
	}
}

func TestShellIntegrationTracksGitBashWorkingDirectory(t *testing.T) {
	environment := map[string]string{}
	shellIntegration("bash.exe", nil, environment)
	if environment["PROMPT_COMMAND"] != `printf '\033]7;file://localhost/%s\007' "$(pwd -W 2>/dev/null || pwd)"` {
		t.Fatalf("PROMPT_COMMAND = %q", environment["PROMPT_COMMAND"])
	}
}
