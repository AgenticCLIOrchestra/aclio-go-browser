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
// aria-describedby resolves to the referenced text; the enclosing fieldset's
// <legend> is in Legend; for composite widgets (grid, listbox, menu, …) the
// container's content is not reported, and the item aria-activedescendant
// points at is in ActiveDescendant.
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
// The native states are read off the DOM properties, not the attributes, so
// they reflect what the user did (a radio clicked after load is Checked).
// Checked and Selected are set only on elements that have them (a checkbox or
// radio, an <option>), where false means unchecked / unselected; the other
// flags are omitted when false.
type FocusedElement struct {
	Role             *string           `json:"role,omitempty"`             // computed role, from Chromium's accessibility tree
	Name             *string           `json:"name,omitempty"`             // accessible name, from Chromium's accessibility tree; "" when none
	NameSources      []NameSource      `json:"nameSources,omitempty"`      // the sources that had a value, in precedence order
	Text             *string           `json:"text,omitempty"`             // screen-reader text content
	Value            *string           `json:"value,omitempty"`            // current value (inputs)
	SelectedText     *string           `json:"selectedText,omitempty"`     // text of a <select>'s selected options, ", "-joined
	Legend           *string           `json:"legend,omitempty"`           // legend of the nearest enclosing <fieldset>
	DescribedBy      *string           `json:"describedBy,omitempty"`      // text of the aria-describedby targets
	Checked          *bool             `json:"checked,omitempty"`          // checkbox / radio only
	Indeterminate    bool              `json:"indeterminate,omitempty"`    // checkbox in the mixed state
	Selected         *bool             `json:"selected,omitempty"`         // <option> only
	Disabled         bool              `json:"disabled,omitempty"`         // :disabled, incl. via a disabled fieldset
	Required         bool              `json:"required,omitempty"`         // required form control
	ReadOnly         bool              `json:"readOnly,omitempty"`         // read-only input / textarea
	Aria             map[string]string `json:"aria,omitempty"`             // every aria-* attribute
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

// ActiveDescendant is the item a focused composite widget designates via
// aria-activedescendant: what a screen reader announces while DOM focus stays
// on the container. Missing is set when the id resolves to no element. Role,
// Name, NameSources and the native states follow the same rules as on
// FocusedElement.
type ActiveDescendant struct {
	Role          *string           `json:"role,omitempty"`
	Name          *string           `json:"name,omitempty"`
	NameSources   []NameSource      `json:"nameSources,omitempty"`
	Text          *string           `json:"text,omitempty"`
	DescribedBy   *string           `json:"describedBy,omitempty"`
	Checked       *bool             `json:"checked,omitempty"`
	Indeterminate bool              `json:"indeterminate,omitempty"`
	Selected      *bool             `json:"selected,omitempty"`
	Disabled      bool              `json:"disabled,omitempty"`
	Required      bool              `json:"required,omitempty"`
	ReadOnly      bool              `json:"readOnly,omitempty"`
	Aria          map[string]string `json:"aria,omitempty"`
	Missing       bool              `json:"missing,omitempty"`
}
