# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.9.0] - 2026-11-18

### Added
- `preset.FocusedElement.Name` / `NameSources` and `preset.ActiveDescendant.Name` / `NameSources`, read from Chromium's own accessibility tree like `Role`. `name` is the one accessible name a screen reader announces, as Chromium computes it in the spec's order of precedence (`aria-labelledby`, `aria-label`, a `<label>`, contents, `title`, `placeholder`, …); `""` means the element has no accessible name. `nameSources` lists every source that had a value, in that order, as `{from, value, superseded}`, so it shows where the name came from and what it outranked (e.g. `{"from":"labelfor","value":"Asc"}`, then `{"from":"title","value":"t","superseded":true}`). Both are omitted when the element is left out of the accessibility tree
- `preset.NameSource`
- `DefaultInitialPrompt` explains `role`, `name` and `nameSources` to the agent

### Removed
- `preset.FocusedElement.Label`, `LabelledBy`, `Title` and `Placeholder`, and `preset.ActiveDescendant.LabelledBy`. Each one's value is in `NameSources` whenever it is present (`labelfor` / `labelwrapped`, `aria-labelledby`, `title`, `placeholder`), outranked or not

## [0.8.0] - 2026-11-18

### Removed
- `preset.FocusedElement.Tag`, `preset.FocusedElement.Type` and `preset.ActiveDescendant.Tag`, and the `tag` / `type` fields of the focused-element view (e.g. in the `Blind` preset). A screen reader is never given an element's tag name or input type, only its role, which `Role` now carries. Some native elements that used to be told apart by tag or type now look the same, as they do to a screen reader: every text-like input is `textbox`, for example. On blind runs the full DOM is still saved to the debug dir

## [0.7.0] - 2026-11-18

### Changed
- The focused element's `role` (and its active descendant's) is now the computed role, read from Chromium's own accessibility tree: the one a screen reader is given. It is the explicit `role` attribute when valid, otherwise the element's implicit role (a `<button>` is `button`, a radio input `radio`, a `<select>` `combobox`, a plain `<div>` `generic`). It used to be only the raw `role` attribute, so native elements had none. An invalid role token is skipped as browsers do (`role="bogus button"` is `button`), and a focusable `role="presentation"` is `generic`. `role` is omitted when the element is left out of the accessibility tree. Nothing else in the focused-element view changes
- `session.Focused` now needs Chromium and returns an error on Firefox, since the roles come from Chromium's accessibility tree

## [0.6.0] - 2026-11-18

### Changed
- `Run` and `Loop` refuse Firefox for a preset that perceives the focused element (e.g. `Blind`). Its screen-reader view is to come from Chromium's own accessibility tree, which Firefox doesn't expose. `Run` refuses before creating any directory or launching the browser; `Loop` refuses before the first turn. Presets that don't perceive the focused element (`Default`, `Full`, `Motor`) still run on Firefox

## [0.5.0] - 2026-11-18

### Added
- When the preset doesn't perceive the DOM (e.g. `Blind`), `Run` and `Loop` still save the DOM at each snapshot, before the first turn and after every turn, to the agent's debug dir, out of the agent's reach. The file is named `{stamp}-{name}-dom.html` like aclio-go's own debug dumps (local time, millisecond precision, the normalised run name), so it sorts among them in order, and is formatted exactly like a perceived DOM snapshot. The debug dir is `{WorkingDir}/debug` unless you set `ClaudeOpts.TempDir` / `CodexOpts.TempDir`. Writing it is best-effort, like the CLI's dumps: a failure never fails the run. Presets that perceive the DOM are unchanged
- `session.WriteDOM`, which writes the active tab's DOM to a path, formatted exactly as `Snapshot` formats it

## [0.4.0] - 2026-11-17

