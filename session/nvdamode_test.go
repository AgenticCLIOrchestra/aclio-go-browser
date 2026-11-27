package session

import (
	"reflect"
	"strings"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// Any NVDA mode but Focus turns VoiceOver off: from then on only what NVDA
// says is kept, as NVDA's alone, until the mode is Focus again. Switching
// between browse and focus mode plays NVDA's mode sound.
func TestNVDAModeTurnsVoiceOverOff(t *testing.T) {
	b := &Browser{}
	b.announcer = &announcer{nvda: nvdaOf(b)}
	if got := NVDAMode(b); got != action.NVDAModeFocus {
		t.Errorf("initial mode = %q", got)
	}
	both := preset.Announcement{Text: "Save", VoiceOver: true, NVDA: true}
	voOnly := preset.Announcement{Text: "unchecked", VoiceOver: true}
	nvdaOnly := preset.Announcement{Text: "not checked", NVDA: true}

	appendSaid(b.announcer, both)
	if err := setNVDAMode(b, action.NVDAModeBrowse); err != nil {
		t.Fatal(err)
	}
	appendSaid(b.announcer, both, voOnly, nvdaOnly)
	if err := setNVDAMode(b, ""); err != nil { // the zero value is Focus
		t.Fatal(err)
	}
	appendSaid(b.announcer, voOnly)
	want := []preset.Announcement{
		both, {Text: "(sound: browse mode)", NVDA: true}, {Text: "Save", NVDA: true}, nvdaOnly,
		{Text: "(sound: focus mode)", NVDA: true}, voOnly,
	}
	if got := b.announcer.said; !reflect.DeepEqual(got, want) {
		t.Errorf("said = %+v\nwant %+v", got, want)
	}
	if got := NVDAMode(b); got != action.NVDAModeFocus {
		t.Errorf("mode = %q, want Focus", got)
	}
	if err := setNVDAMode(b, "browse"); err == nil {
		t.Error("an unknown mode should be refused")
	}
	if got := NVDAMode(nil); got != action.NVDAModeFocus {
		t.Errorf("NVDAMode(nil) = %q", got)
	}
}

// One-key pass-through lasts one key: NVDA says "Pass next key through",
// the key isn't handled by NVDA (it reaches the page), and NVDA is back in
// the mode it was in. NVDA+F2 starts it in any mode.
func TestOneKeyPassThrough(t *testing.T) {
	b := &Browser{}
	b.announcer = &announcer{nvda: nvdaOf(b)}
	if err := setNVDAMode(b, action.NVDAModeBrowse); err != nil {
		t.Fatal(err)
	}
	b.announcer.said = nil
	if err := setNVDAMode(b, action.NVDAModeOneKeyPass); err != nil {
		t.Fatal(err)
	}
	if got := NVDAMode(b); got != action.NVDAModeOneKeyPass {
		t.Errorf("mode = %q", got)
	}
	handled, result, err := nvdaKey(b, "h")
	if err != nil || handled || result == nil || !strings.Contains(*result, `back in "Browse" mode`) {
		t.Errorf("the passed key: handled %v, result %v, err %v", handled, result, err)
	}
	if got := NVDAMode(b); got != action.NVDAModeBrowse {
		t.Errorf("after the key, mode = %q, want Browse", got)
	}
	// NVDA+F2 from focus mode, back to focus mode after one key.
	if err := setNVDAMode(b, action.NVDAModeFocus); err != nil {
		t.Fatal(err)
	}
	b.announcer.said = nil
	if handled, _, _ := nvdaKey(b, "NVDA+F2"); !handled || NVDAMode(b) != action.NVDAModeOneKeyPass {
		t.Errorf("NVDA+F2: handled %v, mode %q", handled, NVDAMode(b))
	}
	if want := []preset.Announcement{{Text: "Pass next key through", NVDA: true}}; !reflect.DeepEqual(b.announcer.said, want) {
		t.Errorf("said = %+v, want %+v", b.announcer.said, want)
	}
	if handled, _, _ := nvdaKey(b, "Tab"); handled || NVDAMode(b) != action.NVDAModeFocus {
		t.Errorf("after the key: handled %v, mode %q, want Focus", handled, NVDAMode(b))
	}
	// Without the screen readers, it still lasts one key.
	quiet := &Browser{}
	if err := setNVDAMode(quiet, action.NVDAModeOneKeyPass); err != nil {
		t.Fatal(err)
	}
	if handled, _, _ := nvdaKey(quiet, "h"); handled || NVDAMode(quiet) != action.NVDAModeFocus {
		t.Errorf("not announcing: handled %v, mode %q", handled, NVDAMode(quiet))
	}
}

// Input help says a key's name, the character it would type when that's
// different, and its command's description in the mode NVDA was in, instead
// of doing anything; NVDA+1 turns it on and off, back in the mode NVDA was in.
func TestInputHelp(t *testing.T) {
	b := &Browser{}
	b.announcer = &announcer{nvda: nvdaOf(b)}
	if err := setNVDAMode(b, action.NVDAModeBrowse); err != nil {
		t.Fatal(err)
	}
	b.announcer.said = nil
	if handled, _, _ := nvdaKey(b, "NVDA+1"); !handled || NVDAMode(b) != action.NVDAModeInputHelp {
		t.Fatalf("NVDA+1: handled %v, mode %q", handled, NVDAMode(b))
	}
	for _, c := range []struct {
		key  string
		want []string
	}{
		{"h", []string{"h", "moves to the next heading"}},
		{"Shift+H", []string{"shift+h", "H", "moves to the previous heading"}},
		{"2", []string{"2", "Moves to the next heading at level 2"}},
		{"Enter", []string{"enter", "Activates the current object in the document"}},
		{"ArrowDown", []string{"down arrow"}}, // no description in NVDA's source
		{"Control+Home", []string{"ctrl+home"}},
		{"Tab", []string{"tab"}},
		{"z", []string{"z"}},
		{"NVDA+Space", []string{"insert+space", inputHelpPassThroughDoc}},
		{"NVDA+F2", []string{"insert+f2", inputHelpPassNextKeyDoc}},
		{"NVDA+ArrowDown", []string{"insert+down arrow", inputHelpSayAllDoc}},
	} {
		b.announcer.said = nil
		handled, result, err := nvdaKey(b, c.key)
		if err != nil || !handled || result == nil || !strings.Contains(*result, "described this key") {
			t.Errorf("%s: handled %v, result %v, err %v", c.key, handled, result, err)
		}
		if got := preset.NVDASaid(b.announcer.said); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: NVDA said %q, want %q", c.key, got, c.want)
		}
		if NVDAMode(b) != action.NVDAModeInputHelp {
			t.Errorf("%s: mode %q, want input help still", c.key, NVDAMode(b))
		}
	}
	b.announcer.said = nil
	if handled, _, _ := nvdaKey(b, "NVDA+1"); !handled || NVDAMode(b) != action.NVDAModeBrowse {
		t.Errorf("NVDA+1 again: handled %v, mode %q, want Browse", handled, NVDAMode(b))
	}
	if got, want := preset.NVDASaid(b.announcer.said), []string{"insert+1", "input help off"}; !reflect.DeepEqual(got, want) {
		t.Errorf("turning it off: NVDA said %q, want %q", got, want)
	}
	// From focus mode, a browse mode key has no command to describe.
	if err := setNVDAMode(b, action.NVDAModeFocus); err != nil {
		t.Fatal(err)
	}
	if err := setNVDAMode(b, action.NVDAModeInputHelp); err != nil {
		t.Fatal(err)
	}
	b.announcer.said = nil
	nvdaKey(b, "h")
	if got := preset.NVDASaid(b.announcer.said); !reflect.DeepEqual(got, []string{"h"}) {
		t.Errorf("h from focus mode: NVDA said %q", got)
	}
	// Setting another mode turns it off.
	b.announcer.said = nil
	if err := setNVDAMode(b, action.NVDAModeBrowse); err != nil || NVDAMode(b) != action.NVDAModeBrowse {
		t.Errorf("nvdaChangeMode Browse: mode %q, err %v", NVDAMode(b), err)
	}
	if got := preset.NVDASaid(b.announcer.said); len(got) == 0 || got[0] != "input help off" {
		t.Errorf("leaving by nvdaChangeMode: NVDA said %q", got)
	}
}

func TestQuickNavWording(t *testing.T) {
	for _, c := range []struct {
		key     string
		forward bool
		err     string
		doc     string
	}{
		{"h", true, "no next heading", "moves to the next heading"},
		{"k", false, "no previous link", "moves to the previous link"},
		{"3", true, "No next heading at level 3", "Moves to the next heading at level 3"},
		{"3", false, "No previous heading at level 3", "Moves to the previous heading at level 3"},
		{"e", true, "no next edit field", "moves to the next edit field"},
	} {
		if got := quickNavError(c.key, c.forward); got != c.err {
			t.Errorf("error %s %v = %q, want %q", c.key, c.forward, got, c.err)
		}
		if got := quickNavDoc(c.key, c.forward); got != c.doc {
			t.Errorf("doc %s %v = %q, want %q", c.key, c.forward, got, c.doc)
		}
	}
}
