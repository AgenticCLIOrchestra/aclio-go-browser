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
// aria-labelledby / aria-describedby resolve to the referenced text; native
// labelling (<label for>, a wrapping <label>, a fieldset's <legend>) is in
// Label and Legend; for composite widgets (grid, listbox, menu, …) the
// container's content is not reported, and the item aria-activedescendant
// points at is in ActiveDescendant.
//
// The native states are read off the DOM properties, not the attributes, so
// they reflect what the user did (a radio clicked after load is Checked).
// Checked and Selected are set only on elements that have them (a checkbox or
// radio, an <option>), where false means unchecked / unselected; the other
// flags are omitted when false.
type FocusedElement struct {
	Tag              string            `json:"tag"`                        // "button", "input", …
	Role             *string           `json:"role,omitempty"`             // explicit role attribute
	Type             *string           `json:"type,omitempty"`             // input type
	Title            *string           `json:"title,omitempty"`            // title attribute
	Text             *string           `json:"text,omitempty"`             // screen-reader text content
	Value            *string           `json:"value,omitempty"`            // current value (inputs)
	SelectedText     *string           `json:"selectedText,omitempty"`     // text of a <select>'s selected options, ", "-joined
	Placeholder      *string           `json:"placeholder,omitempty"`      // placeholder attribute
	Label            *string           `json:"label,omitempty"`            // text of its <label>s (for= or wrapping)
	Legend           *string           `json:"legend,omitempty"`           // legend of the nearest enclosing <fieldset>
	LabelledBy       *string           `json:"labelledBy,omitempty"`       // text of the aria-labelledby targets
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

// ActiveDescendant is the item a focused composite widget designates via
// aria-activedescendant: what a screen reader announces while DOM focus stays
// on the container. Missing is set when the id resolves to no element. The
// native states follow the same rules as on FocusedElement.
type ActiveDescendant struct {
	Tag           string            `json:"tag,omitempty"`
	Role          *string           `json:"role,omitempty"`
	Text          *string           `json:"text,omitempty"`
	LabelledBy    *string           `json:"labelledBy,omitempty"`
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
