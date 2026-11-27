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

// One-key pass-through (NVDA+F2, or nvdaChangeMode) lets exactly one key
// reach the page, then NVDA is back in its mode: in browse mode, the next
// letter reaches the page instead of jumping, and the one after it jumps.
func TestE2EOneKeyPassThrough(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/doc")
	keys := func() string {
		v, _ := b.active.Evaluate(`() => window.keys.join(",")`)
		s, _ := v.(string)
		return s
	}
	browseStep(t, b, nvdaMode(action.NVDAModeBrowse), press("Control+Home"))
	nvda, vo, _ := browseStep(t, b, press("NVDA+F2"))
	if !reflect.DeepEqual(nvda, []string{"Pass next key through"}) || len(vo) != 0 || NVDAMode(b) != action.NVDAModeOneKeyPass {
		t.Errorf("NVDA+F2: NVDA %q, VoiceOver %q, mode %q", nvda, vo, NVDAMode(b))
	}
	nvda, _, results := browseStep(t, b, press("h"))
	if keys() != "h" || len(nvda) != 0 || NVDAMode(b) != action.NVDAModeBrowse || len(results) != 1 || !strings.Contains(results[0], "passed this key straight through") {
		t.Errorf("the passed h: page keys %q, NVDA %q, mode %q, results %q", keys(), nvda, NVDAMode(b), results)
	}
	if nvda, _, _ := browseStep(t, b, press("h")); keys() != "h" || !reflect.DeepEqual(nvda, []string{"Search", "heading", "level 2"}) {
		t.Errorf("the next h: page keys %q, NVDA %q", keys(), nvda)
	}
	// Set with nvdaChangeMode, and through type: the first character
	// reaches the page, the rest is NVDA's again.
	if nvda, _, _ := browseStep(t, b, nvdaMode(action.NVDAModeOneKeyPass)); !reflect.DeepEqual(nvda, []string{"Pass next key through"}) {
		t.Errorf("nvdaChangeMode: NVDA %q", nvda)
	}
	nvda, _, _ = browseStep(t, b, action.Action{Type: action.TypeType, Data: action.Typing{Text: "zk"}})
	if keys() != "h,z" || !reflect.DeepEqual(nvda, []string{"no next link"}) || NVDAMode(b) != action.NVDAModeBrowse {
		t.Errorf("typing zk: page keys %q, NVDA %q, mode %q", keys(), nvda, NVDAMode(b))
	}
}

// Input help (NVDA+1) describes keys instead of performing them: nothing
// reaches the page and the reading cursor doesn't move, until NVDA+1 turns it
// off, back in browse mode.
func TestE2EInputHelp(t *testing.T) {
	e2e(t)
	b := openAnnouncing(t, liveServer(t), "/doc")
	keys := func() string {
		v, _ := b.active.Evaluate(`() => window.keys.join(",")`)
		s, _ := v.(string)
		return s
	}
	browseStep(t, b, nvdaMode(action.NVDAModeBrowse), press("Control+Home"))
	nvda, vo, _ := browseStep(t, b, press("NVDA+1"))
	if !reflect.DeepEqual(nvda, []string{"input help on. Press insert+1 again to turn it off."}) || len(vo) != 0 {
		t.Errorf("NVDA+1: NVDA %q, VoiceOver %q", nvda, vo)
	}
	for _, c := range []struct {
		key  string
		want []string
	}{
		{"h", []string{"h", "moves to the next heading"}},
		{"Tab", []string{"tab"}},
		{"ArrowDown", []string{"down arrow"}},
	} {
		if nvda, _, results := browseStep(t, b, press(c.key)); !reflect.DeepEqual(nvda, c.want) || len(results) != 1 || !strings.Contains(results[0], "described this key") {
			t.Errorf("%s: NVDA %q, results %q, want NVDA %q", c.key, nvda, results, c.want)
		}
	}
	nvda, _, _ = browseStep(t, b, action.Action{Type: action.TypeType, Data: action.Typing{Text: "k"}})
	if !reflect.DeepEqual(nvda, []string{"k", "moves to the next link"}) {
		t.Errorf("typing k: NVDA %q", nvda)
	}
	if keys() != "" {
		t.Errorf("keys reached the page: %q", keys())
	}
	if nvda, _, _ := browseStep(t, b, press("NVDA+1")); !reflect.DeepEqual(nvda, []string{"insert+1", "input help off"}) || NVDAMode(b) != action.NVDAModeBrowse {
		t.Errorf("NVDA+1 again: NVDA %q, mode %q", nvda, NVDAMode(b))
	}
	// The cursor didn't move while input help was on: H still finds the
	// second heading next.
	if nvda, _, _ := browseStep(t, b, press("h")); !reflect.DeepEqual(nvda, []string{"Search", "heading", "level 2"}) {
		t.Errorf("h after input help: NVDA %q", nvda)
	}
}
