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

	// Ignored: nothing at all.
	n.Ignored = true
	if ax := axFromNode(n); !reflect.DeepEqual(ax, axInfo{}) {
		t.Errorf("ignored = %+v", ax)
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
