package process

import "testing"

func TestRunningProgram(t *testing.T) {
	items := []Info{{PID: 1, Executable: "powershell.exe"}, {PID: 2, ParentPID: 1, Executable: "cmd.exe"}, {PID: 3, ParentPID: 2, Executable: "node.exe"}, {PID: 4, ParentPID: 1, Executable: "conhost.exe"}, {PID: 5, ParentPID: 99, Executable: "other.exe"}}
	if got := RunningProgram(items, 1, "fallback.exe"); got != "node.exe" {
		t.Fatalf("running child: %s", got)
	}
	if got := RunningProgram(items[:1], 1, "fallback.exe"); got != "powershell.exe" {
		t.Fatalf("shell after child exit: %s", got)
	}
	if got := RunningProgram(nil, 1, "fallback.exe"); got != "fallback.exe" {
		t.Fatalf("missing process: %s", got)
	}
}
