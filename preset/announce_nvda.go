package preset

import (
	"slices"
	"strconv"
)

// NVDA's wording, from its own source (controlTypes role and state labels,
// processAndLabelStates, speech.getPropertiesSpeech) and from how Chrome
// exposes each ARIA role to it on Windows (MSAA / IA2 roles). NVDA has one
// English locale, so there is no British variant.

// nvdaRole is how NVDA sees an element's role: its kind (NVDA's role name,
// which its speech rules depend on) and the label it speaks.
type nvdaRole struct {
	kind  string
	label string
}

// nvdaRoles maps each ARIA role to NVDA's role, as Chrome exposes it. A role
// that isn't here has no label (generic, paragraph, …).
var nvdaRoles = map[string]nvdaRole{
	"alert": {"ALERT", "alert"}, "alertdialog": {"DIALOG", "dialog"},
	"application": {"APPLICATION", "application"}, "article": {"ARTICLE", "article"},
	"banner": {"LANDMARK", "banner landmark"}, "button": {"BUTTON", "button"},
	"cell": {"TABLECELL", "cell"}, "checkbox": {"CHECKBOX", "check box"},
	"columnheader": {"TABLECOLUMNHEADER", "column header"}, "combobox": {"COMBOBOX", "combo box"},
	"complementary": {"LANDMARK", "complementary landmark"},
	"contentinfo":   {"LANDMARK", "content info landmark"},
	"definition":    {"DEFINITION", "definition"}, "dialog": {"DIALOG", "dialog"},
	"document": {"DOCUMENT", "document"}, "feed": {"GROUPING", "grouping"},
	"figure": {"FIGURE", "figure"}, "form": {"FORM", "form"}, "grid": {"TABLE", "table"},
	"gridcell": {"TABLECELL", "cell"}, "group": {"GROUPING", "grouping"},
	"heading": {"HEADING", "heading"}, "image": {"GRAPHIC", "graphic"}, "img": {"GRAPHIC", "graphic"},
	"link": {"LINK", "link"}, "list": {"LIST", "list"}, "listbox": {"LIST", "list"},
	"listitem": {"LISTITEM", "list item"}, "log": {"GROUPING", "grouping"},
	"main": {"LANDMARK", "main landmark"}, "marquee": {"ANIMATION", "animation"},
	"math": {"MATH", "math"}, "menu": {"POPUPMENU", "menu"}, "menubar": {"MENUBAR", "menu bar"},
	"menuitem": {"MENUITEM", "menu item"}, "menuitemcheckbox": {"CHECKMENUITEM", "check menu item"},
	"menuitemradio": {"RADIOMENUITEM", "radio menu item"}, "meter": {"PROGRESSBAR", "progress bar"},
	"navigation": {"LANDMARK", "navigation landmark"}, "note": {"NOTE", "note"},
	"option": {"LISTITEM", "list item"}, "progressbar": {"PROGRESSBAR", "progress bar"},
	"radio": {"RADIOBUTTON", "radio button"}, "radiogroup": {"GROUPING", "grouping"},
	"region": {"REGION", "region"}, "row": {"TABLEROW", "row"},
	"rowheader": {"TABLEROWHEADER", "row header"}, "scrollbar": {"SCROLLBAR", "scroll bar"},
	"search": {"LANDMARK", "search landmark"}, "searchbox": {"EDITABLETEXT", "edit"},
	"separator": {"SEPARATOR", "separator"}, "slider": {"SLIDER", "slider"},
	"spinbutton": {"SPINBUTTON", "spin button"}, "status": {"STATUSBAR", "status bar"},
	"switch": {"SWITCH", "switch"}, "tab": {"TAB", "tab"}, "table": {"TABLE", "table"},
	"tablist": {"TABCONTROL", "tab control"}, "tabpanel": {"PROPERTYPAGE", "property page"},
	"textbox": {"EDITABLETEXT", "edit"}, "toolbar": {"TOOLBAR", "tool bar"},
	"tooltip": {"TOOLTIP", "tool tip"}, "tree": {"TREEVIEW", "tree view"},
	"treegrid": {"TREEVIEW", "tree view"}, "treeitem": {"TREEVIEWITEM", "tree view item"},
}

// nvdaSilentRolesOnFocus are the roles NVDA doesn't speak on focus when the
// element has a name or a value; nvdaSilentValueRoles, those whose value it
// never speaks.
var (
	nvdaSilentRolesOnFocus = []string{"APPLICATION", "TABLECELL", "LISTITEM", "MENUITEM", "CHECKMENUITEM", "TREEVIEWITEM"}
	nvdaSilentValueRoles   = []string{"CHECKBOX", "RADIOBUTTON", "LINK", "MENUITEM"}
)

