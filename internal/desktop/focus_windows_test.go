//go:build windows

package desktop

import "testing"

func TestFocusWebViewWithoutWailsWindow(t *testing.T) {
	if focusWebView() {
		t.Fatal("focusWebView() returned true without a Wails window")
	}
}
