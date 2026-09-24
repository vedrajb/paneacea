package daemon

import (
	"fmt"
	"path/filepath"
	"strings"
)

func bashHistoryEnvironment(path string) (string, error) {
	volume := filepath.VolumeName(path)
	if len(volume) != 2 || volume[1] != ':' {
		return "", fmt.Errorf("Bash history path must use a drive letter: %q", path)
	}
	return "/" + strings.ToLower(volume[:1]) + filepath.ToSlash(strings.TrimPrefix(path, volume)), nil
}

func configureBashStartup(executable string, arguments []string, environment map[string]string) ([]string, bool) {
	if !strings.EqualFold(filepath.Base(executable), "bash.exe") {
		return arguments, false
	}
	interactive, loginShell, noProfile, noRc, customRc, commandMode := false, false, false, false, false, false
	for i, argument := range arguments {
		switch argument {
		case "-i", "--interactive":
			interactive = true
		case "-l", "--login":
			loginShell = true
		case "--noprofile":
			noProfile = true
		case "--norc":
			noRc = true
		case "-c", "--command":
			commandMode = true
		case "--rcfile", "--init-file":
			customRc = i+1 < len(arguments)
		}
		if strings.HasPrefix(argument, "--rcfile=") || strings.HasPrefix(argument, "--init-file=") {
			customRc = true
		}
	}
	if !interactive || commandMode {
		return arguments, false
	}

	// Install from PROMPT_COMMAND before the first prompt so setup never echoes as terminal input.
	var init strings.Builder
	init.WriteString(`if [[ -z ${PANEACEA_BASH_INIT_DONE:-} ]]; then PANEACEA_BASH_INIT_DONE=1; PROMPT_COMMAND=`)
	if loginShell && !noProfile {
		init.WriteString(`; [[ -r /etc/profile ]] && . /etc/profile; if [[ -r ~/.bash_profile ]]; then . ~/.bash_profile; elif [[ -r ~/.bash_login ]]; then . ~/.bash_login; elif [[ -r ~/.profile ]]; then . ~/.profile; fi`)
	} else if !loginShell && !noRc && !customRc {
		init.WriteString(`; [[ -r ~/.bashrc ]] && . ~/.bashrc`)
	}
	init.WriteString(`; __paneacea_prompt_hook() { printf '\033]7;file://localhost/%s\007' "$(pwd -W 2>/dev/null || pwd)"; history -a; }; if declare -p PROMPT_COMMAND 2>/dev/null | grep -q 'declare -a'; then PROMPT_COMMAND+=(__paneacea_prompt_hook); else PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND; }__paneacea_prompt_hook"; fi; shopt -s histappend; fi; __paneacea_prompt_hook`)
	environment["PROMPT_COMMAND"] = init.String()
	updated := make([]string, 0, len(arguments)+2)
	if !noProfile {
		updated = append(updated, "--noprofile")
	}
	if !loginShell && !noRc && !customRc {
		updated = append(updated, "--norc")
	}
	updated = append(updated, arguments...)
	return updated, true
}
