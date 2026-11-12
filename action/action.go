// Package action is the vocabulary an agent uses to drive a browser: every
// action it may ask for, as a {type, data} discriminated union, the JSON schema
// for any chosen subset of them, and the validation every caller applies
// before performing one.
//
// The agent never touches the browser itself. It returns Actions as structured
// output, and the caller decides which of them to perform. Any combination of
// the built-in types can be offered to an agent (see the preset package).
//
// Each type has exactly one payload shape: no payload carries a sub-action
// string whose value changes what its other fields mean.
package action

import (
	"encoding/json"
	"fmt"
)

// Type discriminates which payload an Action carries. Values are camelCase,
// like every JSON name in this package.
type Type string

const (
	TypeClick          Type = "click"
	TypeDoubleClick    Type = "doubleClick"
	TypeHover          Type = "hover"
	TypeMouseDown      Type = "mouseDown"
	TypeMouseUp        Type = "mouseUp"
	TypeScroll         Type = "scroll"
	TypeType           Type = "type"
	TypeFill           Type = "fill"
	TypePress          Type = "press"
	TypeWait           Type = "wait"
	TypeGoTo           Type = "goTo"
	TypeTabOpen        Type = "tabOpen"
	TypeTabSwitch      Type = "tabSwitch"
	TypeTabClose       Type = "tabClose"
	TypeJavascript     Type = "javascript"
	TypeScreenshot     Type = "screenshot"
	TypeCreateArtifact Type = "createArtifact"
	TypeReadArtifact   Type = "readArtifact"
	TypeListArtifacts  Type = "listArtifacts"
)

// Data is an Action's payload. It is a closed set: only the payload structs in
// this package implement it.
type Data interface {
	isData()
}

// Action is one thing the agent wants done: Type plus the matching Data
// payload. It serialises as {"type": "...", "data": {...}}.
type Action struct {
	Type Type `json:"type"`
	Data Data `json:"data"`
}

// UnmarshalJSON decodes the {"type", "data"} envelope into the payload struct
// registered for Type. Unknown types are an error. Decoding checks shape only;
// call Validate for the rules a payload must satisfy.
func (a *Action) UnmarshalJSON(b []byte) error {
	var env struct {
		Type Type            `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return err
	}

	e, ok := lookup(env.Type)
	if !ok {
		return fmt.Errorf("unknown action type %q", env.Type)
	}
	d, err := e.decode(env.Data)
	if err != nil {
		return fmt.Errorf("decoding %s data: %w", env.Type, err)
	}

	a.Type = env.Type
	a.Data = d
	return nil
}

// Performed is the outcome of attempting an Action, reported back to the
// agent. Attempted is true once the action was started; Succeeded only if it
// completed. Reason carries the failure message when it didn't. Result carries
// data the action returns to the agent (readArtifact: the artifact's content;
// listArtifacts: the artifact names).
type Performed struct {
	Action
	Attempted bool    `json:"attempted"`
	Succeeded bool    `json:"succeeded"`
	Reason    *string `json:"reason,omitempty"`
	Result    *string `json:"result,omitempty"`
}
