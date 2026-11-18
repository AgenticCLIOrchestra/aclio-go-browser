package preset

import (
	"fmt"
	"slices"
	"strconv"
)

// Language is the language and spelling a screen reader's announcement is
// worded in. The zero value means DefaultLanguage.
type Language string

const (
	LanguageEnUS Language = "en-US" // US English: "checkbox", "checked"
	LanguageEnGB Language = "en-GB" // British English: "tick box", "ticked"

	DefaultLanguage = LanguageEnUS
)

// Languages returns every supported Language.
func Languages() []Language {
	return []Language{LanguageEnUS, LanguageEnGB}
}

// ResolveLanguage returns l, DefaultLanguage for the zero value, or an error
// for a language that isn't supported.
func ResolveLanguage(l Language) (Language, error) {
	if l == "" {
		return DefaultLanguage, nil
	}
	if slices.Contains(Languages(), l) {
		return l, nil
	}
	return "", fmt.Errorf("language %q is not supported (supported: %v)", l, Languages())
}

// wording is what a screen reader says, in one language. Modelled on
// VoiceOver's vocabulary ("edit text", "pop-up button", "dimmed").
type wording struct {
	roles     map[string]string // ARIA role -> spoken role; "" or missing: not spoken
	checked   string            // a ticked checkbox
	unchecked string            // an unticked one
}

// roleWords are the spoken roles both English variants share. A role that
// isn't here, or maps to "", isn't spoken (generic, paragraph, listitem, …).
var roleWords = map[string]string{
	"alert": "alert", "alertdialog": "alert dialog", "application": "application",
	"article": "article", "banner": "banner", "button": "button",
	"cell": "", "columnheader": "column header", "combobox": "combo box",
	"complementary": "complementary", "contentinfo": "content information",
	"definition": "definition", "dialog": "dialog", "document": "document",
	"feed": "feed", "figure": "figure", "form": "form", "grid": "grid",
	"gridcell": "", "group": "group", "heading": "heading", "image": "image",
	"img": "image", "link": "link", "list": "list", "listbox": "list box",
	"listitem": "", "log": "log", "main": "main", "marquee": "marquee",
	"math": "math", "menu": "menu", "menubar": "menu bar", "menuitem": "menu item",
	"menuitemcheckbox": "menu item", "menuitemradio": "menu item",
	"meter": "level indicator", "navigation": "navigation", "note": "note",
	"option": "", "progressbar": "progress indicator", "radio": "radio button",
	"radiogroup": "radio group", "region": "region", "row": "", "rowheader": "row header",
	"scrollbar": "scroll bar", "search": "search", "searchbox": "search text field",
	"separator": "separator", "slider": "slider", "spinbutton": "stepper",
	"status": "status", "switch": "switch", "tab": "tab", "table": "table",
	"tablist": "tab group", "tabpanel": "tab panel", "term": "term",
	"textbox": "edit text", "timer": "timer", "toolbar": "toolbar",
	"tooltip": "help tag", "tree": "outline", "treegrid": "outline",
	"treeitem": "tree item",
}

// wordings is the wording of each Language.
var wordings = map[Language]wording{
	LanguageEnUS: {roles: withRole(roleWords, "checkbox", "checkbox"), checked: "checked", unchecked: "unchecked"},
	LanguageEnGB: {roles: withRole(roleWords, "checkbox", "tick box"), checked: "ticked", unchecked: "unticked"},
}

