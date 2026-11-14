package session

import (
	"strings"
	"testing"
)

// A page served as one long line comes out with each element on its own line,
// with every element and text kept.
func TestFormatDOM(t *testing.T) {
	dom := `<html><head><title>Plans</title></head><body><div class="a"><button id="sub">Subscribe</button><p>Hello <b>there</b></p></div></body></html>`

	got := formatDOM(dom)

	lines := strings.Split(strings.TrimSpace(got), "\n")
	if len(lines) < 8 {
		t.Fatalf("formatDOM gave %d lines, want one per element:\n%s", len(lines), got)
	}
	for _, line := range lines {
		if len(line) > 80 {
			t.Errorf("line too long (%d chars): %q", len(line), line)
		}
	}
	for _, want := range []string{`<title>`, `Plans`, `<div class="a">`, `<button id="sub">`, `Subscribe`, `Hello`, `<b>`, `there`, `</html>`} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted DOM lost %q:\n%s", want, got)
		}
	}
}
