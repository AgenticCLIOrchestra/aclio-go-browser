package session

import (
	"reflect"
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
