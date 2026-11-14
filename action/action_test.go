package action

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

var (
	sel = Target{Selector: ptr("button.save")}
	pt  = Target{Point: &Point{X: 10, Y: 20}}
)

// valid has one valid sample per built-in type.
var valid = map[Type]Data{
	TypeClick:          Click{Target: sel},
	TypeDoubleClick:    DoubleClick{Target: pt},
	TypeHover:          Hover{Target: sel},
	TypeMouseDown:      MouseDown{},
	TypeMouseUp:        MouseUp{Target: &pt},
	TypeScroll:         Scroll{DeltaY: 300},
	TypeType:           Typing{Text: "hello"},
	TypeFill:           Fill{Target: sel, Value: ""},
	TypePress:          Press{Key: "Enter"},
	TypeWait:           Wait{Milliseconds: 500},
	TypeGoTo:           GoTo{URL: "/home"},
	TypeTabOpen:        TabOpen{},
	TypeTabSwitch:      TabSwitch{Index: 2},
	TypeTabClose:       TabClose{Index: 1},
	TypeJavascript:     Javascript{Code: "1+1"},
	TypeScreenshot:     Screenshot{Name: "pricing", FullPage: true},
	TypeCreateArtifact: CreateArtifact{Name: "notes.md", Content: "hi"},
	TypeReadArtifact:   ReadArtifact{Name: "notes.md"},
	TypeListArtifacts:  ListArtifacts{},
}

func TestSamplesCoverRegistry(t *testing.T) {
	if len(valid) != len(All()) {
		t.Fatalf("samples cover %d types, registry has %d", len(valid), len(All()))
	}
}

// Every built-in survives a marshal → unmarshal round trip as its own payload
// type, so registry entries can't be wired to the wrong struct.
func TestRoundTrip(t *testing.T) {
	for typ, data := range valid {
		b, err := json.Marshal(Action{Type: typ, Data: data})
		if err != nil {
			t.Fatalf("%s: marshal: %v", typ, err)
		}
		var got Action
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("%s: unmarshal: %v", typ, err)
		}
		if got.Type != typ || !reflect.DeepEqual(got.Data, data) {
			t.Errorf("%s: round trip = %#v, want %#v", typ, got.Data, data)
		}
	}
}

func TestUnmarshalUnknownType(t *testing.T) {
	var a Action
	err := json.Unmarshal([]byte(`{"type":"teleport","data":{}}`), &a)
	if err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Fatalf("err = %v, want unknown type error", err)
	}
}

// Every JSON name the agent sees is camelCase.
func TestCamelCase(t *testing.T) {
	camel := regexp.MustCompile(`^[a-z][a-zA-Z0-9]*$`)
	for _, typ := range All() {
		if !camel.MatchString(string(typ)) {
			t.Errorf("type %q is not camelCase", typ)
		}
	}
	s, err := Schema(All(), Targeting{Selector: true, Point: true})
	if err != nil {
		t.Fatal(err)
	}
	var walk func(map[string]any)
	walk = func(node map[string]any) {
		props, _ := node["properties"].(map[string]any)
		for name, child := range props {
			if !camel.MatchString(name) {
				t.Errorf("field %q is not camelCase", name)
			}
			if m, ok := child.(map[string]any); ok {
				walk(m)
			}
		}
	}
	walk(s)
}

func TestAllReturnsCopy(t *testing.T) {
	a := All()
	a[0] = "mutated"
	if All()[0] == "mutated" {
		t.Fatal("All shares its slice between calls")
	}
}

// Artifacts are create-only: no payload carries a way to overwrite one.
func TestArtifactsAreCreateOnly(t *testing.T) {
	s, err := Schema(ArtifactTypes(), Targeting{Selector: true, Point: true})
	if err != nil {
		t.Fatal(err)
	}
	data := s["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	for _, f := range []string{"overwrite", "override", "append", "replace"} {
		if _, ok := data[f]; ok {
			t.Errorf("artifact schema offers %q", f)
		}
	}
}

func TestScreenshotFileName(t *testing.T) {
	if got := ScreenshotFileName(Screenshot{Name: "pricing"}); got != "pricing.png" {
		t.Errorf("ScreenshotFileName = %q", got)
	}
}

func TestArtifactTypes(t *testing.T) {
	want := []Type{TypeScreenshot, TypeCreateArtifact, TypeReadArtifact, TypeListArtifacts}
	if got := ArtifactTypes(); !slices.Equal(got, want) {
		t.Errorf("ArtifactTypes = %v, want %v", got, want)
	}
}

