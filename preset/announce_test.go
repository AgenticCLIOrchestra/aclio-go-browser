package preset

import (
	"reflect"
	"slices"
	"testing"
)

func radioAsc() *FocusedElement {
	return &FocusedElement{
		Role: ptr("radio"), Name: ptr("Ascending"), NodeID: 7,
		Context:  []ContextItem{{Role: "main"}, {Role: "form", Name: "Prefs"}, {Role: "group", Name: "Sort by"}},
		AXState:  AXState{Checked: ptr("true")},
		PosInSet: ptr(1), SetSize: ptr(2),
	}
}

func TestAnnounceFull(t *testing.T) {
	got := Announce(radioAsc(), nil, "")
	want := []string{"main", "Prefs", "form", "Sort by", "group", "Ascending", "selected", "radio button", "1 of 2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Announce =\n%q\nwant\n%q", got, want)
	}
}

func TestAnnounceLanguages(t *testing.T) {
	cb := &FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 1,
		AXState:     AXState{Checked: ptr("false"), Required: true, Invalid: ptr("true")},
		Description: ptr("You must agree")}
	us := Announce(cb, nil, LanguageEnUS)
	if want := []string{"Agree", "unchecked", "required", "invalid data", "checkbox", "You must agree"}; !reflect.DeepEqual(us, want) {
		t.Errorf("en-US = %q, want %q", us, want)
	}
	gb := Announce(cb, nil, LanguageEnGB)
	if want := []string{"Agree", "unticked", "required", "invalid data", "tick box", "You must agree"}; !reflect.DeepEqual(gb, want) {
		t.Errorf("en-GB = %q, want %q", gb, want)
	}
	if def := Announce(cb, nil, ""); !reflect.DeepEqual(def, us) {
		t.Errorf("default = %q, want en-US %q", def, us)
	}
}

func TestAnnounceRoles(t *testing.T) {
	for _, c := range []struct {
		fe   *FocusedElement
		want []string
	}{
		{&FocusedElement{Role: ptr("heading"), Name: ptr("Settings"), AXState: AXState{Level: ptr(2)}}, []string{"Settings", "heading level 2"}},
		{&FocusedElement{Role: ptr("combobox"), Name: ptr("Size"), Value: ptr("Large"), AXState: AXState{Expanded: ptr(false)}}, []string{"Size", "Large", "collapsed", "pop-up button"}},
		{&FocusedElement{Role: ptr("combobox"), Name: ptr("City"), AXState: AXState{Autocomplete: ptr("list")}}, []string{"City", "combo box"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Menu"), AXState: AXState{HasPopup: ptr("menu"), Expanded: ptr(true)}}, []string{"Menu", "expanded", "pop-up button"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Bold"), AXState: AXState{Pressed: ptr("true")}}, []string{"Bold", "selected", "button"}},
		{&FocusedElement{Role: ptr("button"), Name: ptr("Next"), AXState: AXState{RoleDescription: ptr("slide"), Disabled: true}}, []string{"Next", "dimmed", "slide"}},
		{&FocusedElement{Role: ptr("switch"), Name: ptr("Wi-Fi"), AXState: AXState{Checked: ptr("true")}}, []string{"Wi-Fi", "on", "switch"}},
		{&FocusedElement{Role: ptr("slider"), Name: ptr("Vol"), Value: ptr("30"), Aria: map[string]string{"aria-valuetext": "30 percent"}}, []string{"Vol", "30 percent", "slider"}},
		{&FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), Value: ptr("a@b.c")}, []string{"Email", "a@b.c", "edit text"}},
		{&FocusedElement{Role: ptr("generic"), Name: ptr("")}, []string{}},
		{&FocusedElement{Role: ptr("radio"), Name: ptr("Desc"), AXState: AXState{Checked: ptr("false")}}, []string{"Desc", "radio button"}},
	} {
		if got := Announce(c.fe, nil, ""); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Announce(%s) = %q, want %q", deref(c.fe.Name), got, c.want)
		}
	}
}

