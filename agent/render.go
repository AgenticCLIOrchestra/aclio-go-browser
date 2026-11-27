package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/agenticcliorchestra/aclio-go/prompt"
)

// renderInitial renders req's template and returns it with the response
// schema the agent must answer with.
func renderInitial(req InitialRequest) (text, schema string, err error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return "", "", errors.New("agent: Prompt is required (DefaultInitialPrompt is ours)")
	}
	if !strings.Contains(req.Prompt, PlaceholderTask) {
		return "", "", fmt.Errorf("agent: the initial prompt must contain %s, or the agent never sees the task", PlaceholderTask)
	}
	if strings.TrimSpace(req.Task) == "" {
		return "", "", errors.New("agent: Task is required")
	}
	pageState, schema, err := prepare(req.Preset, req.State, req.Request.Dir)
	if err != nil {
		return "", "", err
	}

	examples := req.Examples
	if examples == nil {
		examples = DefaultExamples()
	}

	text, err = render(req.Prompt, map[string]prompt.Param{
		"task":            prompt.Text(req.Task),
		"page_state":      prompt.Text(pageState),
		"response_schema": prompt.JSON(json.RawMessage(schema)),
		"examples":        prompt.Text(RenderExamples(ExamplesFor(req.Preset, examples))),
		"artifacts":       prompt.Text(artifactsGuide(req.Preset)),
		"screen_reader":   prompt.Text(screenReaderText(req.Preset)),
	})
	return text, schema, err
}

// renderSubsequent renders req's template and returns it with the response
// schema the agent must answer with.
func renderSubsequent(req SubsequentRequest) (text, schema string, err error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return "", "", errors.New("agent: Prompt is required (DefaultSubsequentPrompt is ours)")
	}
	pageState, schema, err := prepare(req.Preset, req.State, req.Request.Dir)
	if err != nil {
		return "", "", err
	}
	performed := req.Performed
	if performed == nil {
		performed = []action.Performed{}
	}

	text, err = render(req.Prompt, map[string]prompt.Param{
		"performed_actions": prompt.JSON(performed),
		"page_state":        prompt.Text(pageState),
	})
	return text, schema, err
}

