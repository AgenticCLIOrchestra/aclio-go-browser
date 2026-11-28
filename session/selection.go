package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Text selection in the simulated NVDA's browse mode, as NVDA's
// cursorManager (the select* scripts and copyToClipboard) and
// speech.speakSelectionChange do it, and native selection mode (NVDA+Shift+
// F10, browseMode's toggleNativeAppSelectionMode, which NVDA's Chrome buffer
// supports): while it is on, NVDA mirrors its selection into the page's own
// selection, and Control+C reaches the page, which copies it itself.

// selection is a selected range of the virtual buffer's text (its lines
// joined by "\n"), from anchor to active (the end that moves), in
// characters.
type selection struct {
	anchor, active int
}

// selectionBounds is s's start and end, in order.
func selectionBounds(s selection) (int, int) {
	if s.anchor <= s.active {
		return s.anchor, s.active
	}
	return s.active, s.anchor
}

// flatText is the buffer's text, its lines joined by "\n", with each line's
// start in it.
func flatText(buf *vbuf) ([]rune, []int) {
	var text []rune
	starts := make([]int, len(buf.lines))
	for i, l := range buf.lines {
		if i > 0 {
			text = append(text, '\n')
		}
		starts[i] = len(text)
		text = append(text, []rune(lineText(l))...)
	}
	return text, starts
}

// caretPos is the reading cursor's position in the flat text.
func caretPos(st *nvdaState, starts []int) int {
	if st.line < 0 || st.line >= len(starts) {
		return 0
	}
	return starts[st.line] + st.offset
}

// lineAt is the line the flat position pos is on.
func lineAt(starts []int, pos int) int {
	l := 0
	for i, s := range starts {
		if s <= pos {
			l = i
		}
	}
	return l
}

// moveCaretTo puts the reading cursor at flat position pos (which may be
// just past the end of a line, as a selection's end may be).
func moveCaretTo(st *nvdaState, buf *vbuf, starts []int, pos int) {
	l := lineAt(starts, pos)
	setCursor(st, buf, l, pos-starts[l])
}

// selectionText is how NVDA says a selected or unselected piece of text: a
// single character spelled, blank text as "blank", line breaks as spaces.
func selectionText(t []rune) string {
	if len(t) == 1 {
		return charName(t, 0)
	}
	s := strings.Join(strings.Fields(string(t)), " ")
	if s == "" {
		return "blank"
	}
	return s
}

// speakSelectionChange says what changed between selections old and new, as
// NVDA's speech.speakSelectionChange does: newly selected text "… selected",
// no longer selected text "… unselected".
func speakSelectionChange(a *announcer, text []rune, old, new selection) {
	os, oe := selectionBounds(old)
	ns, ne := selectionBounds(new)
	oldCollapsed, newCollapsed := os == oe, ns == ne
	var selected, unselected [][]rune
	switch {
	case oldCollapsed && newCollapsed:
		return
	case oldCollapsed:
		selected = append(selected, text[ns:ne])
	case newCollapsed:
		unselected = append(unselected, text[os:oe])
	case ns > oe || ne < os:
		selected = append(selected, text[ns:ne])
		unselected = append(unselected, text[os:oe])
	default:
		if ns < os {
			selected = append(selected, text[ns:os])
		}
		if ne > oe {
			selected = append(selected, text[oe:ne])
		}
		if ns > os {
			unselected = append(unselected, text[os:ns])
		}
		if ne < oe {
			unselected = append(unselected, text[ne:oe])
		}
	}
	for _, t := range selected {
		sayNVDA(a, selectionText(t)+" selected")
	}
	for _, t := range unselected {
		sayNVDA(a, selectionText(t)+" unselected")
	}
}