// roleOfNVDA is e's role as NVDA sees it in Chrome: a button with
// aria-pressed is a toggle button, one with a popup a menu button.
func roleOfNVDA(e element) nvdaRole {
	if e.role == "button" {
		switch {
		case e.state.Pressed != nil:
			return nvdaRole{"TOGGLEBUTTON", "toggle button"}
		case hasPopup(e):
			return nvdaRole{"MENUBUTTON", "menu button"}
		}
	}
	return nvdaRoles[e.role]
}

func hasPopup(e element) bool {
	return e.state.HasPopup != nil && *e.state.HasPopup != "" && *e.state.HasPopup != "false"
}

// AnnounceNVDA is Announce as NVDA says it in Chrome: the same events and the
// same rules about what is spoken when, in NVDA's wording and order. NVDA has
// one English locale, so it takes no Language.
//
// Within an element, NVDA's order is name, role, value, states, description,
// keyboard shortcut, position ("2 of 5") and level ("level 2"). Its words
// differ from VoiceOver's: "edit", "check box", "not checked", "toggle button"
// and "pressed", "menu button" and "subMenu", "unavailable", "invalid entry",
// "main landmark", …. It doesn't speak the role of a named list item, menu
// item or tree item, says "not selected" for one that isn't selected, and
// leaves out unnamed groupings it passes through.
func AnnounceNVDA(cur, prev *FocusedElement) []string {
	if cur == nil || cur.Ignored {
		return []string{}
	}
	out := []string{}
	if prev == nil || cur.NodeID == 0 || cur.NodeID != prev.NodeID {
		var prevCtx []ContextItem
		var before *element
		if prev != nil {
			prevCtx = prev.Context
			b := elementOf(prev)
			before = &b
		}
		for _, c := range enteredContext(cur.Context, prevCtx) {
			out = append(out, contextNVDA(c)...)
		}
		now := elementOf(cur)
		out = append(out, announceElementNVDA(now, before)...)
		if ad := cur.ActiveDescendant; ad != nil {
			out = append(out, announceElementNVDA(descendantOf(ad), &now)...)
		}
		return out
	}

	now, before := elementOf(cur), elementOf(prev)
	if now.name != before.name {
		out = appendNonEmpty(out, now.name)
	}
	out = append(out, changedItemsNVDA(now, before)...)
	switch ad, pad := cur.ActiveDescendant, prev.ActiveDescendant; {
	case ad == nil:
	case pad == nil || ad.NodeID == 0 || ad.NodeID != pad.NodeID:
		var was *element
		if pad != nil {
			b := descendantOf(pad)
			was = &b
		}
		out = append(out, announceElementNVDA(descendantOf(ad), was)...)
	default:
		a, b := descendantOf(ad), descendantOf(pad)
		if a.name != b.name {
			out = appendNonEmpty(out, a.name)
		}
		out = append(out, changedItemsNVDA(a, b)...)
	}
	return out
}

// contextNVDA is what NVDA says entering ancestor c: its name and role. An
// unnamed grouping is layout to NVDA, and the items it passes through (list
// items, rows, cells, tree items) aren't announced as context, so it says
// nothing for them.
func contextNVDA(c ContextItem) []string {
	r := nvdaRoles[c.Role]
	silent := []string{"LISTITEM", "TABLEROW", "TABLECELL", "TREEVIEWITEM"}
	if r.label == "" || slices.Contains(silent, r.kind) || (r.kind == "GROUPING" && c.Name == "") {
		return nil
	}
	return appendNonEmpty(nil, c.Name, r.label)
}

// announceElementNVDA is e announced in full, in NVDA's order. before is the
// element announced just before, if any: a tree item's level is spoken
// first, and only when it differs from before's.
func announceElementNVDA(e element, before *element) []string {
	if e.ignored {
		return nil
	}
	r := roleOfNVDA(e)
	var out []string
	if r.kind == "TREEVIEWITEM" && e.state.Level != nil &&
		(before == nil || before.state.Level == nil || *before.state.Level != *e.state.Level) {
		out = append(out, "level "+strconv.Itoa(*e.state.Level))
	}
	out = appendNonEmpty(out, e.name)
	value := e.value
	if slices.Contains(nvdaSilentValueRoles, r.kind) || value == e.name {
		value = ""
	}
	if !slices.Contains(nvdaSilentRolesOnFocus, r.kind) || (e.name == "" && value == "") {
		out = appendNonEmpty(out, roleWordNVDA(e, r))
	}
	out = appendNonEmpty(out, value)
	out = append(out, stateWordsNVDA(e, r, false)...)
	out = appendNonEmpty(out, e.description, deref(e.state.KeyShortcuts))
	if setRoles[e.role] && e.posInSet > 0 && e.setSize > 0 {
		out = append(out, strconv.Itoa(e.posInSet)+" of "+strconv.Itoa(e.setSize))
	}
	if r.kind == "HEADING" && e.state.Level != nil {
		out = append(out, "level "+strconv.Itoa(*e.state.Level))
	}
	return out
}

