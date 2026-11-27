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
