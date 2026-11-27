// Package preset defines agent personas: which actions an agent may take, how
// it may target elements, what page state it perceives each turn, and how that
// state is described to it.
//
// The built-ins (Default, Full, Blind, Motor) are functions that return a
// fresh Preset, so a caller can start from one and change it freely:
//
//	p := preset.Default()
//	p.Name = "no-scripts"
//	p.Actions = slices.DeleteFunc(p.Actions, func(t action.Type) bool {
//	    return t == action.TypeJavascript
//	})
//
// or build one from scratch with any mix of action types. Call Validate before
// using a hand-built Preset.
package preset

import (
	"errors"
	"fmt"
	"slices"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// Preset is an agent persona. A preset shapes what the agent is offered (its
// schema, prompt and perceived state); Check is how a caller enforces it on
// the actions that come back.
type Preset struct {
	// Name labels the preset in logs and prompts.
	Name string
	// Actions are the action types the agent may emit, in schema order.
	Actions []action.Type
	// Targeting says how the agent may pick elements.
	Targeting action.Targeting
	// Perceive says which page state is captured each turn.
	Perceive Perception
	// Describe renders the captured State as the text the agent is given
	// about the page each turn.
	Describe func(State) string
	// Language is what a screen reader's announcement (State.Announcement)
	// is worded in, for a preset that perceives the focused element. The
	// zero value means DefaultLanguage (US English).
	Language Language
	// NVDAModes are the modes an nvdaChangeMode action may set, for a preset
	// that allows it; nil means every mode (action.NVDAModes). When set, it
	// must include action.NVDAModeFocus, the default, so VoiceOver can always
	// be turned back on.
	NVDAModes []action.NVDAMode
}

// Perception selects which page state is captured each turn.
type Perception struct {
	Screenshot bool // a PNG of the viewport
	DOM        bool // the full DOM as HTML
	Focused    bool // a screen-reader view of the focused element
	Tabs       bool // the open tabs, active one flagged
}

// Validate reports every problem with p: a missing name or Describe, no
// actions, unknown or duplicate action types, actions that need a target
// when Targeting allows none, and an unsupported Language.
func Validate(p Preset) error {
	var errs []error
	if p.Name == "" {
		errs = append(errs, errors.New("name is empty"))
	}
	if p.Describe == nil {
		errs = append(errs, errors.New("describe is nil"))
	}
	if _, err := Schema(p); err != nil {
		errs = append(errs, err)
	}
	if _, err := ResolveLanguage(p.Language); err != nil {
		errs = append(errs, err)
	}
	if Allows(p, action.TypeNVDAChangeMode) && !p.Perceive.Focused {
		errs = append(errs, fmt.Errorf("%s needs Perceive.Focused: without it there is no screen reader to switch", action.TypeNVDAChangeMode))
	}
	if p.NVDAModes != nil {
		for _, m := range p.NVDAModes {
			if _, err := action.ResolveNVDAMode(m); err != nil || m == "" {
				errs = append(errs, fmt.Errorf("NVDAModes: unknown mode %q", m))
			}
		}
		if !slices.Contains(p.NVDAModes, action.NVDAModeFocus) {
			errs = append(errs, fmt.Errorf("NVDAModes must include %q, the default", action.NVDAModeFocus))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("preset %q: %w", p.Name, err)
	}
	return nil
}

// Schema returns the JSON schema of one action the agent may emit under p.
// The nvdaChangeMode mode is limited to p.NVDAModes when set.
func Schema(p Preset) (map[string]any, error) {
	s, err := action.Schema(p.Actions, p.Targeting)
	if err != nil || p.NVDAModes == nil || !Allows(p, action.TypeNVDAChangeMode) {
		return s, err
	}
	data, _ := s["properties"].(map[string]any)["data"].(map[string]any)
	if mode, ok := data["properties"].(map[string]any)["mode"].(map[string]any); ok {
		enum := make([]any, len(p.NVDAModes))
		for i, m := range p.NVDAModes {
			enum[i] = string(m)
		}
		mode["enum"] = enum
	}
	return s, nil
}

// AllowedNVDAModes are the modes p lets an nvdaChangeMode action set:
// p.NVDAModes, or every mode when it is nil.
func AllowedNVDAModes(p Preset) []action.NVDAMode {
	if p.NVDAModes == nil {
		return action.NVDAModes()
	}
	return slices.Clone(p.NVDAModes)
}

// Allows reports whether p lets the agent emit action type t.
func Allows(p Preset, t action.Type) bool {
	return slices.Contains(p.Actions, t)
}

// Check reports why a can't be performed under p: a type p doesn't allow, a
// payload that fails action.Validate, or a target kind p's Targeting doesn't
// allow. It returns nil when a is fine to perform.
func Check(p Preset, a action.Action) error {
	if !Allows(p, a.Type) {
		return fmt.Errorf("%s: not allowed by preset %q", a.Type, p.Name)
	}
	if err := action.Validate(a); err != nil {
		return err
	}
	if d, ok := a.Data.(action.NVDAChangeMode); ok {
		m, _ := action.ResolveNVDAMode(d.Mode)
		if !slices.Contains(AllowedNVDAModes(p), m) {
			return fmt.Errorf("%s: mode %q not allowed by preset %q (allowed: %q)", a.Type, m, p.Name, AllowedNVDAModes(p))
		}
	}
	return action.CheckTargeting(a, p.Targeting)
}

// UsesArtifacts reports whether p allows any action that reads or writes the
// artifacts directory, so a run with p needs one configured.
func UsesArtifacts(p Preset) bool {
	return slices.ContainsFunc(p.Actions, action.IsArtifact)
}

// WithArtifacts returns a copy of p that also allows the artifact actions
// (screenshot, createArtifact, readArtifact, listArtifacts) it doesn't
// already allow.
func WithArtifacts(p Preset) Preset {
	p.Actions = slices.Clone(p.Actions)
	for _, t := range action.ArtifactTypes() {
		if !Allows(p, t) {
			p.Actions = append(p.Actions, t)
		}
	}
	return p
}
