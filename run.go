package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	aclio "github.com/agenticcliorchestra/aclio-go"
	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/agent"
	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/agenticcliorchestra/aclio-go-browser/session"
	"github.com/agenticcliorchestra/aclio-go/claude"
	"github.com/agenticcliorchestra/aclio-go/codex"
)

// DefaultMaxTurns is the turn limit when Request.MaxTurns is 0.
const DefaultMaxTurns = 50

// ErrTurnLimit is returned (wrapped) when the agent hasn't finished the task
// within Request.MaxTurns turns. The Result is still filled in.
var ErrTurnLimit = errors.New("turn limit reached before the task was complete")

// Working-directory layout. Everything lives under Dirs.Working.
const (
	agentSubdir = "agent" // the agent's working directory, all it can read
	turnsSubdir = "turns" // each turn's screenshot and DOM, inside agentSubdir
	videoSubdir = "video" // Playwright's recording
	debugSubdir = "debug" // the CLI's per-call debug dumps
)

// Request is one browser task.
type Request struct {
	// Task is what the user wants done. Required.
	Task string
	// StartURLs are opened before the first turn, one tab each, with the
	// first tab active. Empty starts on a blank tab.
	StartURLs []string
	// Preset is the agent's persona. nil means preset.Default().
	Preset *preset.Preset
	// Agent configures the agent's CLI calls: Provider (required), Model,
	// Name, Stream, LogWriter and provider extras. Dir, Prompt, JSONSchema,
	// ResumeID and the agent's tools are set by this package. Debug dumps go
	// to {WorkingDir}/debug unless ClaudeOpts.TempDir / CodexOpts.TempDir is
	// set.
	Agent aclio.Request
	// InitialPrompt and SubsequentPrompt are the agent's prompt templates.
	// nil means agent.DefaultInitialPrompt / agent.DefaultSubsequentPrompt.
	InitialPrompt    *string
	SubsequentPrompt *string
	// MaxTurns caps the agent turns. 0 means DefaultMaxTurns.
	MaxTurns int
	// Options configures the browser and where files go.
	Options Options
}

// Result is the outcome of a task. It is filled in as far as the run got,
// even when an error is returned.
type Result struct {
	// Completed is true when the agent reported the task done.
	Completed bool
	// Answer is the information the task asked for, if any.
	Answer *string
	// Turns is how many agent turns ran.
	Turns int
	// SessionID is the agent's conversation, for resuming or inspection.
	SessionID string
	// Dirs are where the run's files are: the working dir (snapshots, debug
	// dumps, video) and the artifacts.
	Dirs Dirs
	// CostUSD, InputTokens and OutputTokens add up every turn (Codex reports
	// no cost).
	CostUSD      float64
	InputTokens  int
	OutputTokens int
}

// Run does a whole task: it opens the browser, opens the start URLs, lets the
// agent work turn by turn until it reports the task done, and closes the
// browser. The browser never leaves this function; use Loop to run the same
// loop on a browser you manage yourself.
//
// The request is checked before anything is created on disk or launched.
func Run(req Request) (Result, error) {
	p, err := checkRequest(req)
	if err != nil {
		return Result{}, err
	}
	if err := checkBrowserOptions(req.Options); err != nil {
		return Result{}, err
	}
	e, _ := engine.Resolve(req.Options.Browser) // checked just above
	if err := checkEngine(p, e); err != nil {
		return Result{}, err
	}

	dirs, err := ResolveDirs(req.Options)
	if err != nil {
		return Result{}, err
	}
	res := Result{Dirs: dirs}

	opts := sessionOpts(req.Options, dirs)
	now := time.Now()
	if !req.Options.NoConsoleLog {
		opts.ConsoleLog = debugLogPath(req.Agent, dirs, now, "console")
	}
	if !req.Options.NoNetworkLog {
		opts.NetworkLog = debugLogPath(req.Agent, dirs, now, "network")
		if req.Options.NetworkLogAll {
			opts.Network.Mode = session.NetworkLogAll
		}
	}
	b, err := session.Open(opts)
	if err != nil {
		return res, err
	}
	res, err = runOpen(b, req, p, dirs)
	if cerr := session.Close(b); cerr != nil {
		err = errors.Join(err, cerr)
	}
	return res, err
}