func withRole(m map[string]string, role, word string) map[string]string {
	out := make(map[string]string, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	out[role] = word
	return out
}

// setRoles are the roles a screen reader gives a position in ("2 of 5").
var setRoles = map[string]bool{
	"radio": true, "option": true, "tab": true, "menuitem": true,
	"menuitemcheckbox": true, "menuitemradio": true, "treeitem": true,
	"listitem": true,
}

// Announce returns what a screen reader says when cur is focused (or
// changes) right after prev was (nil when nothing was), in lang (the zero
// value meaning DefaultLanguage), one spoken item per string, in the order
// they are spoken. session.StartAnnouncing calls it for every focus change
// and every change to the focused element as it happens, so a batch of
// actions is heard step by step.
//
// Like a real screen reader, it only speaks about what happened:
//   - Focus moved to another element (or nothing was focused before): the
//     context focus has entered (landmarks, groups, … not shared with prev's
//     context), then the element (name, value, states, role, position,
//     description), then its active descendant if it has one.
//   - Focus stayed on the same element: only what changed (its name, value
//     or states, e.g. "checked" after Space on a checkbox), and its active
//     descendant if that moved (in full) or changed (only the change).
//   - Nothing changed, nothing is focused, or the element is ignored:
//     nothing (an empty slice).
//
// The wording is modelled on VoiceOver. The order within an element is name,
// value, states, role, position, description.
func Announce(cur, prev *FocusedElement, lang Language) []string {
	w, ok := wordings[lang]
	if !ok {
		w = wordings[DefaultLanguage]
	}
	if cur == nil || cur.Ignored {
		return []string{}
	}
	out := []string{}
	if prev == nil || cur.NodeID == 0 || cur.NodeID != prev.NodeID {
		var prevCtx []ContextItem
		if prev != nil {
			prevCtx = prev.Context
		}
		for _, c := range enteredContext(cur.Context, prevCtx) {
			if word := w.roles[c.Role]; word != "" {
				out = appendNonEmpty(out, c.Name, word)
			}
		}
		out = append(out, announceElement(elementOf(cur), w)...)
		if ad := cur.ActiveDescendant; ad != nil {
			out = append(out, announceElement(descendantOf(ad), w)...)
		}
		return out
	}

	now, before := elementOf(cur), elementOf(prev)
	if now.name != before.name {
		out = appendNonEmpty(out, now.name)
	}
	out = append(out, changedItems(now, before, w)...)
	switch ad, pad := cur.ActiveDescendant, prev.ActiveDescendant; {
	case ad == nil:
	case pad == nil || ad.NodeID == 0 || ad.NodeID != pad.NodeID:
		out = append(out, announceElement(descendantOf(ad), w)...)
	default:
		a, b := descendantOf(ad), descendantOf(pad)
		if a.name != b.name {
			out = appendNonEmpty(out, a.name)
		}
		out = append(out, changedItems(a, b, w)...)
	}
	return out
}

// element is what Announce needs of a FocusedElement or an ActiveDescendant.
type element struct {
	ignored     bool
	role        string
	name        string
	value       string
	description string
	state       AXState
	posInSet    int
	setSize     int
}

func elementOf(fe *FocusedElement) element {
	return element{
		ignored: fe.Ignored, role: deref(fe.Role), name: deref(fe.Name),
		value: spokenValue(fe.Value, fe.ValueText, fe.Aria), description: deref(fe.Description),
		state: fe.AXState, posInSet: derefInt(fe.PosInSet), setSize: derefInt(fe.SetSize),
	}
}

func descendantOf(ad *ActiveDescendant) element {
	if ad.Missing {
		return element{ignored: true}
	}
	return element{
		ignored: ad.Ignored, role: deref(ad.Role), name: deref(ad.Name),
		value: spokenValue(ad.Value, ad.ValueText, ad.Aria), description: deref(ad.Description),
		state: ad.AXState, posInSet: derefInt(ad.PosInSet), setSize: derefInt(ad.SetSize),
	}
}

// spokenValue is the value a screen reader reads: the human-readable value
// text when there is one (Chromium's, else aria-valuetext), else the value.
func spokenValue(value, valueText *string, aria map[string]string) string {
	if t := deref(valueText); t != "" {
		return t
	}
	if t := aria["aria-valuetext"]; t != "" {
		return t
	}
	return deref(value)
}

// announceElement is e announced in full: name, value, states, role,
// position, description. An ignored element says nothing.
func announceElement(e element, w wording) []string {
	if e.ignored {
		return nil
	}
	out := appendNonEmpty(nil, e.name)
	if e.value != e.name {
		out = appendNonEmpty(out, e.value)
	}
	out = append(out, stateWords(e, w)...)
	out = appendNonEmpty(out, roleWord(e, w))
	if setRoles[e.role] && e.posInSet > 0 && e.setSize > 0 {
		out = append(out, strconv.Itoa(e.posInSet)+" of "+strconv.Itoa(e.setSize))
	}
	return appendNonEmpty(out, e.description)
}

// changedItems is what a screen reader says when e is still focused but has
// changed since before: its new value, then the state words it didn't have.
func changedItems(e, before element, w wording) []string {
	if e.ignored {
		return nil
	}
	var out []string
	if e.value != before.value {
		out = appendNonEmpty(out, e.value)
	}
	was := stateWords(before, w)
	for _, s := range stateWords(e, w) {
		if !slices.Contains(was, s) {
			out = append(out, s)
		}
	}
	return out
}

// stateWords are e's states as spoken, in a fixed order.
func stateWords(e element, w wording) []string {
	var out []string
	st := e.state
	if st.Checked != nil {
		switch c := *st.Checked; {
		case e.role == "radio" || e.role == "menuitemradio":
			if c == "true" {
				out = append(out, "selected")
			}
		case e.role == "switch":
			out = append(out, map[string]string{"true": "on", "false": "off", "mixed": "mixed"}[c])
		default:
			out = append(out, map[string]string{"true": w.checked, "false": w.unchecked, "mixed": "mixed"}[c])
		}
	}
	if st.Pressed != nil {
		switch *st.Pressed {
		case "true":
			out = append(out, "selected")
		case "mixed":
			out = append(out, "mixed")
		}
	}
	if st.Selected != nil && *st.Selected && !slices.Contains(out, "selected") {
		out = append(out, "selected")
	}
	if st.Expanded != nil {
		if *st.Expanded {
			out = append(out, "expanded")
		} else {
			out = append(out, "collapsed")
		}
	}
	if st.Required {
		out = append(out, "required")
	}
	if st.Invalid != nil {
		out = append(out, "invalid data")
	}
	if st.Disabled {
		out = append(out, "dimmed")
	}
	return out
}

// roleWord is e's role as spoken: its aria-roledescription if it has one;
// "heading level N"; "pop-up button" for a button that opens a popup and for
// a combobox that isn't typed into (a native <select>); otherwise the
// wording's word for the role.
func roleWord(e element, w wording) string {
	if d := deref(e.state.RoleDescription); d != "" {
		return d
	}
	switch e.role {
	case "heading":
		if e.state.Level != nil {
			return "heading level " + strconv.Itoa(*e.state.Level)
		}
	case "button":
		if e.state.HasPopup != nil {
			return "pop-up button"
		}
	case "combobox":
		if e.state.Autocomplete == nil {
			return "pop-up button"
		}
	}
	return w.roles[e.role]
}

// enteredContext is the part of cur not shared with prev, counted from the
// outermost: the contexts focus has just moved into.
func enteredContext(cur, prev []ContextItem) []ContextItem {
	i := 0
	for i < len(cur) && i < len(prev) && cur[i] == prev[i] {
		i++
	}
	return cur[i:]
}

func appendNonEmpty(out []string, items ...string) []string {
	for _, s := range items {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
