package action

import (
	"encoding/json"
	"slices"
)

// targetUse says whether a payload takes a Target.
type targetUse int

const (
	targetNone targetUse = iota
	targetOptional
	targetRequired
)

// entry is everything known about one built-in Type.
type entry struct {
	typ      Type
	payload  Data
	decode   func(json.RawMessage) (Data, error)
	target   targetUse
	artifact bool // reads or writes the artifacts directory
}

// registry is every built-in action, in the order All returns them.
var registry = []entry{
	{TypeClick, Click{}, decodeAs[Click], targetRequired, false},
	{TypeDoubleClick, DoubleClick{}, decodeAs[DoubleClick], targetRequired, false},
	{TypeHover, Hover{}, decodeAs[Hover], targetRequired, false},
	{TypeMouseDown, MouseDown{}, decodeAs[MouseDown], targetOptional, false},
	{TypeMouseUp, MouseUp{}, decodeAs[MouseUp], targetOptional, false},
	{TypeScroll, Scroll{}, decodeAs[Scroll], targetNone, false},
	{TypeType, Typing{}, decodeAs[Typing], targetOptional, false},
	{TypeFill, Fill{}, decodeAs[Fill], targetRequired, false},
	{TypePress, Press{}, decodeAs[Press], targetNone, false},
	{TypeWait, Wait{}, decodeAs[Wait], targetNone, false},
	{TypeGoTo, GoTo{}, decodeAs[GoTo], targetNone, false},
	{TypeTabOpen, TabOpen{}, decodeAs[TabOpen], targetNone, false},
	{TypeTabSwitch, TabSwitch{}, decodeAs[TabSwitch], targetNone, false},
	{TypeTabClose, TabClose{}, decodeAs[TabClose], targetNone, false},
	{TypeJavascript, Javascript{}, decodeAs[Javascript], targetNone, false},
	{TypeScreenshot, Screenshot{}, decodeAs[Screenshot], targetOptional, true},
	{TypeCreateArtifact, CreateArtifact{}, decodeAs[CreateArtifact], targetNone, true},
	{TypeReadArtifact, ReadArtifact{}, decodeAs[ReadArtifact], targetNone, true},
	{TypeListArtifacts, ListArtifacts{}, decodeAs[ListArtifacts], targetNone, true},
	{TypeNVDAChangeMode, NVDAChangeMode{}, decodeAs[NVDAChangeMode], targetNone, false},
}

func decodeAs[T Data](raw json.RawMessage) (Data, error) {
	var d T
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	return d, nil
}

func lookup(t Type) (entry, bool) {
	i := slices.IndexFunc(registry, func(e entry) bool { return e.typ == t })
	if i < 0 {
		return entry{}, false
	}
	return registry[i], true
}

func typesWhere(keep func(entry) bool) []Type {
	var out []Type
	for _, e := range registry {
		if keep(e) {
			out = append(out, e.typ)
		}
	}
	return out
}

// All returns every built-in Type, in a stable order. Each call returns a new
// slice.
func All() []Type {
	return typesWhere(func(entry) bool { return true })
}

// ArtifactTypes returns the types that use the artifacts directory
// (screenshot, createArtifact, readArtifact, listArtifacts). A run that allows
// any of them needs an artifacts directory. Each call returns a new slice.
func ArtifactTypes() []Type {
	return typesWhere(func(e entry) bool { return e.artifact })
}

// IsScreenReader reports whether t acts on the simulated screen readers
// (nvdaChangeMode) rather than the page. It only makes sense to an agent that
// hears them.
func IsScreenReader(t Type) bool {
	return t == TypeNVDAChangeMode
}

// Known reports whether t is a built-in Type.
func Known(t Type) bool {
	_, ok := lookup(t)
	return ok
}

// IsArtifact reports whether t reads or writes the artifacts directory.
func IsArtifact(t Type) bool {
	e, _ := lookup(t)
	return e.artifact
}

// RequiresTarget reports whether t can't be performed without a Target, so an
// agent offered t must be allowed some kind of Targeting.
func RequiresTarget(t Type) bool {
	e, _ := lookup(t)
	return e.target == targetRequired
}

// Payload returns the zero value of t's payload struct, and false if t is not
// a built-in Type.
func Payload(t Type) (Data, bool) {
	e, ok := lookup(t)
	return e.payload, ok
}
