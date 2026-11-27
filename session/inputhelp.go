package session

import (
	"strings"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// The simulated NVDA's input help mode (NVDA+1, as globalCommands'
// toggleInputHelp and inputCore's _handleInputHelp): every key says what it
// is and what it would do, in the mode NVDA was in, instead of doing it, and
// nothing reaches the page. NVDA+1 turns it off again.

// Descriptions of the simulated NVDA commands, word for word from NVDA's
// source (their script descriptions). The cursor movement commands (arrows,
// Home / End, Page Up / Down, Control+Home / End) have none there, so input
// help says only the key's name for them.
const (
	inputHelpToggleDoc      = "Turns input help on or off. When on, any input such as pressing a key on the keyboard will tell you what script is associated with that input, if any."
	inputHelpPassThroughDoc = "Toggles between browse mode and focus mode. When in focus mode, keys will pass straight through to the application, allowing you to interact directly with a control. When in browse mode, you can navigate the document with the cursor, quick navigation keys, etc."
	inputHelpPassNextKeyDoc = "The next key that is pressed will not be handled at all by NVDA, it will be passed directly through to Windows."
	inputHelpSayAllDoc      = "Reads from the system caret up to the end of the text, moving the caret as it goes"
	inputHelpActivateDoc    = "Activates the current object in the document"
)

// enterInputHelp turns input help on, remembering the mode NVDA was in. NVDA
// says "input help on. Press insert+1 again to turn it off." a is nil when b
// doesn't announce.
func enterInputHelp(a *announcer, st *nvdaState) {
	if modeOf(st) != action.NVDAModeInputHelp {
		st.before = modeOf(st)
	}
	st.mode = action.NVDAModeInputHelp
	if a != nil {
		sayNVDA(a, "input help on. Press insert+1 again to turn it off.")
	}
}

// leaveInputHelp turns input help off, back in the mode NVDA was in. NVDA
// says "input help off".
func leaveInputHelp(a *announcer, st *nvdaState) {
	st.mode = st.before
	if st.mode == action.NVDAModeInputHelp || st.mode == action.NVDAModeOneKeyPass {
		st.mode = action.NVDAModeFocus
	}
	st.before = ""
	if a != nil {
		sayNVDA(a, "input help off")
	}
}

// inputHelpKey is input help for key k: NVDA says the key's name, the
// character it would type when that's different, and the description of the
// command bound to it in the mode NVDA was in, if any. Nothing else happens,
// except for NVDA+1, which turns input help off.
func inputHelpKey(a *announcer, st *nvdaState, k key) string {
	name := keyDisplayName(k)
	sayNVDA(a, name)
	if k.nvda && k.base == "1" {
		leaveInputHelp(a, st)
		return "NVDA turned input help off."
	}
	if c := inputHelpCharacter(k); c != "" && c != name {
		sayNVDA(a, c)
	}
	if doc := commandDoc(k, st.before); doc != "" {
		sayNVDA(a, doc)
	}
	return "NVDA input help described this key instead of performing it: nothing happened, and it didn't reach the page."
}

// commandDoc is the description of the simulated NVDA command bound to k in
// mode (the mode input help was turned on from), or "" when k has none: no
// command, or a command NVDA's source gives no description.
func commandDoc(k key, mode action.NVDAMode) string {
	switch {
	case k.nvda && k.base == "1":
		return inputHelpToggleDoc
	case k.nvda && k.base == "Space":
		return inputHelpPassThroughDoc
	case k.nvda && k.base == "F2":
		return inputHelpPassNextKeyDoc
	case k.nvda && k.base == "ArrowDown" && !k.ctrl && !k.alt:
		return inputHelpSayAllDoc
	case mode != action.NVDAModeBrowse || k.nvda || k.ctrl || k.alt || k.meta:
		return ""
	case k.base == "Enter" || k.base == "Space":
		return inputHelpActivateDoc
	}
	if name := strings.ToLower(k.base); quickNavKeys[name].match != nil && k.isChar() {
		return quickNavDoc(name, !k.shift)
	}
	return ""
}

// keyLabels are NVDA's spoken names of keys (keyLabels.localizedKeyLabels),
// by Playwright's key name.
var keyLabels = map[string]string{
	"ArrowDown": "down arrow", "ArrowUp": "up arrow", "ArrowLeft": "left arrow", "ArrowRight": "right arrow",
	"PageUp": "page up", "PageDown": "page down", "Home": "home", "End": "end", "Enter": "enter",
	"Escape": "escape", "Space": "space", "Tab": "tab", "Backspace": "backspace", "Delete": "delete",
	"Insert": "insert", "CapsLock": "caps lock", "NumLock": "num lock", "ScrollLock": "scroll lock",
	"PrintScreen": "print screen", "ContextMenu": "applications", "Pause": "break",
}

// keyDisplayName is k's name as NVDA's input help says it: the modifiers,
// then the key, joined with "+" ("insert+f2", "ctrl+home", "shift+h",
// "down arrow"). NVDA's modifier is the Insert key.
func keyDisplayName(k key) string {
	var parts []string
	if k.nvda {
		parts = append(parts, "insert")
	}
	if k.ctrl {
		parts = append(parts, "ctrl")
	}
	if k.alt {
		parts = append(parts, "alt")
	}
	if k.shift {
		parts = append(parts, "shift")
	}
	if k.meta {
		parts = append(parts, "windows")
	}
	base, ok := keyLabels[k.base]
	if !ok {
		base = strings.ToLower(k.base)
	}
	return strings.Join(append(parts, base), "+")
}

// inputHelpCharacter is the character k would type, or "" for a key that
// types none.
func inputHelpCharacter(k key) string {
	if !k.isChar() {
		return ""
	}
	if k.shift {
		return strings.ToUpper(k.base)
	}
	return k.base
}
