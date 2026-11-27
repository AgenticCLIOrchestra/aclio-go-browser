# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.27.0] - 2026-11-27

### Added
- NVDA's input help. The `Input help` mode, or NVDA+1 (as NVDA's `toggleInputHelp`), makes NVDA say "input help on. Press insert+1 again to turn it off.", and from then on every key, and every character of a `type` action, says what it would do instead of doing it, as NVDA's `_handleInputHelp` does:
  - the key's name as NVDA says it ("shift+h", "ctrl+home", "down arrow", "insert+f2"), from NVDA's key labels
  - the character it would type when that's different ("H")
  - the description of the command bound to it in the mode NVDA was in, word for word from NVDA's source ("moves to the next heading", "Moves to the next heading at level 2", "Activates the current object in the document", and those of NVDA+Space, NVDA+F2, NVDA+1 and say all). The cursor movement keys have no description there, so only their name is said
- Nothing happens and no key reaches the page while input help is on, and the action's result says so. NVDA+1 again, or `nvdaChangeMode`, turns it off ("input help off"), back in the mode NVDA was in

### Changed
- `DefaultInitialPrompt` explains input help to a preset that allows it

### Fixed
- In browse mode, the heading-level quick navigation keys (1–6) now say "No next heading at level 2" / "No previous heading at level 2", as NVDA does, instead of "no next heading at level 2"

## [0.26.0] - 2026-11-27

### Added
- NVDA's one-key pass-through. The `One-key pass-through` mode, or NVDA+F2 in any mode (as NVDA's `passNextKeyThrough`), lasts one key: NVDA says "Pass next key through", the next key goes straight to the page whatever NVDA would do with it (in browse mode, a letter types instead of jumping, an arrow key reaches the page), and NVDA is then back in the mode it was in, including whether it had entered focus mode by itself. With `type`, the first character goes through and the rest go to NVDA again. The action's result says the key was passed through and which mode NVDA is back in

### Changed
- `DefaultInitialPrompt` explains one-key pass-through to a preset that allows it, and names the modes that still only turn VoiceOver off (`Speech`, `Sleep`, `Input help`, `Review`, `Native selection`) instead of "the other modes"

## [0.25.0] - 2026-11-27

### Added
- NVDA's browse mode. In the `Browse` mode, the simulated NVDA reads the page as NVDA users do, following NVDA's own source (`browseMode`, `cursorManager`, its speech and presentation rules) with its defaults. Its virtual buffer is the active tab's main frame as lines, built from Chromium's accessibility tree and each element's CSS `display`, wrapping at 100 characters. With the `press` action:
  - ArrowDown / ArrowUp read the next / previous line, ArrowRight / ArrowLeft a character, Control+ArrowRight / Control+ArrowLeft a word, Home / End the line's ends, PageDown / PageUp move 25 lines, Control+Home / Control+End go to the top / bottom, and NVDA+ArrowDown reads to the end (say all, up to 200 lines per key press)
  - quick navigation: H, 1–6, K, F, E, B, X, R, C, L, I, G, D, T, S and Q jump to the next heading, heading of a level, link, form field, edit field, button, check box, radio button, combo box, list, list item, graphic, landmark, table, separator or block quote (with Shift, the previous one), or say "no next heading", …
  - Enter and Space activate what the cursor is on (a link or button is clicked, a field is focused)
