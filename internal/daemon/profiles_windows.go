package daemon

import (
	"context"
	"github.com/paneacea/paneacea/internal/model"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Profiles() []model.Profile {
	result := []model.Profile{
		{ID: "git-bash", Name: "Git Bash", Executable: filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"), Arguments: []string{"--login", "-i"}},
		{ID: "pwsh", Name: "PowerShell 7", Executable: "pwsh.exe", Arguments: []string{"-NoLogo"}},
		{ID: "powershell", Name: "Windows PowerShell", Executable: "powershell.exe", Arguments: []string{"-NoLogo"}},
		{ID: "cmd", Name: "Command Prompt", Executable: "cmd.exe", Arguments: []string{}},
	}
	for i := range result {
		if path, err := exec.LookPath(result[i].Executable); err == nil {
			result[i].Executable = path
			result[i].Available = true
		}
	}
	if path, err := exec.LookPath("wsl.exe"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, path, "--list", "--quiet")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		data, err := cmd.Output()
		if err == nil {
			units := make([]uint16, len(data)/2)
			for i := range units {
				units[i] = uint16(data[i*2]) | uint16(data[i*2+1])<<8
			}
			for _, name := range strings.Split(windows.UTF16ToString(units), "\n") {
				name = strings.TrimSpace(name)
				if name != "" {
					result = append(result, model.Profile{ID: "wsl:" + name, Name: "WSL · " + name, Executable: path, Arguments: []string{"--distribution", name}, Available: true})
				}
			}
		}
	}
	return result
}

func firstAvailableProfile() model.Profile {
	for _, profile := range Profiles() {
		if profile.Available {
			return profile
		}
	}
	return model.Profile{ID: "cmd", Name: "Command Prompt", Executable: "cmd.exe", Arguments: []string{}}
}
