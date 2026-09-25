//go:build windows

package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procSendMessageW             = user32.NewProc("SendMessageW")
)

// focusWebView sends WM_SETFOCUS to this process's Wails window so Wails moves keyboard focus into WebView2.
func focusWebView() bool {
	processID := windows.GetCurrentProcessId()
	found := false

	callback := windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var windowProcessID uint32
		threadID, _, _ := procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&windowProcessID)))
		if threadID == 0 || windowProcessID != processID {
			return 1
		}
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}

		className := make([]uint16, 256)
		length, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&className[0])), uintptr(len(className)))
		if length == 0 || windows.UTF16ToString(className[:int(length)]) != "wailsWindow" {
			return 1
		}

		procSendMessageW.Call(hwnd, 0x0007 /* WM_SETFOCUS */, 0, 0)
		found = true
		return 0
	})

	_, _, _ = procEnumWindows.Call(callback, 0)
	return found
}
