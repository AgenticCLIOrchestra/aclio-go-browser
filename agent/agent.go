// Package agent runs the agent's side of a browser task, one turn at a time.
// It never touches the browser: each turn it shows the agent the page state
// the caller captured and returns the actions the agent asks for. Performing
// them, and capturing the next state, is the caller's job (see the session
// package).
//
// Every turn function takes a prompt template, and it is required. To use
// ours, pass DefaultInitialPrompt / DefaultSubsequentPrompt:
//
//	turn, err := agent.Initial(agent.InitialRequest{
//	    Prompt:  agent.DefaultInitialPrompt,
//	    Task:    "Find the cheapest flight to Lisbon next Friday",
//	    Preset:  preset.Default(),
//	    State:   state,
//	    Request: aclio.Request{Provider: aclio.Claude, Dir: workDir},
//	})
//
// The agent gets no direct access to anything but its own working directory:
// Claude is allowed only the Read and Grep tools, limited to that directory,
// and Codex runs in a read-only sandbox. Whatever else it does goes through the actions
// its preset allows.
package agent

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	aclio "github.com/agenticcliorchestra/aclio-go"
	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/agenticcliorchestra/aclio-go/claude"
	"github.com/agenticcliorchestra/aclio-go/codex"
)

// DefaultInitialPrompt is our template for the first turn. It uses every
// initial placeholder.
//
//go:embed INITIAL_PROMPT.md
var DefaultInitialPrompt string

// DefaultSubsequentPrompt is our template for every later turn. It uses every
// subsequent placeholder.
//
//go:embed SUBSEQUENT_PROMPT.md
var DefaultSubsequentPrompt string

// Placeholders a template may use. A template can use any subset; any other
// {lower_snake_case} token in it is an error.
const (
	// Initial turn.
	PlaceholderTask           = "{task}"            // the user's task
	PlaceholderPageState      = "{page_state}"      // the preset's description of the page
	PlaceholderResponseSchema = "{response_schema}" // the JSON schema the agent must answer with
	PlaceholderExamples       = "{examples}"        // examples the preset allows
	PlaceholderArtifacts      = "{artifacts}"       // how to use artifacts; empty if the preset has none

	// Subsequent turns (plus PlaceholderPageState).
	PlaceholderPerformedActions = "{performed_actions}" // the last batch, with outcomes
)

// claudeTools are the only tools Claude gets: Read and Grep, both limited to
// its working directory, where each turn's snapshot files are. Grep lets it
// find the part of a large DOM it needs before reading around it.
var claudeTools = []string{"Read(./**)", "Grep(./**)"}

// Response is what the agent answers each turn with.
type Response struct {
	// TaskComplete is true once the task is done.
	TaskComplete bool `json:"taskComplete"`
	// Actions to perform next, in order. Empty when TaskComplete.
	Actions []action.Action `json:"actions,omitempty"`
	// Answer is the information the task asked for, set on the final turn.
	Answer *string `json:"answer,omitempty"`
}

// InitialRequest starts a task.
type InitialRequest struct {
	// Prompt is the template. Required; DefaultInitialPrompt is ours.
	Prompt string
	// Task is what the user wants done. Required.
	Task string
	// Preset decides which actions the agent is offered and how the page is
	// described.
	Preset preset.Preset
	// State is the page state the caller captured. Its file paths must be
	// inside Request.Dir, since that is all the agent can read.
	State preset.State
	// Examples shown to the agent. nil means DefaultExamples(); either way
	// only the examples Preset allows are shown. Pass an empty, non-nil slice
	// for none.
	Examples []Example
	// Request carries the provider, its working directory (Dir, required and
	// absolute), model, name, streaming, log writer and provider extras such
	// as ClaudeOpts.TempDir. Prompt, JSONSchema and ResumeID are set by this
	// package, and so are the agent's tools.
	Request aclio.Request
	// Session, when set, runs the turn on this session (see StartSession)
	// instead of a fresh CLI call. Request.Dir must still be the session's
	// working dir, since snapshot paths are made relative to it.
	Session *aclio.Session
}

// SubsequentRequest continues a task with the outcome of the last batch.
type SubsequentRequest struct {
	// Prompt is the template. Required; DefaultSubsequentPrompt is ours.
	Prompt string
	// SessionID continues the conversation. Required without Session:
	// Turn.SessionID from the previous turn.
	SessionID string
	// Performed is the outcome of the last batch of actions.
	Performed []action.Performed
	// Preset must be the same preset as the first turn's.
	Preset preset.Preset
	// State is the page state after the last batch.
	State preset.State
	// Request as for InitialRequest.
	Request aclio.Request
	// Session, when set, continues the conversation on this session (the one
	// the first turn ran on) instead of resuming SessionID in a fresh CLI
	// call.
	Session *aclio.Session
}

// Turn is one agent turn.
type Turn struct {
	// SessionID continues the conversation in the next SubsequentRequest.
	SessionID string
	// Response is what the agent asked for.
	Response Response
	// Result is the provider's report on the call (cost, tokens, …).
	Result aclio.Result
}

