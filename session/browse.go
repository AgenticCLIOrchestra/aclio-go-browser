package session

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// The simulated NVDA's browse mode: its virtual buffer read with a reading
// cursor (line, character and word movement, quick navigation keys, say
// all), the keys it keeps from the page, and its switching between browse
// and focus mode, as NVDA's browseMode, cursorManager and speech modules do
// (with NVDA's defaults). It runs only while b announces: without the screen
// readers there is nothing to hear, and keys reach the page.

// nvdaState is the simulated NVDA's state.
type nvdaState struct {
	mode action.NVDAMode // "" means action.NVDAModeFocus
	// auto is set while in focus mode NVDA entered by itself (focus landed
	// on a field or widget from browse mode): NVDA goes back to browse mode
	// when focus moves on to something else, or on Escape. Focus mode set by
	// hand (nvdaChangeMode, NVDA+Space, the default) stays until changed.
	auto bool
	// before is the mode to go back to once one-key pass-through has let
	// its key through, input help is turned off or review mode is left.
	before action.NVDAMode
	// inReview is set while in review mode, also while its other keys are
	// handled as in the mode NVDA was in (VoiceOver stays off).
	inReview bool
	// review is the review cursor (see review.go).
	review cursorState
	// The reading cursor: a line of the virtual buffer and a character
	// offset in it. anchor (the backendDOMNodeId of the line's first node)
	// finds the line again in a rebuilt buffer; url resets it on another
	// document.
	line, offset int
	anchor       int
	url          string
	stack        []int // backendDOMNodeIds of the containers the cursor is in, outermost first
}

const (
	// nvdaSayAllLimit caps how many lines say all reads in one key press.
	nvdaSayAllLimit = 200
	// nvdaLinesPerPage is how far Page Up / Page Down move
	// (virtualBuffers.linesPerPage).
	nvdaLinesPerPage = 25
	// Results reported for keys NVDA keeps from the page.
	nvdaHandled = "NVDA browse mode handled this key: it didn't reach the page."
	nvdaTrapped = "NVDA browse mode kept this key (no command is bound to it): it didn't reach the page."
)

func nvdaOf(b *Browser) *nvdaState {
	if b.nvda == nil {
		b.nvda = &nvdaState{}
	}
	return b.nvda
}

func modeOf(st *nvdaState) action.NVDAMode {
	if st == nil || st.mode == "" {
		return action.NVDAModeFocus
	}
	return st.mode
}

// enterPassThrough starts one-key pass-through (NVDA+F2, or nvdaChangeMode):
// the next key goes straight to the page, whatever NVDA would do with it,
// and NVDA then goes back to the mode it was in. NVDA says "Pass next key
// through" (globalCommands.script_passNextKeyThrough). a is nil when b
// doesn't announce.
func enterPassThrough(a *announcer, st *nvdaState) {
	if modeOf(st) != action.NVDAModeOneKeyPass {
		st.before = modeOf(st)
	}
	st.mode = action.NVDAModeOneKeyPass
	if a != nil {
		sayNVDA(a, "Pass next key through")
	}
}

// passThroughDone ends one-key pass-through after its key, back in the mode
// NVDA was in before (with whether NVDA had entered it by itself).
func passThroughDone(st *nvdaState) {
	st.mode = st.before
	if st.mode == action.NVDAModeOneKeyPass {
		st.mode = action.NVDAModeFocus
	}
	st.before = ""
}

func resetCursor(st *nvdaState) {
	st.line, st.offset, st.anchor, st.url, st.stack = 0, 0, 0, "", nil
}

// NVDAMode is b's simulated NVDA mode: action.NVDAModeFocus until an
// nvdaChangeMode action, NVDA+Space or NVDA itself (browse mode switching to
// focus mode in a field, and back) sets another. VoiceOver is off while it
// isn't Focus.
func NVDAMode(b *Browser) action.NVDAMode {
	if b == nil {
		return action.NVDAModeFocus
	}
	return modeOf(b.nvda)
}

// appendSaid adds items to what a has said. While VoiceOver is off (NVDA's
// mode isn't Focus), only what NVDA says is kept, as NVDA's alone.
func appendSaid(a *announcer, items ...preset.Announcement) []preset.Announcement {
	off := modeOf(a.nvda) != action.NVDAModeFocus || (a.nvda != nil && a.nvda.inReview)
	for _, it := range items {
		if off {
			if !it.NVDA {
				continue
			}
			it.VoiceOver = false
		}
		a.said = append(a.said, it)
	}
	return a.said
}

