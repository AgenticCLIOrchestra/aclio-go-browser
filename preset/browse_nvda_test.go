package preset

import (
	"reflect"
	"testing"
)

// Browse mode wording: single-line fields read whole (role first when moving
// by line, content first for quick navigation and focus), containers
// announced on entry and left with "out of …" (but not landmarks).
func TestNVDABrowseEnter(t *testing.T) {
	for _, c := range []struct {
		name   string
		f      BrowseField
		reason BrowseReason
		want   []string
	}{
		{"heading by line", BrowseField{Role: "heading", Name: "Welcome", Content: "Welcome", AXState: AXState{Level: ptr(1)}}, BrowseCaret, []string{"heading", "level 1", "Welcome"}},
		{"heading by H", BrowseField{Role: "heading", Name: "Welcome", Content: "Welcome", AXState: AXState{Level: ptr(1)}}, BrowseQuickNav, []string{"Welcome", "heading", "level 1"}},
		{"link by line", BrowseField{Role: "link", Name: "terms", Content: "terms"}, BrowseCaret, []string{"link", "terms"}},
		{"link by K", BrowseField{Role: "link", Name: "terms", Content: "terms"}, BrowseQuickNav, []string{"terms", "link"}},
		{"edit by E", BrowseField{Role: "textbox", Name: "Query", Value: "cats"}, BrowseQuickNav, []string{"Query", "edit", "cats"}},
		{"check box", BrowseField{Role: "checkbox", Name: "Agree", AXState: AXState{Checked: ptr("false")}}, BrowseCaret, []string{"Agree", "check box", "not checked"}},
		{"plain list", BrowseField{Role: "list", Items: 3}, BrowseCaret, []string{"list", "with 3 items"}},
		{"one item", BrowseField{Role: "list", Items: 1}, BrowseCaret, []string{"list", "with 1 item"}},
		{"table", BrowseField{Role: "table", Name: "Prices", Rows: 3, Columns: 4}, BrowseCaret, []string{"Prices", "table", "with 3 rows and 4 columns"}},
		{"main", BrowseField{Role: "main"}, BrowseCaret, []string{"main landmark"}},
		{"named nav by D", BrowseField{Role: "navigation", Name: "Primary"}, BrowseQuickNav, []string{"Primary", "navigation landmark"}},
		{"unnamed group", BrowseField{Role: "group"}, BrowseCaret, nil},
		{"separator", BrowseField{Role: "separator"}, BrowseCaret, []string{"separator"}},
		{"paragraph", BrowseField{Role: "paragraph"}, BrowseCaret, nil},
	} {
		if got := NVDABrowseEnter(c.f, c.reason); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	for _, c := range []struct {
		f    BrowseField
		want []string
	}{
		{BrowseField{Role: "list", Items: 2}, []string{"out of list"}},
		{BrowseField{Role: "table"}, []string{"out of table"}},
		{BrowseField{Role: "main"}, nil},
		{BrowseField{Role: "link"}, nil},
	} {
		if got := NVDABrowseExit(c.f); !reflect.DeepEqual(got, c.want) {
			t.Errorf("exit %s: %q, want %q", c.f.Role, got, c.want)
		}
	}
}

// NVDA switches to focus mode by itself in fields and widgets it can't
// operate in browse mode, not on links and buttons.
func TestNVDAShouldPassThrough(t *testing.T) {
	for role, want := range map[string]bool{
		"textbox": true, "searchbox": true, "combobox": true, "listbox": true, "option": true, "slider": true,
		"spinbutton": true, "tree": true, "treeitem": true, "menu": true, "menuitem": true, "radio": true, "tab": true,
		"gridcell": true, "link": false, "button": false, "checkbox": false, "heading": false, "list": false,
	} {
		if got := NVDAShouldPassThrough(&FocusedElement{Role: ptr(role)}); got != want {
			t.Errorf("%s: %v, want %v", role, got, want)
		}
	}
	inToolbar := &FocusedElement{Role: ptr("button"), Context: []ContextItem{{Role: "toolbar"}}}
	if !NVDAShouldPassThrough(inToolbar) {
		t.Error("a button in a toolbar should switch to focus mode")
	}
	// Read-only widgets stay in browse mode, except the fields NVDA lets
	// through anyway (a read-only text field still switches).
	if NVDAShouldPassThrough(&FocusedElement{Role: ptr("slider"), AXState: AXState{ReadOnly: true}}) {
		t.Error("a read-only slider shouldn't switch")
	}
	if !NVDAShouldPassThrough(&FocusedElement{Role: ptr("textbox"), AXState: AXState{ReadOnly: true}}) {
		t.Error("a read-only text field should switch")
	}
}
