package preset

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

func ptr[T any](v T) *T { return &v }

func TestBuiltinsValidate(t *testing.T) {
	for _, p := range Builtins() {
		if err := Validate(p); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
	}
}

// Modifying one returned preset must not leak into the next call.
func TestBuiltinsAreFreshCopies(t *testing.T) {
	for _, mk := range []func() Preset{Default, Full, Blind, Motor} {
		p := mk()
		p.Actions[0] = "mutated"
		if mk().Actions[0] == "mutated" {
			t.Errorf("%s shares its Actions between calls", p.Name)
		}
	}
}

func TestDefaultVersusFull(t *testing.T) {
	def, full := Default(), Full()
	if !Allows(def, action.TypeJavascript) {
		t.Error("default should allow javascript")
	}
	if UsesArtifacts(def) {
		t.Error("default should not use artifacts")
	}
	if want := slices.DeleteFunc(action.All(), action.IsScreenReader); !UsesArtifacts(full) || !slices.Equal(full.Actions, want) {
		t.Errorf("full should allow every action but the screen-reader one, got %v", full.Actions)
	}
	for _, p := range []Preset{def, full, Motor()} {
		if Allows(p, action.TypeNVDAChangeMode) {
			t.Errorf("%s hears no screen reader, but allows %s", p.Name, action.TypeNVDAChangeMode)
		}
	}
	if !Allows(Blind(), action.TypeNVDAChangeMode) {
		t.Errorf("blind should allow %s", action.TypeNVDAChangeMode)
	}
	bad := Default()
	bad.Actions = append(bad.Actions, action.TypeNVDAChangeMode)
	if err := Validate(bad); err == nil {
		t.Errorf("Validate should refuse %s without Perceive.Focused", action.TypeNVDAChangeMode)
	}
}

func TestKeyboardOnlyPresets(t *testing.T) {
	for _, p := range []Preset{Blind(), Motor()} {
		for _, t2 := range []action.Type{action.TypeClick, action.TypeHover, action.TypeScroll} {
			if Allows(p, t2) {
				t.Errorf("%s allows %s", p.Name, t2)
			}
		}
		if p.Targeting != (action.Targeting{}) {
			t.Errorf("%s allows targeting", p.Name)
		}
		if UsesArtifacts(p) {
			t.Errorf("%s uses artifacts", p.Name)
		}
	}
}

func TestWithArtifacts(t *testing.T) {
	p := Blind()
	q := WithArtifacts(p)
	if UsesArtifacts(p) {
		t.Error("WithArtifacts modified its input")
	}
	for _, at := range action.ArtifactTypes() {
		if !Allows(q, at) {
			t.Errorf("missing %s", at)
		}
	}
	if err := Validate(q); err != nil {
		t.Error(err)
	}
	if got := WithArtifacts(q); len(got.Actions) != len(q.Actions) {
		t.Error("WithArtifacts added duplicates")
	}
}

func TestCheck(t *testing.T) {
	click := action.Action{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("a")}}}
	tab := action.Action{Type: action.TypePress, Data: action.Press{Key: "Tab"}}
	typeInto := action.Action{Type: action.TypeType, Data: action.Typing{Text: "x", Target: &action.Target{Selector: ptr("input")}}}
	badWait := action.Action{Type: action.TypeWait, Data: action.Wait{}}

	if err := Check(Default(), click); err != nil {
		t.Errorf("default, selector click: %v", err)
	}
	if err := Check(Blind(), click); err == nil {
		t.Error("blind should refuse a click")
	}
	if err := Check(Blind(), tab); err != nil {
		t.Errorf("blind, Tab: %v", err)
	}
	if err := Check(Motor(), typeInto); err == nil {
		t.Error("motor should refuse a targeted type")
	}
	if err := Check(Default(), badWait); err == nil {
		t.Error("an invalid payload should be refused")
	}
}

