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
// aria-labelledby / aria-describedby resolve to the referenced text; for
// composite widgets (grid, listbox, menu, …) the container's content is not
// reported, and the item aria-activedescendant points at is in
// ActiveDescendant.
type FocusedElement struct {
	Tag              string            `json:"tag"`                        // "button", "input", …
	Role             *string           `json:"role,omitempty"`             // explicit role attribute
	Type             *string           `json:"type,omitempty"`             // input type
	Title            *string           `json:"title,omitempty"`            // title attribute
	Text             *string           `json:"text,omitempty"`             // screen-reader text content
	Value            *string           `json:"value,omitempty"`            // current value (inputs)
	Placeholder      *string           `json:"placeholder,omitempty"`      // placeholder attribute
	LabelledBy       *string           `json:"labelledBy,omitempty"`       // text of the aria-labelledby targets
	DescribedBy      *string           `json:"describedBy,omitempty"`      // text of the aria-describedby targets
	Aria             map[string]string `json:"aria,omitempty"`             // every aria-* attribute
	ActiveDescendant *ActiveDescendant `json:"activeDescendant,omitempty"` // element aria-activedescendant points at
}

// ActiveDescendant is the item a focused composite widget designates via
// aria-activedescendant: what a screen reader announces while DOM focus stays
// on the container. Missing is set when the id resolves to no element.
type ActiveDescendant struct {
	Tag         string            `json:"tag,omitempty"`
	Role        *string           `json:"role,omitempty"`
	Text        *string           `json:"text,omitempty"`
	LabelledBy  *string           `json:"labelledBy,omitempty"`
	DescribedBy *string           `json:"describedBy,omitempty"`
	Aria        map[string]string `json:"aria,omitempty"`
	Missing     bool              `json:"missing,omitempty"`
}
