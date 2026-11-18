package session

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// Sources without a value are dropped, duplicates kept once, the rest kept in
// Chromium's order with superseded marked; From is the attribute, else the
// native source, else the type. (Shapes as Chromium returns them.)
func TestAXFromNode(t *testing.T) {
	const raw = `{"ignored":false,"role":{"type":"role","value":"textbox"},"name":{"type":"computedString","value":"From labelledby","sources":[
		{"type":"relatedElement","attribute":"aria-labelledby","value":{"type":"computedString","value":"From labelledby"}},
		{"type":"attribute","attribute":"aria-label","superseded":true,"value":{"type":"computedString","value":"ignored"}},
		{"type":"relatedElement","nativeSource":"labelfor","superseded":true,"value":{"type":"computedString","value":"Email"}},
		{"type":"contents","superseded":true,"value":{"type":"computedString","value":"x"}},
		{"type":"placeholder","attribute":"placeholder","superseded":true},
		{"type":"attribute","attribute":"title","superseded":true,"value":{"type":"computedString","value":"t"}},
		{"type":"attribute","attribute":"title","superseded":true,"value":{"type":"computedString","value":"t"}}]}}`
	var n cdpAXNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	ax := axFromNode(n)
	if ax.Role == nil || *ax.Role != "textbox" || ax.Name == nil || *ax.Name != "From labelledby" {
		t.Fatalf("role/name = %v/%v", ax.Role, ax.Name)
	}
	want := []preset.NameSource{
		{From: "aria-labelledby", Value: "From labelledby"},
		{From: "aria-label", Value: "ignored", Superseded: true},
		{From: "labelfor", Value: "Email", Superseded: true},
		{From: "contents", Value: "x", Superseded: true},
		{From: "title", Value: "t", Superseded: true},
	}
	if !reflect.DeepEqual(ax.NameSources, want) {
		t.Errorf("sources = %+v\nwant      %+v", ax.NameSources, want)
	}

	// No name: "" (not nil) and no sources.
	n = cdpAXNode{}
	if err := json.Unmarshal([]byte(`{"role":{"value":"textbox"},"name":{"value":"","sources":[{"type":"attribute","attribute":"aria-label"}]}}`), &n); err != nil {
		t.Fatal(err)
	}
	if ax := axFromNode(n); ax.Name == nil || *ax.Name != "" || ax.NameSources != nil {
		t.Errorf("unnamed = %+v", ax)
	}

	// Ignored: only that and its reasons, even with a role, name and value.
	n = cdpAXNode{}
	if err := json.Unmarshal([]byte(`{"ignored":true,"ignoredReasons":[{"name":"ariaHiddenElement","value":{"type":"boolean","value":true}}],
		"role":{"value":"none"},"name":{"value":"B"},"value":{"value":"x"},"properties":[{"name":"selected","value":{"value":true}}]}`), &n); err != nil {
		t.Fatal(err)
	}
	wantIgnored := axInfo{State: preset.AXState{Ignored: true, IgnoredReasons: []string{"ariaHiddenElement"}}}
	if ax := axFromNode(n); !reflect.DeepEqual(ax, wantIgnored) {
		t.Errorf("ignored = %+v, want %+v", ax, wantIgnored)
	}
}

// A string value is kept as is, a number (slider, spinbutton) written out
// plainly, and no value stays nil.
func TestAXFromNodeValue(t *testing.T) {
	for raw, want := range map[string]string{
		`{"value":{"type":"string","value":"typed"}}`: "typed",
		`{"value":{"type":"number","value":7}}`:       "7",
		`{"value":{"type":"number","value":0.25}}`:    "0.25",
		`{"value":{"type":"number","value":1500000}}`: "1500000",
		`{"value":{"type":"string","value":""}}`:      "",
	} {
		var n cdpAXNode
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatal(err)
		}
		if ax := axFromNode(n); ax.Value == nil || *ax.Value != want {
			t.Errorf("%s: value = %v, want %q", raw, ax.Value, want)
		}
	}
	var n cdpAXNode
	if err := json.Unmarshal([]byte(`{"role":{"value":"checkbox"}}`), &n); err != nil {
		t.Fatal(err)
	}
	if ax := axFromNode(n); ax.Value != nil {
		t.Errorf("no value = %q, want nil", *ax.Value)
	}
}

