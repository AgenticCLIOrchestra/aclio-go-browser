package preset

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The Describe* functions are the building blocks the built-in presets use to
// render State for the agent. They are exported so custom presets can reuse
// them, alone or via Combine.

// DescribeFiles points the agent at the captured screenshot and DOM files.
func DescribeFiles(s State) string {
	return fmt.Sprintf("- Screenshot (PNG): %s\n- Full DOM (HTML): %s", s.ScreenshotPath, s.DOMPath)
}

// DescribeFocused gives the screen-reader view of the focused element.
func DescribeFocused(s State) string {
	if s.Focused == nil {
		return "No element is currently focused."
	}
	b, _ := json.Marshal(s.Focused)
	return "Current focused element: " + string(b)
}

// DescribeAnnouncement gives what a screen reader said this turn
// (State.Announcement), one spoken item per line, in order; it says so when
// the screen reader said nothing.
func DescribeAnnouncement(s State) string {
	if len(s.Announcement) == 0 {
		return "The screen reader said nothing this turn."
	}
	return "The screen reader said, in order:\n- " + strings.Join(s.Announcement, "\n- ")
}

// DescribeTabs lists the open tabs as compact JSON, one object per tab.
func DescribeTabs(s State) string {
	tabs := s.Tabs
	if tabs == nil {
		tabs = []Tab{}
	}
	b, _ := json.Marshal(tabs)
	return "Open tabs (you act on the one with \"active\":true; tabSwitch/tabClose take a 1-based index):\n" + string(b)
}

// Combine returns a Describe that runs each of fns and joins their output with
// a blank line.
func Combine(fns ...func(State) string) func(State) string {
	return func(s State) string {
		parts := make([]string, len(fns))
		for i, f := range fns {
			parts[i] = f(s)
		}
		return strings.Join(parts, "\n\n")
	}
}