// selectKey handles the selection keys of browse mode: Shift with the arrow
// keys, Home / End and Page Up / Down, Shift+Control with the arrows and
// Home / End, and Control+A. It reports false for any other key.
func selectKey(a *announcer, buf *vbuf, k key) (bool, error) {
	st := a.nvda
	text, starts := flatText(buf)
	old := selection{caretPos(st, starts), caretPos(st, starts)}
	if st.sel != nil {
		old = *st.sel
	}
	l := lineAt(starts, old.active)
	lineEnd := starts[l] + utf8.RuneCountInString(lineText(buf.lines[l]))
	nextLine := func(n int) int {
		if l+n >= len(starts) {
			return len(text)
		}
		return starts[l+n]
	}
	prevLine := func(n int) int {
		if old.active > starts[l] {
			n--
		}
		return starts[max(0, l-n)]
	}
	new := old
	switch {
	case k.ctrl && !k.shift && !k.alt && !k.nvda && strings.ToLower(k.base) == "a":
		new = selection{0, len(text)}
	case !k.shift || k.alt || k.meta || k.nvda:
		return false, nil
	case k.ctrl && k.base == "ArrowRight":
		new.active = nextWordStart(text, old.active)
	case k.ctrl && k.base == "ArrowLeft":
		new.active = prevWordStart(text, old.active)
	case k.ctrl && k.base == "Home":
		new.active = 0
	case k.ctrl && k.base == "End":
		new.active = len(text)
	case k.ctrl:
		return false, nil
	case k.base == "ArrowRight":
		new.active = min(old.active+1, len(text))
	case k.base == "ArrowLeft":
		new.active = max(old.active-1, 0)
	case k.base == "ArrowDown":
		new.active = nextLine(1)
	case k.base == "ArrowUp":
		new.active = prevLine(1)
	case k.base == "PageDown":
		new.active = nextLine(nvdaLinesPerPage)
	case k.base == "PageUp":
		new.active = prevLine(nvdaLinesPerPage)
	case k.base == "End":
		new.active = lineEnd
	case k.base == "Home":
		new.active = starts[l]
	default:
		return false, nil
	}
	speakSelectionChange(a, text, old, new)
	setSelection(st, new)
	moveCaretTo(st, buf, starts, new.active)
	return true, syncAppSelection(a, buf)
}

func setSelection(st *nvdaState, s selection) {
	if s.anchor == s.active {
		st.sel = nil
		return
	}
	st.sel = &s
}

// unselect drops the selection when the reading cursor moves without
// Shift, saying what is no longer selected, as NVDA does after reading where
// the cursor moved to.
func unselect(a *announcer, buf *vbuf, old selection) error {
	text, _ := flatText(buf)
	s, e := selectionBounds(old)
	if e > len(text) {
		e = len(text)
	}
	if s < e {
		sayNVDA(a, selectionText(text[s:e])+" unselected")
	}
	a.nvda.sel = nil
	return syncAppSelection(a, buf)
}

func nextWordStart(text []rune, pos int) int {
	for pos < len(text) && !unicode.IsSpace(text[pos]) {
		pos++
	}
	for pos < len(text) && unicode.IsSpace(text[pos]) {
		pos++
	}
	return pos
}

func prevWordStart(text []rune, pos int) int {
	for pos > 0 && unicode.IsSpace(text[pos-1]) {
		pos--
	}
	for pos > 0 && !unicode.IsSpace(text[pos-1]) {
		pos--
	}
	return pos
}

// copyJS copies its argument to the clipboard as the system clipboard NVDA
// writes to: through a copy event it answers first, so the page's own copy
// handlers don't run.
const copyJS = `function (text) {
  const onCopy = (e) => { e.clipboardData.setData('text/plain', text); e.preventDefault(); e.stopImmediatePropagation(); };
  window.addEventListener('copy', onCopy, { capture: true, once: true });
  let ok = false;
  try { ok = document.execCommand('copy'); } catch (e) {}
  window.removeEventListener('copy', onCopy, { capture: true });
  return ok;
}`

// copySelection copies NVDA's selection to the clipboard (Control+C in browse
// mode without native selection) and says so: "Copied to clipboard: …" (or
// the number of characters, past 1024), "No selection", or "Unable to
// copy".
func copySelection(a *announcer, buf *vbuf) error {
	if a.nvda.sel == nil {
		sayNVDA(a, "No selection")
		return nil
	}
	text, _ := flatText(buf)
	s, e := selectionBounds(*a.nvda.sel)
	e = min(e, len(text))
	copied := string(text[s:e])
	var res struct {
		Result struct {
			Value bool `json:"value"`
		} `json:"result"`
	}
	arg, _ := json.Marshal(copied)
	if err := cdpSend(a.cdp, "Runtime.evaluate", map[string]any{
		"expression":    "(" + copyJS + ")(" + string(arg) + ")",
		"userGesture":   true,
		"returnByValue": true,
	}, &res); err != nil || !res.Result.Value {
		sayNVDA(a, "Unable to copy")
		return nil
	}
	spoken := copied
	if n := utf8.RuneCountInString(copied); n >= 1024 {
		spoken = fmt.Sprintf("%d characters", n)
	}
	sayNVDA(a, "Copied to clipboard: "+spoken)
	return nil
}

