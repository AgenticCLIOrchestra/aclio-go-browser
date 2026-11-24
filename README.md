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
- **`Agent`** is an ordinary aclio-go request. You choose the provider, model, run name, streaming, log writer and the CLI's stderr; this library sets the working directory, the prompt, the schema and the session the task runs on, and **always** sets the agent's tools (see [agent](#agent)). Debug dumps go to `{WorkingDir}/debug` unless you set `ClaudeOpts.TempDir` / `CodexOpts.TempDir`.
- **Each turn**, every action the agent returns goes through `preset.Check`. The first one the preset doesn't allow is refused (reported back as `refused: <reason>`) and the batch stops there, just as it stops at the first action that fails while running.
- **One session per task.** `Run` and `Loop` open one aclio-go session for the whole task (`agent.StartSession`) and run every turn on it. With Claude, one `claude` process serves every turn, instead of a new process resuming the conversation each turn; if it dies, aclio-go resumes the conversation in a new one. With Codex, each turn is still a run resuming the previous thread.
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

- **`Open(opts)`** launches Chromium or Firefox. With a `UserDataDir` it uses a persistent context; without one, a fresh context with no profile directory at all. Playwright's video recording is always on. Options are checked before anything launches, and anything half-started is shut down again on failure. With `Opts.ConsoleLog` / `Opts.NetworkLog` set it keeps a [console log](#console-log) / [network log](#network-log).
- **`Close(b)`** closes the context (which finishes the video), the browser and the driver. Closing twice is fine.
- **`Perform(b, actions)`** runs `action.Validate` on every action, performs them in order, and stops at the first failure. Failures come back in each action's `Reason`, not as an error, so they can go straight to the agent. After each action that touches the page it waits for the page's load event. `javascript` returns its result as JSON.
- **`Snapshot(b, perception, dir)`** captures only what the preset perceives. A preset that perceives neither screenshot nor DOM (e.g. `Blind()`) writes no files at all.
- **`LogTurn(b, n)`** writes a turn marker to the console and network logs; it does nothing when there are none.
- **`StartAnnouncing(b, language)` / `TakeAnnouncements(b)`** record what the screen readers say as it happens (focus changes and live regions), and hand it over as `[]preset.Announcement`, each item marked with whether VoiceOver, NVDA or both spoke it (emptying the store); see [Screen reader announcement](#screen-reader-announcement). Chromium only.
- **`Tabs`, `Focused`, `Navigate`.** `Navigate` is for you (e.g. the start page), so unlike the agent's `goTo` it accepts any URL. `Focused` needs Chromium: the focused element's `role`, `name`, `nameSources`, `value`, `description`, states and properties and `context` (and, except `context`, its active descendant's) come from Chromium's own accessibility tree, the one a screen reader is given. `role` is the explicit role when valid and otherwise the implicit one (a `<button>` is `button`, a plain `<div>` `generic`). For the few elements Chromium gives one of its own internal roles instead of an ARIA one, the nearest ARIA role is reported: `textbox` for the date and time inputs, `button` for `<summary>` and the colour input, and `generic` for those with no corresponding ARIA role (`<video>`, `<audio>`, `<canvas>`, `<iframe>`, `<label>`, …). `name` is the one accessible name a screen reader announces (`""` when there is none), and `nameSources` lists every source that had a value (`aria-labelledby`, `aria-label`, `labelfor`, `contents`, `title`, …) in precedence order, marking the outranked ones `superseded`. `value` is what a screen reader reads as the value: the text in a field (masked for a password), a drop-down's selected text, a slider's number; it is absent for checkboxes, radios, buttons and links. `description` is the accessible description a screen reader reads after the rest: the text of the `aria-describedby` targets (even hidden ones), else `aria-description`, else a `title` the name didn't already use. The states and properties (`preset.AXState`: `checked` as `"true"`, `"false"` or `"mixed"`, `pressed`, `selected`, `expanded`, `disabled`, `readOnly`, `required`, `invalid`, `level`, `hasPopup`, `multiselectable`, `orientation`, `valueMin` / `valueMax` / `valueText`, `autocomplete`, `keyShortcuts`, `roleDescription`, `modal`, `ignored` / `ignoredReasons`) are the same for native and ARIA markup and reflect what the user did. Chromium leaves `required` out for some controls (e.g. a required checkbox), so the DOM's `required` / `aria-required` stands in then. An element the tree leaves out (`ignored`, e.g. an active descendant with `aria-hidden` or `display:none`) is reported as only that and its `ignoredReasons`, since a screen reader announces nothing for it. Chromium never leaves out the focused element itself, even inside `aria-hidden`. `context` is the focused element's ancestors in the tree, outermost first, as `{role, name}`: the landmarks, groups, lists, tables and dialogs a screen reader announces when focus moves into them (e.g. `main`, `form` "Prefs", `group` "Sort by" for a radio in a fieldset). Generic wrappers, ignored nodes and the document itself are left out. A screen reader announces only the ones focus has just entered; `context` has them all, every turn. `aria` keeps only the aria-* attributes none of these cover (e.g. `aria-current`, `aria-valuetext`, `aria-setsize`), which the tree doesn't report.

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
- **Templates** may use any subset of the placeholders, which are exported as constants: `{task}` (required in the initial template), `{page_state}`, `{response_schema}`, `{examples}`, `{artifacts}` and `{screen_reader}` for the first turn; `{performed_actions}` and `{page_state}` after that. Any other `{snake_case}` token is an error, so a typo fails instead of reaching the model. `{artifacts}` and `{screen_reader}` are empty when the preset has nothing for them: `{screen_reader}` explains the screen readers' output and the focused element only to a preset that perceives the focused element, so Default and Motor aren't told about screen readers they don't hear.
- **The agent's tools are fixed, whatever the request asks for.** Claude gets only `Read(./**)` and `Grep(./**)`, the Read and Grep tools limited to its working directory. Codex runs in a read-only sandbox. Your `ClaudeOpts` / `CodexOpts` are copied, never modified.
- **Snapshot paths must be inside the agent's working directory.** They are shown to the agent relative to it, and a path outside it fails the turn up front, since the agent couldn't read it.
- **Sessions.** `agent.StartSession(request, preset)` opens an aclio-go session for a whole task, with the agent's tools and the preset's response schema fixed as for a single turn. Set it as `InitialRequest.Session` and `SubsequentRequest.Session` to run every turn on it (`SubsequentRequest.SessionID` is then not needed), and close it with `aclio.CloseSession`. `Request.Dir` must still be the session's working dir, since snapshot paths are made relative to it. Without a session, each turn is its own CLI call, and `Subsequent` resumes `SessionID`.
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

- **`goTo` only navigates to http(s) URLs, `about:blank` or relative URLs.** `javascript:` (including spellings padded with whitespace or tabs), `file:`, `data:`, `chrome:`, every other `about:` page (e.g. Firefox's `about:config`) and every other scheme are refused, so turning off the `javascript` action can't be bypassed through navigation. A relative URL must also resolve to http(s), so it can't inherit `about:` or `file:` from the current page.
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
    Language  preset.Language    // the screen reader's wording: LanguageEnUS (default) or LanguageEnGB
}
```

### Built-ins

| Preset      | Actions | Targeting | Perceives |
|-------------|---------|-----------|-----------|
| `Default()` | everything except the artifact actions (`javascript` included) | selector, point | screenshot, DOM, tabs |
| `Full()`    | everything | selector, point | screenshot, DOM, tabs |
| `Blind()`   | `type`, `press`, `wait`, tabs | none | what two screen readers say, VoiceOver and NVDA (focused element), tabs |
| `Motor()`   | `Blind`'s actions plus `goTo` | none | screenshot, DOM, tabs |

**`Blind()` runs on Chromium only.** Any preset that perceives the focused element (`Blind()` and every copy of it, renamed or not) is refused on Firefox: its screen-reader view comes partly from Chromium's own accessibility tree (the computed `role`, the accessible `name`, the `value`, the `description`, the states and properties and the `context`), which Firefox doesn't expose. `Run` refuses before creating any directory or launching the browser; `Loop` refuses before the first turn. `Default()`, `Full()` and `Motor()` run on either browser.

### Screen reader announcement

`Blind()` is told only what two screen readers, VoiceOver and NVDA, said since its last turn (`State.Announcement`, rendered by `DescribeAnnouncement`), plus the open tabs: one spoken item per line, in order, VoiceOver's feed first, then NVDA's.

The two feeds are the same except for one thing: in a live region, nodes replaced by nodes with identical text (a re-render that remounts a chat log's messages, say) are spoken by NVDA and not by VoiceOver. NVDA in Chrome reads every node inserted into a live region, while VoiceOver skips a change whose text is identical. So a page that re-announces its whole log on every re-render, a real defect for NVDA users, is heard. Each `preset.Announcement` in `State.Announcement` is one spoken item with its `Text` and whether `VoiceOver` and `NVDA` spoke it; `preset.VoiceOverSaid` and `preset.NVDASaid` give each feed. Both use the VoiceOver-modelled wording below. For example:

```
VoiceOver said, in order:
- Ask
- button
NVDA said, in order:
- Q A
- Ask
- button
```

When NVDA said the same, its part is one line, `NVDA said the same.`

It is recorded as it happens, not worked out from where the page ends up: `Run` and `Loop` call `session.StartAnnouncing` for a preset that perceives the focused element. From then on the page queues the element focused at every `focusin`, and at every change to the focused element or its active descendant (`input`, `change`, and the attributes a screen reader announces). Go turns each into an announcement, in the background as the page signals, after each action `Perform` runs, and when a turn takes them with `session.TakeAnnouncements`, which also empties the store. So a batch of actions (Tab, Tab, Space, …) is heard step by step, and so are changes the page makes by itself, such as a dialog taking focus while the agent is thinking. Switching tabs announces where focus is in the new one. Focus inside iframes isn't heard yet.

Live regions are heard too, in order with the focus announcements: what the page announces without moving focus, such as search result counts, "Saved" toasts and form errors. A live region is an element with `aria-live="polite"` or `"assertive"`, or the role `status` or `log` (polite) or `alert` (assertive), inherited from the nearest ancestor that sets one; an explicit `aria-live` wins over the role, and `aria-live="off"`, `timer` and `marquee` are silent. They follow the screen-reader rules:

- **What is spoken:** only changes `aria-relevant` covers (by default `additions text`: the added content and the changed text; `removals` only when listed) that change what is rendered. With `aria-atomic="true"` (the default for `alert` and `status`) such a change speaks the whole region; otherwise only the change itself. Changes in the same region from one task are spoken once, so a re-render isn't read word by word.
- **What is silent:** hidden content (`display:none`, `visibility:hidden`, `hidden`, `aria-hidden="true"`), blank text, replacing text with identical text (compared exactly, so a repeat an app forces with an NBSP or a thin space is spoken; replacing nodes is still spoken to NVDA, as above, while setting a text node to the same value is silent for both), a region under `aria-busy="true"` (spoken once, whole, when it stops being busy), and a region inserted already filled, except an `alert`, which is spoken, as is one revealed or given the `alert` role. What the page shows while it first loads is silent too. One known limit: a removed node is already detached, so its CSS can't be read, and removing stylesheet-hidden text from a region whose `aria-relevant` lists `removals` is spoken (text hidden with `hidden` or `aria-hidden` stays silent). An extra announcement is the safer mistake than a missed one.
- **What is heard:** the text alone, as a screen reader says it, whatever the politeness, each announcement capped at 1024 characters. Chatty regions aren't throttled: they are recorded in full, within the page queue's limit of 1000 entries.

Only the active tab's main frame is heard: live regions in background tabs and iframes are silent.

Each focus announcement comes from `preset.Announce(cur, prev, language)`, which behaves like a real screen reader, speaking only about what happened:

- **Focus moved** (or nothing was focused before): the context focus has just entered, then the element, then its active descendant if it has one. The context is the landmarks, groups, lists and dialogs not shared with the previous focus, each as its name and role. The element is its name, value, states, role, position ("2 of 5") and description.
- **Focus stayed on the same element:** only what changed: the new value, the states it just gained (e.g. `ticked` after Space on a checkbox), or the new active item when arrowing through a listbox.
- **Nothing changed, nothing focused, or the element is hidden from screen readers:** nothing. When neither screen reader said anything since the last turn, the agent is told so.

For example, tabbing into a radio group and then pressing the down arrow:

```
main · Prefs · form · Sort by · group · Ascending · selected · radio button · 1 of 2
Descending · selected · radio button · 2 of 2
```

The wording is modelled on VoiceOver ("edit text", "pop-up button", "dimmed", "heading level 2"). `Preset.Language` picks US English (`preset.LanguageEnUS`, the default: "checkbox", "checked") or British English (`preset.LanguageEnGB`: "tick box", "ticked"); `preset.Validate` refuses any other. The position comes from `aria-posinset` / `aria-setsize` when the page sets them, otherwise it is counted from the DOM (radios with the same name in the same form, or visible siblings with the same role), since Chromium's accessibility tree doesn't report it.

The raw fields the announcement is built from stay in `State.Focused` (see [session](#session)), for you and for custom presets: `DescribeFocused` renders them as JSON.

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

`preset.Validate` catches an unknown or duplicate action, an action that needs a target when `Targeting` allows none, a missing name or `Describe`, and an unsupported `Language`.

`preset.Check(p, a)` is the one call to make on each returned action. It returns an error explaining why `a` can't be performed under `p`: a type `p` doesn't offer, an invalid payload, or a target kind `p` doesn't allow. That error is meant to go back to the agent.

`preset.UsesArtifacts(p)` reports whether a run with `p` uses the artifacts directory.

## Options

```go
type Options struct {
    Browser      browser.Browser // browser.Chromium (default) or browser.Firefox (not with Blind)
    WorkingDir   *string         // nil: a new dir under the system temp dir
    ArtifactsDir *string         // nil: {WorkingDir}/artifacts
    UserDataDir  *string         // nil: anonymous session, no profile dir anywhere
    Headed       bool            // show the browser window; headless by default
    Viewport     *browser.Size   // nil: browser.DefaultViewport() (1280x720)
    NoConsoleLog bool            // don't keep the console log in the debug dir
    NoNetworkLog bool            // don't keep the network log in the debug dir
    NetworkLogAll bool           // the network log records every request, not just failures
}
```

- **`Browser`** is the browser to drive. The zero value means Chromium. (The type lives in the `engine` package and is re-exported from the root.)
- **`WorkingDir`** holds everything the run needs on disk (see the layout below). When it's nil, a new directory `aclio-go-browser-{UTC timestamp}-{random}` is created under the system temp dir. The timestamp is fixed-width, so listing these directories alphabetically lists them oldest first.
- **`ArtifactsDir`** is the only place the `screenshot`, `createArtifact`, `readArtifact` and `listArtifacts` actions can reach, and the agent only ever reaches it through them, never with a file tool. Artifacts are create-only: nothing edits, overwrites or deletes one. The directory is created on the first write.
- **`UserDataDir`** is a browser profile to run with, so logins and cookies carry over between runs. A profile belongs to one browser, so don't share a directory between Chromium and Firefox. When it's nil the run is anonymous: a fresh, non-persistent browser context with no profile directory at all, not even a temporary one.
- **`Viewport`** is the page size in CSS pixels, and also the size of the recorded video.
- **`NoConsoleLog`** turns off the [console log](#console-log), which `Run` otherwise keeps whenever there is a debug dir.
- **`NoNetworkLog`** turns off the [network log](#network-log), kept the same way; **`NetworkLogAll`** makes it record every request instead of only failures.
- **Nothing is ever deleted.** Working directories, artifacts and recordings all stay after the run; cleaning them up is your call.

Set a path field to choose it, or leave it nil for the default. A pointer to `""` is an error, not a default. `browser.ResolveDirs(opts)` returns the absolute paths a run will use.

### Working directory layout

```
{WorkingDir}/
├── agent/          the agent's working directory: the only place it can read
│   └── turns/      each turn's {UTC timestamp}.png and .html, if the preset perceives them
├── video/          Playwright's recording of the run
├── debug/          the CLI's per-call debug dumps and the console and network logs (unless you set your own TempDir)
└── artifacts/      the artifacts, unless ArtifactsDir points elsewhere
```

### Console log

`Run` writes everything the browser logs to `{stamp}-{name}-console.jsonl` in the debug dir, named and stamped like the CLI's dumps so it sorts just before the first turn's. It is one JSON object per line, from the moment the browser opens until it closes:

```jsonl
{"time":"2026-11-20T22:02:22.337412Z","stamp":"20261120-230222.337","kind":"open","engine":"chromium"}
{"time":"…","stamp":"…","kind":"pageopen","tab":"t1","page":"about:blank"}
{"time":"…","stamp":"…","kind":"active","tab":"t1"}
{"time":"…","stamp":"…","kind":"navigation","tab":"t1","page":"http://localhost:9900/"}
{"time":"…","stamp":"…","kind":"turn","turn":1}
{"time":"…","stamp":"…","kind":"console","tab":"t1","page":"http://localhost:9900/","level":"error","text":"…","source":{"url":"…","line":1,"column":18233},"browserTime":1795212142337,"worker":false}
{"time":"…","stamp":"…","kind":"pageerror","tab":"t1","page":"…","name":"TypeError","message":"…","stack":"…","source":{"url":"…","line":3,"column":7}}
{"time":"…","stamp":"…","kind":"crash","tab":"t1","page":"…"}
{"time":"…","stamp":"…","kind":"pageclose","tab":"t2","page":"…"}
{"time":"…","stamp":"…","kind":"dropped","reason":"file size cap reached"}
{"time":"…","stamp":"…","kind":"closed","dropped":1234}
```

- **What's in it:** every console message at every level (with `worker` and `workerURL` for workers), every uncaught page error, every page crash, tabs opening and closing, main-frame navigations, the active tab changing, and one `turn` marker per snapshot, numbered like `Result.Turns`. It covers every tab, including inactive ones and tabs the page opens by itself. `tab` is a stable id (`t1`, `t2`, …), since tab indexes shift when tabs close. `stamp` lines a record up with the per-turn dumps; `time` is UTC.
- **It survives a hard stop.** Each record is written straight to the file as it arrives, with no buffering, so a killed process (SIGKILL, a CI timeout), a crashed browser or a failed run keeps everything received so far. Only a clean `Close` writes `closed`.
- **URLs are redacted:** the values of sensitive query and fragment parameters (`code`, `state`, `token`, `access_token`, `id_token`, `refresh_token`, `password`, `passwd`, `secret`, `client_secret`, `key`, `api_key`, `apikey`, `signature`, `sig`, `session`, and any name containing `token` or `secret`) become `REDACTED`, in every URL of both logs. That covers OAuth redirects such as `?code=…&state=…` and `#access_token=…`.
- **Caps:** each `text`, `name`, `message`, `stack` and URL (`page`, `source.url`, `workerURL`) is cut at 16 KiB and flagged `"truncated": true`. Past 50 MiB the file stops growing: one `dropped` record is written, and `closed` reports how many records were dropped. Set `session.Opts.ConsoleLogFieldCap` / `ConsoleLogMaxBytes` to change them.
- **No agent sees it**, whatever its preset: the debug dir is outside the agent's working directory, and nothing of it goes into `preset.State` or a prompt.
- **It never fails a run.** If the file can't be created or written, logging just stops.
- **It holds whatever the page logs**, tokens and personal data included, like the DOM dumps and prompts already in the debug dir. Mind that before uploading the debug dir anywhere.
- **Both engines,** with no CDP.
- **Your own browser:** with `Loop`, set `session.Opts.ConsoleLog` to an absolute path yourself; `Loop` writes the turn markers.
- **Very heavy logging:** console messages cross the Playwright driver's pipe one by one. A page logging tens of thousands of messages a second can outrun it and kill the driver connection.

### Network log

`Run` also writes the browser's failed network requests to `{stamp}-{name}-network.jsonl`, next to the console log and with the same guarantees: written as it happens, kept when the process is killed, never seen by an agent, never failing a run, both engines, no CDP. It shares the console log's record envelope, tab ids, tab records, `turn` markers, URL redaction and caps (`session.Opts.Network.FieldCap` / `MaxBytes`).

```jsonl
{"time":"…","stamp":"…","kind":"response","id":17,"tab":"t1","page":"http://localhost:9900/…","method":"POST","url":"http://localhost:9900/api/v1/…/execute","resourceType":"fetch","status":400,"statusText":"Bad Request","fromServiceWorker":false,"ttfbMs":142,"headers":{"content-type":"application/problem+json","x-request-id":"…"}}
{"time":"…","stamp":"…","kind":"responsebody","id":17,"contentType":"application/problem+json","body":"{\"title\":\"Bad Request\",\"detail\":\"…\",\"traceId\":\"…\"}"}
{"time":"…","stamp":"…","kind":"requestfailed","id":18,"tab":"t1","page":"…","method":"GET","url":"…/chunk-123.js","resourceType":"script","failure":"net::ERR_CONNECTION_REFUSED","aborted":false}
{"time":"…","stamp":"…","kind":"closed","dropped":0,"pending":0}
```

- **What's in it (the default, `failures`):** a `response` record for every response with status 400 or above, and a `requestfailed` record for every request that got no response (refused, DNS, CORS, TLS, aborted), from every tab, popup, worker and service worker. `aborted` flags a cancelled request (`net::ERR_ABORTED` or the engine's equivalent). A request whose response was recorded and that then failed (e.g. midway through its body) gets its `requestfailed` with the response's `id`. A fetch cancelled because its page navigated away isn't reported by Playwright at all, so it isn't in the log.
- **Error bodies:** for a text response (`text/*`, JSON, XML, including `+json` / `+xml` types such as `application/problem+json`), the first 8 KiB of the body follow in a `responsebody` record with the same `id` (`truncated` when cut, `error` when the body couldn't be had). Any other content type gives `bodyOmitted` on the `response` instead. The `response` record is written at once and the body is fetched in the background (`Body` is a round trip to the driver), so a killed run still has the status line. At most 64 bodies wait or are fetched at a time, each with a 5 s timeout (a fetch that timed out counts until it actually ends); past that a response gets `bodySkipped` rather than holding anything up. `Close` gives queued bodies 2 s to arrive, before the context goes, and `closed` reports how many were still `pending`.
- **`all` mode** (`NetworkLogAll`, or `session.NetworkLogAll`) adds a `response` record for every response and a `finished` record, with `durationMs`, for every request. Bodies are still fetched only for errors.
- **Headers:** only `content-type`, `content-length`, `location` and any header whose name contains `trace`, `request-id` or `correlation` (for finding the backend's logs), plus any you add in `session.Opts.Network.Headers`. `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key` and `X-Auth-Token` are never recorded, even if you add them.
- **Request bodies** are never recorded, unless the URL matches one of `session.Opts.Network.RequestBodyURLs` (regular expressions); then it goes in `requestBody`, capped like response bodies. A login form can't end up in an artifact by accident.
- **Response bodies are kept as the server sent them** and may hold personal data. Mind that before uploading the debug dir anywhere.
- **Recording is passive:** nothing is intercepted or routed, so the page behaves exactly as it would without it.
- **Your own browser:** with `Loop`, set `session.Opts.NetworkLog` (and `Network`) yourself.

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
