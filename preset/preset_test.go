package preset

import (
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
	if !UsesArtifacts(full) || !slices.Equal(full.Actions, action.All()) {
		t.Errorf("full should allow every action, got %v", full.Actions)
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
	if !strings.Contains(blind, "No element is currently focused.") {
		t.Errorf("blind describe = %q", blind)
	}

	s.Focused = &FocusedElement{Tag: "button", Text: ptr("Save")}
	if got := DescribeFocused(s); !strings.Contains(got, `"text":"Save"`) {
		t.Errorf("DescribeFocused = %q", got)
	}

	if got := DescribeTabs(State{}); !strings.HasSuffix(got, "[]") {
		t.Errorf("DescribeTabs with no tabs = %q", got)
	}
}
