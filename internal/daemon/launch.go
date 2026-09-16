package daemon

import (
	"encoding/base64"
	"encoding/binary"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

func shellIntegration(executable string, arguments []string, environment map[string]string) []string {
	termSet := false
	for name, value := range environment {
		if strings.EqualFold(name, "TERM") {
			termSet = true
			if value == "" || strings.EqualFold(value, "dumb") {
				environment[name] = "xterm-256color"
			}
			break
		}
	}
	if !termSet {
		environment["TERM"] = "xterm-256color"
	}
	colorSet := false
	for name, value := range environment {
		if strings.EqualFold(name, "COLORTERM") {
			colorSet = true
			if value == "" {
				environment[name] = "truecolor"
			}
			break
		}
	}
	if !colorSet {
		environment["COLORTERM"] = "truecolor"
	}
	name := strings.ToLower(filepath.Base(executable))
	for _, arg := range arguments {
		switch strings.ToLower(arg) {
		case "-command", "-c", "-file", "-f", "-encodedcommand", "-ec":
			return arguments
		}
	}
	if name == "powershell.exe" || name == "pwsh.exe" {
		script := `$global:PaneaceaOriginalPrompt = $function:prompt; function global:prompt { $p = (Get-Location).ProviderPath; if ($p) { [Console]::Write(([char]27).ToString() + ']7;' + ([Uri]$p).AbsoluteUri + [char]7) }; if ($global:PaneaceaOriginalPrompt) { & $global:PaneaceaOriginalPrompt } else { 'PS ' + $p + '> ' } }`
		units := utf16.Encode([]rune(script))
		data := make([]byte, len(units)*2)
		for i, v := range units {
			binary.LittleEndian.PutUint16(data[i*2:], v)
		}
		return append(append([]string{}, arguments...), "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	}
	if name == "bash.exe" {
		environment["PROMPT_COMMAND"] = `printf '\033]7;file://localhost/%s\007' "$(pwd -W 2>/dev/null || pwd)"`
	}
	return arguments
}
