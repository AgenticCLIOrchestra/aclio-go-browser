package preset

// State is the page state captured for one turn. Only the parts the preset's
// Perception asks for are filled; the rest stay zero.
type State struct {
	ScreenshotPath string          // PNG of the viewport, if Perceive.Screenshot
	DOMPath        string          // full DOM as HTML, if Perceive.DOM
	Focused        *FocusedElement // nil when nothing meaningful is focused
	Tabs           []Tab           // in browser order, if Perceive.Tabs
}

// Tab summarises one open tab.
type Tab struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

// FocusedElement is a screen-reader view of the focused element, read off the
// DOM node. Optional fields are pointers so a present-but-empty value ("")
// stays distinct from an absent one (omitted).
//
// It reports what a screen reader would announce rather than raw DOM content:
// Text skips aria-hidden / hidden subtrees and uses descendants' aria-label;
// for composite widgets (grid, listbox, menu, …) the container's content is not reported, and the
// item aria-activedescendant points at is in ActiveDescendant. A fieldset's
// <legend> shows up in Context, as the group it names.
//
// Role is the computed role a screen reader is given, read from Chromium's
// accessibility tree: the explicit role attribute when valid, otherwise the
// element's implicit one (a <button> is "button", a radio input "radio", a
// plain <div> "generic"). It is omitted when the element is left out of that
// tree.
//
// Name is the accessible name a screen reader announces, as Chromium computes
// it: one name, from the first source that has a value in the spec's order
// (aria-labelledby, aria-label, a <label>, contents, title, placeholder, …).
// "" means the element has no accessible name. NameSources lists every source
// that had a value, in that order, so it shows where the name came from and
// what it outranked. Both are omitted when the element is left out of the
// accessibility tree.
//
// Value is the value a screen reader announces, from Chromium's accessibility
// tree: the text typed into a field (masked for a password), a drop-down
// <select>'s selected text, a slider's or spinbutton's number (written out,
// e.g. "7"). It is omitted when the element has none: an empty field, a
// checkbox, a radio, a button, a link. A list-style <select> (multiple, or
// size > 1) has no value; its selected options are in SelectedText.
//
// Description is the accessible description a screen reader reads after the
// rest (often after a pause), as Chromium computes it: the text of the
// aria-describedby targets (even hidden ones), else aria-description, else a
// title the name didn't already use. It is omitted when there is none.
//
// The states and properties in AXState (checked, expanded, disabled, level,
// …) come from Chromium's accessibility tree too, so native and ARIA ones are
// the same (a native indeterminate checkbox and aria-checked="mixed" are both
// Checked "mixed") and they reflect what the user did.
//
// Context is the focused element's ancestors in Chromium's accessibility tree,
// outermost first: the landmarks, groups, lists, tables, dialogs, … a screen
// reader announces as context when focus moves into them (e.g. main, form
// "Prefs", group "Sort by" for a radio in a fieldset). Plain wrappers
// (generic), nodes the tree ignores and the document itself are left out. A
// screen reader announces only the ones focus has just entered; Context has
// them all, every turn.
//
// Aria holds only the aria-* attributes that none of the fields above cover
// (e.g. aria-current, aria-valuetext, aria-setsize / aria-posinset,
// aria-controls), since Chromium's accessibility tree doesn't report them.
type FocusedElement struct {
	Role             *string           `json:"role,omitempty"`         // computed role, from Chromium's accessibility tree
	Name             *string           `json:"name,omitempty"`         // accessible name, from Chromium's accessibility tree; "" when none
	NameSources      []NameSource      `json:"nameSources,omitempty"`  // the sources that had a value, in precedence order
	Text             *string           `json:"text,omitempty"`         // screen-reader text content
	Value            *string           `json:"value,omitempty"`        // value, from Chromium's accessibility tree
	SelectedText     *string           `json:"selectedText,omitempty"` // a list-style <select>'s selected options, ", "-joined
	Description      *string           `json:"description,omitempty"`  // accessible description, from Chromium's accessibility tree
	Context          []ContextItem     `json:"context,omitempty"`      // ancestors a screen reader may announce, outermost first
	AXState                            // states and properties, from Chromium's accessibility tree
	Aria             map[string]string `json:"aria,omitempty"`             // aria-* attributes no other field covers
	ActiveDescendant *ActiveDescendant `json:"activeDescendant,omitempty"` // element aria-activedescendant points at
}

