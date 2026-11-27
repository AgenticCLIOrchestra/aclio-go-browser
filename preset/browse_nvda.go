package preset

import (
	"fmt"
	"slices"
	"strconv"
)

// NVDA's browse mode wording, from its source (speech.getControlFieldSpeech
// and textInfos.ControlField.getPresentationCategory): what NVDA says for the
// fields of its virtual buffer as the reading cursor enters and leaves them.

// BrowseField is one field (an element) of NVDA's browse mode virtual buffer,
// as the session builds it from Chromium's accessibility tree.
type BrowseField struct {
	Role        string // ARIA role, as in FocusedElement.Role
	Name        string // accessible name
	Value       string
	Description string
	AXState
	// Content is the field's text, for a field read as one piece (a link, a
	// button, a heading, …).
	Content   string
	Items     int  // a list: its items
	Rows      int  // a table: its rows
	Columns   int  // a table: its columns
	Editable  bool // takes typed text (a text field, a contenteditable)
	Multiline bool // a multi-line text field
	Nested    bool // inside another browse field (a dialog that is, say, a marker)
}

// BrowseCategory is how NVDA presents a field in browse mode.
type BrowseCategory int

const (
	// BrowseLayout fields aren't announced: only their content is read.
	BrowseLayout BrowseCategory = iota
	// BrowseSingleLine fields are read as one piece with their role and
	// states: links, headings, buttons, form fields, graphics, ….
	BrowseSingleLine
	// BrowseContainer fields are announced when the cursor enters them
	// ("list with 3 items", "main landmark") and, except landmarks, when it
	// leaves them ("out of list").
	BrowseContainer
	// BrowseMarker fields are announced on entry only (a separator).
	BrowseMarker
)

// BrowseReason is why NVDA reads a field in browse mode.
type BrowseReason int

const (
	// BrowseCaret is moving the reading cursor by line, or say all: the
	// field's role and states come before its content.
	BrowseCaret BrowseReason = iota
	// BrowseQuickNav is a quick navigation key or a focus change: the
	// content comes first.
	BrowseQuickNav
)

// browseElement is f as Announce's element, for the shared NVDA wording.
func browseElement(f BrowseField) element {
	return element{role: f.Role, name: f.Name, value: f.Value, description: f.Description, state: f.AXState}
}

// NVDABrowseCategory is how NVDA presents f in browse mode.
func NVDABrowseCategory(f BrowseField) BrowseCategory {
	r := roleOfNVDA(browseElement(f))
	switch r.kind {
	case "LINK", "HEADING", "BUTTON", "RADIOBUTTON", "CHECKBOX", "SWITCH", "GRAPHIC", "MENUITEM", "TAB",
		"COMBOBOX", "SLIDER", "SPINBUTTON", "PROGRESSBAR", "TOGGLEBUTTON", "MENUBUTTON", "TREEVIEW",
		"CHECKMENUITEM", "RADIOMENUITEM":
		return BrowseSingleLine
	case "EDITABLETEXT":
		if f.Multiline {
			return BrowseContainer
		}
		return BrowseSingleLine
	case "LIST":
		if f.Role == "listbox" { // a list box takes focus; a plain list is read-only
			return BrowseSingleLine
		}
		return BrowseContainer
	case "SEPARATOR", "MATH":
		return BrowseMarker
	case "DIALOG", "APPLICATION":
		if f.Nested {
			return BrowseMarker
		}
		return BrowseLayout
	case "GROUPING", "REGION":
		if f.Name == "" {
			return BrowseLayout
		}
		return BrowseContainer
	case "FORM":
		if f.Name == "" {
			return BrowseLayout
		}
		return BrowseContainer
	case "BLOCKQUOTE", "FIGURE", "TOOLBAR", "MENUBAR", "POPUPMENU", "TABLE", "ARTICLE", "LANDMARK":
		return BrowseContainer
	}
	if f.Editable {
		return BrowseContainer
	}
	return BrowseLayout
}

