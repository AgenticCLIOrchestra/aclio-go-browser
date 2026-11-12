package preset

import (
	"slices"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// Each built-in returns a fresh Preset, so callers can modify the result
// without affecting anyone else's copy.
//
// Only Full allows the artifact actions (screenshot, createArtifact,
// readArtifact, listArtifacts), since they need an artifacts directory. Use
// WithArtifacts to add them to any other preset.

// Default is the most capable persona that stays inside the browser: every
// action except the artifact ones, including javascript, with selector and
// point targeting, and a screenshot + DOM each turn.
func Default() Preset {
	return Preset{
		Name:      "default",
		Actions:   slices.DeleteFunc(action.All(), action.IsArtifact),
		Targeting: action.Targeting{Selector: true, Point: true},
		Perceive:  Perception{Screenshot: true, DOM: true, Tabs: true},
		Describe:  Combine(DescribeFiles, DescribeTabs),
	}
}

// Full is Default plus the artifact actions: everything.
func Full() Preset {
	p := WithArtifacts(Default())
	p.Name = "full"
	return p
}

// Blind is the screen-reader persona: keyboard only, no screenshot or DOM, no
// targeting. It perceives the page through the focused element.
func Blind() Preset {
	return Preset{
		Name:     "blind",
		Actions:  keyboardOnly(),
		Perceive: Perception{Focused: true, Tabs: true},
		Describe: Combine(DescribeFocused, DescribeTabs),
	}
}

// Motor is the sighted keyboard-only persona: it sees the screenshot + DOM but
// has no mouse and no targeting, so it must reach elements by keyboard.
func Motor() Preset {
	return Preset{
		Name:     "motor",
		Actions:  append(keyboardOnly(), action.TypeGoTo),
		Perceive: Perception{Screenshot: true, DOM: true, Tabs: true},
		Describe: Combine(DescribeFiles, DescribeTabs),
	}
}

// Builtins returns fresh copies of every built-in preset.
func Builtins() []Preset {
	return []Preset{Default(), Full(), Blind(), Motor()}
}

// keyboardOnly is the action set of the keyboard-only personas.
func keyboardOnly() []action.Type {
	return []action.Type{
		action.TypeType, action.TypePress, action.TypeWait,
		action.TypeTabOpen, action.TypeTabSwitch, action.TypeTabClose,
	}
}
