package desktop

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var blockedLinkExtensions = map[string]bool{
	".exe": true, ".com": true, ".bat": true, ".cmd": true, ".scr": true, ".msi": true,
	".ps1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true, ".wsf": true,
	".lnk": true, ".reg": true, ".hta": true, ".cpl": true, ".jar": true, ".pif": true,
}

// linkPath returns "" for http(s) URLs, or a cleaned local path for file:// URLs and drive paths.
func linkPath(target string) (string, error) {
	lower := strings.ToLower(target)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		if _, err := url.ParseRequestURI(target); err != nil {
			return "", fmt.Errorf("invalid link")
		}
		return "", nil
	case strings.HasPrefix(lower, "file://"):
		parsed, err := url.Parse(target)
		if err != nil {
			return "", fmt.Errorf("invalid file link")
		}
		p := parsed.Path
		if parsed.Host != "" && parsed.Host != "localhost" {
			return "", fmt.Errorf("remote file links are not supported")
		}
		if len(p) > 2 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		return filepath.Clean(filepath.FromSlash(p)), nil
	case len(target) > 2 && target[1] == ':' && (target[2] == '\\' || target[2] == '/'):
		return filepath.Clean(filepath.FromSlash(target)), nil
	}
	return "", fmt.Errorf("unsupported link")
}

func openPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path not found: %s", path)
	}
	if !info.IsDir() && blockedLinkExtensions[strings.ToLower(filepath.Ext(path))] {
		return fmt.Errorf("refusing to open executable file")
	}
	if runtime.GOOS != "windows" {
		return exec.Command("xdg-open", path).Start()
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
}