func runOpen(b *session.Browser, req Request, p preset.Preset, dirs Dirs) (Result, error) {
	if err := openStartURLs(b, req.StartURLs); err != nil {
		return Result{Dirs: dirs}, err
	}
	return loop(b, req, p, dirs)
}

// Loop runs a task on b, a browser the caller opened (session.Open) and will
// close (session.Close); Loop never does either. It starts from whatever b is
// showing. dirs says where the agent's files go (see ResolveDirs); b should
// have been opened with the same artifacts dir.
func Loop(b *session.Browser, req Request, dirs Dirs) (Result, error) {
	p, err := checkRequest(req)
	if err != nil {
		return Result{Dirs: dirs}, err
	}
	if dirs.Working == "" || !filepath.IsAbs(dirs.Working) {
		return Result{Dirs: dirs}, fmt.Errorf("browser: dirs.Working %q must be an absolute path", dirs.Working)
	}
	if b != nil {
		if err := checkEngine(p, session.Engine(b)); err != nil {
			return Result{Dirs: dirs}, err
		}
	}
	return loop(b, req, p, dirs)
}

func loop(b *session.Browser, req Request, p preset.Preset, dirs Dirs) (Result, error) {
	res := Result{Dirs: dirs}
	agentDir := filepath.Join(dirs.Working, agentSubdir)
	turnsDir := filepath.Join(agentDir, turnsSubdir)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return res, fmt.Errorf("browser: creating the agent dir: %w", err)
	}
	base := agentRequest(req.Agent, agentDir, filepath.Join(dirs.Working, debugSubdir))
	maxTurns := req.MaxTurns
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	}

	if p.Perceive.Focused {
		// From here on, everything a screen reader says is recorded as it
		// happens, and each snapshot takes what was said since the last.
		if err := session.StartAnnouncing(b, p.Language); err != nil {
			return res, err
		}
	}
	st, err := snapshot(b, p, turnsDir, base, 1)
	if err != nil {
		return res, err
	}
	turn, err := agent.Initial(agent.InitialRequest{
		Prompt:  valueOr(req.InitialPrompt, agent.DefaultInitialPrompt),
		Task:    req.Task,
		Preset:  p,
		State:   st,
		Request: base,
	})
	if err != nil {
		return res, err
	}
	addTurn(&res, turn)

	for !turn.Response.TaskComplete {
		if res.Turns >= maxTurns {
			return res, fmt.Errorf("browser: %w (%d)", ErrTurnLimit, maxTurns)
		}
		performed := Perform(b, p, turn.Response.Actions)
		if st, err = snapshot(b, p, turnsDir, base, res.Turns+1); err != nil {
			return res, err
		}
		turn, err = agent.Subsequent(agent.SubsequentRequest{
			Prompt:    valueOr(req.SubsequentPrompt, agent.DefaultSubsequentPrompt),
			SessionID: res.SessionID,
			Performed: performed,
			Preset:    p,
			State:     st,
			Request:   base,
		})
		if err != nil {
			return res, err
		}
		addTurn(&res, turn)
	}

	res.Completed = true
	res.Answer = turn.Response.Answer
	return res, nil
}

func addTurn(res *Result, t agent.Turn) {
	res.Turns++
	if t.SessionID != "" {
		res.SessionID = t.SessionID
	}
	res.CostUSD += t.Result.CostUSD
	res.InputTokens += t.Result.InputTokens
	res.OutputTokens += t.Result.OutputTokens
}

