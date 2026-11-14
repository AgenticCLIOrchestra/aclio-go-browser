# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.1]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/releases/tag/v0.1.0