- What NVDA says in browse mode: line reading puts a field's role and states before its content ("link", "terms"), quick navigation and focus changes put the content first ("terms", "link"). Containers are announced on entry ("list", "with 2 items", "main landmark", tables "with 3 rows and 4 columns") and, except landmarks, on exit ("out of list"); list bullets are read as "bullet"
- Browse mode keeps its keys from the page: those above, and any other character key, as NVDA traps keys with no command. The action's result says so. `type` in browse mode sends each character to NVDA, so letters jump around instead of typing, and the result says how many characters NVDA took. Tab, Escape, function keys and Control/Alt shortcuts still reach the page
- Automatic switching, as NVDA's `shouldPassThrough` does it: from browse mode, NVDA switches to focus mode by itself when focus lands on a text field or a widget it can't operate in browse mode (combo box, list box, option, slider, tab list, menu, menu item, tree, tree item, spin button, grid cell, radio button, tab, or anything in a toolbar), with VoiceOver back on. It switches back to browse mode when focus moves on to something else, or on Escape. NVDA+Space (`Insert+`, `CapsLock+` too) switches by hand. Focus mode set by hand stays until changed. Each switch between the two plays NVDA's sound, recorded in the NVDA feed as `(sound: focus mode)` / `(sound: browse mode)`
- `preset.BrowseField`, `preset.BrowseCategory`, `preset.BrowseReason`, `preset.NVDABrowseCategory`, `preset.NVDABrowseEnter`, `preset.NVDABrowseExit` and `preset.NVDAShouldPassThrough`: NVDA's browse mode wording and switching rules
- `DefaultInitialPrompt` explains browse mode's keys and switching to a preset that allows the `Browse` mode

### Changed
- Entering browse mode puts NVDA's reading cursor where focus is; switching tabs or loading another document starts it at the top

## [0.24.0] - 2026-11-27

### Added
- A `nvdaChangeMode` action (`action.TypeNVDAChangeMode`, payload `action.NVDAChangeMode`) that sets the simulated NVDA's mode: `Focus` (the default, also for an empty `mode`), `Browse`, `One-key pass-through`, `Speech`, `Sleep`, `Input help`, `Review` or `Native selection` (`action.NVDAMode`, with `action.NVDAModes` and `action.ResolveNVDAMode`). Any mode but `Focus` turns VoiceOver off, since VoiceOver has no such modes: from the switch on, only what NVDA says is recorded, as NVDA's alone, until the mode is `Focus` again. What was said before the switch keeps both readers. Only the mode and VoiceOver are simulated so far: NVDA goes on speaking as in focus mode, and the agent is told so
- `session.NVDAMode(b)` and `State.NVDAMode`: the simulated NVDA's mode, filled each turn with `State.Announcement`
- Every turn VoiceOver is off, `DescribeAnnouncement` says so first (`VoiceOver is off because NVDA's mode is set to "Browse", not "Focus". Set it back to "Focus" with nvdaChangeMode to hear VoiceOver again.`), then what VoiceOver said before it was turned off, if anything, then NVDA's feed in full
- `Preset.NVDAModes`: the modes a preset's `nvdaChangeMode` may set (nil: all of them; it must include `Focus`), with `preset.AllowedNVDAModes`. `preset.Check` refuses the others and `preset.Schema` doesn't offer them
- `action.IsScreenReader`, which tells the screen-reader action apart from the page actions

### Changed
- `Blind()` allows `nvdaChangeMode`, with every mode but `Sleep` and `Speech`: both silence NVDA, which, with VoiceOver off, would leave a blind user with no speech at all
- `Default()` and `Full()` leave out `nvdaChangeMode`, as they hear no screen reader, so `Full()` is now everything but that action. `preset.Validate` refuses `nvdaChangeMode` for a preset that doesn't perceive the focused element
- `DefaultInitialPrompt` explains `nvdaChangeMode` and the modes the preset allows, only for a preset that allows it

## [0.23.0] - 2026-11-27

### Added
- `preset.AnnounceNVDA(cur, prev)`: a focus announcement in NVDA's own wording and order, taken from NVDA's source (its role and state labels and speech rules) and from how Chrome exposes each ARIA role to it on Windows. The order is name, role, value, states, description, keyboard shortcut (`aria-keyshortcuts`), position, level. Its words include "edit", "check box", "checked" / "not checked" / "half checked" (radios too), "toggle button" with "pressed" / "not pressed", "menu button" with "subMenu" / "opens dialog" / "opens list", "combo box" for a native `<select>` too, "unavailable", "invalid entry", "has auto complete", "tool bar", "tree view", "graphic", and "main landmark" / "navigation landmark" / … for landmarks. It doesn't speak the role of a named option, menu item or tree item, says a tree item's level first when it changes, and says nothing for unnamed groupings, list items, rows and cells it passes through. NVDA has one English locale, so it takes no `Language`