// prepare checks what every turn needs and returns the page description and
// the response schema.
func prepare(p preset.Preset, st preset.State, dir string) (pageState, schema string, err error) {
	if err := preset.Validate(p); err != nil {
		return "", "", fmt.Errorf("agent: %w", err)
	}
	if err := checkDir(dir); err != nil {
		return "", "", err
	}
	rel, err := relativeState(st, dir)
	if err != nil {
		return "", "", err
	}
	s, err := ResponseSchema(p)
	if err != nil {
		return "", "", fmt.Errorf("agent: %w", err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", "", fmt.Errorf("agent: encoding the response schema: %w", err)
	}
	return p.Describe(rel), string(b), nil
}

// render binds only the params whose placeholder the template uses, so a
// custom template may leave any of them out. Any other placeholder left in the
// template is an error.
func render(tmpl string, params map[string]prompt.Param) (string, error) {
	used := make(map[string]prompt.Param, len(params))
	for key, p := range params {
		if strings.Contains(tmpl, "{"+key+"}") {
			used[key] = p
		}
	}
	res, err := prompt.Render(tmpl, used)
	if err != nil {
		return "", fmt.Errorf("agent: rendering prompt: %w", err)
	}
	if len(res.Unreplaced) > 0 {
		return "", fmt.Errorf("agent: prompt has unknown placeholders: %s", strings.Join(res.Unreplaced, ", "))
	}
	return res.LLM, nil
}

// screenReaderText explains the screen readers' output and the focused
// element when p perceives the focused element, and is empty otherwise, so
// an agent that hears no screen reader isn't told about one.
func screenReaderText(p preset.Preset) string {
	if !p.Perceive.Focused {
		return ""
	}
	text := strings.TrimSpace(screenReaderGuide)
	if preset.Allows(p, action.TypeNVDAChangeMode) {
		text += "\n\n" + nvdaModeGuide(p)
	}
	return text
}

// nvdaModeGuide explains the nvdaChangeMode action and the modes p allows,
// for a preset that allows it.
func nvdaModeGuide(p preset.Preset) string {
	allowed := preset.AllowedNVDAModes(p)
	modes := make([]string, len(allowed))
	for i, m := range allowed {
		modes[i] = fmt.Sprintf("%q", m)
	}
	text := fmt.Sprintf(`The %s action sets NVDA's mode: %s. %q is the default. Any other mode
turns VoiceOver off until NVDA is in %q again, and every turn VoiceOver is off
you are told so.`,
		action.TypeNVDAChangeMode, strings.Join(modes, ", "), action.NVDAModeFocus, action.NVDAModeFocus)
	if slices.Contains(allowed, action.NVDAModeBrowse) {
		text += `

In "Browse" mode NVDA reads the page as NVDA users do, with its own reading
cursor, and keeps these keys from the page (use the press action):
ArrowDown / ArrowUp read the next / previous line, ArrowRight / ArrowLeft a
character, Control+ArrowRight / Control+ArrowLeft a word, PageDown / PageUp
move 25 lines, Control+Home / Control+End go to the top / bottom, and
NVDA+ArrowDown reads from the cursor to the end (say all). Single letters jump
to the next element of a kind (with Shift, the previous one): H heading, 1-6
heading of that level, K link, F form field, E edit field, B button, X check
box, R radio button, C combo box, L list, I list item, G graphic, D landmark,
T table, S separator, Q block quote; "no next heading" means there is none.
Enter or Space activates what the cursor is on. Any other character key is
kept from the page, so typing letters in browse mode jumps around instead of
typing: move focus into the field (Tab, or Enter on it) first. Tab, Escape and
other keys still reach the page. Like real NVDA, it switches to "Focus" mode by
itself when focus lands in a text field or a widget it can't operate in browse
mode (a combo box, list box, tree, menu, radio button, tab, …), and back to
"Browse" when focus moves on to something else or you press Escape;
NVDA+Space switches by hand. Switching plays a sound, shown as "(sound: focus
mode)" / "(sound: browse mode)".`
	}
	if slices.Contains(allowed, action.NVDAModeOneKeyPass) {
		text += fmt.Sprintf(`

%q (or pressing NVDA+F2) lasts one key: NVDA says "Pass next key
through", the next key goes straight to the page whatever NVDA would do with
it (in browse mode, a letter types instead of jumping, an arrow key reaches the
page), and NVDA is then back in the mode it was in.`, action.NVDAModeOneKeyPass)
	}
	var other []string
	for _, m := range allowed {
		switch m {
		case action.NVDAModeFocus, action.NVDAModeBrowse, action.NVDAModeOneKeyPass:
		default:
			other = append(other, fmt.Sprintf("%q", m))
		}
	}
	if len(other) > 0 {
		text += fmt.Sprintf(`

%s only turn VoiceOver off: what NVDA does differently in them isn't
simulated, so NVDA goes on speaking as in focus mode there, and you can't draw
conclusions about those modes from it.`, strings.Join(other, ", "))
	}
	return text
}

// artifactsGuide explains the artifact actions p allows, or is empty when it
// allows none.
func artifactsGuide(p preset.Preset) string {
	if !preset.UsesArtifacts(p) {
		return ""
	}
	var b strings.Builder
	b.WriteString("Artifacts are files you leave for the user. You can only reach them with these\n")
	b.WriteString("actions, never with a file tool. They are create-only: nothing edits,\n")
	b.WriteString("overwrites or deletes one, so creating a name that already exists fails; pick\n")
	b.WriteString("another name instead. Names are plain file names with no directories.\n")
	lines := []struct {
		t    action.Type
		text string
	}{
		{action.TypeScreenshot, "- `screenshot` saves a PNG of the page (or of one `target`) as `{name}.png` for the user. You never see it; it is not your per-turn capture."},
		{action.TypeCreateArtifact, "- `createArtifact` saves `content` as `name`; include the extension, e.g. `report.md`."},
		{action.TypeReadArtifact, "- `readArtifact` returns a text artifact's content in that action's `result` next turn. Use it to keep notes across turns."},
		{action.TypeListArtifacts, "- `listArtifacts` returns every artifact name in its `result` next turn."},
	}
	for _, l := range lines {
		if preset.Allows(p, l.t) {
			b.WriteString(l.text + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
