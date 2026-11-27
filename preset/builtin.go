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
// action except the artifact ones and the screen-reader one (it hears no
// screen reader), including javascript, with selector and point targeting,
// and a screenshot + DOM each turn.
func Default() Preset {
	return Preset{
		Name:      "default",
		Actions:   slices.DeleteFunc(action.All(), func(t action.Type) bool { return action.IsArtifact(t) || action.IsScreenReader(t) }),
		Targeting: action.Targeting{Selector: true, Point: true},
		Perceive:  Perception{Screenshot: true, DOM: true, Tabs: true},
		Describe:  Combine(DescribeFiles, DescribeTabs),
	}
}

// Full is Default plus the artifact actions: everything but the
// screen-reader action, since it hears no screen reader.
func Full() Preset {
	p := WithArtifacts(Default())
	p.Name = "full"
	return p
}

// Blind is the screen-reader persona: keyboard only, no screenshot or DOM, no
// targeting. It perceives the page the way a screen reader user does: it is
// told only what two screen readers say, VoiceOver and NVDA
// (DescribeAnnouncement), in US English unless Language says otherwise, plus
// the open tabs.
func Blind() Preset {
	return Preset{
		Name:     "blind",
		Actions:  append(keyboardOnly(), action.TypeNVDAChangeMode),
		Perceive: Perception{Focused: true, Tabs: true},
		// Not Sleep or Speech: both silence NVDA (sleep mode, or a speech
		// mode other than talk), and with VoiceOver off in any mode but
		// Focus, a blind user would be left with no speech at all.
		NVDAModes: blindNVDAModes(),
		Describe:  Combine(DescribeAnnouncement, DescribeTabs),
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

// blindNVDAModes are the NVDA modes a blind user can work in: every mode but
// Sleep and Speech.
func blindNVDAModes() []action.NVDAMode {
	return slices.DeleteFunc(action.NVDAModes(), func(m action.NVDAMode) bool {
		return m == action.NVDAModeSleep || m == action.NVDAModeSpeech
	})
}

// keyboardOnly is the action set of the keyboard-only personas.
func keyboardOnly() []action.Type {
	return []action.Type{
		action.TypeType, action.TypePress, action.TypeWait,
		action.TypeTabOpen, action.TypeTabSwitch, action.TypeTabClose,
	}
}
