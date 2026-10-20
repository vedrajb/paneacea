package desktop

import "testing"

func TestLinkPath(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a":          "",
		"file:///C:/Users/x/report.html": `C:\Users\x\report.html`,
		`C:\Users\x\report.html`:         `C:\Users\x\report.html`,
		"C:/Users/x/report.html":         `C:\Users\x\report.html`,
	}
	for in, want := range cases {
		got, err := linkPath(in)
		if err != nil || got != want {
			t.Errorf("linkPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"javascript:alert(1)", "file://server/share/a", "relative/path"} {
		if _, err := linkPath(bad); err == nil {
			t.Errorf("linkPath(%q) should fail", bad)
		}
	}
}
