package agent

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	aclio "github.com/agenticcliorchestra/aclio-go"
	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/agenticcliorchestra/aclio-go/claude"
	"github.com/agenticcliorchestra/aclio-go/codex"
)

func state(dir string) preset.State {
	return preset.State{
		ScreenshotPath: filepath.Join(dir, "turns", "1.png"),
		DOMPath:        filepath.Join(dir, "turns", "1.html"),
		Tabs:           []preset.Tab{{Title: "Home", URL: "https://x.com", Active: true}},
	}
}

func initial(t *testing.T, p preset.Preset) InitialRequest {
	t.Helper()
	dir := t.TempDir()
	return InitialRequest{
		Prompt:  DefaultInitialPrompt,
		Task:    "find the price",
		Preset:  p,
		State:   state(dir),
		Request: aclio.Request{Provider: aclio.Claude, Dir: dir},
	}
}

// Our templates render cleanly under every built-in preset.
func TestDefaultPromptsRender(t *testing.T) {
	for _, p := range append(preset.Builtins(), preset.WithArtifacts(preset.Blind())) {
		req := initial(t, p)
		text, err := RenderInitial(req)
		if err != nil {
			t.Fatalf("%s initial: %v", p.Name, err)
		}
		for _, want := range []string{"find the price", `"taskComplete"`, "turns/1.png"} {
			if p.Perceive.Screenshot || want != "turns/1.png" {
				if !strings.Contains(text, want) {
					t.Errorf("%s: prompt lacks %q", p.Name, want)
				}
			}
		}
		if strings.Contains(text, req.Request.Dir) {
			t.Errorf("%s: prompt leaks the absolute working dir", p.Name)
		}

		sub, err := RenderSubsequent(SubsequentRequest{
			Prompt:    DefaultSubsequentPrompt,
			SessionID: "s",
			Performed: []action.Performed{{Action: action.Action{Type: action.TypeWait, Data: action.Wait{Milliseconds: 1}}, Attempted: true, Succeeded: true}},
			Preset:    p,
			State:     req.State,
			Request:   req.Request,
		})
		if err != nil {
			t.Fatalf("%s subsequent: %v", p.Name, err)
		}
		if !strings.Contains(sub, `"succeeded":true`) {
			t.Errorf("%s: subsequent prompt lacks the performed actions", p.Name)
		}
	}
}

func TestArtifactsGuideFollowsPreset(t *testing.T) {
	text, err := RenderInitial(initial(t, preset.Default()))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "createArtifact") {
		t.Error("default preset has no artifacts, but the prompt explains them")
	}
	text, err = RenderInitial(initial(t, preset.Full()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "create-only") || !strings.Contains(text, "listArtifacts") {
		t.Error("full preset's prompt doesn't explain artifacts")
	}
}

