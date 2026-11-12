package action

import (
	"encoding/json"
	"fmt"

	"github.com/agenticcliorchestra/aclio-go/jsonschema"
)

// Targeting says which kinds of Target an agent may use.
type Targeting struct {
	Selector bool // target elements by CSS selector
	Point    bool // target elements by viewport coordinates
}

// Schema returns the JSON schema of one Action restricted to types and
// targeting: a {type, data} object whose type enum lists types (in the given
// order) and whose data is the union of their payload fields, all optional.
// Target kinds that targeting disallows are removed, and target disappears
// entirely when neither kind is allowed. Which data fields belong to which
// type is enforced by Validate and the prompt, not the schema (structured
// outputs have no usable per-variant oneOf).
//
// Unknown or duplicate types are an error, as is a type that needs a target
// when targeting allows none.
func Schema(types []Type, targeting Targeting) (map[string]any, error) {
	if len(types) == 0 {
		return nil, fmt.Errorf("action: no action types given")
	}
	payloads := make([]any, 0, len(types))
	enum := make([]any, 0, len(types))
	seen := map[Type]bool{}
	for _, t := range types {
		if seen[t] {
			return nil, fmt.Errorf("action: duplicate action type %q", t)
		}
		seen[t] = true
		e, ok := lookup(t)
		if !ok {
			return nil, fmt.Errorf("action: unknown action type %q", t)
		}
		if e.target == targetRequired && !targeting.Selector && !targeting.Point {
			return nil, fmt.Errorf("action: %q needs a target but targeting allows neither selector nor point", t)
		}
		payloads = append(payloads, e.payload)
		enum = append(enum, string(t))
	}

	data, err := jsonschema.UnionOf(payloads...)
	if err != nil {
		return nil, err
	}
	restrictTarget(data["properties"].(map[string]any), targeting)

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"type": map[string]any{"type": "string", "enum": enum},
			"data": data,
		},
		"required": []string{"type", "data"},
	}, nil
}

// restrictTarget removes the target kinds targeting disallows from the data
// properties, and target itself when no kind is left.
func restrictTarget(props map[string]any, targeting Targeting) {
	target, ok := props["target"].(map[string]any)
	if !ok {
		return
	}
	if !targeting.Selector && !targeting.Point {
		delete(props, "target")
		return
	}
	tprops, _ := target["properties"].(map[string]any)
	if !targeting.Selector {
		delete(tprops, "selector")
	}
	if !targeting.Point {
		delete(tprops, "point")
	}
}

// Compact returns d as a JSON object with zero-valued top-level fields ("", 0,
// false, null) dropped, so it shows only the fields in use. Handy for showing
// actions to an agent, e.g. in prompt examples.
func Compact(d Data) map[string]any {
	b, _ := json.Marshal(d)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	for k, v := range m {
		switch x := v.(type) {
		case nil:
			delete(m, k)
		case string:
			if x == "" {
				delete(m, k)
			}
		case float64:
			if x == 0 {
				delete(m, k)
			}
		case bool:
			if !x {
				delete(m, k)
			}
		}
	}
	return m
}