// Perform performs the actions p allows, in order, and reports every
// outcome. Each action goes through preset.Check first; the first one that
// fails it is refused (Attempted false, with the reason) and the batch stops
// there, exactly as it stops at the first action that fails while running.
// Nothing is returned as an error: every outcome is meant for the agent.
func Perform(b *session.Browser, p preset.Preset, actions []action.Action) []action.Performed {
	refused := len(actions)
	var reason error
	for i, a := range actions {
		if err := preset.Check(p, a); err != nil {
			refused, reason = i, err
			break
		}
	}

	out := make([]action.Performed, len(actions))
	for i, a := range actions {
		out[i].Action = a
	}
	done := session.Perform(b, actions[:refused])
	copy(out, done)

	if refused < len(actions) && allSucceeded(done) {
		msg := "refused: " + reason.Error()
		out[refused].Reason = &msg
	}
	return out
}

func allSucceeded(ps []action.Performed) bool {
	for _, p := range ps {
		if !p.Succeeded {
			return false
		}
	}
	return true
}

// checkRequest checks everything about req that doesn't touch the disk or
// the browser, and returns the preset to use.
func checkRequest(req Request) (preset.Preset, error) {
	var errs []error
	if strings.TrimSpace(req.Task) == "" {
		errs = append(errs, errors.New("Task is required"))
	}
	switch req.Agent.Provider {
	case aclio.Claude, aclio.Codex:
	default:
		errs = append(errs, fmt.Errorf("Agent.Provider %q is not a known provider", req.Agent.Provider))
	}
	if req.MaxTurns < 0 {
		errs = append(errs, fmt.Errorf("MaxTurns %d must not be negative", req.MaxTurns))
	}
	for name, tmpl := range map[string]*string{"InitialPrompt": req.InitialPrompt, "SubsequentPrompt": req.SubsequentPrompt} {
		if tmpl != nil && strings.TrimSpace(*tmpl) == "" {
			errs = append(errs, fmt.Errorf("%s is set but empty; leave it nil for ours", name))
		}
	}
	p := preset.Default()
	if req.Preset != nil {
		p = *req.Preset
	}
	if err := preset.Validate(p); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return preset.Preset{}, fmt.Errorf("browser: %w", err)
	}
	return p, nil
}

// checkEngine refuses Firefox for a preset that perceives the focused element
// (e.g. Blind): its screen-reader view is to come from Chromium's own
// accessibility tree, which Firefox doesn't expose.
func checkEngine(p preset.Preset, e engine.Browser) error {
	if p.Perceive.Focused && e == engine.Firefox {
		return fmt.Errorf("browser: preset %q perceives the focused element, which needs Chromium; Firefox is not supported", p.Name)
	}
	return nil
}

// checkBrowserOptions catches the option errors session.Open would report,
// before ResolveDirs creates anything on disk.
func checkBrowserOptions(o Options) error {
	if _, err := engine.Resolve(o.Browser); err != nil {
		return fmt.Errorf("browser: %w", err)
	}
	if v := o.Viewport; v != nil && (v.Width <= 0 || v.Height <= 0) {
		return fmt.Errorf("browser: Viewport %dx%d must be positive", v.Width, v.Height)
	}
	return nil
}

func sessionOpts(o Options, dirs Dirs) session.Opts {
	return session.Opts{
		Browser:      o.Browser,
		Headed:       o.Headed,
		Viewport:     o.Viewport,
		UserDataDir:  dirs.UserData,
		VideoDir:     filepath.Join(dirs.Working, videoSubdir),
		ArtifactsDir: dirs.Artifacts,
	}
}

// debugStampLayout matches the names of aclio-go's debug dumps ({stamp}-{name}-
// {kind}, local time, millisecond precision), so a DOM dump sorts among them
// in chronological order.
const debugStampLayout = "20060102-150405.000"

// debugLogPath is where Run keeps the kind ("console", "network") debug log:
// the run's debug dir (as loop will set it up), named like the CLI's dumps,
// {stamp}-{name}-{kind}.jsonl, stamped at open so it sorts just before the
// first turn's dumps. "" when there is no debug dir.
func debugLogPath(r aclio.Request, dirs Dirs, now time.Time, kind string) string {
	r = agentRequest(r, filepath.Join(dirs.Working, agentSubdir), filepath.Join(dirs.Working, debugSubdir))
	dir := debugDir(r)
	if dir == "" {
		return ""
	}
	if !filepath.IsAbs(dir) {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return ""
		}
		dir = abs
	}
	return filepath.Join(dir, now.Format(debugStampLayout)+"-"+debugName(r.Name)+"-"+kind+".jsonl")
}

