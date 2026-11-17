package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	aclio "github.com/agenticcliorchestra/aclio-go"
	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/agenticcliorchestra/aclio-go/claude"
	"github.com/agenticcliorchestra/aclio-go/codex"
)

func validRequest() Request {
	return Request{Task: "find the price", Agent: aclio.Request{Provider: aclio.Claude}}
}

func TestCheckRequest(t *testing.T) {
	p, err := checkRequest(validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "default" {
		t.Errorf("nil Preset should mean default, got %q", p.Name)
	}

	blind := preset.Blind()
	r := validRequest()
	r.Preset = &blind
	if p, _ := checkRequest(r); p.Name != "blind" {
		t.Errorf("preset = %q, want blind", p.Name)
	}

	bad := Request{
		Agent:         aclio.Request{Provider: "gemini"},
		MaxTurns:      -1,
		InitialPrompt: ptr(" "),
		Preset:        &preset.Preset{},
	}
	_, err = checkRequest(bad)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"Task", "Provider", "MaxTurns", "InitialPrompt", "preset"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}

// A bad request must fail before a working dir is created or a browser is
// launched.
func TestRunFailsBeforeTouchingAnything(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	cases := map[string]Request{
		"no task":      {Agent: aclio.Request{Provider: aclio.Claude}},
		"bad browser":  {Task: "x", Agent: aclio.Request{Provider: aclio.Claude}, Options: Options{Browser: "webkit"}},
		"bad viewport": {Task: "x", Agent: aclio.Request{Provider: aclio.Claude}, Options: Options{Viewport: &Size{}}},
		"empty dir":    {Task: "x", Agent: aclio.Request{Provider: aclio.Claude}, Options: Options{ArtifactsDir: ptr("")}},
	}
	for name, r := range cases {
		if _, err := Run(r); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if entries, _ := os.ReadDir(tmp); len(entries) != 0 {
		t.Errorf("a bad request left files behind: %v", entries)
	}
}

func TestLoopChecks(t *testing.T) {
	if _, err := Loop(nil, Request{}, Dirs{Working: "/abs"}); err == nil {
		t.Error("invalid request should fail")
	}
	if _, err := Loop(nil, validRequest(), Dirs{Working: "rel"}); err == nil {
		t.Error("relative working dir should fail")
	}
}

func press(key string) action.Action {
	return action.Action{Type: action.TypePress, Data: action.Press{Key: key}}
}

var click = action.Action{Type: action.TypeClick, Data: action.Click{Target: action.Target{Selector: ptr("a")}}}

// The first action the preset doesn't allow is refused with a reason, and the
// batch stops there without running anything after it.
func TestPerformRefuses(t *testing.T) {
	got := Perform(nil, preset.Blind(), []action.Action{click, press("Tab")})
	if got[0].Attempted || got[0].Succeeded || got[0].Reason == nil || !strings.HasPrefix(*got[0].Reason, "refused: ") {
		t.Errorf("refused action = %+v", got[0])
	}
	if got[1].Attempted || got[1].Reason != nil {
		t.Errorf("action after a refusal ran: %+v", got[1])
	}
}

// When an allowed action fails before the refused one is reached, the failure
// is what's reported; the refused action is just never attempted.
func TestPerformFailureBeforeRefusal(t *testing.T) {
	got := Perform(nil, preset.Blind(), []action.Action{press("Tab"), click})
	if !got[0].Attempted || got[0].Succeeded || got[0].Reason == nil {
		t.Errorf("first action = %+v, want a failure (nil browser)", got[0])
	}
	if got[1].Attempted || got[1].Reason != nil {
		t.Errorf("refused action = %+v, want untouched", got[1])
	}
}

func TestPerformEmpty(t *testing.T) {
	if got := Perform(nil, preset.Default(), nil); len(got) != 0 {
		t.Errorf("Perform(nil actions) = %v", got)
	}
}

func TestAgentRequest(t *testing.T) {
	caller := &claude.RunOpts{MaxTurns: 3}
	r := agentRequest(aclio.Request{Provider: aclio.Claude, ClaudeOpts: caller}, "/w/agent", "/w/debug")
	if r.Dir != "/w/agent" || r.Name != "browser" {
		t.Errorf("dir/name = %q/%q", r.Dir, r.Name)
	}
	if r.ClaudeOpts.TempDir != "/w/debug" || r.ClaudeOpts.MaxTurns != 3 {
		t.Errorf("claude opts = %+v", r.ClaudeOpts)
	}
	if caller.TempDir != "" {
		t.Error("the caller's ClaudeOpts were modified")
	}

	r = agentRequest(aclio.Request{Provider: aclio.Claude, Name: "mine", ClaudeOpts: &claude.RunOpts{TempDir: "/own"}}, "/w/agent", "/w/debug")
	if r.Name != "mine" || r.ClaudeOpts.TempDir != "/own" {
		t.Error("caller's name or TempDir was overridden")
	}

	r = agentRequest(aclio.Request{Provider: aclio.Codex}, "/w/agent", "/w/debug")
	if r.CodexOpts == nil || r.CodexOpts.TempDir != "/w/debug" {
		t.Errorf("codex opts = %+v", r.CodexOpts)
	}
}

func TestSessionOpts(t *testing.T) {
	ud := "/profile"
	o := sessionOpts(Options{Browser: Firefox, Headed: true, Viewport: &Size{Width: 800, Height: 600}},
		Dirs{Working: "/w", Artifacts: "/a", UserData: &ud})
	if o.Browser != Firefox || !o.Headed || o.Viewport.Width != 800 || *o.UserDataDir != "/profile" {
		t.Errorf("opts = %+v", o)
	}
	if o.VideoDir != filepath.Join("/w", "video") || o.ArtifactsDir != "/a" {
		t.Errorf("dirs = %q, %q", o.VideoDir, o.ArtifactsDir)
	}
}

func TestErrTurnLimitIsMatchable(t *testing.T) {
	err := errors.Join(errors.New("x"), ErrTurnLimit)
	if !errors.Is(err, ErrTurnLimit) {
		t.Error("ErrTurnLimit not matchable")
	}
}

func TestDebugDir(t *testing.T) {
	cases := []struct {
		r    aclio.Request
		want string
	}{
		{aclio.Request{Provider: aclio.Claude, ClaudeOpts: &claude.RunOpts{TempDir: "/w/debug"}}, "/w/debug"},
		{aclio.Request{Provider: aclio.Codex, CodexOpts: &codex.RunOpts{TempDir: "/own"}}, "/own"},
		{aclio.Request{Provider: aclio.Claude}, ""},
		{aclio.Request{Provider: aclio.Claude, CodexOpts: &codex.RunOpts{TempDir: "/x"}}, ""},
	}
	for _, c := range cases {
		if got := debugDir(c.r); got != c.want {
			t.Errorf("debugDir(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

// The DOM dump's name sorts among aclio-go's dumps: same stamp layout, same
// name normalisation.
func TestDebugName(t *testing.T) {
	for in, want := range map[string]string{"": "unnamed", "browser": "browser", "My Run/1": "my-run-1", "a_b-C": "a_b-c"} {
		if got := debugName(in); got != want {
			t.Errorf("debugName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := time.Date(2026, 11, 18, 9, 3, 1, 5e6, time.Local).Format(debugStampLayout); got != "20261118-090301.005" {
		t.Errorf("stamp = %q", got)
	}
}

// A preset that perceives the focused element (Blind) is refused on Firefox,
// before anything touches the disk; other presets and Chromium are fine.
func TestBlindRefusesFirefox(t *testing.T) {
	if err := checkEngine(preset.Blind(), engine.Firefox); err == nil {
		t.Error("Blind on Firefox should be refused")
	}
	for _, c := range []struct {
		p preset.Preset
		e engine.Browser
	}{{preset.Blind(), engine.Chromium}, {preset.Default(), engine.Firefox}, {preset.Motor(), engine.Firefox}} {
		if err := checkEngine(c.p, c.e); err != nil {
			t.Errorf("checkEngine(%s, %s) = %v", c.p.Name, c.e, err)
		}
	}

	req := validRequest()
	req.Preset = ptr(preset.Blind())
	wd := t.TempDir()
	req.Options = Options{Browser: Firefox, WorkingDir: ptr(wd)}
	if _, err := Run(req); err == nil || !strings.Contains(err.Error(), "Firefox") {
		t.Errorf("Run(Blind, Firefox) = %v", err)
	}
	if es, _ := os.ReadDir(wd); len(es) != 0 {
		t.Errorf("Run wrote %d entries before refusing", len(es))
	}
}
