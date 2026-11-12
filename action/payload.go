package action

// Payload structs, one per Type. Their json, enum and description tags drive
// both decoding and the generated schema, so the two can't drift apart.
// Descriptions are prefixed with the action type because the schema merges
// every allowed payload's fields into one object.

// Target picks an element: by CSS Selector or by the viewport Point it sits
// at. Exactly one must be set. Which kinds an agent may use is controlled by
// Targeting.
type Target struct {
	Selector *string `json:"selector,omitempty" description:"CSS selector of the element"`
	Point    *Point  `json:"point,omitempty" description:"viewport coordinates of the element, in CSS pixels"`
}

// Point is a position in the viewport, in CSS pixels.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Click clicks the target once.
type Click struct {
	Target Target `json:"target" description:"click: the element to click (selector or point, not both)"`
}

// DoubleClick double-clicks the target.
type DoubleClick struct {
	Target Target `json:"target" description:"doubleClick: the element to double-click (selector or point, not both)"`
}

// Hover moves the pointer over the target.
type Hover struct {
	Target Target `json:"target" description:"hover: the element to move the pointer over (selector or point, not both)"`
}

// MouseDown presses the primary button, over Target if given, else wherever
// the pointer is.
type MouseDown struct {
	Target *Target `json:"target,omitempty" description:"mouseDown: where to press; omit to press at the current pointer position"`
}

// MouseUp releases the primary button, over Target if given, else wherever the
// pointer is.
type MouseUp struct {
	Target *Target `json:"target,omitempty" description:"mouseUp: where to release; omit to release at the current pointer position"`
}

// Scroll turns the mouse wheel at the current pointer position. Positive
// deltas scroll right and down. At least one delta must be non-zero.
type Scroll struct {
	DeltaX float64 `json:"deltaX,omitempty" description:"scroll: horizontal distance in CSS pixels, positive scrolls right"`
	DeltaY float64 `json:"deltaY,omitempty" description:"scroll: vertical distance in CSS pixels, positive scrolls down"`
}

// Typing is the payload of TypeType (it can't be called Type: that name is
// the action discriminator). It types Text key by key into the focused
// element, focusing Target first if given, appending to whatever the element
// holds; use Fill to replace the value instead.
type Typing struct {
	Text   string  `json:"text" description:"type: the text to type"`
	Target *Target `json:"target,omitempty" description:"type: element to focus before typing; omit to type into the focused element"`
}

// Fill replaces the value of the target input with Value. An empty Value
// clears it.
type Fill struct {
	Target Target `json:"target" description:"fill: the input to fill (selector or point, not both)"`
	Value  string `json:"value" description:"fill: the new value; empty clears the input"`
}

// Press presses one key or chord on the focused element, e.g. "Enter",
// "Tab", "Shift+Tab", "Control+A".
type Press struct {
	Key string `json:"key" description:"press: key or chord to press, e.g. Enter, Tab, Shift+Tab"`
}

// Wait pauses for Milliseconds (at most MaxWait), for pages that settle
// asynchronously.
type Wait struct {
	Milliseconds int `json:"milliseconds" description:"wait: how long to wait, in milliseconds"`
}

// GoTo navigates the active tab to URL: an absolute http(s) URL, a
// root-relative path (/some/path) or a path relative to the current page
// (../some/path).
type GoTo struct {
	URL string `json:"url" description:"goTo: absolute http(s) URL, root-relative path, or path relative to the current page"`
}

// TabOpen opens a new blank tab and makes it active.
type TabOpen struct{}

// TabSwitch makes the tab at the 1-based Index active.
type TabSwitch struct {
	Index int `json:"index" description:"tabSwitch/tabClose: 1-based position of the tab"`
}

// TabClose closes the tab at the 1-based Index (refused if it's the only one).
type TabClose struct {
	Index int `json:"index" description:"tabSwitch/tabClose: 1-based position of the tab"`
}

// Javascript evaluates Code in the active page.
type Javascript struct {
	Code string `json:"code" description:"javascript: script to evaluate in the active page"`
}

// Artifacts are files the agent leaves for the caller, in the artifacts
// directory. They are create-only: no action edits, overwrites, renames or
// deletes one, so whatever the agent produces stays as first written.

// Screenshot saves the artifact {Name}.png: the element at Target if given,
// else the whole page if FullPage, else the visible viewport. FullPage and
// Target are mutually exclusive. An artifact of the same name fails the
// action.
type Screenshot struct {
	Name     string  `json:"name" description:"screenshot/createArtifact/readArtifact: plain file name, no directories; screenshot adds .png"`
	FullPage bool    `json:"fullPage,omitempty" description:"screenshot: capture the whole scrollable page; not with target"`
	Target   *Target `json:"target,omitempty" description:"screenshot: capture only this element; not with fullPage"`
}

// CreateArtifact creates the artifact Name with Content. An artifact of the
// same name fails the action.
type CreateArtifact struct {
	Name    string `json:"name" description:"screenshot/createArtifact/readArtifact: plain file name, no directories; screenshot adds .png"`
	Content string `json:"content" description:"createArtifact: the file's contents"`
}

// ReadArtifact reads the artifact Name. Its content comes back in
// Performed.Result.
type ReadArtifact struct {
	Name string `json:"name" description:"screenshot/createArtifact/readArtifact: plain file name, no directories; screenshot adds .png"`
}

// ListArtifacts lists the names of every artifact so far. They come back in
// Performed.Result.
type ListArtifacts struct{}

func (Click) isData()          {}
func (DoubleClick) isData()    {}
func (Hover) isData()          {}
func (MouseDown) isData()      {}
func (MouseUp) isData()        {}
func (Scroll) isData()         {}
func (Typing) isData()         {}
func (Fill) isData()           {}
func (Press) isData()          {}
func (Wait) isData()           {}
func (GoTo) isData()           {}
func (TabOpen) isData()        {}
func (TabSwitch) isData()      {}
func (TabClose) isData()       {}
func (Javascript) isData()     {}
func (Screenshot) isData()     {}
func (CreateArtifact) isData() {}
func (ReadArtifact) isData()   {}
func (ListArtifacts) isData()  {}