// snapshot takes turn n's snapshot, first marking the turn in the console log
// (session.LogTurn). When the preset perceives the focused
// element, it adds everything the screen reader has said since the previous
// snapshot (State.Announcement, see session.StartAnnouncing). When the preset doesn't perceive the DOM (e.g. Blind), the DOM is still
// saved, formatted the same way, to the debug dir as
// {stamp}-{name}-dom.html, out of the agent's reach. That copy is
// best-effort, like the CLI's own debug dumps: failing to write it never
// fails the run.
func snapshot(b *session.Browser, p preset.Preset, turnsDir string, r aclio.Request, n int) (preset.State, error) {
	session.LogTurn(b, n)
	st, err := session.Snapshot(b, p.Perceive, turnsDir)
	if err != nil {
		return st, err
	}
	if p.Perceive.Focused {
		st.Announcement = session.TakeAnnouncements(b)
	}
	if p.Perceive.DOM {
		return st, nil
	}
	if dir := debugDir(r); dir != "" {
		name := time.Now().Format(debugStampLayout) + "-" + debugName(r.Name) + "-dom.html"
		_ = session.WriteDOM(b, filepath.Join(dir, name))
	}
	return st, nil
}

// debugDir is where r's CLI writes its debug dumps, "" when nowhere.
func debugDir(r aclio.Request) string {
	switch {
	case r.Provider == aclio.Claude && r.ClaudeOpts != nil:
		return r.ClaudeOpts.TempDir
	case r.Provider == aclio.Codex && r.CodexOpts != nil:
		return r.CodexOpts.TempDir
	}
	return ""
}

// debugName normalises a run name for a dump file name the way aclio-go does
// for its own dumps: lowercased, anything outside [a-z0-9_-] mapped to '-',
// empty as "unnamed".
func debugName(name string) string {
	if name == "" {
		return "unnamed"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, name)
}

// agentRequest returns the caller's agent request with the agent's working
// dir set and debug dumps defaulted to debugDir. The caller's provider
// extras are copied, never modified.
func agentRequest(r aclio.Request, agentDir, debugDir string) aclio.Request {
	r.Dir = agentDir
	if r.Name == "" {
		r.Name = "browser"
	}
	switch r.Provider {
	case aclio.Claude:
		c := claude.RunOpts{}
		if r.ClaudeOpts != nil {
			c = *r.ClaudeOpts
		}
		if c.TempDir == "" {
			c.TempDir = debugDir
		}
		r.ClaudeOpts = &c
	case aclio.Codex:
		c := codex.RunOpts{}
		if r.CodexOpts != nil {
			c = *r.CodexOpts
		}
		if c.TempDir == "" {
			c.TempDir = debugDir
		}
		r.CodexOpts = &c
	}
	return r
}

// openStartURLs opens one tab per URL, in order, and leaves the first active.
func openStartURLs(b *session.Browser, urls []string) error {
	for i, u := range urls {
		if i > 0 {
			if err := performOrFail(b, action.Action{Type: action.TypeTabOpen, Data: action.TabOpen{}}); err != nil {
				return err
			}
		}
		if err := session.Navigate(b, u); err != nil {
			return err
		}
	}
	if len(urls) > 1 {
		return performOrFail(b, action.Action{Type: action.TypeTabSwitch, Data: action.TabSwitch{Index: 1}})
	}
	return nil
}

func performOrFail(b *session.Browser, a action.Action) error {
	done := session.Perform(b, []action.Action{a})
	if !done[0].Succeeded {
		return fmt.Errorf("browser: opening start tabs: %s", *done[0].Reason)
	}
	return nil
}

func valueOr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}