// Initial runs the first turn of a task.
func Initial(req InitialRequest) (Turn, error) {
	text, schema, err := renderInitial(req)
	if err != nil {
		return Turn{}, err
	}
	if req.Session != nil {
		return send(req.Session, text)
	}
	return run(req.Request, text, schema, "")
}

// Subsequent runs a later turn of a task.
func Subsequent(req SubsequentRequest) (Turn, error) {
	if req.SessionID == "" && req.Session == nil {
		return Turn{}, errors.New("agent: SessionID or Session is required")
	}
	text, schema, err := renderSubsequent(req)
	if err != nil {
		return Turn{}, err
	}
	if req.Session != nil {
		return send(req.Session, text)
	}
	return run(req.Request, text, schema, req.SessionID)
}

// RenderInitial returns the prompt Initial would send, without running
// anything.
func RenderInitial(req InitialRequest) (string, error) {
	text, _, err := renderInitial(req)
	return text, err
}

// RenderSubsequent returns the prompt Subsequent would send, without running
// anything.
func RenderSubsequent(req SubsequentRequest) (string, error) {
	text, _, err := renderSubsequent(req)
	return text, err
}

// ResponseSchema returns the JSON schema of a Response under p: the actions
// it may contain are p's.
func ResponseSchema(p preset.Preset) (map[string]any, error) {
	item, err := preset.Schema(p)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"taskComplete": map[string]any{"type": "boolean", "description": "true once the task is done"},
			"actions":      map[string]any{"type": "array", "items": item, "description": "the next actions, in order; omit when taskComplete"},
			"answer":       map[string]any{"type": "string", "description": "information the task asked for; only with taskComplete"},
		},
		"required":             []string{"taskComplete"},
		"additionalProperties": false,
	}, nil
}

func run(base aclio.Request, text, schema, sessionID string) (Turn, error) {
	req, err := lockDown(base)
	if err != nil {
		return Turn{}, err
	}
	req.Prompt = text
	req.JSONSchema = schema
	req.ResumeID = sessionID

	resp, result, err := aclio.RunStructured[Response](req)
	if err != nil {
		return Turn{}, fmt.Errorf("agent: %w", err)
	}
	return Turn{SessionID: result.SessionID, Response: resp, Result: result}, nil
}

// StartSession opens a session for a whole task under preset p: every turn
// runs on it (set InitialRequest.Session / SubsequentRequest.Session), so with
// Claude one CLI process serves the whole task instead of one per turn. base
// is as for InitialRequest.Request; the agent's tools and the response schema
// are fixed here, exactly as for a single turn. Close it with
// aclio.CloseSession.
func StartSession(base aclio.Request, p preset.Preset) (*aclio.Session, error) {
	req, err := lockDown(base)
	if err != nil {
		return nil, err
	}
	if err := preset.Validate(p); err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}
	s, err := ResponseSchema(p)
	if err != nil {
		return nil, fmt.Errorf("agent: %w", err)
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("agent: encoding the response schema: %w", err)
	}
	req.Prompt = ""
	req.JSONSchema = string(b)
	return aclio.StartSession(req)
}

func send(s *aclio.Session, text string) (Turn, error) {
	resp, result, err := aclio.SendStructured[Response](s, text)
	if err != nil {
		return Turn{}, fmt.Errorf("agent: %w", err)
	}
	return Turn{SessionID: result.SessionID, Response: resp, Result: result}, nil
}

// lockDown returns base with the agent's tools fixed, whatever base asked
// for: Claude may only Read and Grep inside its working directory, Codex runs in a
// read-only sandbox. Every other provider extra is kept. The caller's
// ClaudeOpts / CodexOpts are copied, never modified.
func lockDown(base aclio.Request) (aclio.Request, error) {
	if err := checkDir(base.Dir); err != nil {
		return aclio.Request{}, err
	}
	switch base.Provider {
	case aclio.Claude:
		c := claude.RunOpts{}
		if base.ClaudeOpts != nil {
			c = *base.ClaudeOpts
		}
		c.AllowedTools = slices.Clone(claudeTools)
		base.ClaudeOpts = &c
	case aclio.Codex:
		c := codex.RunOpts{}
		if base.CodexOpts != nil {
			c = *base.CodexOpts
		}
		c.Sandbox = codex.SandboxReadOnly
		// The working dir is usually a fresh directory, not a git checkout.
		c.SkipGitRepoCheck = true
		base.CodexOpts = &c
	default:
		return aclio.Request{}, fmt.Errorf("agent: unknown provider %q", base.Provider)
	}
	return base, nil
}

func checkDir(dir string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("agent: Request.Dir %q must be an absolute path", dir)
	}
	return nil
}

// relativeState rewrites the snapshot paths in st relative to dir, so the
// prompt shows paths the agent can open with its Read tool. A path outside
// dir is an error: the agent couldn't read it.
func relativeState(st preset.State, dir string) (preset.State, error) {
	for _, p := range []*string{&st.ScreenshotPath, &st.DOMPath} {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return preset.State{}, err
		}
		rel, err := filepath.Rel(dir, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return preset.State{}, fmt.Errorf("agent: snapshot file %q is outside the agent's working dir %q, so the agent can't read it", *p, dir)
		}
		*p = filepath.ToSlash(rel)
	}
	return st, nil
}