func targetProps(t *testing.T, s map[string]any) (map[string]any, bool) {
	t.Helper()
	data := s["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
	target, ok := data["target"].(map[string]any)
	if !ok {
		return nil, false
	}
	return target["properties"].(map[string]any), true
}

func TestSchemaTargeting(t *testing.T) {
	types := []Type{TypeType, TypeScreenshot}

	s, err := Schema(types, Targeting{Selector: true})
	if err != nil {
		t.Fatal(err)
	}
	tp, ok := targetProps(t, s)
	if !ok {
		t.Fatal("target missing")
	}
	if _, ok := tp["point"]; ok {
		t.Error("point offered without point targeting")
	}
	if _, ok := tp["selector"]; !ok {
		t.Error("selector missing")
	}

	s, err = Schema(types, Targeting{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := targetProps(t, s); ok {
		t.Error("target offered with no targeting")
	}

	enum := s["properties"].(map[string]any)["type"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(enum, []any{"type", "screenshot"}) {
		t.Errorf("enum = %v", enum)
	}
}

func TestSchemaErrors(t *testing.T) {
	cases := map[string][]Type{
		"empty":                   nil,
		"unknown type":            {"teleport"},
		"duplicate":               {TypeWait, TypeWait},
		"needs target, gets none": {TypeClick},
	}
	for name, types := range cases {
		if _, err := Schema(types, Targeting{}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestValidateAcceptsSamples(t *testing.T) {
	for typ, data := range valid {
		if err := Validate(Action{Type: typ, Data: data}); err != nil {
			t.Errorf("%s: %v", typ, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]Action{
		"unknown type":                       {Type: "teleport", Data: Wait{Milliseconds: 1}},
		"mismatched data":                    {Type: TypeClick, Data: Wait{Milliseconds: 1}},
		"nil data":                           {Type: TypeWait},
		"click without target":               {Type: TypeClick, Data: Click{}},
		"click with both":                    {Type: TypeClick, Data: Click{Target: Target{Selector: ptr("a"), Point: &Point{}}}},
		"empty selector":                     {Type: TypeHover, Data: Hover{Target: Target{Selector: ptr(" ")}}},
		"zero scroll":                        {Type: TypeScroll, Data: Scroll{}},
		"empty text":                         {Type: TypeType, Data: Typing{}},
		"empty key":                          {Type: TypePress, Data: Press{}},
		"zero wait":                          {Type: TypeWait, Data: Wait{}},
		"long wait":                          {Type: TypeWait, Data: Wait{Milliseconds: 60_001}},
		"tab index 0":                        {Type: TypeTabSwitch, Data: TabSwitch{}},
		"empty code":                         {Type: TypeJavascript, Data: Javascript{}},
		"fullPage and target":                {Type: TypeScreenshot, Data: Screenshot{Name: "x", FullPage: true, Target: &sel}},
		"screenshot no name":                 {Type: TypeScreenshot, Data: Screenshot{}},
		"screenshot name too long with .png": {Type: TypeScreenshot, Data: Screenshot{Name: strings.Repeat("a", 252)}},
		"artifact escapes dir":               {Type: TypeCreateArtifact, Data: CreateArtifact{Name: "../../some-shit.md"}},
		"read escapes dir":                   {Type: TypeReadArtifact, Data: ReadArtifact{Name: "../secret"}},
	}
	for name, a := range cases {
		if err := Validate(a); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestValidateURL(t *testing.T) {
	for _, ok := range []string{"https://example.com/a?b=c", "http://x", "/root", "../up", "page.html", "?q=1", "#frag", "//cdn.example.com/x", "about:blank", "ABOUT:blank"} {
		if err := ValidateURL(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "javascript:alert(1)", "JavaScript:alert(1)", " javascript:alert(1)", "java\tscript:alert(1)", "file:///etc/passwd", "data:text/html,hi", "chrome://settings", "about:config", "about:srcdoc", "about:Blank", "about:blank?x", "about:blank#x", "about://blank"} {
		if err := ValidateURL(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestValidateFileName(t *testing.T) {
	for _, ok := range []string{"notes.md", "report 2026.csv", ".hidden", "a..b"} {
		if err := ValidateFileName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", ".", "..", "../../some-shit.md", "a/b", `a\b`, "/etc/passwd", "C:x", "a\x00b", strings.Repeat("a", 256)} {
		if err := ValidateFileName(bad); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestCheckTargeting(t *testing.T) {
	bySel := Action{Type: TypeClick, Data: Click{Target: sel}}
	byPoint := Action{Type: TypeClick, Data: Click{Target: pt}}
	untargeted := Action{Type: TypeType, Data: Typing{Text: "x"}}

	if err := CheckTargeting(bySel, Targeting{Point: true}); err == nil {
		t.Error("selector allowed without selector targeting")
	}
	if err := CheckTargeting(byPoint, Targeting{Selector: true}); err == nil {
		t.Error("point allowed without point targeting")
	}
	if err := CheckTargeting(byPoint, Targeting{Point: true}); err != nil {
		t.Error(err)
	}
	if err := CheckTargeting(untargeted, Targeting{}); err != nil {
		t.Error(err)
	}
}

func TestCompact(t *testing.T) {
	got := Compact(Screenshot{Name: "x"})
	if !reflect.DeepEqual(got, map[string]any{"name": "x"}) {
		t.Errorf("Compact = %v", got)
	}
}
