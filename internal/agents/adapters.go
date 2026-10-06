package agents

import (
	"path/filepath"
	"strings"
)

type Adapter struct {
	Type  string
	Names []string
	// Packages are npm package names used to recognise agents launched through
	// a script host such as node.exe, bun.exe or deno.exe.
	Packages []string
	// Modules are Python module names used to recognise agents launched
	// through python.exe (for example "python -m aider").
	Modules []string
}

var Adapters = []Adapter{
	{Type: "codex", Names: []string{"codex"}, Packages: []string{"@openai/codex"}},
	{Type: "claude", Names: []string{"claude"}, Packages: []string{"@anthropic-ai/claude-code"}},
	{Type: "opencode", Names: []string{"opencode"}, Packages: []string{"opencode-ai"}},
	{Type: "cursor", Names: []string{"cursor-agent", "agent"}},
	{Type: "copilot", Names: []string{"copilot"}, Packages: []string{"@github/copilot"}},
	{Type: "hermes", Names: []string{"hermes"}},
	{Type: "gemini", Names: []string{"gemini"}, Packages: []string{"@google/gemini-cli"}},
	{Type: "qwen", Names: []string{"qwen"}, Packages: []string{"@qwen-code/qwen-code"}},
	{Type: "aider", Names: []string{"aider"}, Modules: []string{"aider"}},
	{Type: "crush", Names: []string{"crush"}},
	{Type: "goose", Names: []string{"goose"}},
	{Type: "amp", Names: []string{"amp"}, Packages: []string{"@sourcegraph/amp"}},
	{Type: "kiro", Names: []string{"kiro-cli"}},
	{Type: "amazonq", Names: []string{"qchat"}},
	{Type: "openhands", Names: []string{"openhands"}, Modules: []string{"openhands"}},
	{Type: "pi", Names: []string{"pi"}, Packages: []string{"@earendil-works/pi-coding-agent", "@mariozechner/pi-coding-agent"}},
	{Type: "cline", Names: []string{"cline"}, Packages: []string{"cline"}},
	{Type: "kilocode", Names: []string{"kilocode"}, Packages: []string{"@kilocode/cli"}},
}

// resumeCommands open each agent's interactive session picker (no session ID),
// so the user can choose which session to resume.
var resumeCommands = map[string]string{
	"codex":  "codex resume",
	"claude": "claude --resume",
	"pi":     "pi --resume",
	"cursor": "cursor-agent --resume",
}

// ResumeCommand returns the session picker command for agentType, or "" when
// the agent has no picker-style resume command.
func ResumeCommand(agentType string) string {
	return resumeCommands[agentType]
}

// scriptHosts are runtimes whose image name says nothing about the agent they
// run; for these the command line is inspected instead.
var scriptHosts = map[string]bool{
	"node": true, "bun": true, "deno": true,
	"python": true, "pythonw": true, "python3": true, "py": true,
}

func executableName(executable string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
}

// IsScriptHost reports whether executable is a script runtime (node, bun,
// deno, python) that may be hosting an agent.
func IsScriptHost(executable string) bool {
	return scriptHosts[executableName(executable)]
}

// DetectCommandLine matches a script host command line against adapter
// packages/modules. Matches are anchored (node_modules/<pkg>/, npm:<pkg>,
// site-packages/<module>/, -m <module>) so short names such as "pi" or "amp"
// never match arbitrary arguments.
func DetectCommandLine(commandLine string) string {
	cl := strings.ReplaceAll(strings.ToLower(commandLine), "\\", "/")
	if cl == "" {
		return ""
	}
	for _, a := range Adapters {
		for _, pkg := range a.Packages {
			if containsAnchored(cl, "node_modules/"+pkg, "/\"' ") || containsAnchored(cl, "npm:"+pkg, "/@\"' ") {
				return a.Type
			}
		}
		for _, mod := range a.Modules {
			if containsAnchored(cl, "site-packages/"+mod, "/\"' ") || containsArgPair(cl, "-m", mod) {
				return a.Type
			}
		}
	}
	return ""
}

// containsAnchored reports whether needle occurs in s followed by end of
// string or one of the terminator characters.
func containsAnchored(s, needle, terminators string) bool {
	for offset := 0; ; {
		i := strings.Index(s[offset:], needle)
		if i < 0 {
			return false
		}
		end := offset + i + len(needle)
		if end == len(s) || strings.ContainsRune(terminators, rune(s[end])) {
			return true
		}
		offset = offset + i + 1
	}
}

// containsArgPair reports whether flag is immediately followed by value as
// separate command line arguments (quotes ignored).
func containsArgPair(s, flag, value string) bool {
	fields := strings.Fields(strings.ReplaceAll(s, "\"", " "))
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == flag && fields[i+1] == value {
			return true
		}
	}
	return false
}

func Detect(executable string) string {
	name := executableName(executable)
	for _, a := range Adapters {
		for _, n := range a.Names {
			if name == n {
				return a.Type
			}
		}
	}
	return ""
}
func ValidState(state string) bool {
	switch state {
	case "working", "waiting", "done", "idle", "unknown":
		return true
	}
	return false
}
