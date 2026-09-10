package agents

import (
	"fmt"
	"github.com/paneacea/paneacea/internal/model"
	"path/filepath"
	"strings"
)

type Adapter struct {
	Type          string
	Names         []string
	ResumeFlag    string
	ResumeCommand string
}

var Adapters = []Adapter{
	{Type: "codex", Names: []string{"codex"}, ResumeCommand: "resume"},
	{Type: "claude", Names: []string{"claude"}, ResumeFlag: "--resume"},
	{Type: "opencode", Names: []string{"opencode"}, ResumeFlag: "--session"},
	{Type: "cursor", Names: []string{"cursor-agent", "agent"}, ResumeFlag: "--resume"},
	{Type: "copilot", Names: []string{"copilot"}},
	{Type: "hermes", Names: []string{"hermes"}, ResumeFlag: "--resume"},
}

func Detect(executable string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(executable)), ".exe")
	for _, a := range Adapters {
		for _, n := range a.Names {
			if name == n {
				return a.Type
			}
		}
	}
	return ""
}
func BuildResumeCommand(agent *model.Agent) (string, []string, error) {
	if agent == nil || agent.SessionID == "" || strings.HasPrefix(agent.SessionID, "-") {
		return "", nil, fmt.Errorf("a captured session ID is required")
	}
	for _, a := range Adapters {
		if a.Type != agent.Type {
			continue
		}
		if a.ResumeFlag == "" && a.ResumeCommand == "" {
			return "", nil, fmt.Errorf("resume is unsupported for %s", a.Type)
		}
		if agent.Executable == "" {
			return "", nil, fmt.Errorf("agent launch executable was not captured")
		}
		args := []string{}
		for i := 0; i < len(agent.Arguments); i++ {
			arg := agent.Arguments[i]
			if arg == "--resume" || arg == "--session" {
				i++
				continue
			}
			if strings.HasPrefix(arg, "--resume=") || strings.HasPrefix(arg, "--session=") {
				continue
			}
			if i == 0 && arg == "resume" {
				i++
				continue
			}
			args = append(args, arg)
		}
		if a.ResumeCommand != "" {
			args = append([]string{a.ResumeCommand, agent.SessionID}, args...)
		} else {
			args = append(args, a.ResumeFlag, agent.SessionID)
		}
		return agent.Executable, args, nil
	}
	return "", nil, fmt.Errorf("unknown agent type")
}
func ValidState(state string) bool {
	switch state {
	case "working", "waiting", "done", "idle", "unknown":
		return true
	}
	return false
}