// Focus moving within the same group announces only the element, not the
// context it was already in; moving out to another landmark announces that.
func TestAnnounceContextDiff(t *testing.T) {
	prev := radioAsc()
	next := radioAsc()
	next.NodeID, next.Name, next.Checked, next.PosInSet = 8, ptr("Descending"), ptr("false"), ptr(2)
	if got, want := Announce(next, prev, ""), []string{"Descending", "radio button", "2 of 2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same group = %q, want %q", got, want)
	}

	nav := &FocusedElement{Role: ptr("link"), Name: ptr("Home"), NodeID: 9,
		Context: []ContextItem{{Role: "navigation", Name: "Primary"}, {Role: "list"}, {Role: "listitem"}}}
	if got, want := Announce(nav, prev, ""), []string{"Primary", "navigation", "list", "Home", "link"}; !reflect.DeepEqual(got, want) {
		t.Errorf("into nav = %q, want %q", got, want)
	}
}

// Focus staying on the same element announces only what changed, and
// nothing when nothing did.
func TestAnnounceSameElement(t *testing.T) {
	before := &FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 3,
		AXState: AXState{Checked: ptr("false"), Required: true, Invalid: ptr("true")}}
	after := &FocusedElement{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 3,
		AXState: AXState{Checked: ptr("true"), Required: true}}
	if got, want := Announce(after, before, LanguageEnGB), []string{"ticked"}; !reflect.DeepEqual(got, want) {
		t.Errorf("toggled = %q, want %q", got, want)
	}
	if got := Announce(after, after, ""); len(got) != 0 {
		t.Errorf("unchanged = %q, want nothing", got)
	}

	typed := &FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), NodeID: 4, Value: ptr("a@b")}
	empty := &FocusedElement{Role: ptr("textbox"), Name: ptr("Email"), NodeID: 4}
	if got, want := Announce(typed, empty, ""), []string{"a@b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("typed = %q, want %q", got, want)
	}
}

// Entering a listbox announces it and its active item; arrowing within it
// announces only the new item; a hidden item says nothing.
func TestAnnounceActiveDescendant(t *testing.T) {
	lb := func(ad *ActiveDescendant) *FocusedElement {
		return &FocusedElement{Role: ptr("listbox"), Name: ptr("Fruit"), NodeID: 10, ActiveDescendant: ad}
	}
	a := &ActiveDescendant{Role: ptr("option"), Name: ptr("Apple"), NodeID: 11, AXState: AXState{Selected: ptr(true)}, PosInSet: ptr(1), SetSize: ptr(3)}
	b := &ActiveDescendant{Role: ptr("option"), Name: ptr("Banana"), NodeID: 12, AXState: AXState{Selected: ptr(true)}, PosInSet: ptr(2), SetSize: ptr(3)}

	if got, want := Announce(lb(a), nil, ""), []string{"Fruit", "list box", "Apple", "selected", "1 of 3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("enter = %q, want %q", got, want)
	}
	if got, want := Announce(lb(b), lb(a), ""), []string{"Banana", "selected", "2 of 3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("arrow = %q, want %q", got, want)
	}
	hidden := &ActiveDescendant{NodeID: 13, AXState: AXState{Ignored: true, IgnoredReasons: []string{"ariaHiddenElement"}}}
	if got := Announce(lb(hidden), lb(a), ""); len(got) != 0 {
		t.Errorf("hidden item = %q, want nothing", got)
	}
}