// sayNVDA adds what NVDA alone says.
func sayNVDA(a *announcer, items ...string) {
	for _, t := range items {
		if t != "" {
			appendSaid(a, preset.Announcement{Text: t, NVDA: true})
		}
	}
}

// switchMode sets st's mode. Between browse and focus mode NVDA plays a
// sound (its default, virtualBuffers.passThroughAudioIndication), recorded
// as "(sound: focus mode)" / "(sound: browse mode)".
func switchMode(a *announcer, st *nvdaState, m action.NVDAMode, auto bool) {
	old := modeOf(st)
	st.mode, st.auto = m, auto
	if a == nil || old == m {
		return
	}
	switch {
	case old == action.NVDAModeBrowse && m == action.NVDAModeFocus:
		sayNVDA(a, "(sound: focus mode)")
	case old == action.NVDAModeFocus && m == action.NVDAModeBrowse:
		sayNVDA(a, "(sound: browse mode)")
	}
}

// setNVDAMode sets b's simulated NVDA mode by hand (the zero value meaning
// Focus), as nvdaChangeMode does. Any mode but Focus turns VoiceOver off,
// Focus turns it back on. What was queued before the change is processed
// first, so it keeps who said it. Entering browse mode puts the reading
// cursor where focus is. One-key pass-through lasts one key, then NVDA is
// back in the mode it was in.
func setNVDAMode(b *Browser, m action.NVDAMode) error {
	m, err := action.ResolveNVDAMode(m)
	if err != nil {
		return err
	}
	a := b.announcer
	if a == nil {
		st := nvdaOf(b)
		switch m {
		case action.NVDAModeOneKeyPass:
			enterPassThrough(nil, st)
		case action.NVDAModeInputHelp:
			enterInputHelp(nil, st)
		case action.NVDAModeReview:
			enterReview(nil, st)
		default:
			if st.inReview {
				leaveReview(st)
			}
			st.mode, st.auto = m, false
		}
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.nvda == nil {
		a.nvda = nvdaOf(b)
	}
	processLocked(a)
	switch {
	case m == action.NVDAModeOneKeyPass:
		enterPassThrough(a, a.nvda)
		return nil
	case m == action.NVDAModeInputHelp:
		enterInputHelp(a, a.nvda)
		return nil
	case m == action.NVDAModeReview:
		enterReview(a, a.nvda)
		return nil
	case modeOf(a.nvda) == action.NVDAModeInputHelp:
		leaveInputHelp(a, a.nvda) // then on to the mode asked for
	}
	if a.nvda.inReview && modeOf(a.nvda) == action.NVDAModeReview {
		leaveReview(a.nvda) // then on to the mode asked for
	}
	switchMode(a, a.nvda, m, false)
	if m == action.NVDAModeBrowse {
		cursorToFocus(a)
	}
	return nil
}

// announceFocus says what the screen readers say for fe, the element focused
// now (a.last is the one before), as NVDA's mode has it. In browse mode, NVDA
// switches to focus mode by itself when fe is a field or widget it can't
// operate in browse mode, and reads fe in its browse mode wording otherwise;
// in focus mode it entered by itself, it switches back to browse mode when
// focus moves on to something else. Manual focus mode is as before.
func announceFocus(a *announcer, fe *preset.FocusedElement) {
	st := a.nvda
	if modeOf(st) == action.NVDAModeReview {
		// Focus is handled as in the mode NVDA was in (which may switch
		// between browse and focus mode), then the review cursor follows it.
		st.mode = st.before
		announceFocus(a, fe)
		st.before, st.mode = modeOf(st), action.NVDAModeReview
		a.last = fe
		reviewToFocus(a)
		return
	}
	mode := modeOf(st)
	pass := preset.NVDAShouldPassThrough(fe)
	same := a.last != nil && fe.NodeID != 0 && fe.NodeID == a.last.NodeID
	switch {
	case mode == action.NVDAModeBrowse && same:
		// The focused element changed in place (e.g. a check box ticked):
		// NVDA says what changed.
		sayNVDA(a, preset.AnnounceNVDA(fe, a.last)...)
	case mode == action.NVDAModeBrowse && pass:
		switchMode(a, st, action.NVDAModeFocus, true)
		appendSaid(a, preset.AnnounceAll(fe, a.last, a.lang)...)
	case mode == action.NVDAModeBrowse || (mode == action.NVDAModeFocus && st.auto && !pass):
		switchMode(a, st, action.NVDAModeBrowse, false)
		if !browseFocus(a, fe) {
			sayNVDA(a, preset.AnnounceNVDA(fe, a.last)...)
		}
	default:
		appendSaid(a, preset.AnnounceAll(fe, a.last, a.lang)...)
	}
}

// browseFocus reads fe as NVDA does in browse mode, moving the reading
// cursor to it. It reports false when fe isn't in the virtual buffer.
func browseFocus(a *announcer, fe *preset.FocusedElement) bool {
	buf, err := vbufFor(a)
	if err != nil {
		return false
	}
	n, ok := buf.byBackend[fe.NodeID]
	if !ok {
		return false
	}
	if _, ok := buf.lineOf[n]; !ok {
		return false
	}
	speakNode(a, buf, n)
	return true
}

// cursorToFocus puts the reading cursor on the focused element, silently.
func cursorToFocus(a *announcer) {
	if a.last == nil || a.cdp == nil {
		return
	}
	buf, err := vbufFor(a)
	if err != nil {
		return
	}
	if n, ok := buf.byBackend[a.last.NodeID]; ok {
		if l, ok := buf.lineOf[n]; ok {
			setCursor(a.nvda, buf, l, segOffset(buf, l, n))
			a.nvda.stack = backendIDs(buf, buf.stackOf(n))
		}
	}
}

// vbufFor builds the active tab's virtual buffer and fits the reading
// cursor into it: back to the top on another document, otherwise on the
// line it was on.
func vbufFor(a *announcer) (*vbuf, error) {
	if a.cdp == nil {
		return nil, fmt.Errorf("session: no page to read")
	}
	buf, err := buildVbuf(a.cdp)
	if err != nil {
		return nil, err
	}
	st := a.nvda
	if url := a.page.URL(); url != st.url {
		resetCursor(st)
		st.url = url
	}
	if st.anchor != 0 {
		if n, ok := buf.byBackend[st.anchor]; ok {
			if l, ok := buf.lineOf[n]; ok {
				st.line = l
			}
		}
	}
	st.line = max(0, min(st.line, len(buf.lines)-1))
	if len(buf.lines) > 0 {
		st.offset = max(0, min(st.offset, utf8.RuneCountInString(lineText(buf.lines[st.line]))-1))
	}
	return buf, nil
}

func setCursor(st *nvdaState, buf *vbuf, line, offset int) {
	st.line, st.offset, st.anchor = line, offset, 0
	if line >= 0 && line < len(buf.lines) && len(buf.lines[line].segs) > 0 {
		st.anchor = buf.nodes[buf.lines[line].segs[0].node].ax.BackendNodeID
	}
}

func backendIDs(buf *vbuf, nodes []int) []int {
	out := make([]int, 0, len(nodes))
	for _, n := range nodes {
		if id := buf.nodes[n].ax.BackendNodeID; id != 0 {
			out = append(out, id)
		}
	}
	return out
}

func nodesOf(buf *vbuf, ids []int) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if n, ok := buf.byBackend[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

// transition is what NVDA says as the cursor moves from inside the
// containers from to inside the containers to: "out of …" for each one left,
// innermost first, then each one entered.
func transition(buf *vbuf, from, to []int, reason preset.BrowseReason) []string {
	k := 0
	for k < len(from) && k < len(to) && from[k] == to[k] {
		k++
	}
	var out []string
	for i := len(from) - 1; i >= k; i-- {
		out = append(out, preset.NVDABrowseExit(buf.nodes[from[i]].field)...)
	}
	for _, n := range to[k:] {
		out = append(out, preset.NVDABrowseEnter(buf.nodes[n].field, reason)...)
	}
	return out
}

// speakSegs is what NVDA says reading segs from inside the containers from,
// and the containers it ends up in.
func speakSegs(buf *vbuf, segs []vbufSeg, from []int, reason preset.BrowseReason) ([]string, []int) {
	stack := from
	var out []string
	prevText := ""
	for _, s := range segs {
		to := buf.stackOf(s.node)
		out = append(out, transition(buf, stack, to, reason)...)
		stack = to
		switch {
		case s.field:
			f := buf.nodes[s.node].field
			if f.Name != "" && f.Name == prevText {
				f.Name = "" // its label was just read (approximates NVDA not repeating a label)
			}
			out = append(out, preset.NVDABrowseEnter(f, reason)...)
			prevText = ""
		case buf.nodes[s.node].internal == "ListMarker":
			out = append(out, symbolName(s.text))
			prevText = ""
		case s.text != "":
			out = append(out, s.text)
			prevText = s.text
		}
	}
	return out, stack
}

// speakLine moves the cursor to line l and reads it, as moving by line and
// say all do.
func speakLine(a *announcer, buf *vbuf, l int) {
	st := a.nvda
	items, stack := speakSegs(buf, buf.lines[l].segs, nodesOf(buf, st.stack), preset.BrowseCaret)
	sayNVDA(a, items...)
	st.stack = backendIDs(buf, stack)
	setCursor(st, buf, l, 0)
}

// speakNode moves the cursor to node n and reads it, as a quick navigation
// key and a focus change in browse mode do: a single-line field whole,
// content first; a container announced, then the rest of its first line.
func speakNode(a *announcer, buf *vbuf, n int) {
	st := a.nvda
	from := nodesOf(buf, st.stack)
	l := buf.lineOf[n]
	to := buf.stackOf(n)
	var items []string
	switch cat := buf.nodes[n].cat; {
	case cat == preset.BrowseSingleLine || (cat == preset.BrowseMarker && len(buf.nodes[n].children) == 0):
		items = append(transition(buf, from, to, preset.BrowseQuickNav), preset.NVDABrowseEnter(buf.nodes[n].field, preset.BrowseQuickNav)...)
	default:
		if cat == preset.BrowseContainer || cat == preset.BrowseMarker {
			to = append(to, n)
		}
		items = transition(buf, from, to, preset.BrowseQuickNav)
		var rest []string
		rest, to = speakSegs(buf, segsIn(buf, l, n), to, preset.BrowseQuickNav)
		items = append(items, rest...)
	}
	sayNVDA(a, items...)
	st.stack = backendIDs(buf, to)
	setCursor(st, buf, l, segOffset(buf, l, n))
}

// segsIn is line l's segments inside node n.
func segsIn(buf *vbuf, l, n int) []vbufSeg {
	var out []vbufSeg
	for _, s := range buf.lines[l].segs {
		if buf.isInside(s.node, n) {
			out = append(out, s)
		}
	}
	return out
}

func (b *vbuf) isInside(i, ancestor int) bool {
	for ; i >= 0; i = b.nodes[i].parent {
		if i == ancestor {
			return true
		}
	}
	return false
}

// segOffset is the character offset in line l of the first segment inside
// node n (0 when there is none).
func segOffset(buf *vbuf, l, n int) int {
	off := 0
	for _, s := range buf.lines[l].segs {
		if buf.isInside(s.node, n) {
			return off
		}
		if s.text != "" {
			off += utf8.RuneCountInString(s.text) + 1
		}
	}
	return 0
}

// segAt is the segment of line l at character offset off.
func segAt(buf *vbuf, l, off int) (vbufSeg, bool) {
	segs := buf.lines[l].segs
	if len(segs) == 0 {
		return vbufSeg{}, false
	}
	pos := 0
	for _, s := range segs {
		n := utf8.RuneCountInString(s.text)
		if off < pos+n+1 {
			return s, true
		}
		pos += n + 1
	}
	return segs[len(segs)-1], true
}

// nvdaKey handles key as the simulated NVDA would, and reports whether it
// did (the key then doesn't reach the page) and what to report back. Any
// mode: NVDA+Space toggles browse and focus mode; Escape leaves focus mode
// NVDA entered by itself. In browse mode: the arrow keys, Home / End,
// Page Up / Page Down and Control+Home / End move the reading cursor; Enter
// and Space activate what is under it; the quick navigation letters (H, 1–6,
// K, F, L, I, B, E, R, C, X, G, D, T, S, Q; with Shift, backwards) jump;
// NVDA+Down arrow reads to the end (say all); any other character key is
// kept from the page. NVDA's modifier is written NVDA+, Insert+ or
// CapsLock+. Without the screen readers, nothing is handled.
func nvdaKey(b *Browser, key string) (bool, *string, error) {
	a := b.announcer
	if a == nil {
		if st := b.nvda; modeOf(st) == action.NVDAModeOneKeyPass {
			passThroughDone(st)
		}
		return false, nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.nvda == nil {
		a.nvda = nvdaOf(b)
	}
	processLocked(a)
	st := a.nvda
	k := parseKey(key)
	mode := modeOf(st)
	handled := func(msg string) (bool, *string, error) { return true, &msg, nil }

	// One-key pass-through: this key goes straight to the page (false: not
	// handled), then NVDA is back in the mode it was in.
	if mode == action.NVDAModeOneKeyPass {
		passThroughDone(st)
		msg := fmt.Sprintf("NVDA passed this key straight through to the page; it is back in %q mode.", modeOf(st))
		return false, &msg, nil
	}
	// Input help: every key says what it would do instead of doing it.
	if mode == action.NVDAModeInputHelp {
		return handled(inputHelpKey(a, st, k))
	}
	if k.nvda && k.base == "1" {
		enterInputHelp(a, st)
		return handled("NVDA turned input help on: each key now says what it would do instead of doing it.")
	}
	if k.nvda && k.base == "F2" {
		enterPassThrough(a, st)
		return handled("NVDA will pass the next key straight through to the page, whatever its mode.")
	}
	if a.cdp == nil {
		return false, nil, nil
	}
	// Review mode: the review commands move the review cursor; any other key
	// does what it did in the mode NVDA was in, and the review cursor then
	// follows the browse mode cursor.
	if mode == action.NVDAModeReview {
		if cmd := reviewCommandOf(k); cmd != reviewNone {
			result, err := reviewKey(a, st, cmd)
			if err != nil {
				return true, nil, err
			}
			return handled(result)
		}
		st.mode = st.before
		defer func() {
			st.before, st.mode = modeOf(st), action.NVDAModeReview
			if st.before == action.NVDAModeBrowse {
				reviewFollow(st)
			}
		}()
		mode = modeOf(st)
	}

	switch {
	case k.nvda && k.base == "Space":
		if mode == action.NVDAModeBrowse {
			switchMode(a, st, action.NVDAModeFocus, false)
			return handled("NVDA switched to focus mode: keys reach the page.")
		}
		switchMode(a, st, action.NVDAModeBrowse, false)
		cursorToFocus(a)
		return handled("NVDA switched to browse mode: NVDA keeps the reading keys.")
	case k.plain() && k.base == "Escape" && mode == action.NVDAModeFocus && st.auto:
		switchMode(a, st, action.NVDAModeBrowse, false)
		cursorToFocus(a)
		return handled("NVDA left the focus mode it entered by itself, back to browse mode.")
	case mode != action.NVDAModeBrowse:
		if k.nvda {
			return handled("NVDA command not simulated: nothing happened, and the key didn't reach the page.")
		}
		return false, nil, nil
	}

	buf, err := vbufFor(a)
	if err != nil {
		return true, nil, err
	}
	if len(buf.lines) == 0 {
		if k.nvda || k.isChar() || browseKeys[k.base] {
			sayNVDA(a, "blank")
			return handled(nvdaHandled)
		}
		return false, nil, nil
	}
	last := len(buf.lines) - 1
	switch {
	case k.nvda && k.base == "ArrowDown" && !k.ctrl && !k.alt:
		n := 0
		for l := st.line; l <= last && n < nvdaSayAllLimit; l++ {
			speakLine(a, buf, l)
			n++
		}
		msg := fmt.Sprintf("NVDA read %d lines with say all.", n)
		if st.line < last {
			msg = fmt.Sprintf("NVDA read %d lines with say all (the simulation stops at %d; press it again to read on).", n, nvdaSayAllLimit)
		}
		return handled(msg)
	case k.nvda:
		return handled("NVDA command not simulated: nothing happened, and the key didn't reach the page.")
	case k.alt || k.meta:
		return false, nil, nil
	case k.ctrl && k.base == "Home":
		speakLine(a, buf, 0)
	case k.ctrl && k.base == "End":
		speakLine(a, buf, last)
	case k.ctrl && (k.base == "ArrowRight" || k.base == "ArrowLeft"):
		moveWord(a, buf, k.base == "ArrowRight")
	case k.ctrl:
		return false, nil, nil // Control+… shortcuts reach the page
	case k.base == "ArrowDown":
		speakLine(a, buf, min(st.line+1, last))
	case k.base == "ArrowUp":
		speakLine(a, buf, max(st.line-1, 0))
	case k.base == "PageDown":
		speakLine(a, buf, min(st.line+nvdaLinesPerPage, last))
	case k.base == "PageUp":
		speakLine(a, buf, max(st.line-nvdaLinesPerPage, 0))
	case k.base == "ArrowRight" || k.base == "ArrowLeft":
		moveChar(a, buf, k.base == "ArrowRight")
	case k.base == "Home" || k.base == "End":
		text := []rune(lineText(buf.lines[st.line]))
		st.offset = 0
		if k.base == "End" && len(text) > 0 {
			st.offset = len(text) - 1
		}
		sayNVDA(a, charName(text, st.offset))
	case k.base == "Enter" || k.base == "Space":
		if err := activate(a, buf); err != nil {
			return true, nil, err
		}
		return handled("NVDA activated the element under its reading cursor.")
	case k.isChar():
		if name := strings.ToLower(k.base); quickNavKeys[name].match != nil {
			quickNav(a, buf, name, !k.shift)
			return handled(nvdaHandled)
		}
		return handled(nvdaTrapped)
	default:
		return false, nil, nil // Tab, Escape, F-keys, Backspace, … reach the page
	}
	return handled(nvdaHandled)
}

// nvdaTakesKeys reports whether NVDA's mode makes it look at typed
// characters one by one (browse mode, one-key pass-through, input help) while
// b announces; otherwise text is typed straight into the page.
func nvdaTakesKeys(b *Browser) bool {
	if b.announcer == nil {
		return false
	}
	switch NVDAMode(b) {
	case action.NVDAModeBrowse, action.NVDAModeOneKeyPass, action.NVDAModeInputHelp:
		return true
	case action.NVDAModeReview:
		return b.nvda.before == action.NVDAModeBrowse // its other keys are browse mode's
	}
	return false
}

// typeThroughNVDA types text key by key in browse mode: each character goes
// to NVDA first, so letters are quick navigation keys (as with real NVDA),
// and only what NVDA doesn't keep reaches the page. In one-key pass-through,
// the first character reaches the page and NVDA goes back to its mode for
// the rest. When NVDA is in focus mode (or switches to it along the way),
// the rest is typed as usual.
func typeThroughNVDA(b *Browser, text string) (*string, error) {
	kept := 0
	for i, r := range text {
		if !nvdaTakesKeys(b) {
			return keptResult(kept), b.active.Keyboard().Type(text[i:])
		}
		ch := string(r)
		if r == '\n' {
			ch = "Enter"
		}
		handled, _, err := nvdaKey(b, ch)
		if err != nil {
			return nil, err
		}
		if handled {
			kept++
		} else if err := b.active.Keyboard().Type(string(r)); err != nil {
			return nil, err
		}
		processAnnouncements(b.announcer)
	}
	return keptResult(kept), nil
}

func keptResult(kept int) *string {
	if kept == 0 {
		return nil
	}
	msg := fmt.Sprintf("NVDA browse mode took %d of the typed characters as its own keys (quick navigation, or kept): they didn't reach the page. Switch to focus mode (or move focus to the field) to type.", kept)
	return &msg
}

// browseKeys are the non-character keys browse mode binds.
var browseKeys = map[string]bool{
	"ArrowDown": true, "ArrowUp": true, "ArrowLeft": true, "ArrowRight": true,
	"Home": true, "End": true, "PageUp": true, "PageDown": true, "Enter": true, "Space": true,
}

// key is a key press in Playwright's notation, split up.
type key struct {
	base                         string
	shift, ctrl, alt, meta, nvda bool
}

func (k key) plain() bool { return !k.shift && !k.ctrl && !k.alt && !k.meta && !k.nvda }

// isChar reports whether k types a character (a letter, digit or symbol,
// with Shift at most).
func (k key) isChar() bool {
	return !k.ctrl && !k.alt && !k.meta && !k.nvda && utf8.RuneCountInString(k.base) == 1 && k.base != " "
}

func parseKey(s string) key {
	var k key
	if s == " " {
		return key{base: "Space"}
	}
	parts := strings.Split(s, "+")
	if strings.HasSuffix(s, "++") { // a "+" key with modifiers
		parts = append(strings.Split(strings.TrimSuffix(s, "++"), "+"), "+")
	} else if s == "+" {
		parts = []string{"+"}
	}
	k.base = parts[len(parts)-1]
	for _, m := range parts[:len(parts)-1] {
		switch strings.ToLower(m) {
		case "shift":
			k.shift = true
		case "control", "ctrl", "controlormeta":
			k.ctrl = true
		case "alt":
			k.alt = true
		case "meta":
			k.meta = true
		case "nvda", "insert", "capslock":
			k.nvda = true
		}
	}
	if k.base == " " {
		k.base = "Space"
	}
	if r, _ := utf8.DecodeRuneInString(k.base); utf8.RuneCountInString(k.base) == 1 && unicode.IsUpper(r) {
		k.shift = true
		k.base = strings.ToLower(k.base)
	}
	return k
}

// moveChar moves the cursor by one character and says it; past the end of a
// line it goes on to the next one.
func moveChar(a *announcer, buf *vbuf, forward bool) {
	st := a.nvda
	text := []rune(lineText(buf.lines[st.line]))
	switch {
	case forward && st.offset+1 < len(text):
		st.offset++
	case forward && st.line < len(buf.lines)-1:
		setCursor(st, buf, st.line+1, 0)
	case !forward && st.offset > 0:
		st.offset--
	case !forward && st.line > 0:
		setCursor(st, buf, st.line-1, 0)
		st.offset = max(0, utf8.RuneCountInString(lineText(buf.lines[st.line]))-1)
	}
	sayNVDA(a, charName([]rune(lineText(buf.lines[st.line])), st.offset))
}

// moveWord moves the cursor to the next or previous word and says it.
func moveWord(a *announcer, buf *vbuf, forward bool) {
	st := a.nvda
	text := []rune(lineText(buf.lines[st.line]))
	starts := wordStarts(text)
	next := -1
	for _, s := range starts {
		if forward && s > st.offset {
			next = s
			break
		}
		if !forward && s < st.offset {
			next = s
		}
	}
	switch {
	case next >= 0:
		st.offset = next
	case forward && st.line < len(buf.lines)-1:
		setCursor(st, buf, st.line+1, 0)
	case !forward && st.line > 0:
		setCursor(st, buf, st.line-1, 0)
		if s := wordStarts([]rune(lineText(buf.lines[st.line]))); len(s) > 0 {
			st.offset = s[len(s)-1]
		}
	}
	sayNVDA(a, wordAt([]rune(lineText(buf.lines[st.line])), st.offset))
}

func wordStarts(text []rune) []int {
	var out []int
	for i, r := range text {
		if !unicode.IsSpace(r) && (i == 0 || unicode.IsSpace(text[i-1])) {
			out = append(out, i)
		}
	}
	return out
}

func wordAt(text []rune, off int) string {
	if off >= len(text) {
		return "blank"
	}
	end := off
	for end < len(text) && !unicode.IsSpace(text[end]) {
		end++
	}
	return string(text[off:end])
}

// charName is the character at off as NVDA says it.
func charName(text []rune, off int) string {
	if off < 0 || off >= len(text) {
		return "blank"
	}
	if unicode.IsSpace(text[off]) {
		return "space"
	}
	return symbolName(string(text[off]))
}

// quickNavKey is one quick navigation command: what it looks for, and the
// element types it matches.
type quickNavKey struct {
	what  string // "heading", "link", …, as in "no next heading"
	match func(f preset.BrowseField) bool
}

// headingLevel is the heading level quick navigation key k (lower case)
// moves by: 1-6 for the digit keys, 0 otherwise.
func headingLevel(k string) int {
	if len(k) == 1 && k[0] >= '1' && k[0] <= '6' {
		return int(k[0] - '0')
	}
	return 0
}

// quickNavError is what NVDA says when there's no next (or previous)
// element for quick navigation key k: "no next heading", or, for a heading
// level, "No next heading at level 2" (browseMode.py's nextError /
// prevError).
func quickNavError(k string, forward bool) string {
	dir := "next"
	if !forward {
		dir = "previous"
	}
	if l := headingLevel(k); l > 0 {
		return "No " + dir + " heading at level " + strconv.Itoa(l)
	}
	return "no " + dir + " " + quickNavKeys[k].what
}

// quickNavDoc is quick navigation key k's command description, as input
// help says it: "moves to the next heading", or "Moves to the next heading
// at level 2" (browseMode.py's nextDoc / prevDoc).
func quickNavDoc(k string, forward bool) string {
	dir := "next"
	if !forward {
		dir = "previous"
	}
	if l := headingLevel(k); l > 0 {
		return "Moves to the " + dir + " heading at level " + strconv.Itoa(l)
	}
	return "moves to the " + dir + " " + quickNavKeys[k].what
}

func roleIn(roles ...string) func(preset.BrowseField) bool {
	return func(f preset.BrowseField) bool {
		for _, r := range roles {
			if f.Role == r {
				return true
			}
		}
		return false
	}
}

func headingAt(level int) func(preset.BrowseField) bool {
	return func(f preset.BrowseField) bool {
		return f.Role == "heading" && f.Level != nil && *f.Level == level
	}
}

// quickNavKeys are NVDA's single-letter quick navigation keys in browse
// mode (browseMode.py's qn table), with Shift moving backwards.
var quickNavKeys = map[string]quickNavKey{
	"h": {"heading", roleIn("heading")},
	"1": {"heading", headingAt(1)},
	"2": {"heading", headingAt(2)},
	"3": {"heading", headingAt(3)},
	"4": {"heading", headingAt(4)},
	"5": {"heading", headingAt(5)},
	"6": {"heading", headingAt(6)},
	"k": {"link", roleIn("link")},
	"f": {"form field", roleIn("button", "checkbox", "radio", "switch", "combobox", "textbox", "searchbox",
		"listbox", "slider", "spinbutton", "tree", "menuitemcheckbox", "menuitemradio")},
	"l": {"list", roleIn("list", "listbox")},
	"i": {"list item", roleIn("listitem", "option")},
	"b": {"button", roleIn("button")},
	"e": {"edit field", func(f preset.BrowseField) bool {
		return f.Role == "textbox" || f.Role == "searchbox" || f.Editable
	}},
	"r": {"radio button", roleIn("radio")},
	"c": {"combo box", roleIn("combobox")},
	"x": {"check box", roleIn("checkbox")},
	"g": {"graphic", roleIn("img", "image")},
	"d": {"landmark", func(f preset.BrowseField) bool {
		switch f.Role {
		case "banner", "complementary", "contentinfo", "main", "navigation", "search":
			return true
		case "form", "region":
			return f.Name != ""
		}
		return false
	}},
	"t": {"table", roleIn("table", "grid", "treegrid")},
	"s": {"separator", roleIn("separator")},
	"q": {"block quote", roleIn("blockquote")},
}

// quickNav moves to the next (or previous) element quick navigation key name
// matches and reads it, or says there is none ("no next heading").
func quickNav(a *announcer, buf *vbuf, name string, forward bool) {
	q := quickNavKeys[name]
	st := a.nvda
	cur := -1
	if s, ok := segAt(buf, st.line, st.offset); ok {
		cur = s.node
	}
	curOrder := -1
	if cur >= 0 {
		curOrder = buf.orderOf[cur]
	}
	found := -1
	if forward {
		for p := curOrder + 1; p < len(buf.order); p++ {
			if n := buf.order[p]; quickNavMatch(buf, n, q) {
				found = n
				break
			}
		}
	} else {
		for p := curOrder - 1; p >= 0; p-- {
			if n := buf.order[p]; quickNavMatch(buf, n, q) && !(cur >= 0 && buf.isInside(cur, n)) {
				found = n
				break
			}
		}
	}
	if found < 0 {
		sayNVDA(a, quickNavError(name, forward))
		return
	}
	speakNode(a, buf, found)
}

func quickNavMatch(buf *vbuf, n int, q quickNavKey) bool {
	if buf.nodes[n].ax.Ignored {
		return false
	}
	if _, ok := buf.lineOf[n]; !ok {
		return false
	}
	return q.match(buf.nodes[n].field)
}

// activateJS is how NVDA activates an element in browse mode: it focuses it
// and, unless it takes typed text (where focusing is the point), clicks it.
const activateJS = `function () {
  const el = this.nodeType === 1 ? this : this.parentElement;
  if (!el) return;
  const typed = (el.tagName === 'INPUT' && !['checkbox', 'radio', 'button', 'submit', 'reset', 'image', 'file', 'color', 'range'].includes(el.type))
    || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable;
  if (el.focus) el.focus();
  if (!typed) el.click();
}`

// activate activates the element under the reading cursor (Enter or Space in
// browse mode). Focusing a field there makes NVDA switch to focus mode, as
// any focus change does.
func activate(a *announcer, buf *vbuf) error {
	s, ok := segAt(buf, a.nvda.line, a.nvda.offset)
	if !ok {
		return nil
	}
	n := s.node
	for n >= 0 && (buf.nodes[n].ax.BackendNodeID == 0 || buf.nodes[n].internal == "StaticText") {
		n = buf.nodes[n].parent
	}
	if n < 0 {
		return nil
	}
	var res struct {
		Object struct {
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	if err := cdpSend(a.cdp, "DOM.resolveNode", map[string]any{"backendNodeId": buf.nodes[n].ax.BackendNodeID}, &res); err != nil {
		return err
	}
	var out map[string]any
	return cdpSend(a.cdp, "Runtime.callFunctionOn", map[string]any{
		"objectId":            res.Object.ObjectID,
		"functionDeclaration": activateJS,
	}, &out)
}