func TestValidateCustom(t *testing.T) {
	ok := Preset{
		Name:     "custom",
		Actions:  []action.Type{action.TypeGoTo, action.TypeWait},
		Describe: DescribeTabs,
	}
	if err := Validate(ok); err != nil {
		t.Fatalf("valid custom preset: %v", err)
	}

	bad := Preset{Actions: []action.Type{action.TypeClick}}
	err := Validate(bad)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"name is empty", "describe is nil", "needs a target"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

func TestDescribe(t *testing.T) {
	s := State{
		ScreenshotPath: "shot.png",
		DOMPath:        "dom.html",
		Tabs:           []Tab{{Title: "Home", URL: "https://x", Active: true}},
	}

	def := Default().Describe(s)
	if !strings.Contains(def, "shot.png") || !strings.Contains(def, `"active":true`) {
		t.Errorf("default describe = %q", def)
	}

	blind := Blind().Describe(s)
	if strings.Contains(blind, "shot.png") {
		t.Errorf("blind describe leaks screenshot: %q", blind)
	}
	if !strings.Contains(blind, "VoiceOver and NVDA said nothing this turn.") || !strings.Contains(blind, `"active":true`) {
		t.Errorf("blind describe = %q", blind)
	}
	s.Announcement = []Announcement{{Text: "Save", VoiceOver: true, NVDA: true}, {Text: "button", VoiceOver: true, NVDA: true}}
	if got := Blind().Describe(s); !strings.Contains(got, "VoiceOver said, in order:\n- Save\n- button\nNVDA said the same.") {
		t.Errorf("blind describe with an announcement = %q", got)
	}

	s.Focused = &FocusedElement{Role: ptr("button"), Text: ptr("Save")}
	if got := DescribeFocused(s); !strings.Contains(got, `"text":"Save"`) {
		t.Errorf("DescribeFocused = %q", got)
	}

	if got := DescribeTabs(State{}); !strings.HasSuffix(got, "[]") {
		t.Errorf("DescribeTabs with no tabs = %q", got)
	}
}

// DescribeAnnouncement shows VoiceOver's feed, then NVDA's: in one line when
// NVDA said the same, in full when it heard more.
func TestDescribeAnnouncement(t *testing.T) {
	both := func(text string) Announcement { return Announcement{Text: text, VoiceOver: true, NVDA: true} }
	nvda := func(text string) Announcement { return Announcement{Text: text, NVDA: true} }
	cases := []struct {
		name string
		said []Announcement
		want string
	}{
		{"nothing", []Announcement{}, "VoiceOver and NVDA said nothing this turn."},
		{"nil", nil, "VoiceOver and NVDA said nothing this turn."},
		{"same", []Announcement{both("Save"), both("button")},
			"VoiceOver said, in order:\n- Save\n- button\nNVDA said the same."},
		{"NVDA heard more", []Announcement{both("Chat"), nvda("Hello"), both("log")},
			"VoiceOver said, in order:\n- Chat\n- log\nNVDA said, in order:\n- Chat\n- Hello\n- log"},
		{"only NVDA heard", []Announcement{nvda("Hello")},
			"VoiceOver said nothing this turn.\nNVDA said, in order:\n- Hello"},
	}
	for _, c := range cases {
		if got := DescribeAnnouncement(State{Announcement: c.said}); got != c.want {
			t.Errorf("%s: got\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

// While NVDA's mode isn't Focus, VoiceOver is off: the agent is told so first
// every such turn, then what VoiceOver said before, then NVDA's feed in full.
func TestDescribeAnnouncementVoiceOverOff(t *testing.T) {
	both := func(text string) Announcement { return Announcement{Text: text, VoiceOver: true, NVDA: true} }
	nvda := func(text string) Announcement { return Announcement{Text: text, NVDA: true} }
	off := `VoiceOver is off because NVDA's mode is set to "Browse", not "Focus". Set it back to "Focus" with nvdaChangeMode to hear VoiceOver again.`
	for _, c := range []struct {
		name string
		said []Announcement
		want string
	}{
		{"silent", nil, off + "\nNVDA said nothing this turn."},
		{"NVDA spoke", []Announcement{nvda("Save"), nvda("button")}, off + "\nNVDA said, in order:\n- Save\n- button"},
		{"turned off mid-turn", []Announcement{both("Agree"), nvda("Save")},
			off + "\nBefore it was turned off, VoiceOver said, in order:\n- Agree\nNVDA said, in order:\n- Agree\n- Save"},
	} {
		if got := DescribeAnnouncement(State{Announcement: c.said, NVDAMode: action.NVDAModeBrowse}); got != c.want {
			t.Errorf("%s: got\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
	for _, m := range []action.NVDAMode{"", action.NVDAModeFocus} {
		if got := DescribeAnnouncement(State{NVDAMode: m}); got != "VoiceOver and NVDA said nothing this turn." {
			t.Errorf("mode %q: got %q", m, got)
		}
	}
}

func TestSaidFeeds(t *testing.T) {
	said := []Announcement{{Text: "a", VoiceOver: true, NVDA: true}, {Text: "b", NVDA: true}, {Text: "c", VoiceOver: true}}
	if got := VoiceOverSaid(said); !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Errorf("VoiceOverSaid = %q", got)
	}
	if got := NVDASaid(said); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("NVDASaid = %q", got)
	}
	if got := NVDASaid(nil); got == nil || len(got) != 0 {
		t.Errorf("NVDASaid(nil) = %#v, want empty", got)
	}
}

// Blind can't set the modes a blind user can't work in (Sleep, Speech): the
// schema doesn't offer them and Check refuses them. A preset without
// NVDAModes allows every mode; one that sets them must include Focus.
func TestNVDAModesRestriction(t *testing.T) {
	mode := func(m action.NVDAMode) action.Action {
		return action.Action{Type: action.TypeNVDAChangeMode, Data: action.NVDAChangeMode{Mode: m}}
	}
	blind := Blind()
	for _, m := range []action.NVDAMode{action.NVDAModeSleep, action.NVDAModeSpeech} {
		if err := Check(blind, mode(m)); err == nil {
			t.Errorf("blind allows %q", m)
		}
	}
	for _, m := range []action.NVDAMode{"", action.NVDAModeFocus, action.NVDAModeBrowse, action.NVDAModeOneKeyPass,
		action.NVDAModeInputHelp, action.NVDAModeReview, action.NVDAModeNativeSelection} {
		if err := Check(blind, mode(m)); err != nil {
			t.Errorf("blind refuses %q: %v", m, err)
		}
	}
	s, err := Schema(blind)
	if err != nil {
		t.Fatal(err)
	}
	enum := s["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)["mode"].(map[string]any)["enum"]
	if want := []any{"Focus", "Browse", "One-key pass-through", "Input help", "Review", "Native selection"}; !reflect.DeepEqual(enum, want) {
		t.Errorf("blind mode enum = %v, want %v", enum, want)
	}
	// Other presets aren't restricted: one without NVDAModes allows every mode.
	open := Blind()
	open.NVDAModes = nil
	if err := Check(open, mode(action.NVDAModeSleep)); err != nil {
		t.Errorf("unrestricted preset refuses Sleep: %v", err)
	}
	// Fresh copies: changing one Blind() doesn't change the next.
	blind.NVDAModes[0] = action.NVDAModeSleep
	if Blind().NVDAModes[0] != action.NVDAModeFocus {
		t.Error("Blind() shares its NVDAModes")
	}
	for _, modes := range [][]action.NVDAMode{{action.NVDAModeBrowse}, {action.NVDAModeFocus, "Hover"}} {
		bad := Blind()
		bad.NVDAModes = modes
		if err := Validate(bad); err == nil {
			t.Errorf("Validate accepts NVDAModes %q", modes)
		}
	}
}
