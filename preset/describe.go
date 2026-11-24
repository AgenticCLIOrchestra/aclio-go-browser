package preset

import (
	"encoding/json"
	"fmt"
	"slices"
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

// DescribeAnnouncement gives what the two screen readers said this turn
// (State.Announcement), VoiceOver's feed then NVDA's, each one spoken item per
// line, in order. NVDA's part is one line when it said the same, and the
// whole is one line when neither said anything.
func DescribeAnnouncement(s State) string {
	vo, nvda := VoiceOverSaid(s.Announcement), NVDASaid(s.Announcement)
	if len(vo) == 0 && len(nvda) == 0 {
		return "VoiceOver and NVDA said nothing this turn."
	}
	return describeSaid("VoiceOver", vo) + "\n" + describeNVDA(vo, nvda)
}

func describeNVDA(vo, nvda []string) string {
	if slices.Equal(vo, nvda) {
		return "NVDA said the same."
	}
	return describeSaid("NVDA", nvda)
}

func describeSaid(reader string, said []string) string {
	if len(said) == 0 {
		return reader + " said nothing this turn."
	}
	return reader + " said, in order:\n- " + strings.Join(said, "\n- ")
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