### Added
- `preset.FocusedElement` reports native labelling and state: `Label` (the text of its `<label>`s, `for=` or wrapping), `Legend` (the nearest enclosing fieldset's `<legend>`), `SelectedText` (a `<select>`'s selected options) and `Checked`, `Indeterminate`, `Selected`, `Disabled`, `Required` and `ReadOnly`. `ActiveDescendant` gets the same states. States come from the live DOM properties, so they reflect what the user did. `Checked` and `Selected` are `*bool` and set only on checkboxes, radios and `<option>`s, where `false` means unchecked or unselected; the other flags are omitted when false. `DefaultInitialPrompt` explains the new fields to the agent

### Fixed
- The focused-element view (e.g. in the `Blind` preset) no longer shows a natively labelled control as unnamed and stateless: a radio with a `<label for>` and `checked` used to come back with no name and no checked state, because only text content, `aria-labelledby` and `aria-*` attributes were read
- A focused `<select>` no longer reports every option as its `text`; what is chosen is in `SelectedText`

## [0.3.0] - 2026-11-14

### Changed
- `goTo` (and `action.ValidateURL`) now allows `about:blank`, an empty page, as well as http(s) and relative URLs. Only exactly `about:blank` is allowed: every other `about:` page (e.g. Firefox's `about:config`) is still refused, as are `javascript:`, `file:`, `data:` and every other scheme. A relative `goTo` still needs an http(s) page to resolve against. The `goTo` action's schema description now mentions `about:blank`

## [0.2.0] - 2026-11-14

### Changed
- Claude's agent now gets the Grep tool as well as Read, both limited to its working directory (`Grep(./**)` alongside `Read(./**)`), so it can find what it needs in a large DOM snapshot before reading around it. As before, the agent's tools are fixed whatever the request asks for. `DefaultInitialPrompt` now tells the agent to grep a large DOM first, then read in chunks around the matches. Codex is unchanged (read-only sandbox)

## [0.1.3] - 2026-11-14

### Fixed
- DOM snapshots are formatted again, with each element on its own indented line (via `github.com/yosssi/gohtml`). Pages often come back from the browser as a few enormous lines, which the agent couldn't read by line range: Claude's `Read` tool cuts long lines short, so most of such a page was invisible to it. Whitespace inside text (e.g. in `<pre>`) may be re-indented; the structure is unchanged

## [0.1.2] - 2026-11-14

### Changed
- Bumped `github.com/agenticcliorchestra/aclio-go` from v0.11.3 to v0.12.0. No API changes here, and the agent's log output is unchanged. aclio-go's new event handlers work through the existing `Request.Agent`: set `Agent.EventHandlerFactory` to filter or rewrite the agent's log lines. Each agent turn is a separate CLI run, so the factory is called once per turn, not once per task

## [0.1.1] - 2026-11-14

### Changed
- Bumped `github.com/agenticcliorchestra/aclio-go` from v0.11.0 to v0.11.3, which parses Claude's stream events into typed events internally. No API changes; the agent's log output is unchanged except that a malformed Claude `init` event is now logged raw instead of as the `[system] init` marker

## [0.1.0] - 2026-11-12

### Added
- Module `github.com/agenticcliorchestra/aclio-go-browser`, depending on `github.com/agenticcliorchestra/aclio-go` v0.11.0
- `browser.Run` — one call does a whole task: checks the request before touching the disk or launching anything, resolves the dirs, opens the browser and the `StartURLs` (one tab each), runs the agent turn by turn until it reports the task done or `MaxTurns` (default 50, `ErrTurnLimit`) is reached, and always closes the browser. `Request` takes the task, an optional `Preset` (default `preset.Default()`), the agent's `aclio.Request` (provider, model, name, streaming, `LogWriter`, `Stderr`, …) and optional prompt templates (ours by default). `Result` reports `Completed`, `Answer`, `Turns`, `SessionID`, `Dirs` and the cost and tokens summed over every turn, filled in as far as the run got even on error
- `browser.Loop` (the same loop on a browser the caller opens and closes) and `browser.Perform` (every action goes through `preset.Check`; the first refused one is reported back as `refused: <reason>` and stops the batch)
- `session` package — one live browser and the only package that talks to Playwright: `Open` (Chromium or Firefox; a persistent context with `UserDataDir`, otherwise one with no profile dir at all; Playwright video always recorded; options checked before launching) and `Close`; `Perform` (validates every action, stops at the first failure, reports failures as reasons rather than errors, waits for the load event after page actions); `Snapshot` (captures only what the preset perceives, writing no files for a preset that perceives no screenshot or DOM); `Tabs`, `Focused` and `Navigate`. Artifacts are reached only through an `os.Root`, so names and symlinks can't escape the artifacts dir; creates are exclusive; `readArtifact` refuses binary files and files over 1 MB. A relative `goTo` must resolve to http(s). No package-level state
- `agent` package — the agent's side of each turn: `Initial` and `Subsequent` (the prompt template is required; `DefaultInitialPrompt` / `DefaultSubsequentPrompt` are ours), `RenderInitial` / `RenderSubsequent`, `ResponseSchema`, `Response` (`taskComplete`, `actions`, `answer`), exported placeholder constants (a template may use any subset; unknown placeholders are an error), and `DefaultExamples` / `ExamplesFor` / `RenderExamples`, with examples filtered by `preset.Check`. The agent's tools are fixed whatever the request asks for: Claude gets only `Read(./**)` and Codex a read-only sandbox, and snapshot paths outside the agent's working dir fail the turn
- `action` package — every action an agent may request (`click`, `doubleClick`, `hover`, `mouseDown`, `mouseUp`, `scroll`, `type`, `fill`, `press`, `wait`, `goTo`, `tabOpen`, `tabSwitch`, `tabClose`, `javascript`, `screenshot`, `createArtifact`, `readArtifact`, `listArtifacts`) as a `{type, data}` union with one payload shape per type and camelCase JSON throughout; a shared `Target` (CSS selector or viewport point); `Schema` for any mix of types under a `Targeting`; `Validate`, which every caller should run before performing an action (target shape, wait bounds, http(s)-only `goTo` URLs, plain artifact file names that can't escape their directory); `CheckTargeting`; `Compact`. Artifacts are create-only: no action edits, overwrites or deletes one
- `preset` package — the public `Preset` type (actions, targeting, perception, state description) with `Validate`, `Schema`, `Allows`, `Check`, `UsesArtifacts` and `WithArtifacts`; built-ins `Default` (everything in-browser, including javascript), `Full` (`Default` plus the artifact actions), `Blind` and `Motor` (keyboard only), each returned fresh; page-state types (`State`, `Tab`, `FocusedElement`) and reusable `Describe*` renderers with `Combine`
- `engine` package — the string-based `Browser` enum (`Chromium`, `Firefox`; zero value means `Chromium`) with `All` and `Resolve`; re-exported from the root package as `browser.Browser`, `browser.Chromium` and `browser.Firefox`
- `Options` and `ResolveDirs`. `Browser` selects Chromium (default) or Firefox. `WorkingDir`, `ArtifactsDir` and `UserDataDir` are optional `*string`; `Headed` is off by default; `Viewport` is an optional `*Size`, nil meaning `DefaultViewport()` (1280x720). `Size` is re-exported from `session`. A nil `WorkingDir` creates `aclio-go-browser-{UTC timestamp}-{random}` under the system temp dir, so the names sort chronologically. A nil `ArtifactsDir` means `{WorkingDir}/artifacts`, created on the first write. A nil `UserDataDir` means an anonymous, non-persistent session with no profile directory. Every run keeps Playwright's video recording in the working dir. Nothing a run writes is ever deleted
- `browserinstall` package — opt-in `Ensure` that installs the Playwright driver and the requested browsers (Chromium by default, Firefox on request, never WebKit) only when missing (`playwright-go` v0.6401.0), with optional Linux system deps. Calls are serialised, and it never redirects the caller's global `log` output
- `README.md`
- MIT `LICENSE`
- CI: PR checks (Go build, vet + test, changelog and version validation) and a post-merge release pipeline (git tag, GitHub release with the version's changelog entry, Go proxy trigger)

[0.9.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.1.3...v0.2.0
[0.1.3]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.1.2...v0.1.3
[0.1.2]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/releases/tag/v0.1.0
