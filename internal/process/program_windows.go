package process

import "strings"

func RunningProgram(items []Info, root uint32, fallback string) string {
	for _, p := range items {
		if p.PID == root {
			fallback = p.Executable
			break
		}
	}
	for _, p := range Descendants(items, root) {
		switch strings.ToLower(p.Executable) {
		case "conhost.exe", "openconsole.exe":
			continue
		}
		fallback = p.Executable
	}
	return fallback
}
