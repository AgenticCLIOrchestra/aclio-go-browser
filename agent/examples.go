package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// Example shows the agent a situation and the actions that suit it.
type Example struct {
	Situation string
	Actions   []action.Action
	// Why is optional reasoning shown after the actions.
	Why string
}

func ptr[T any](v T) *T { return &v }

// DefaultExamples returns our examples. Each call returns fresh values.
func DefaultExamples() []Example {
	return []Example{
		{
			Situation: "an animation or a short load is still in progress",
			Actions:   []action.Action{},
			Why:       "each turn takes about half a second anyway, so sending no actions is the right way to wait briefly",
		},
		{
			Situation: "waiting is needed but you don't know for how long",
			Actions: []action.Action{
				{Type: action.TypeWait, Data: action.Wait{Milliseconds: 2000}},
			},
			Why: "two seconds is a sensible first wait; if the page hasn't changed next turn, wait longer",
		},
		{
			Situation: "clicking the login button after filling in the credentials",
			Actions: []action.Action{
				{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("button[type=submit]")}}},
			},
			Why: "a precise selector found in the DOM survives layout changes, unlike coordinates",
		},
		{
			Situation: "moving to a control using only the keyboard",
			Actions: []action.Action{
				{Type: action.TypePress, Data: action.Press{Key: "Tab"}},
			},
			Why: "move focus one step at a time and check the focused element next turn; activate it with Enter (or Space for buttons and checkboxes) only once the right control is focused",
		},
		{
			Situation: "the user asked for a screenshot of the pricing table",
			Actions: []action.Action{
				{Type: action.TypeScreenshot, Data: action.Screenshot{Name: "pricing-table", Target: &action.Target{Selector: ptr("table.pricing")}}},
			},
		},
		{
			Situation: "keeping track of progress over a long task",
			Actions: []action.Action{
				{Type: action.TypeCreateArtifact, Data: action.CreateArtifact{Name: "progress-1.md", Content: "- [x] logged in\n- [ ] export the report"}},
			},
			Why: "artifacts can't be overwritten, so number each new version and read the latest back with readArtifact",
		},
	}
}

// ExamplesFor returns the examples p lets the agent follow: every action in
// them must pass preset.Check. The agent is never shown an example it isn't
// allowed to act on.
func ExamplesFor(p preset.Preset, examples []Example) []Example {
	var out []Example
	for _, e := range examples {
		ok := true
		for _, a := range e.Actions {
			if preset.Check(p, a) != nil {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
		}
	}
	return out
}

// RenderExamples renders examples as prose, one paragraph each, with each
// example's actions as compact JSON showing only the fields in use.
func RenderExamples(examples []Example) string {
	type compactAction struct {
		Type action.Type    `json:"type"`
		Data map[string]any `json:"data"`
	}
	parts := make([]string, 0, len(examples))
	for _, e := range examples {
		compact := make([]compactAction, 0, len(e.Actions))
		for _, a := range e.Actions {
			compact = append(compact, compactAction{Type: a.Type, Data: action.Compact(a.Data)})
		}
		actions, _ := json.Marshal(compact)
		s := fmt.Sprintf("When %s, the actions could be: %s", e.Situation, actions)
		if e.Why != "" {
			s += ". " + upperFirst(e.Why) + "."
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
