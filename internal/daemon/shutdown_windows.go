package daemon

import "golang.org/x/sys/windows"

var getSystemMetrics = windows.NewLazySystemDLL("user32.dll").NewProc("GetSystemMetrics")

func systemShuttingDown() bool {
	const smShuttingDown = 0x2000
	result, _, _ := getSystemMetrics.Call(smShuttingDown)
	return result != 0
}