### Changed
- The NVDA feed now uses NVDA's wording and order everywhere, from `AnnounceNVDA`, instead of VoiceOver's. The VoiceOver feed is unchanged. `preset.AnnounceAll` merges the two by aligning them, so an item both say (a name, a position, a live region's text) is one item for both, and a word only one of them uses is marked for that one
- The NVDA feed no longer says "selected" on arriving at a selected option, tree item or menu item, as NVDA's state rules leave it out on focus; it still says "selected" when the item becomes selected, and "not selected" as before
- `DefaultInitialPrompt` tells the agent that the two screen readers word things differently, and that a difference in wording alone isn't a finding

## [0.22.0] - 2026-11-27

### Added
- The NVDA feed now says "not selected" for a selectable item that isn't selected, as NVDA's own state rules do: an option, tree item, row, grid cell or column / row header whose `aria-selected` is false, and an option with no `aria-selected` at all (as documented for NVDA). It is said when focus (or the active descendant) lands on the item, and when the item is deselected while focused (e.g. Space in a multi-select list), whether or not the list is multi-selectable. The VoiceOver feed is unchanged: VoiceOver says nothing for an unselected item, so deselecting an option used to be silent in both feeds, which made correct `aria-selected="false"` markup look unannounced
- `preset.AnnounceAll(cur, prev, language)`: `Announce` for both screen readers, as `[]Announcement` marked with who says each item. `Announce` still returns VoiceOver's feed

### Changed
- `DefaultInitialPrompt` tells the agent about NVDA's "not selected", and that VoiceOver's silence for an unselected item isn't a defect

## [0.21.0] - 2026-11-24

### Added
- A second screen reader, NVDA, next to the VoiceOver one. The blind preset, and any preset that perceives the focused element, hears both every turn. The two feeds are the same except for one thing: in a live region, nodes replaced by nodes with identical text (e.g. a re-render that remounts every message in a chat log) are spoken by NVDA and not by VoiceOver, since NVDA in Chrome reads every node inserted into a live region while VoiceOver skips a change whose text is identical. Such an item is spoken like any other addition: the added text, or the whole `aria-atomic` element. Setting a text node to the same value stays silent in both. Both feeds use the VoiceOver-modelled wording
- `preset.Announcement` (`Text`, `VoiceOver`, `NVDA`): one spoken item and which screen readers spoke it, with `preset.VoiceOverSaid` and `preset.NVDASaid`, which give each feed

### Changed
- **Breaking:** `State.Announcement` is now `[]preset.Announcement` instead of `[]string`, one ordered list with each item marked with who spoke it, and `session.TakeAnnouncements` returns `[]preset.Announcement`. Use `preset.VoiceOverSaid(s.Announcement)` for the old `[]string`
- `DescribeAnnouncement` renders VoiceOver's feed (`VoiceOver said, in order:`), then NVDA's: `NVDA said the same.` when it did, otherwise its full list. When neither said anything, it says `VoiceOver and NVDA said nothing this turn.` instead of `The screen reader said nothing this turn.`
- `DefaultInitialPrompt` tells the agent it hears two screen readers and how they differ, and that content repeated only in the NVDA feed is a real finding. Everything it says about screen readers and the focused element now comes from a new `{screen_reader}` placeholder (`agent.PlaceholderScreenReader`), filled only for a preset that perceives the focused element, so the Default and Motor agents are no longer told about screen readers they don't hear

## [0.20.0] - 2026-11-21

### Added
- `agent.StartSession(request, preset)`, which opens an aclio-go session for a whole task, with the agent's tools and the preset's response schema fixed as for a single turn, and `InitialRequest.Session` / `SubsequentRequest.Session`, which run a turn on it. With a session, `SubsequentRequest.SessionID` isn't needed. Close it with `aclio.CloseSession`

### Changed
- `Run` and `Loop` run a whole task on one session. With Claude, one `claude` process serves every turn instead of a new process resuming the conversation each turn, so later turns skip the process start and transcript reload; if the process dies, the conversation is resumed in a new one. With Codex, each turn is still a run resuming the previous thread. `Result`'s cost and tokens are still summed over every turn, each turn counting only its own
- Requires aclio-go v0.13.0

## [0.19.0] - 2026-11-21

### Added
- The screen reader now speaks live regions, as a real one does: what the page announces without moving focus, such as search result counts ("3 results available", "No matching users or groups."), "Saved" toasts and form errors. They go into `State.Announcement` in order with the focus announcements, as the text alone. A live region is an element with `aria-live="polite"` / `"assertive"` or the role `status` / `log` (polite) or `alert` (assertive), inherited from the nearest ancestor that sets one; an explicit `aria-live` wins over the role. Only a change `aria-relevant` covers (by default `additions text`) that changes what is rendered is spoken: with `aria-atomic` (true by default for `alert` and `status`) as the whole region, otherwise as the change alone. Changes in one region from one task are spoken once. Hidden content, blank text, `aria-live="off"` and regions inserted already filled stay silent, except an inserted, revealed or newly assigned `alert`. So does replacing text with identical text, compared exactly, so a repeat an app forces with an NBSP or a thin space is spoken. A known limit: a removed node's CSS can't be read, so removing stylesheet-hidden text from a region whose `aria-relevant` lists `removals` is spoken. A region under `aria-busy="true"` is silent and is spoken once, whole, when it stops being busy. Each announcement is capped at 1024 characters. Only the active tab's main frame is heard: background tabs and iframes are silent. Nothing changes for presets that don't perceive the focused element

### Changed
- `DefaultInitialPrompt` tells the agent that the screen reader also speaks messages the page announces without moving focus

## [0.18.1] - 2026-11-21

### Fixed
- In the network log's default (`failures`) mode, a request whose error response was recorded and that then failed (e.g. midway through its body) now gets its `requestfailed` record with the response's `id`, as in `all` mode, instead of a new one
- A response body fetch that times out now keeps its place in the body queue until it actually ends, so `NetworkLogOpts.QueueSize` also caps fetches still hanging after their timeout. Before, each one freed its place at once, so fetches stuck on a slow server could pile up without limit

## [0.18.0] - 2026-11-21

### Added
- A network log. `Run` writes the browser's failed network requests to `{stamp}-{name}-network.jsonl` in the debug dir, as it happens, with the console log's guarantees: unbuffered, so it survives the process being killed; best-effort; never seen by an agent; Chromium and Firefox. By default it records a `response` for every status of 400 or above, with allowlisted headers and `ttfbMs`, and a `requestfailed` for every request that got no response, with `failure` and `aborted`, from every tab, popup, worker and service worker. For a text error response (`text/*`, JSON, XML, `+json` / `+xml`), the first 8 KiB of the body follow in a `responsebody` record with the same `id`. Bodies are fetched off Playwright's dispatch goroutine, through a bounded queue (64, 5 s per body), so a slow body never holds anything up: past the queue, a response gets `bodySkipped`. `Close` gives queued bodies 2 s to arrive, and `closed` reports how many were still `pending`. A fetch cancelled because its page navigated away isn't reported by Playwright, so it isn't logged
- `Options.NoNetworkLog`, to turn the network log off, and `Options.NetworkLogAll`, to record every request too: a `response` for every response and a `finished` record with `durationMs` for every request
- `session.Opts.NetworkLog` (the log's absolute path; `""` keeps none) and `session.Opts.Network` (`session.NetworkLogOpts`: `Mode` (`session.NetworkLogFailures` or `session.NetworkLogAll`), `BodyCap`, `BodyTimeout`, `QueueSize`, `Headers`, `RequestBodyURLs`, `FieldCap`, `MaxBytes`), with `session.DefaultNetworkLogBodyCap`, `DefaultNetworkLogBodyTimeout` and `DefaultNetworkLogQueueSize`
- Headers are recorded only from an allowlist: `content-type`, `content-length`, `location`, anything with `trace`, `request-id` or `correlation` in its name, plus `NetworkLogOpts.Headers`. `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-Api-Key` and `X-Auth-Token` are never recorded. Request bodies are recorded only for URLs matching `NetworkLogOpts.RequestBodyURLs`

### Changed
- URLs in the console log are now redacted too, like the network log's: the values of sensitive query and fragment parameters (`code`, `state`, `token`, `access_token`, `password`, `secret`, `api_key`, `signature`, `session`, any name containing `token` or `secret`, …) become `REDACTED`
- The console log and the network log share their tab ids, tab records (`pageopen`, `pageclose`, `navigation`, `active`, `crash`) and `turn` markers; `session.LogTurn` writes to both

## [0.17.0] - 2026-11-21

### Added
- A console log. `Run` writes every console message (all levels, from pages, workers and service workers), uncaught page error and page crash to `{stamp}-{name}-console.jsonl` in the debug dir, as JSON Lines, from the moment the browser opens until it closes. It covers every tab, including inactive tabs and tabs the page opens by itself, and also records tabs opening and closing, main-frame navigations, the active tab changing, and one `turn` marker per snapshot. Each record carries `time` (UTC), `stamp` (matching the per-turn dump names) and `kind`, plus a stable `tab` id and the `page` URL where it applies. Each record is written straight to the file, unbuffered, so it survives the process being killed. Text, error names, messages, stacks and URLs are capped at 16 KiB each (flagged `truncated`), and the file at 50 MiB: past it, one `dropped` record is written and every later record is only counted, in the final `closed` record. It is best-effort and never fails a run. No agent sees it, whatever its preset. Works on Chromium and Firefox
- `Options.NoConsoleLog`, to turn the console log off
- `session.Opts.ConsoleLog` (the log's absolute path; `""` keeps none), `session.Opts.ConsoleLogFieldCap` and `session.Opts.ConsoleLogMaxBytes`, with `session.DefaultConsoleLogFieldCap` and `session.DefaultConsoleLogMaxBytes`, for callers that open the browser themselves
- `session.LogTurn(b, n)`, which writes a turn marker to the console log, or does nothing when there is none

## [0.16.0] - 2026-11-18

### Added
- Screen reader announcements. `State.Announcement` is everything a screen reader said since the previous turn, one spoken item per string, in order, e.g. `main`, `Prefs`, `form`, `Sort by`, `group`, `Ascending`, `selected`, `radio button`, `1 of 2`. Like a real screen reader, it speaks only about what happened. When focus moves, it says the context focus has just entered (landmarks, groups, … not shared with the previous focus), then the element (name, value, states, role, position, description), then its active descendant. When focus stays on the same element, it says only what changed (the new value, the states it gained, e.g. `checked` after Space, or the new item when arrowing through a listbox). Otherwise it says nothing. The wording is modelled on VoiceOver ("edit text", "pop-up button", "dimmed", "heading level 2")
- `preset.Language`, a string-based enum for the announcement's wording: `preset.LanguageEnUS` (`"en-US"`, the default: "checkbox", "checked") and `preset.LanguageEnGB` (`"en-GB"`: "tick box", "ticked"), with `preset.Languages`, `preset.ResolveLanguage` and `preset.DefaultLanguage`. Set it on the new `Preset.Language`; `preset.Validate` refuses an unsupported one
- `preset.Announce(cur, prev, language)`, which builds one announcement from the element focused now and the one focused just before
- `session.StartAnnouncing(b, language)` and `session.TakeAnnouncements(b)`, which record what a screen reader says as it happens and hand it over, emptying the store. The page queues the element focused at every `focusin`, and at every change to the focused element or its active descendant (`input`, `change`, and the attributes a screen reader announces). Go turns each into an announcement in the background as the page signals, after each action `Perform` runs, and when the announcements are taken. So a batch of actions (Tab, Tab, Space, …) is heard step by step, and so are changes the page makes by itself, such as a dialog taking focus. Switching tabs announces where focus is in the new tab. `Run` and `Loop` start it for a preset that perceives the focused element, and each snapshot takes what was said into `State.Announcement`. Chromium only. Focus inside iframes isn't heard yet
- `preset.DescribeAnnouncement`, which renders `State.Announcement` for the agent, one item per line, or says the screen reader said nothing
- `PosInSet` / `SetSize` on `FocusedElement` and `ActiveDescendant` (`posInSet` / `setSize`): the position a screen reader announces ("2 of 5"). They come from `aria-posinset` / `aria-setsize` when the page sets them, otherwise they are counted from the DOM (radios with the same name in the same form, or visible siblings with the same role), since Chromium's accessibility tree doesn't report them. `aria-posinset` and `aria-setsize` are gone from `Aria`
- `NodeID` on `FocusedElement` and `ActiveDescendant`: Chromium's id for the element's DOM node, which tells whether focus moved. It isn't part of the JSON

### Changed
- `Blind()` is now told only what a screen reader said (`DescribeAnnouncement`), plus the open tabs, instead of the focused element as JSON. The raw fields stay in `State.Focused`, and `DescribeFocused` still renders them for custom presets. `DefaultInitialPrompt` explains the announcement to the agent
- `session.Focused` now reads the focused element and its accessibility node in one go, so a page that moves focus mid-read can no longer mismatch them

## [0.15.1] - 2026-11-18

### Fixed
- The focused element's `role` (and its active descendant's) no longer leaks Chromium's internal role names, such as `Date` for a date input. Where Chromium gives one of its own roles instead of an ARIA one, the nearest ARIA role is reported: `textbox` for the date and time inputs (`date`, `time`, `datetime-local`, `month`, `week`), `button` for `<summary>` and `<input type=color>`, and `generic` for elements with no corresponding ARIA role (`<label>`, `<abbr>`, `<video>`, `<audio>`, `<canvas>`, `<iframe>`, `<object>`, `<embed>`). An internal role not in this mapping is left out rather than guessed

## [0.15.0] - 2026-11-18

### Changed
- `preset.FocusedElement.DescribedBy` and `preset.ActiveDescendant.DescribedBy` are now `Description` (`description` in JSON), the accessible description from Chromium's own accessibility tree, the one a screen reader reads after the rest. It is the text of the `aria-describedby` targets (even hidden ones), else `aria-description`, else a `title` the name didn't already use, so a button with text and `title="Saves the draft"` now has that description, and `aria-description` is no longer ignored. It used to be worked out by our own JS from `aria-describedby` only. `aria-description` is gone from `Aria`. `DefaultInitialPrompt` explains `description` to the agent

## [0.14.0] - 2026-11-18

### Removed
- `preset.FocusedElement.Legend` and the `legend` field of the focused-element view. A fieldset's `<legend>` is in `Context`, as the `group` it names (e.g. `{"role":"group","name":"Sort by"}`), the way a screen reader gets it

## [0.13.0] - 2026-11-18

### Added
- `preset.FocusedElement.Context` and `preset.ContextItem`: the focused element's ancestors in Chromium's own accessibility tree, outermost first, as `{role, name}`. These are the landmarks, groups, lists, tables and dialogs a screen reader announces as context when focus moves into them, e.g. `main`, `form` "Prefs", then `group` "Sort by" for a radio in a fieldset, or `banner`, `navigation` "Primary", `list`, `listitem` for a nav link. Generic wrappers, ignored nodes and the document itself are left out. A screen reader announces only the contexts focus has just entered; `Context` has the whole chain every turn. The active descendant has no `Context` of its own (its context is the focused container). `DefaultInitialPrompt` explains `context` to the agent

## [0.12.0] - 2026-11-18

### Added
- `preset.AXState.Ignored` and `IgnoredReasons`: whether Chromium's accessibility tree leaves the element out, so a screen reader announces nothing for it, and why (Chromium's reason names, e.g. `ariaHiddenElement`, `ariaHiddenSubtree`, `notRendered`). An ignored element is reported as only `{"ignored": true, "ignoredReasons": [...]}`, with no role, name, text or other fields. Chromium never leaves out the element that has focus, even inside `aria-hidden` (it exposes it so a screen reader can announce it), so in practice this shows up on an `ActiveDescendant`: a listbox pointing at an `aria-hidden` or `display:none` item. `DefaultInitialPrompt` explains `ignored` to the agent

### Changed
- An ignored active descendant used to come back with no role or name but with its DOM text and attributes, which looked like an announced item with no name. It is now reported as ignored, with its reasons
- `aria-hidden` is gone from `Aria`. When it takes effect, `ignored` and its reasons show it; on the focused element Chromium disregards it, so a screen reader never gets it

## [0.11.0] - 2026-11-18

### Added
- `preset.AXState`, an element's states and properties as Chromium's own accessibility tree reports them, embedded in `FocusedElement` and `ActiveDescendant` (so its fields sit at their top level in JSON): `checked` and `pressed` (`"true"`, `"false"` or `"mixed"`), `selected`, `expanded`, `disabled`, `readOnly`, `required`, `invalid` (`"true"`, `"grammar"` or `"spelling"`), `level`, `hasPopup`, `multiselectable`, `orientation`, `valueMin`, `valueMax`, `valueText`, `autocomplete`, `keyShortcuts`, `roleDescription` and `modal`. Native and ARIA markup give the same states (a native indeterminate checkbox and `aria-checked="mixed"` are both `checked: "mixed"`), and they reflect what the user did. Chromium leaves `required` out for some controls (e.g. a required checkbox), so the DOM's `required` / `aria-required` stands in then. `DefaultInitialPrompt` explains the states to the agent

### Changed
- `Checked` is now a `*string` (`"true"`, `"false"` or `"mixed"`) instead of a `*bool`, and covers `aria-checked` as well as native checkboxes and radios. `Selected`, `Disabled`, `ReadOnly` and `Required` now come from the accessibility tree, so they cover ARIA too (`aria-selected`, `aria-disabled`, …)
- `Aria` keeps only the aria-* attributes no other field covers (e.g. `aria-current`, `aria-valuetext`, `aria-setsize` / `aria-posinset`, `aria-controls`), which the accessibility tree doesn't report. Those it covers (`aria-label`, `aria-labelledby`, `aria-describedby`, `aria-activedescendant`, `aria-valuenow`, `aria-checked`, `aria-expanded`, …) are gone from it

### Removed
- `preset.FocusedElement.Indeterminate` and `preset.ActiveDescendant.Indeterminate`: a mixed checkbox is now `checked: "mixed"`

## [0.10.0] - 2026-11-18

### Added
- `preset.ActiveDescendant.Value`, following the same rules as `FocusedElement.Value`

### Changed
- The focused element's `value` now comes from Chromium's own accessibility tree, like `role` and `name`: it is the value a screen reader announces. That is the text typed into a field (masked for a password), a drop-down `<select>`'s selected text (`Large`, not its option's `value` attribute), or a slider's, spinbutton's or progress bar's number, written out (e.g. `"7"`). It is omitted when the element has none: an empty field, a checkbox, a radio, a button or a link, so a checkbox's meaningless `"on"` and a button's `""` are gone. `DefaultInitialPrompt` explains `value` to the agent
- `selectedText` is now set only for a list-style `<select>` (`multiple`, or `size` > 1), which has no value in the accessibility tree. For a drop-down `<select>` it repeated `value` and is gone

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

[0.27.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.26.0...v0.27.0
[0.26.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.18.1...v0.19.0
[0.18.1]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.18.0...v0.18.1
[0.18.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.17.0...v0.18.0
[0.17.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.15.1...v0.16.0
[0.15.1]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.15.0...v0.15.1
[0.15.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/AgenticCLIOrchestra/aclio-go-browser/compare/v0.9.0...v0.10.0
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