// NameSource is one source of an element's accessible name that had a value,
// as Chromium's accessibility tree reports it. The first one that isn't
// Superseded is the name; a Superseded one was there but outranked.
type NameSource struct {
	// From is where the value came from: an attribute ("aria-labelledby",
	// "aria-label", "title", "placeholder", "alt", …), a native source
	// ("labelfor" for <label for>, "labelwrapped" for a wrapping <label>,
	// "legend", "figcaption", …) or "contents" (the element's own content).
	From       string `json:"from"`
	Value      string `json:"value"`
	Superseded bool   `json:"superseded,omitempty"`
}

// AXState is an element's states and properties as Chromium's accessibility
// tree reports them: what a screen reader is told about it besides its role,
// name and value. Embedded in FocusedElement and ActiveDescendant, its fields
// appear at their top level in JSON. A field is set only when Chromium reports
// it: pointers for the ones where false or 0 is worth saying (an unchecked
// checkbox, a collapsed menu, a minimum of 0), plain bools omitted when false.
// The one exception: Required is also set from the DOM (the required
// attribute or aria-required="true") when the tree leaves it out, as it does
// for a required checkbox.
//
// Ignored means Chromium leaves the element out of the accessibility tree,
// so a screen reader announces nothing for it; IgnoredReasons says why
// (Chromium's reason names, e.g. "ariaHiddenElement", "ariaHiddenSubtree",
// "notRendered"). An ignored element reports only these two fields. Chromium
// never ignores the element that has focus, even inside aria-hidden (it
// exposes it so a screen reader can announce it), so in practice this is set
// only on an ActiveDescendant.
type AXState struct {
	Checked         *string  `json:"checked,omitempty"`         // "true", "false" or "mixed": checkboxes, radios, switches, …
	Pressed         *string  `json:"pressed,omitempty"`         // "true", "false" or "mixed": toggle buttons
	Selected        *bool    `json:"selected,omitempty"`        // options, tabs, rows, …
	Expanded        *bool    `json:"expanded,omitempty"`        // comboboxes, menu buttons, disclosures, tree items, …
	Disabled        bool     `json:"disabled,omitempty"`        // native or aria-disabled, incl. via a disabled fieldset
	ReadOnly        bool     `json:"readOnly,omitempty"`        // read-only field
	Required        bool     `json:"required,omitempty"`        // required field (also from the DOM, see above)
	Invalid         *string  `json:"invalid,omitempty"`         // "true", "grammar" or "spelling"; omitted when valid
	Level           *int     `json:"level,omitempty"`           // heading level; list / tree nesting depth
	HasPopup        *string  `json:"hasPopup,omitempty"`        // "menu", "listbox", "tree", "grid", "dialog" or "true"
	Multiselectable bool     `json:"multiselectable,omitempty"` // listboxes, grids, … that allow several selected
	Orientation     *string  `json:"orientation,omitempty"`     // "horizontal" or "vertical"
	ValueMin        *float64 `json:"valueMin,omitempty"`        // range widgets: sliders, spinbuttons, progress bars, …
	ValueMax        *float64 `json:"valueMax,omitempty"`        // range widgets
	ValueText       *string  `json:"valueText,omitempty"`       // range widgets' human-readable value, when Chromium has one
	Autocomplete    *string  `json:"autocomplete,omitempty"`    // "list", "inline" or "both"
	KeyShortcuts    *string  `json:"keyShortcuts,omitempty"`    // aria-keyshortcuts, e.g. "Alt+S"
	RoleDescription *string  `json:"roleDescription,omitempty"` // aria-roledescription, spoken instead of the role
	Modal           bool     `json:"modal,omitempty"`           // modal dialogs
	Ignored         bool     `json:"ignored,omitempty"`         // left out of the tree: a screen reader gets nothing for it
	IgnoredReasons  []string `json:"ignoredReasons,omitempty"`  // why, e.g. "ariaHiddenElement", "notRendered"
}

// ContextItem is one ancestor in FocusedElement.Context: its role and, when it
// has one, its accessible name, both from Chromium's accessibility tree.
type ContextItem struct {
	Role string `json:"role"`
	Name string `json:"name,omitempty"`
}

// ActiveDescendant is the item a focused composite widget designates via
// aria-activedescendant: what a screen reader announces while DOM focus stays
// on the container. Missing is set when the id resolves to no element. Role,
// Name, NameSources, Value, Description, AXState and Aria follow the same rules as on
// FocusedElement.
type ActiveDescendant struct {
	Role        *string      `json:"role,omitempty"`
	Name        *string      `json:"name,omitempty"`
	NameSources []NameSource `json:"nameSources,omitempty"`
	Value       *string      `json:"value,omitempty"`
	Text        *string      `json:"text,omitempty"`
	Description *string      `json:"description,omitempty"`
	AXState
	Aria    map[string]string `json:"aria,omitempty"`
	Missing bool              `json:"missing,omitempty"`
}
