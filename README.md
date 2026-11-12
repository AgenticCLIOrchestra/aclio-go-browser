# aclio-go-browser

Browser automation for [aclio-go](https://github.com/AgenticCLIOrchestra/aclio-go): an agentic CLI (Claude Code or Codex) decides what to do on a web page, and your Go program does it.

The agent never touches the browser or the network, and it can read nothing outside its own working directory. Each turn it is shown the page (a screenshot and the DOM, or a screen-reader view of the focused element) and answers with **structured output**: a list of actions such as `click`, `type` or `goTo`. This library checks every action against what the agent was allowed to do, performs the ones that pass, and reports the outcome back next turn. Anything the agent can affect goes through an action you chose to offer.

```
go get github.com/agenticcliorchestra/aclio-go-browser
```

> **Status: early, pre-1.0.** The API may still move between minor versions.

## Quick start

```go
// Once, before the first run: download the Playwright driver and Chromium.
if err := browserinstall.Ensure(browserinstall.Opts{}); err != nil { … }

res, err := browser.Run(browser.Request{
    Task:      "Find the price of the cheapest plan and tell me what it includes",
    StartURLs: []string{"https://example.com/pricing"},
    Agent:     aclio.Request{Provider: aclio.Claude},
})
// res.Completed, res.Answer, res.Turns, res.CostUSD, res.Dirs (where the files are)
```

`Run` opens the browser, lets the agent work turn by turn until it reports the task done, and closes the browser. You never handle the browser yourself. When you want to, every layer below `Run` is public too (see [Running it yourself](#running-it-yourself)).

## Packages

| Package  | What it gives you |
|----------|-------------------|
| `browser` (root) | `Run` (one call does a whole task), `Loop` and `Perform` (the same loop on a browser you manage), `Options`, `ResolveDirs` |
| `session` | One live browser: `Open` / `Close`, `Perform`, `Snapshot`, `Tabs`, `Focused`, `Navigate`. The only package that talks to Playwright |
| `agent` | The agent's side of each turn: `Initial` / `Subsequent`, our prompt templates, the response schema, examples |
| `action` | Every action an agent may request, as a `{type, data}` union; the JSON schema for any mix of them; `Validate` |
| `preset` | Agent personas: which actions are offered, how elements may be targeted, what page state is perceived and how it is described. Built-ins plus a public type for your own |
| `engine` | The `Browser` enum (`Chromium`, `Firefox`), re-exported from the root |
| `browserinstall` | Opt-in `Ensure` that downloads the Playwright driver and the browsers you ask for, if they're missing |

## Running a task

```go
type Request struct {
    Task             string         // required
    StartURLs        []string       // one tab each, first active; empty: a blank tab
    Preset           *preset.Preset // nil: preset.Default()
    Agent            aclio.Request  // Provider (required), Model, Name, Stream, LogWriter, Stderr, …
    InitialPrompt    *string        // nil: agent.DefaultInitialPrompt
    SubsequentPrompt *string        // nil: agent.DefaultSubsequentPrompt
    MaxTurns         int            // 0: DefaultMaxTurns (50)
    Options          Options
}
```

- **The request is checked before anything happens.** A bad request creates no directory and launches no browser.
- **`Agent`** is an ordinary aclio-go request. You choose the provider, model, run name, streaming, log writer and the CLI's stderr; this library sets the working directory, the prompt, the schema and session resumption, and **always** sets the agent's tools (see [agent](#agent)). Debug dumps go to `{WorkingDir}/debug` unless you set `ClaudeOpts.TempDir` / `CodexOpts.TempDir`.
- **Each turn**, every action the agent returns goes through `preset.Check`. The first one the preset doesn't allow is refused (reported back as `refused: <reason>`) and the batch stops there, just as it stops at the first action that fails while running.
- **`Result`** is filled in as far as the run got, even when an error comes back: `Completed`, `Answer`, `Turns`, `SessionID`, `Dirs`, and the cost and tokens summed over every turn. Reaching `MaxTurns` without finishing returns an error wrapping `browser.ErrTurnLimit`.

### Running it yourself

Each layer under `Run` is public, and the higher ones are built only from the lower ones:

| Layer | Owns the browser? | Prompt |
|-------|-------------------|--------|
| `browser.Run(req)` | yes: opens and closes it | optional (ours by default) |
| `browser.Loop(b, req, dirs)` | no: you `session.Open` and `session.Close` | optional (ours by default) |
| `browser.Perform(b, preset, actions)` | no | none |
| `agent.Initial` / `agent.Subsequent` | never touches a browser | **required** |
| `session.Open` / `Perform` / `Snapshot` / `Close` | `Open` and `Close` do | none |

```go
dirs, _ := browser.ResolveDirs(opts)
b, err := session.Open(session.Opts{ /* VideoDir, ArtifactsDir, … */ })
if err != nil { … }
defer session.Close(b)

_ = session.Navigate(b, "https://example.com")
res, err := browser.Loop(b, req, dirs)
```

## session

One `*session.Browser` is one live browser: its own Playwright driver, browser, tabs and directories.

- **`Open(opts)`** launches Chromium or Firefox. With a `UserDataDir` it uses a persistent context; without one, a fresh context with no profile directory at all. Playwright's video recording is always on. Options are checked before anything launches, and anything half-started is shut down again on failure.
- **`Close(b)`** closes the context (which finishes the video), the browser and the driver. Closing twice is fine.
- **`Perform(b, actions)`** runs `action.Validate` on every action, performs them in order, and stops at the first failure. Failures come back in each action's `Reason`, not as an error, so they can go straight to the agent. After each action that touches the page it waits for the page's load event. `javascript` returns its result as JSON.
- **`Snapshot(b, perception, dir)`** captures only what the preset perceives. A preset that perceives neither screenshot nor DOM (e.g. `Blind()`) writes no files at all.
- **`Tabs`, `Focused`, `Navigate`.** `Navigate` is for you (e.g. the start page), so unlike the agent's `goTo` it accepts any URL.

Artifacts are reached only through an `os.Root` on the artifacts directory, so neither a crafted name nor a planted symlink can reach outside it. Creating an artifact that exists fails. `readArtifact` refuses binary files and files over 1 MB.

## agent

```go
turn, err := agent.Initial(agent.InitialRequest{
    Prompt:  agent.DefaultInitialPrompt, // required: ours, or your own template
    Task:    "…",
    Preset:  preset.Default(),
    State:   state,                      // from session.Snapshot
    Request: aclio.Request{Provider: aclio.Claude, Dir: agentDir},
})
// turn.Response.TaskComplete, turn.Response.Actions, turn.Response.Answer, turn.SessionID
```

- **The prompt is required** at this level; `DefaultInitialPrompt` and `DefaultSubsequentPrompt` are ours to pass in. `RenderInitial` / `RenderSubsequent` return the prompt text without running anything.
- **Templates** may use any subset of the placeholders, which are exported as constants: `{task}` (required in the initial template), `{page_state}`, `{response_schema}`, `{examples}` and `{artifacts}` for the first turn; `{performed_actions}` and `{page_state}` after that. Any other `{snake_case}` token is an error, so a typo fails instead of reaching the model.
- **The agent's tools are fixed, whatever the request asks for.** Claude gets only `Read(./**)`, the Read tool limited to its working directory. Codex runs in a read-only sandbox. Your `ClaudeOpts` / `CodexOpts` are copied, never modified.
- **Snapshot paths must be inside the agent's working directory.** They are shown to the agent relative to it, and a path outside it fails the turn up front, since the agent couldn't read it.
- **Examples** shown to the agent are filtered with `preset.Check`, so it never sees an example it isn't allowed to follow. `DefaultExamples()` are ours; pass your own in `InitialRequest.Examples`.

## action

Every action has exactly one payload shape, and every JSON name is camelCase.

| Group     | Types |
|-----------|-------|
| Pointer   | `click`, `doubleClick`, `hover`, `mouseDown`, `mouseUp`, `scroll` |
| Keyboard  | `type` (keystrokes, appends), `fill` (replaces an input's value), `press` (a key or chord like `Shift+Tab`) |
| Page      | `wait`, `goTo`, `javascript` |
| Tabs      | `tabOpen`, `tabSwitch`, `tabClose` |
| Artifacts | `screenshot`, `createArtifact`, `readArtifact`, `listArtifacts`: create-only files in the artifacts directory (never edited or overwritten) |

Elements are picked with a `Target`: a CSS `selector` **or** a viewport `point`, never both.

```go
// What the agent may emit: any mix of types, plus which target kinds it may use.
schema, err := action.Schema(
    []action.Type{action.TypeClick, action.TypeType, action.TypeGoTo},
    action.Targeting{Selector: true},
)

// What comes back is decoded from {"type": "...", "data": {...}}.
var a action.Action
err = json.Unmarshal(raw, &a)

// Always validate before performing.
if err := action.Validate(a); err != nil {
    // report err back to the agent instead of performing a
}
```

`Validate` checks that the payload matches the type and follows that type's rules. Two of them are safety rules, so they apply whatever the agent sends:

- **`goTo` only navigates to http(s) or relative URLs.** `javascript:` (including spellings padded with whitespace or tabs), `file:`, `data:`, `chrome:` and every other scheme are refused, so turning off the `javascript` action can't be bypassed through navigation. A relative URL must also resolve to http(s), so it can't inherit `about:` or `file:` from the current page.
- **Artifact file names must be plain names.** No path separators of either kind, no `:`, no `.`/`..`, no NUL, at most 255 bytes (including `.png` for screenshots). `"../../.bashrc"` is refused.

## preset

A `Preset` is plain data you can inspect, copy and build yourself:

```go
type Preset struct {
    Name      string
    Actions   []action.Type      // what the agent may do
    Targeting action.Targeting   // selector and/or point
    Perceive  preset.Perception  // screenshot, DOM, focused element, tabs
    Describe  func(preset.State) string // how the page state is put to the agent
}
```

### Built-ins

| Preset      | Actions | Targeting | Perceives |
|-------------|---------|-----------|-----------|
| `Default()` | everything except the artifact actions (`javascript` included) | selector, point | screenshot, DOM, tabs |
| `Full()`    | everything | selector, point | screenshot, DOM, tabs |
| `Blind()`   | `type`, `press`, `wait`, tabs | none | focused element (screen-reader view), tabs |
| `Motor()`   | `Blind`'s actions plus `goTo` | none | screenshot, DOM, tabs |

Each built-in is a function that returns a fresh copy, so changing yours never affects anyone else's:

```go
p := preset.Default()
p.Name = "no-scripts"
p.Actions = slices.DeleteFunc(p.Actions, func(t action.Type) bool {
    return t == action.TypeJavascript
})

blindWithFiles := preset.WithArtifacts(preset.Blind()) // + screenshot, createArtifact, readArtifact, listArtifacts
```

Or build one from scratch, reusing the built-in describers:

```go
p := preset.Preset{
    Name:      "nav-only",
    Actions:   []action.Type{action.TypeGoTo, action.TypeClick, action.TypeWait},
    Targeting: action.Targeting{Selector: true},
    Perceive:  preset.Perception{DOM: true, Tabs: true},
    Describe:  preset.Combine(preset.DescribeFiles, preset.DescribeTabs),
}
if err := preset.Validate(p); err != nil { … }
```

`preset.Validate` catches an unknown or duplicate action, an action that needs a target when `Targeting` allows none, and a missing name or `Describe`.

`preset.Check(p, a)` is the one call to make on each returned action. It returns an error explaining why `a` can't be performed under `p`: a type `p` doesn't offer, an invalid payload, or a target kind `p` doesn't allow. That error is meant to go back to the agent.

`preset.UsesArtifacts(p)` reports whether a run with `p` uses the artifacts directory.

## Options

```go
type Options struct {
    Browser      browser.Browser // browser.Chromium (default) or browser.Firefox
    WorkingDir   *string         // nil: a new dir under the system temp dir
    ArtifactsDir *string         // nil: {WorkingDir}/artifacts
    UserDataDir  *string         // nil: anonymous session, no profile dir anywhere
    Headed       bool            // show the browser window; headless by default
    Viewport     *browser.Size   // nil: browser.DefaultViewport() (1280x720)
}
```

- **`Browser`** is the browser to drive. The zero value means Chromium. (The type lives in the `engine` package and is re-exported from the root.)
- **`WorkingDir`** holds everything the run needs on disk (see the layout below). When it's nil, a new directory `aclio-go-browser-{UTC timestamp}-{random}` is created under the system temp dir. The timestamp is fixed-width, so listing these directories alphabetically lists them oldest first.
- **`ArtifactsDir`** is the only place the `screenshot`, `createArtifact`, `readArtifact` and `listArtifacts` actions can reach, and the agent only ever reaches it through them, never with a file tool. Artifacts are create-only: nothing edits, overwrites or deletes one. The directory is created on the first write.
- **`UserDataDir`** is a browser profile to run with, so logins and cookies carry over between runs. A profile belongs to one browser, so don't share a directory between Chromium and Firefox. When it's nil the run is anonymous: a fresh, non-persistent browser context with no profile directory at all, not even a temporary one.
- **`Viewport`** is the page size in CSS pixels, and also the size of the recorded video.
- **Nothing is ever deleted.** Working directories, artifacts and recordings all stay after the run; cleaning them up is your call.

Set a path field to choose it, or leave it nil for the default. A pointer to `""` is an error, not a default. `browser.ResolveDirs(opts)` returns the absolute paths a run will use.

### Working directory layout

```
{WorkingDir}/
├── agent/          the agent's working directory: the only place it can read
│   └── turns/      each turn's {UTC timestamp}.png and .html, if the preset perceives them
├── video/          Playwright's recording of the run
├── debug/          the CLI's per-call debug dumps (unless you set your own TempDir)
└── artifacts/      the artifacts, unless ArtifactsDir points elsewhere
```

## browserinstall

A run needs the Playwright driver (Node.js plus playwright-core) and its browser on disk. Nothing downloads them behind your back: call `Ensure` once, before the first run.

```go
err := browserinstall.Ensure(browserinstall.Opts{}) // driver + Chromium

err := browserinstall.Ensure(browserinstall.Opts{
    Browsers: []engine.Browser{engine.Chromium, engine.Firefox},
})
```

- `Opts.Browsers` empty means Chromium only. WebKit is never installed. An unsupported browser fails before anything is downloaded.
- When everything is already installed, `Ensure` only runs a quick version check.
- Downloads go to Playwright's default cache locations. Set `PLAYWRIGHT_DRIVER_PATH` / `PLAYWRIGHT_BROWSERS_PATH` to move them; the browser layer reads the same variables.
- `Opts.Output` receives the progress output (nil means stderr; `io.Discard` silences it). `Opts.WithSystemDeps` also installs the browsers' OS packages, on Linux only, and usually needs root.
- It's safe to call from several goroutines; calls run one at a time.
- It never touches your process's global `log` output.

## Concurrency

There is no mutable package-level state, apart from the lock that makes `browserinstall.Ensure` calls run one at a time. Every `session.Open` (and so every `Run`) starts its own Playwright driver and browser, so separate runs share nothing and can go in separate goroutines. Runs that use the default `WorkingDir` each get their own directory, even if they start in the same nanosecond. Give each run its own `Agent.LogWriter` / `Agent.Stderr` if you want their output apart.

A single `*session.Browser` must be used by one goroutine at a time.

## Testing

`go test ./...` needs neither a browser nor an agent CLI. The tests that drive real Chromium and Firefox are opt-in:

```
ACLIO_BROWSER_E2E=1 go test ./session/...
```

They need `browserinstall.Ensure` to have installed both browsers. CI doesn't run them.

## Requirements

- Go 1.26+
- [aclio-go](https://github.com/AgenticCLIOrchestra/aclio-go) and [playwright-go](https://github.com/mxschmitt/playwright-go) (pulled in by `go get`)
- The Playwright driver and Chromium and/or Firefox, via `browserinstall.Ensure`
- The `claude` or `codex` CLI for the agent (aclio-go's `claudeinstall` / `codexinstall` can install them)

## Versioning & releases

[SemVer](https://semver.org), tracked in the `VERSION` file, with a [Keep a Changelog](https://keepachangelog.com) `CHANGELOG.md`. Merges to master tag `v{VERSION}` and publish a GitHub release with that version's changelog entry; CI enforces the version bump and changelog format on every PR.

Pre-1.0: the API may still move between minor versions.

## License

MIT