// roleWordNVDA is e's role as NVDA speaks it: its aria-roledescription if it
// has one, otherwise NVDA's label.
func roleWordNVDA(e element, r nvdaRole) string {
	if d := deref(e.state.RoleDescription); d != "" {
		return d
	}
	return r.label
}

// changedItemsNVDA is what NVDA says when e is still focused but has changed
// since before: its new value, then the state words it didn't have
// (including the negative ones, e.g. "not checked").
func changedItemsNVDA(e, before element) []string {
	if e.ignored {
		return nil
	}
	r := roleOfNVDA(e)
	var out []string
	if e.value != before.value && !slices.Contains(nvdaSilentValueRoles, r.kind) {
		out = appendNonEmpty(out, e.value)
	}
	was := stateWordsNVDA(before, roleOfNVDA(before), true)
	for _, s := range stateWordsNVDA(e, r, true) {
		if !slices.Contains(was, s) {
			out = append(out, s)
		}
	}
	return out
}

// nvdaState is one state word, with NVDA's State value, which orders them.
type nvdaState struct {
	order int
	word  string
}

// stateWordsNVDA are e's states as NVDA speaks them, ordered by NVDA's State
// values, following its processAndLabelStates. change is set when the
// element changed while focused (NVDA's OutputReason.CHANGE) rather than
// gained focus.
func stateWordsNVDA(e element, r nvdaRole, change bool) []string {
	st := e.state
	var ss []nvdaState
	add := func(order int, word string) { ss = append(ss, nvdaState{order, word}) }
	if st.Disabled {
		add(0, "unavailable")
	}
	// An option with no aria-selected is selectable and not selected to NVDA
	// too (Chrome exposes every option as selectable).
	if st.Selected != nil || e.role == "option" {
		selected := st.Selected != nil && *st.Selected
		silentWhenSelected := []string{"LISTITEM", "TREEVIEWITEM", "MENUITEM", "TABLEROW", "CHECKBOX"}
		negativeWhenNot := []string{"LISTITEM", "TREEVIEWITEM", "TABLEROW", "TABLECELL", "TABLECOLUMNHEADER", "TABLEROWHEADER", "CHECKBOX"}
		switch {
		case selected && (change || !slices.Contains(silentWhenSelected, r.kind)):
			add(2, "selected")
		case !selected && slices.Contains(negativeWhenNot, r.kind):
			add(2, "not selected")
		}
	}
	if r.kind == "TOGGLEBUTTON" {
		switch *st.Pressed {
		case "true":
			add(4, "pressed")
		case "mixed":
			add(45, "half pressed")
		default:
			add(4, "not pressed")
		}
	}
	if st.Checked != nil {
		switch c := *st.Checked; r.kind {
		case "SWITCH":
			if c == "true" {
				add(46, "on")
			} else {
				add(46, "off")
			}
		case "CHECKBOX", "RADIOBUTTON", "CHECKMENUITEM", "RADIOMENUITEM":
			switch c {
			case "true":
				add(5, "checked")
			case "mixed":
				add(6, "half checked")
			default:
				add(5, "not checked")
			}
		}
	}
	if st.ReadOnly && (r.kind == "EDITABLETEXT" || r.kind == "CHECKBOX") {
		add(7, "read only")
	}
	popup := hasPopup(e) && r.kind != "COMBOBOX" && st.Autocomplete == nil
	if st.Expanded != nil && !(r.kind == "MENUITEM" && popup) {
		if *st.Expanded {
			add(8, "expanded")
		} else {
			add(9, "collapsed")
		}
	}
	if popup {
		switch *st.HasPopup {
		case "dialog":
			add(47, "opens dialog")
		case "grid":
			add(48, "opens grid")
		case "listbox":
			add(49, "opens list")
		case "tree":
			add(50, "opens tree")
		default:
			add(13, "subMenu")
		}
	}
	if st.Required {
		add(15, "required")
	}
	if st.Invalid != nil {
		add(17, "invalid entry")
	}
	if st.Autocomplete != nil {
		add(19, "has auto complete")
	}
	slices.SortStableFunc(ss, func(a, b nvdaState) int { return a.order - b.order })
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.word
	}
	return out
}
