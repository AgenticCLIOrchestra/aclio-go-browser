package preset

import (
	"reflect"
	"testing"
)

// The same events as the VoiceOver tests, in NVDA's wording and order:
// name, role, value, states, description, position, level.

func TestAnnounceNVDAFull(t *testing.T) {
	got := AnnounceNVDA(radioAsc(), nil)
	want := []string{"main landmark", "Prefs", "form", "Sort by", "grouping", "Ascending", "radio button", "checked", "1 of 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AnnounceNVDA =\n%q\nwant\n%q", got, want)
	}
}

func TestAnnounceNVDARoles(t *testing.T) {
	for _, c := range []struct {
		fe   *FocusedElement
		want []string
	}{
		{&FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), AXState: AXState{Checked: ptr("false"), Required: true, Invalid: ptr("true")}, Description: ptr("You must agree")},
			[]string{"Agree", "check box", "not checked", "required", "invalid entry", "You must agree"}},
		{&FocusedElement{Role: ptr("checkbox"), Name: ptr("All"), AXState: AXState{Checked: ptr("mixed")}}, []string{"All", "check box", "half checked"}},
		{&FocusedElement{Role: ptr("heading"), Name: ptr("Settings"), AXState: AXState{Level: ptr(2)}}, []string{"Settings", "heading", "level 2"}},
		{&FocusedElement{Role: ptr("combobox"), Name: ptr("Size"), Value: ptr("Large"), AXState: AXState{Expanded: ptr(false), HasPopup: ptr("listbox")}},
			[]string{"Size", "combo box", "Large", "collapsed"}},
		{&FocusedElement{Role: ptr("combobox"), Name: ptr("City"), AXState: AXState{Autocomplete: ptr("list"), Expanded: ptr(false), HasPopup: ptr("listbox")}},
			[]string{"City", "combo box", "collapsed", "has auto complete"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Menu"), AXState: AXState{HasPopup: ptr("menu"), Expanded: ptr(true)}}, []string{"Menu", "menu button", "expanded", "subMenu"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Share"), AXState: AXState{HasPopup: ptr("dialog")}}, []string{"Share", "menu button", "opens dialog"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Bold"), AXState: AXState{Pressed: ptr("true")}}, []string{"Bold", "toggle button", "pressed"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Italic"), AXState: AXState{Pressed: ptr("false")}}, []string{"Italic", "toggle button", "not pressed"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Next"), AXState: AXState{RoleDescription: ptr("slide"), Disabled: true}}, []string{"Next", "slide", "unavailable"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Save"), AXState: AXState{KeyShortcuts: ptr("Control+S")}}, []string{"Save", "button", "Control+S"}},
		{&FocusedElement{Role: ptr("switch"), Name: ptr("Wi-Fi"), AXState: AXState{Checked: ptr("true")}}, []string{"Wi-Fi", "switch", "on"}},
		{&FocusedElement{Role: ptr("switch"), Name: ptr("Wi-Fi"), AXState: AXState{Checked: ptr("false")}}, []string{"Wi-Fi", "switch", "off"}},
		{&FocusedElement{Role: ptr("slider"), Name: ptr("Vol"), Value: ptr("30"), Aria: map[string]string{"aria-valuetext": "30 percent"}}, []string{"Vol", "slider", "30 percent"}},
		{&FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), Value: ptr("a@b.c"), AXState: AXState{ReadOnly: true}}, []string{"Email", "edit", "a@b.c", "read only"}},
		{&FocusedElement{Role: ptr("link"), Name: ptr("Home"), Value: ptr("https://example.com/")}, []string{"Home", "link"}},
		{&FocusedElement{Role: ptr("tab"), Name: ptr("General"), AXState: AXState{Selected: ptr(true)}, PosInSet: ptr(1), SetSize: ptr(3)}, []string{"General", "tab", "selected", "1 of 3"}},
		{&FocusedElement{Role: ptr("menuitem"), Name: ptr("Copy"), PosInSet: ptr(2), SetSize: ptr(4)}, []string{"Copy", "2 of 4"}},
		{&FocusedElement{Role: ptr("menuitemcheckbox"), Name: ptr("Wrap"), AXState: AXState{Checked: ptr("false")}}, []string{"Wrap", "not checked"}},
		{&FocusedElement{Role: ptr("menuitemradio"), Name: ptr("Small"), AXState: AXState{Checked: ptr("true")}}, []string{"Small", "radio menu item", "checked"}},
		{&FocusedElement{Role: ptr("treeitem"), Name: ptr("src"), AXState: AXState{Level: ptr(1), Expanded: ptr(false)}}, []string{"level 1", "src", "collapsed"}},
		{&FocusedElement{Role: ptr("generic"), Name: ptr("")}, []string{}},
		{&FocusedElement{Role: ptr("radio"), Name: ptr("Desc"), AXState: AXState{Checked: ptr("false")}}, []string{"Desc", "radio button", "not checked"}},
	} {
		if got := AnnounceNVDA(c.fe, nil); !reflect.DeepEqual(got, c.want) {
			t.Errorf("AnnounceNVDA(%s) = %q, want %q", deref(c.fe.Name), got, c.want)
		}
	}
}