// NVDA says "not selected" for a selectable item that isn't selected, on
// arrival and when it is deselected in place; VoiceOver never does. Both say
// "selected" alike, and NVDA says nothing extra for items that aren't
// selectable or whose role it doesn't report it for.
func TestAnnounceAllNotSelected(t *testing.T) {
	both := func(text string) Announcement { return Announcement{Text: text, VoiceOver: true, NVDA: true} }
	nvda := func(text string) Announcement { return Announcement{Text: text, NVDA: true} }
	lb := func(ad *ActiveDescendant) *FocusedElement {
		return &FocusedElement{Role: ptr("listbox"), Name: ptr("Fruit"), NodeID: 10, AXState: AXState{Multiselectable: true}, ActiveDescendant: ad}
	}
	opt := func(id int, name string, selected *bool) *ActiveDescendant {
		return &ActiveDescendant{Role: ptr("option"), Name: ptr(name), NodeID: id, AXState: AXState{Selected: selected}, PosInSet: ptr(id - 10), SetSize: ptr(3)}
	}
	apple, banana := opt(11, "Apple", ptr(true)), opt(12, "Banana", ptr(false))

	if got, want := AnnounceAll(lb(banana), lb(apple), ""), []Announcement{both("Banana"), nvda("not selected"), both("2 of 3")}; !reflect.DeepEqual(got, want) {
		t.Errorf("arrive on an unselected option = %v, want %v", got, want)
	}
	if got, want := AnnounceAll(lb(apple), lb(banana), ""), []Announcement{both("Apple"), both("selected"), both("1 of 3")}; !reflect.DeepEqual(got, want) {
		t.Errorf("arrive on a selected option = %v, want %v", got, want)
	}
	deselected := opt(11, "Apple", ptr(false))
	if got, want := AnnounceAll(lb(deselected), lb(apple), ""), []Announcement{nvda("not selected")}; !reflect.DeepEqual(got, want) {
		t.Errorf("deselect in place = %v, want %v", got, want)
	}
	if got, want := AnnounceAll(lb(apple), lb(deselected), ""), []Announcement{both("selected")}; !reflect.DeepEqual(got, want) {
		t.Errorf("select in place = %v, want %v", got, want)
	}
	// Single-select lists too: NVDA's rule doesn't depend on multiselectable.
	single := &FocusedElement{Role: ptr("listbox"), Name: ptr("Fruit"), NodeID: 10, ActiveDescendant: banana}
	if got := NVDASaid(AnnounceAll(single, nil, "")); !slices.Contains(got, "not selected") {
		t.Errorf("single-select unselected option, NVDA = %q", got)
	}
	// An option with no aria-selected is "not selected" to NVDA too.
	if got, want := AnnounceAll(lb(opt(13, "Cherry", nil)), lb(banana), ""), []Announcement{both("Cherry"), nvda("not selected"), both("3 of 3")}; !reflect.DeepEqual(got, want) {
		t.Errorf("option with no aria-selected = %v, want %v", got, want)
	}
	// Not selectable (no selected state on a non-option), or a role NVDA
	// doesn't report it for.
	for _, fe := range []*FocusedElement{
		{Role: ptr("treeitem"), Name: ptr("Docs"), NodeID: 22},
		{Role: ptr("checkbox"), Name: ptr("Agree"), NodeID: 23, AXState: AXState{Checked: ptr("false")}},
		{Role: ptr("tab"), Name: ptr("Home"), NodeID: 20, AXState: AXState{Selected: ptr(false)}},
		{Role: ptr("radio"), Name: ptr("Asc"), NodeID: 21, AXState: AXState{Checked: ptr("false")}},
	} {
		if got := AnnounceAll(fe, nil, ""); !reflect.DeepEqual(NVDASaid(got), VoiceOverSaid(got)) {
			t.Errorf("%s: NVDA %q, VoiceOver %q, want the same", deref(fe.Name), NVDASaid(got), VoiceOverSaid(got))
		}
	}
	// Announce stays VoiceOver's feed.
	if got := Announce(lb(banana), lb(apple), ""); slices.Contains(got, "not selected") {
		t.Errorf("Announce = %q, want no not selected", got)
	}
}

func TestAnnounceSilence(t *testing.T) {
	if got := Announce(nil, radioAsc(), ""); got == nil || len(got) != 0 {
		t.Errorf("nothing focused = %#v, want empty non-nil", got)
	}
	if got := Announce(&FocusedElement{AXState: AXState{Ignored: true}}, nil, ""); len(got) != 0 {
		t.Errorf("ignored = %q", got)
	}
}

func TestResolveLanguage(t *testing.T) {
	if l, err := ResolveLanguage(""); err != nil || l != LanguageEnUS {
		t.Errorf("zero = %q, %v", l, err)
	}
	if l, err := ResolveLanguage(LanguageEnGB); err != nil || l != LanguageEnGB {
		t.Errorf("en-GB = %q, %v", l, err)
	}
	if _, err := ResolveLanguage("de-DE"); err == nil {
		t.Error("de-DE should be refused")
	}
	p := Blind()
	p.Language = "fr"
	if err := Validate(p); err == nil {
		t.Error("Validate should refuse an unsupported language")
	}
}