// Properties are read into AXState with their types; "false" invalid, empty
// valuetext, "none" autocomplete and unannounced properties stay unset.
func TestAXState(t *testing.T) {
	const raw = `{"properties":[
		{"name":"focusable","value":{"type":"booleanOrUndefined","value":true}},
		{"name":"checked","value":{"type":"tristate","value":"mixed"}},
		{"name":"pressed","value":{"type":"tristate","value":"false"}},
		{"name":"selected","value":{"type":"booleanOrUndefined","value":false}},
		{"name":"expanded","value":{"type":"booleanOrUndefined","value":false}},
		{"name":"disabled","value":{"type":"boolean","value":true}},
		{"name":"readonly","value":{"type":"boolean","value":false}},
		{"name":"required","value":{"type":"boolean","value":true}},
		{"name":"invalid","value":{"type":"token","value":"spelling"}},
		{"name":"level","value":{"type":"integer","value":3}},
		{"name":"hasPopup","value":{"type":"token","value":"menu"}},
		{"name":"multiselectable","value":{"type":"boolean","value":true}},
		{"name":"orientation","value":{"type":"token","value":"vertical"}},
		{"name":"valuemin","value":{"type":"number","value":0}},
		{"name":"valuemax","value":{"type":"number","value":100}},
		{"name":"valuetext","value":{"type":"string","value":"30 percent"}},
		{"name":"autocomplete","value":{"type":"token","value":"list"}},
		{"name":"keyshortcuts","value":{"type":"string","value":"Alt+S"}},
		{"name":"roledescription","value":{"type":"string","value":"slide"}},
		{"name":"modal","value":{"type":"boolean","value":true}}]}`
	var n cdpAXNode
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(axState(n))
	const want = `{"checked":"mixed","pressed":"false","selected":false,"expanded":false,"disabled":true,"required":true,"invalid":"spelling","level":3,"hasPopup":"menu","multiselectable":true,"orientation":"vertical","valueMin":0,"valueMax":100,"valueText":"30 percent","autocomplete":"list","keyShortcuts":"Alt+S","roleDescription":"slide","modal":true}`
	if string(got) != want {
		t.Errorf("axState =\n%s\nwant\n%s", got, want)
	}

	n = cdpAXNode{}
	if err := json.Unmarshal([]byte(`{"properties":[
		{"name":"invalid","value":{"type":"token","value":"false"}},
		{"name":"valuetext","value":{"type":"string","value":""}},
		{"name":"autocomplete","value":{"type":"token","value":"none"}},
		{"name":"editable","value":{"type":"token","value":"plaintext"}}]}`), &n); err != nil {
		t.Fatal(err)
	}
	if got := axState(n); !reflect.DeepEqual(got, preset.AXState{}) {
		t.Errorf("axState = %+v, want zero", got)
	}
}

// Context is the ancestors, outermost first, without ignored nodes, generic
// wrappers, role "none" or internal roles (the document root); a parent loop
// can't hang it.
func TestAXContext(t *testing.T) {
	const raw = `[
		{"nodeId":"1","role":{"type":"internalRole","value":"RootWebArea"},"name":{"value":"Shop"}},
		{"nodeId":"2","parentId":"1","ignored":true,"role":{"type":"role","value":"none"}},
		{"nodeId":"3","parentId":"2","role":{"type":"role","value":"main"},"name":{"value":""}},
		{"nodeId":"4","parentId":"3","role":{"type":"role","value":"form"},"name":{"value":"Prefs"}},
		{"nodeId":"5","parentId":"4","role":{"type":"role","value":"group"},"name":{"value":"Sort by"}},
		{"nodeId":"6","parentId":"5","role":{"type":"role","value":"generic"},"name":{"value":""}},
		{"nodeId":"7","parentId":"6","role":{"type":"role","value":"radio"},"name":{"value":"Asc"}},
		{"nodeId":"8","parentId":"7","role":{"type":"internalRole","value":"StaticText"}}]`
	var nodes []cdpAXNode
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		t.Fatal(err)
	}
	want := []preset.ContextItem{{Role: "main"}, {Role: "form", Name: "Prefs"}, {Role: "group", Name: "Sort by"}}
	if got := axContext(nodes, "7"); !reflect.DeepEqual(got, want) {
		t.Errorf("axContext = %+v, want %+v", got, want)
	}
	if got := axContext(nodes, "1"); got != nil {
		t.Errorf("root context = %+v, want none", got)
	}

	loop := []cdpAXNode{{NodeID: "a", ParentID: "b"}, {NodeID: "b", ParentID: "a"}}
	if got := axContext(loop, "a"); got != nil {
		t.Errorf("loop context = %+v", got)
	}
}

// The description is kept when Chromium has one and left nil when it has
// none or an empty one.
func TestAXFromNodeDescription(t *testing.T) {
	for raw, want := range map[string]*string{
		`{"description":{"type":"computedString","value":"Must be 8 chars"}}`: ptr("Must be 8 chars"),
		`{"description":{"type":"computedString","value":""}}`:                nil,
		`{"role":{"value":"button"}}`:                                         nil,
	} {
		var n cdpAXNode
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatal(err)
		}
		if got := axFromNode(n).Description; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: description = %v, want %v", raw, got, want)
		}
	}
}

// ARIA roles pass through; known internal roles map to their ARIA stand-in;
// an unknown internal role gives no role rather than a guess.
func TestARIARole(t *testing.T) {
	for raw, want := range map[string]*string{
		`{"role":{"type":"role","value":"button"}}`:                     ptr("button"),
		`{"role":{"type":"internalRole","value":"Date"}}`:               ptr("textbox"),
		`{"role":{"type":"internalRole","value":"DisclosureTriangle"}}`: ptr("button"),
		`{"role":{"type":"internalRole","value":"Video"}}`:              ptr("generic"),
		`{"role":{"type":"internalRole","value":"SomethingNew"}}`:       nil,
		`{}`: nil,
	} {
		var n cdpAXNode
		if err := json.Unmarshal([]byte(raw), &n); err != nil {
			t.Fatal(err)
		}
		if got := ariaRole(n); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: role = %v, want %v", raw, got, want)
		}
	}
}