// Landmarks are "… landmark"; unnamed groupings are silent; lists and
// named regions are spoken.
func TestAnnounceNVDAContext(t *testing.T) {
	prev := radioAsc()
	next := radioAsc()
	next.NodeID, next.Name, next.Checked, next.PosInSet = 8, ptr("Descending"), ptr("false"), ptr(2)
	if got, want := AnnounceNVDA(next, prev), []string{"Descending", "radio button", "not checked", "2 of 2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same group = %q, want %q", got, want)
	}

	nav := &FocusedElement{Role: ptr("link"), Name: ptr("Home"), NodeID: 9,
		Context: []ContextItem{{Role: "navigation", Name: "Primary"}, {Role: "group"}, {Role: "list"}, {Role: "listitem"}}}
	if got, want := AnnounceNVDA(nav, prev), []string{"Primary", "navigation landmark", "list", "Home", "link"}; !reflect.DeepEqual(got, want) {
		t.Errorf("into nav = %q, want %q", got, want)
	}
	for role, want := range map[string]string{"banner": "banner landmark", "contentinfo": "content info landmark",
		"complementary": "complementary landmark", "search": "search landmark", "dialog": "dialog", "toolbar": "tool bar"} {
		fe := &FocusedElement{Role: ptr("button"), Name: ptr("Go"), NodeID: 1, Context: []ContextItem{{Role: role}}}
		if got := AnnounceNVDA(fe, nil); len(got) == 0 || got[0] != want {
			t.Errorf("%s context = %q, want %q first", role, got, want)
		}
	}
}

// On a change, NVDA says the new value and the new states, negative ones
// included.
func TestAnnounceNVDASameElement(t *testing.T) {
	unticked := &FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 3, AXState: AXState{Checked: ptr("false")}}
	ticked := &FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 3, AXState: AXState{Checked: ptr("true")}}
	if got, want := AnnounceNVDA(ticked, unticked), []string{"checked"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ticked = %q, want %q", got, want)
	}
	if got, want := AnnounceNVDA(unticked, ticked), []string{"not checked"}; !reflect.DeepEqual(got, want) {
		t.Errorf("unticked = %q, want %q", got, want)
	}
	if got := AnnounceNVDA(ticked, ticked); len(got) != 0 {
		t.Errorf("unchanged = %q, want nothing", got)
	}
	typed := &FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), NodeID: 4, Value: ptr("a@b")}
	empty := &FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), NodeID: 4}
	if got, want := AnnounceNVDA(typed, empty), []string{"a@b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("typed = %q, want %q", got, want)
	}
}

// Options: no role when named; "not selected" when unselected, nothing when
// selected on focus; "selected" when it becomes selected.
func TestAnnounceNVDAActiveDescendant(t *testing.T) {
	lb := func(ad *ActiveDescendant) *FocusedElement {
		return &FocusedElement{Role: ptr("listbox"), Name: ptr("Fruit"), NodeID: 10, ActiveDescendant: ad}
	}
	opt := func(id int, name string, selected bool, pos int) *ActiveDescendant {
		return &ActiveDescendant{Role: ptr("option"), Name: ptr(name), NodeID: id, AXState: AXState{Selected: ptr(selected)}, PosInSet: ptr(pos), SetSize: ptr(3)}
	}
	a, b := opt(11, "Apple", true, 1), opt(12, "Banana", false, 2)
	if got, want := AnnounceNVDA(lb(a), nil), []string{"Fruit", "list", "Apple", "1 of 3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("enter = %q, want %q", got, want)
	}
	if got, want := AnnounceNVDA(lb(b), lb(a)), []string{"Banana", "not selected", "2 of 3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("arrow = %q, want %q", got, want)
	}
	if got, want := AnnounceNVDA(lb(opt(12, "Banana", true, 2)), lb(b)), []string{"selected"}; !reflect.DeepEqual(got, want) {
		t.Errorf("select = %q, want %q", got, want)
	}
	hidden := &ActiveDescendant{NodeID: 13, AXState: AXState{Ignored: true}}
	if got := AnnounceNVDA(lb(hidden), lb(a)); len(got) != 0 {
		t.Errorf("hidden item = %q, want nothing", got)
	}
}

// A tree item's level is said first, and only when it changes.
func TestAnnounceNVDATreeLevel(t *testing.T) {
	item := func(id, level int, name string) *FocusedElement {
		return &FocusedElement{Role: ptr("treeitem"), Name: ptr(name), NodeID: id, AXState: AXState{Level: ptr(level)}}
	}
	if got, want := AnnounceNVDA(item(2, 1, "lib"), item(1, 1, "src")), []string{"lib"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same level = %q, want %q", got, want)
	}
	if got, want := AnnounceNVDA(item(3, 2, "a.go"), item(2, 1, "lib")), []string{"level 2", "a.go"}; !reflect.DeepEqual(got, want) {
		t.Errorf("deeper = %q, want %q", got, want)
	}
}

func TestAnnounceNVDASilence(t *testing.T) {
	if got := AnnounceNVDA(nil, radioAsc()); got == nil || len(got) != 0 {
		t.Errorf("nothing focused = %#v, want empty non-nil", got)
	}
	if got := AnnounceNVDA(&FocusedElement{AXState: AXState{Ignored: true}}, nil); len(got) != 0 {
		t.Errorf("ignored = %q", got)
	}
}