// NVDABrowseEnter is what NVDA says when the reading cursor enters f in
// browse mode, for reason. A single-line field is read whole, its content
// included; a container or marker says what it is; a layout field says
// nothing.
func NVDABrowseEnter(f BrowseField, reason BrowseReason) []string {
	e := browseElement(f)
	r := roleOfNVDA(e)
	role := roleWordNVDA(e, r)
	states := stateWordsNVDA(e, r, false)
	level := ""
	if f.Level != nil && r.kind == "HEADING" {
		level = "level " + strconv.Itoa(*f.Level)
	}
	switch NVDABrowseCategory(f) {
	case BrowseSingleLine:
		contentFirst := reason == BrowseQuickNav && !f.Editable &&
			r.kind != "EDITABLETEXT" && r.kind != "COMBOBOX" && r.kind != "TREEVIEW" && r.kind != "LIST"
		var out []string
		if contentFirst {
			out = appendNonEmpty(out, f.Content)
		}
		if f.Name != f.Content {
			out = appendNonEmpty(out, f.Name)
		}
		if r.kind == "LINK" { // a link's states come before its role
			out = append(out, states...)
			out = appendNonEmpty(out, role)
		} else {
			out = appendNonEmpty(out, role)
			out = append(out, states...)
		}
		value := f.Value
		if value == f.Name || value == f.Content || slices.Contains(nvdaSilentValueRoles, r.kind) {
			value = ""
		}
		out = appendNonEmpty(out, value, f.Description, level, deref(f.KeyShortcuts))
		if !contentFirst {
			out = appendNonEmpty(out, f.Content)
		}
		return out
	case BrowseContainer:
		if reason == BrowseQuickNav && f.Name != "" && (r.kind == "GROUPING" || r.kind == "PROPERTYPAGE" || r.kind == "LANDMARK" || r.kind == "REGION") {
			return appendNonEmpty(nil, f.Name, role)
		}
		out := appendNonEmpty(nil, f.Name, role)
		out = append(out, states...)
		switch {
		case r.kind == "TABLE" && f.Rows > 0 && f.Columns > 0:
			out = append(out, fmt.Sprintf("with %s and %s", plural(f.Rows, "row"), plural(f.Columns, "column")))
		case r.kind == "LIST" && f.Items > 0:
			out = append(out, "with "+plural(f.Items, "item"))
		}
		return appendNonEmpty(out, f.Description, level)
	case BrowseMarker:
		out := appendNonEmpty(nil, f.Name, role)
		return append(out, states...)
	}
	return nil
}

// NVDABrowseExit is what NVDA says when the reading cursor leaves f in browse
// mode: "out of list", "out of table", …, for a container that isn't a
// landmark, and nothing otherwise.
func NVDABrowseExit(f BrowseField) []string {
	e := browseElement(f)
	r := roleOfNVDA(e)
	if NVDABrowseCategory(f) != BrowseContainer || r.kind == "LANDMARK" {
		return nil
	}
	role := roleWordNVDA(e, r)
	if role == "" {
		return nil
	}
	return []string{"out of " + role}
}

// NVDAShouldPassThrough reports whether NVDA switches from browse mode to
// focus mode by itself when focus lands on fe (browseMode.shouldPassThrough,
// for a focus change): an editable field, and a widget NVDA can't operate in
// browse mode (a combo box, list box, slider, tab list, menu, tree, spin
// button, grid cell, …), a radio button, tab, menu item or option, or
// anything inside a toolbar.
func NVDAShouldPassThrough(fe *FocusedElement) bool {
	if fe == nil || fe.Ignored {
		return false
	}
	e := elementOf(fe)
	r := roleOfNVDA(e)
	editable := r.kind == "EDITABLETEXT"
	if editable && !fe.Disabled {
		return true
	}
	always := []string{"COMBOBOX", "EDITABLETEXT", "LIST", "LISTITEM", "SLIDER", "TABCONTROL", "MENUBAR",
		"POPUPMENU", "TREEVIEW", "TREEVIEWITEM", "SPINBUTTON", "TABLEROW", "TABLECELL", "TABLEROWHEADER", "TABLECOLUMNHEADER"}
	onFocus := []string{"LISTITEM", "RADIOBUTTON", "TAB", "MENUITEM", "RADIOMENUITEM", "CHECKMENUITEM"}
	if fe.ReadOnly && !slices.Contains([]string{"EDITABLETEXT", "COMBOBOX", "TABLEROW", "TABLECELL", "TABLEROWHEADER", "TABLECOLUMNHEADER"}, r.kind) {
		return false
	}
	if r.kind == "LIST" && fe.Role != nil && *fe.Role == "list" {
		return false // a plain list is read-only to NVDA
	}
	if slices.Contains(always, r.kind) || slices.Contains(onFocus, r.kind) {
		return true
	}
	for _, c := range fe.Context {
		if c.Role == "toolbar" {
			return true
		}
	}
	return false
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