// Only a preset that perceives the focused element is told about screen
// readers: Default and Motor hear none.
func TestScreenReaderGuideFollowsPreset(t *testing.T) {
	for _, p := range []preset.Preset{preset.Default(), preset.Motor()} {
		text, err := RenderInitial(initial(t, p))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(text, "screen reader") || strings.Contains(text, "NVDA") || strings.Contains(text, PlaceholderScreenReader) {
			t.Errorf("%s preset hears no screen reader, but the prompt explains one", p.Name)
		}
	}
	text, err := RenderInitial(initial(t, preset.Blind()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "VoiceOver and NVDA") || !strings.Contains(text, "A focused element, when given") {
		t.Error("blind preset's prompt doesn't explain the screen readers")
	}
}

func TestCustomTemplate(t *testing.T) {
	req := initial(t, preset.Default())

	req.Prompt = "Do this: {task}"
	if text, err := RenderInitial(req); err != nil || text != "Do this: find the price" {
		t.Errorf("subset template = %q, %v", text, err)
	}

	for name, tmpl := range map[string]string{
		"empty":               " ",
		"no task":             "Look at {page_state}",
		"unknown placeholder": "{task} {pagestate}",
	} {
		req.Prompt = tmpl
		if _, err := RenderInitial(req); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestRequestChecks(t *testing.T) {
	req := initial(t, preset.Default())

	r := req
	r.Task = ""
	if _, err := RenderInitial(r); err == nil {
		t.Error("empty task should fail")
	}
	r = req
	r.Request.Dir = "relative"
	if _, err := RenderInitial(r); err == nil {
		t.Error("relative dir should fail")
	}
	r = req
	r.State.DOMPath = filepath.Join(filepath.Dir(r.Request.Dir), "elsewhere.html")
	if _, err := RenderInitial(r); err == nil || !strings.Contains(err.Error(), "can't read it") {
		t.Errorf("snapshot outside the dir: err = %v", err)
	}
	r = req
	r.Preset = preset.Preset{}
	if _, err := RenderInitial(r); err == nil {
		t.Error("invalid preset should fail")
	}
	if _, err := Subsequent(SubsequentRequest{Prompt: DefaultSubsequentPrompt}); err == nil {
		t.Error("missing SessionID should fail")
	}
}

// Whatever tools the caller asks for, the agent only ever gets Read and Grep
// inside its working dir (Claude) or a read-only sandbox (Codex), and the caller's
// options are not modified.
func TestLockDown(t *testing.T) {
	callerClaude := &claude.RunOpts{AllowedTools: []string{"Bash", "Write"}, TempDir: "/tmp/dbg", MaxTurns: 3}
	got, err := lockDown(aclio.Request{Provider: aclio.Claude, Dir: "/abs", ClaudeOpts: callerClaude})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.ClaudeOpts.AllowedTools, []string{"Read(./**)", "Grep(./**)"}) {
		t.Errorf("claude tools = %v", got.ClaudeOpts.AllowedTools)
	}
	if got.ClaudeOpts.TempDir != "/tmp/dbg" || got.ClaudeOpts.MaxTurns != 3 {
		t.Error("other claude extras were dropped")
	}
	if !slices.Equal(callerClaude.AllowedTools, []string{"Bash", "Write"}) {
		t.Error("the caller's ClaudeOpts were modified")
	}

	got, err = lockDown(aclio.Request{Provider: aclio.Claude, Dir: "/abs"})
	if err != nil || !slices.Equal(got.ClaudeOpts.AllowedTools, []string{"Read(./**)", "Grep(./**)"}) {
		t.Errorf("nil ClaudeOpts: %+v, %v", got.ClaudeOpts, err)
	}

	callerCodex := &codex.RunOpts{Sandbox: codex.SandboxDangerFull, TempDir: "/tmp/dbg"}
	got, err = lockDown(aclio.Request{Provider: aclio.Codex, Dir: "/abs", CodexOpts: callerCodex})
	if err != nil {
		t.Fatal(err)
	}
	if got.CodexOpts.Sandbox != codex.SandboxReadOnly || got.CodexOpts.TempDir != "/tmp/dbg" {
		t.Errorf("codex opts = %+v", got.CodexOpts)
	}
	if callerCodex.Sandbox != codex.SandboxDangerFull {
		t.Error("the caller's CodexOpts were modified")
	}

	if _, err := lockDown(aclio.Request{Provider: "gemini", Dir: "/abs"}); err == nil {
		t.Error("unknown provider should fail")
	}
}

func TestResponseSchemaAndDecode(t *testing.T) {
	s, err := ResponseSchema(preset.Blind())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	if !strings.Contains(string(raw), `"press"`) || strings.Contains(string(raw), `"click"`) {
		t.Errorf("blind schema offers the wrong actions: %s", raw)
	}

	var r Response
	err = json.Unmarshal([]byte(`{"taskComplete":false,"actions":[{"type":"press","data":{"key":"Tab"}}]}`), &r)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Actions) != 1 || r.Actions[0].Data != (action.Press{Key: "Tab"}) {
		t.Errorf("decoded = %+v", r)
	}
}

func TestExamplesFor(t *testing.T) {
	for _, p := range preset.Builtins() {
		for _, e := range ExamplesFor(p, DefaultExamples()) {
			for _, a := range e.Actions {
				if err := preset.Check(p, a); err != nil {
					t.Errorf("%s shows an example it can't follow: %v", p.Name, err)
				}
			}
		}
	}
	if len(ExamplesFor(preset.Blind(), DefaultExamples())) == 0 {
		t.Error("blind gets no examples")
	}
	for _, e := range DefaultExamples() {
		for _, a := range e.Actions {
			if err := action.Validate(a); err != nil {
				t.Errorf("example %q is invalid: %v", e.Situation, err)
			}
		}
	}
}

func TestRenderExamples(t *testing.T) {
	got := RenderExamples([]Example{{
		Situation: "pressing tab",
		Actions:   []action.Action{{Type: action.TypePress, Data: action.Press{Key: "Tab"}}},
		Why:       "it moves focus",
	}})
	want := `When pressing tab, the actions could be: [{"type":"press","data":{"key":"Tab"}}]. It moves focus.`
	if got != want {
		t.Errorf("RenderExamples =\n%s\nwant\n%s", got, want)
	}
}