// toggleNativeSelection turns native selection mode on or off (NVDA+Shift+
// F10), saying "Native app selection mode enabled" / "disabled". Turning it
// on puts NVDA's selection into the page; turning it off collapses the
// page's selection.
func toggleNativeSelection(a *announcer, on bool) error {
	st := a.nvda
	st.native = on
	if on {
		sayNVDA(a, "Native app selection mode enabled")
	} else {
		sayNVDA(a, "Native app selection mode disabled")
	}
	buf, err := vbufFor(a)
	if err != nil {
		return err
	}
	if !on {
		return setPageSelection(a, buf, nil)
	}
	return syncAppSelection(a, buf)
}

// syncAppSelection mirrors NVDA's selection into the page's own selection
// while native selection mode is on.
func syncAppSelection(a *announcer, buf *vbuf) error {
	if !a.nvda.native {
		return nil
	}
	return setPageSelection(a, buf, a.nvda.sel)
}

// domPoint is the DOM text position of flat position pos: the backend id of
// a text node and an offset in it (approximate where the accessibility
// tree's text differs from the DOM's whitespace).
func domPoint(buf *vbuf, starts []int, pos int) (int, int, bool) {
	l := lineAt(starts, pos)
	off := pos - starts[l]
	at := 0
	segs := buf.lines[l].segs
	for i, s := range segs {
		n := utf8.RuneCountInString(s.text)
		if off <= at+n || i == len(segs)-1 {
			node := s.node
			if s.field {
				if t := firstText(buf, node); t >= 0 {
					node = t
				}
			}
			id := buf.nodes[node].ax.BackendNodeID
			if id == 0 {
				return 0, 0, false
			}
			within := min(max(off-at, 0), n)
			if full := buf.nodes[node].ax.Name; full != nil && !s.field {
				if idx := strings.Index(full.Value, s.text); idx >= 0 {
					within += utf8.RuneCountInString(full.Value[:idx])
				}
			}
			return id, within, true
		}
		at += n + 1
	}
	return 0, 0, false
}

// firstText is node n's first static text descendant, or -1.
func firstText(buf *vbuf, n int) int {
	for _, c := range buf.nodes[n].children {
		if buf.nodes[c].internal == "StaticText" {
			return c
		}
		if t := firstText(buf, c); t >= 0 {
			return t
		}
	}
	return -1
}

// setPageSelectionJS selects from (anchor node, offset) to (focus node,
// offset) in the page, or clears the selection without arguments.
const setPageSelectionJS = `function (focusNode, anchorOffset, focusOffset) {
  const s = window.getSelection();
  if (!focusNode) { s.removeAllRanges(); return; }
  const clamp = (n, o) => Math.min(o, n.nodeType === 3 ? n.data.length : n.childNodes.length);
  s.setBaseAndExtent(this, clamp(this, anchorOffset), focusNode, clamp(focusNode, focusOffset));
}`

// setPageSelection sets the page's selection to sel, or clears it for nil.
func setPageSelection(a *announcer, buf *vbuf, sel *selection) error {
	if sel == nil {
		var out map[string]any
		return cdpSend(a.cdp, "Runtime.evaluate", map[string]any{
			"expression": "window.getSelection().removeAllRanges()",
		}, &out)
	}
	_, starts := flatText(buf)
	anchorID, anchorOff, ok1 := domPoint(buf, starts, sel.anchor)
	focusID, focusOff, ok2 := domPoint(buf, starts, sel.active)
	if !ok1 || !ok2 {
		return nil
	}
	resolve := func(id int) (string, error) {
		var res struct {
			Object struct {
				ObjectID string `json:"objectId"`
			} `json:"object"`
		}
		err := cdpSend(a.cdp, "DOM.resolveNode", map[string]any{"backendNodeId": id}, &res)
		return res.Object.ObjectID, err
	}
	anchorObj, err := resolve(anchorID)
	if err != nil {
		return err
	}
	focusObj, err := resolve(focusID)
	if err != nil {
		return err
	}
	var out map[string]any
	return cdpSend(a.cdp, "Runtime.callFunctionOn", map[string]any{
		"objectId":            anchorObj,
		"functionDeclaration": setPageSelectionJS,
		"arguments":           []map[string]any{{"objectId": focusObj}, {"value": anchorOff}, {"value": focusOff}},
	}, &out)
}
