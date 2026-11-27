package session

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// browseStep performs as and returns what NVDA and VoiceOver said and the
// results the actions reported.
func browseStep(t *testing.T, b *Browser, as ...action.Action) (nvda, vo, results []string) {
	t.Helper()
	for _, d := range Perform(b, as) {
		if !d.Succeeded {
			t.Fatalf("%v failed: %s", d.Action.Type, *d.Reason)
		}
		if d.Result != nil {
			results = append(results, *d.Result)
		}
	}
	time.Sleep(200 * time.Millisecond)
	said := TakeAnnouncements(b)
	return preset.NVDASaid(said), preset.VoiceOverSaid(said), results
}

func nvdaMode(m action.NVDAMode) action.Action {
	return action.Action{Type: action.TypeNVDAChangeMode, Data: action.NVDAChangeMode{Mode: m}}
}

// In browse mode NVDA reads the page with its reading cursor, line by line,
// announcing the containers it enters and leaves; the quick navigation keys
// jump, content first; other character keys are kept from the page.
func TestE2EBrowseModeReading(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/doc")
	if nvda, _, _ := browseStep(t, b, nvdaMode(action.NVDAModeBrowse)); !reflect.DeepEqual(nvda, []string{"(sound: browse mode)"}) {
		t.Errorf("entering browse mode: NVDA %q", nvda)
	}
	for _, c := range []struct {
		key  string
		want []string
	}{
		{"Control+Home", []string{"main landmark", "heading", "level 1", "Welcome"}},
		{"ArrowDown", []string{"Read the", "link", "terms", "first."}},
		{"ArrowDown", []string{"list", "with 2 items", "bullet", "Apples"}},
		{"ArrowDown", []string{"bullet", "Pears"}},
		{"ArrowDown", []string{"out of list", "heading", "level 2", "Search"}},
		{"ArrowDown", []string{"Query", "edit", "button", "Go"}},
		{"ArrowDown", []string{"Query", "edit", "button", "Go"}}, // the last line again
		{"Control+Home", []string{"heading", "level 1", "Welcome"}},
		{"h", []string{"Search", "heading", "level 2"}},
		{"h", []string{"no next heading"}},
		{"Shift+H", []string{"Welcome", "heading", "level 1"}},
		{"2", []string{"Search", "heading", "level 2"}},
		{"Shift+K", []string{"terms", "link"}},
		{"i", []string{"list", "with 2 items", "bullet", "Apples"}},
		{"ArrowRight", []string{"space"}},
		{"Control+ArrowRight", []string{"Apples"}},
		{"x", []string{"no next check box"}},
	} {
		nvda, vo, results := browseStep(t, b, press(c.key))
		if !reflect.DeepEqual(nvda, c.want) || len(vo) != 0 {
			t.Errorf("%s: NVDA %q (VoiceOver %q), want NVDA %q", c.key, nvda, vo, c.want)
		}
		if len(results) != 1 || !strings.Contains(results[0], "didn't reach the page") {
			t.Errorf("%s: results %q", c.key, results)
		}
	}
	// A key with no command is kept from the page too.
	if _, _, results := browseStep(t, b, press("z")); len(results) != 1 || !strings.Contains(results[0], "no command is bound") {
		t.Errorf("z: results %q", results)
	}
	if got := NVDAMode(b); got != action.NVDAModeBrowse {
		t.Errorf("mode = %q", got)
	}
}

// NVDA switches to focus mode by itself in a field (activating it, or focus
// landing on it), with VoiceOver back on, and back to browse mode when focus
// moves on to a button; typing in browse mode is quick navigation, not text.
func TestE2EBrowseModeSwitching(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/doc")
	browseStep(t, b, nvdaMode(action.NVDAModeBrowse))

	// Typing without focusing the field: letters are NVDA's keys.
	nvda, _, results := browseStep(t, b, action.Action{Type: action.TypeType, Data: action.Typing{Text: "e"}})
	if !reflect.DeepEqual(nvda, []string{"main landmark", "Query", "edit"}) || len(results) != 1 {
		t.Errorf("typing e in browse mode: NVDA %q, results %q", nvda, results)
	}
	if v, _ := b.active.Evaluate(`() => document.getElementById('q').value`); v != "" {
		t.Errorf("the field got %q", v)
	}

	nvda, vo, _ := browseStep(t, b, press("Enter"))
	if NVDAMode(b) != action.NVDAModeFocus || len(nvda) == 0 || nvda[0] != "(sound: focus mode)" || !reflect.DeepEqual(vo[len(vo)-2:], []string{"Query", "edit text"}) {
		t.Errorf("Enter on the field: mode %q, NVDA %q, VoiceOver %q", NVDAMode(b), nvda, vo)
	}
	browseStep(t, b, typeText("#q", "cats"))
	if v, _ := b.active.Evaluate(`() => document.getElementById('q').value`); v != "cats" {
		t.Errorf("in focus mode the field got %q, want cats", v)
	}
	nvda, vo, _ = browseStep(t, b, press("Tab"))
	if NVDAMode(b) != action.NVDAModeBrowse || !reflect.DeepEqual(nvda, []string{"(sound: browse mode)", "Go", "button"}) || len(vo) != 0 {
		t.Errorf("Tab to the button: mode %q, NVDA %q, VoiceOver %q", NVDAMode(b), nvda, vo)
	}
	browseStep(t, b, press("Enter"))
	if v, _ := b.active.Evaluate(`() => document.getElementById('out').textContent`); v != "Searched cats" {
		t.Errorf("Enter on the button: out = %q", v)
	}

	// Escape leaves focus mode NVDA entered by itself; NVDA+Space toggles.
	browseStep(t, b, press("Shift+Tab"))
	if NVDAMode(b) != action.NVDAModeFocus {
		t.Fatalf("Shift+Tab to the field: mode %q", NVDAMode(b))
	}
	if nvda, _, _ := browseStep(t, b, press("Escape")); NVDAMode(b) != action.NVDAModeBrowse || !reflect.DeepEqual(nvda, []string{"(sound: browse mode)"}) {
		t.Errorf("Escape: mode %q, NVDA %q", NVDAMode(b), nvda)
	}
	browseStep(t, b, press("NVDA+Space"))
	if NVDAMode(b) != action.NVDAModeFocus {
		t.Errorf("NVDA+Space: mode %q", NVDAMode(b))
	}
	// Focus mode set by hand stays: Escape goes to the page.
	if _, _, results := browseStep(t, b, press("Escape")); NVDAMode(b) != action.NVDAModeFocus || len(results) != 0 {
		t.Errorf("Escape in manual focus mode: mode %q, results %q", NVDAMode(b), results)
	}
	// Back in browse mode, say all reads to the end.
	nvda, _, results = browseStep(t, b, press("NVDA+Space"), press("Control+Home"), press("NVDA+ArrowDown"))
	if len(nvda) == 0 || !strings.Contains(strings.Join(nvda, "|"), "Read the|link|terms|first.") || !strings.Contains(results[len(results)-1], "say all") {
		t.Errorf("say all: NVDA %q, results %q", nvda, results)
	}
}
