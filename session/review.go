package session

import (
	"fmt"
	"unicode"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
)

// The simulated NVDA's review mode: NVDA's review cursor in document review
// (review.py's "Document review"), reading the virtual buffer without moving
// focus, the page or the browse mode cursor, with NVDA's review commands on
// its desktop keyboard layout (the numpad), as globalCommands' script_review_*
// do. Other keys do what they did in the mode NVDA was in. The review cursor
// follows focus and the browse mode cursor (reviewCursor.followFocus and
// followCaret, on by default).

// cursorState is a reading cursor in the virtual buffer.
type cursorState struct {
	line, offset, anchor int
	url                  string
	stack                []int
}

func saveCursor(st *nvdaState) cursorState {
	return cursorState{line: st.line, offset: st.offset, anchor: st.anchor, url: st.url, stack: st.stack}
}

func loadCursor(st *nvdaState, c cursorState) {
	st.line, st.offset, st.anchor, st.url, st.stack = c.line, c.offset, c.anchor, c.url, c.stack
}

// withReview runs fn with the review cursor standing in for the browse mode
// cursor (the helpers that read and move move st's cursor), then puts the
// browse mode cursor back.
func withReview(st *nvdaState, fn func()) {
	browse := saveCursor(st)
	loadCursor(st, st.review)
	defer func() {
		st.review = saveCursor(st)
		loadCursor(st, browse)
	}()
	fn()
}

// enterReview turns review mode on, remembering the mode NVDA was in. The
// review cursor starts where the browse mode cursor is, or else at focus.
// NVDA says "Document review", the review mode's name.
func enterReview(a *announcer, st *nvdaState) {
	if modeOf(st) == action.NVDAModeReview {
		return
	}
	st.before = modeOf(st)
	st.mode = action.NVDAModeReview
	st.inReview = true
	if st.before == action.NVDAModeBrowse {
		st.review = saveCursor(st)
	} else if a != nil {
		withReview(st, func() { cursorToFocus(a) })
	}
	if a != nil {
		sayNVDA(a, "Document review")
	}
}

// leaveReview turns review mode off, back in the mode NVDA was in.
func leaveReview(st *nvdaState) {
	st.mode = st.before
	switch st.mode {
	case "", action.NVDAModeReview, action.NVDAModeOneKeyPass, action.NVDAModeInputHelp:
		st.mode = action.NVDAModeFocus
	}
	st.before = ""
	st.inReview = false
}

// reviewCommand is one of NVDA's review commands.
type reviewCommand int

const (
	reviewNone reviewCommand = iota
	reviewPreviousLine
	reviewCurrentLine
	reviewNextLine
	reviewTop
	reviewBottom
	reviewPreviousWord
	reviewCurrentWord
	reviewNextWord
	reviewPreviousCharacter
	reviewCurrentCharacter
	reviewNextCharacter
	reviewStartOfLine
	reviewEndOfLine
	reviewPreviousPage
	reviewNextPage
	reviewSayAll
)

// reviewCommandOf is the review command bound to k on NVDA's desktop
// keyboard layout: Numpad7 / 8 / 9 the previous / current / next line,
// Numpad4 / 5 / 6 the word, Numpad1 / 2 / 3 the character, Shift+Numpad7 /
// Shift+Numpad9 the top / bottom line, Shift+Numpad1 / Shift+Numpad3 the
// start / end of the line, NVDA+PageUp / NVDA+PageDown the page, and
// NumpadAdd say all.
func reviewCommandOf(k key) reviewCommand {
	if k.ctrl || k.alt || k.meta {
		return reviewNone
	}
	switch {
	case k.nvda && !k.shift && k.base == "PageUp":
		return reviewPreviousPage
	case k.nvda && !k.shift && k.base == "PageDown":
		return reviewNextPage
	case k.nvda:
		return reviewNone
	case k.shift:
		return map[string]reviewCommand{"Numpad7": reviewTop, "Numpad9": reviewBottom,
			"Numpad1": reviewStartOfLine, "Numpad3": reviewEndOfLine}[k.base]
	}
	return map[string]reviewCommand{
		"Numpad7": reviewPreviousLine, "Numpad8": reviewCurrentLine, "Numpad9": reviewNextLine,
		"Numpad4": reviewPreviousWord, "Numpad5": reviewCurrentWord, "Numpad6": reviewNextWord,
		"Numpad1": reviewPreviousCharacter, "Numpad2": reviewCurrentCharacter, "Numpad3": reviewNextCharacter,
		"NumpadAdd": reviewSayAll,
	}[k.base]
}

// reviewKey performs review command cmd with the review cursor and returns
// the result to report. What NVDA says at the edges is NVDA's: "Top" /
// "Bottom" past the first / last line or word, "Left" / "Right" past the
// start / end of the line by character, then what is under the cursor
// again.
func reviewKey(a *announcer, st *nvdaState, cmd reviewCommand) (string, error) {
	var err error
	result := "NVDA's review cursor read this: focus, the page and the browse mode cursor didn't move."
	withReview(st, func() {
		var buf *vbuf
		if buf, err = vbufFor(a); err != nil {
			return
		}
		if len(buf.lines) == 0 {
			sayNVDA(a, "blank")
			return
		}
		last := len(buf.lines) - 1
		text := []rune(lineText(buf.lines[st.line]))
		switch cmd {
		case reviewPreviousLine:
			if st.line == 0 {
				sayNVDA(a, "Top")
				speakLine(a, buf, 0)
			} else {
				speakLine(a, buf, st.line-1)
			}
		case reviewCurrentLine:
			speakLine(a, buf, st.line)
		case reviewNextLine:
			if st.line == last {
				sayNVDA(a, "Bottom")
				speakLine(a, buf, last)
			} else {
				speakLine(a, buf, st.line+1)
			}
		case reviewTop:
			speakLine(a, buf, 0)
		case reviewBottom:
			speakLine(a, buf, last)
		case reviewPreviousCharacter:
			if st.offset == 0 {
				sayNVDA(a, "Left")
			} else {
				st.offset--
			}
			sayNVDA(a, charName(text, st.offset))
		case reviewCurrentCharacter:
			sayNVDA(a, charName(text, st.offset))
		case reviewNextCharacter:
			if st.offset+1 >= len(text) {
				sayNVDA(a, "Right")
			} else {
				st.offset++
			}
			sayNVDA(a, charName(text, st.offset))
		case reviewStartOfLine:
			st.offset = 0
			sayNVDA(a, charName(text, 0))
		case reviewEndOfLine:
			st.offset = max(0, len(text)-1)
			sayNVDA(a, charName(text, st.offset))
		case reviewCurrentWord:
			sayNVDA(a, wordAt(text, wordStartAt(text, st.offset)))
		case reviewPreviousWord, reviewNextWord:
			reviewWord(a, buf, cmd == reviewNextWord)
		case reviewPreviousPage, reviewNextPage:
			// A virtual buffer has no page unit (browse mode emulates pages
			// with lines), so NVDA's review page commands report this.
			sayNVDA(a, "Movement by page not supported")
		case reviewSayAll:
			n := 0
			for l := st.line; l <= last && n < nvdaSayAllLimit; l++ {
				speakLine(a, buf, l)
				n++
			}
			result = fmt.Sprintf("NVDA read %d lines with the review cursor's say all; focus, the page and the browse mode cursor didn't move.", n)
		}
	})
	return result, err
}

// reviewWord moves the review cursor to the next or previous word, across
// lines, and says it; past the last / first word, NVDA says "Bottom" / "Top"
// and the word again.
func reviewWord(a *announcer, buf *vbuf, forward bool) {
	st := a.nvda
	text := []rune(lineText(buf.lines[st.line]))
	cur := wordStartAt(text, st.offset)
	next := -1
	for _, s := range wordStarts(text) {
		if forward && s > cur {
			next = s
			break
		}
		if !forward && s < cur {
			next = s
		}
	}
	switch {
	case next >= 0:
		st.offset = next
	case forward && st.line < len(buf.lines)-1:
		setCursor(st, buf, st.line+1, 0)
		if s := wordStarts([]rune(lineText(buf.lines[st.line]))); len(s) > 0 {
			st.offset = s[0]
		}
	case !forward && st.line > 0:
		setCursor(st, buf, st.line-1, 0)
		if s := wordStarts([]rune(lineText(buf.lines[st.line]))); len(s) > 0 {
			st.offset = s[len(s)-1]
		}
	case forward:
		sayNVDA(a, "Bottom")
	default:
		sayNVDA(a, "Top")
	}
	sayNVDA(a, wordAt([]rune(lineText(buf.lines[st.line])), st.offset))
}

// wordStartAt is where the word at off starts (off itself on a space).
func wordStartAt(text []rune, off int) int {
	if off >= len(text) || off < 0 {
		return max(0, off)
	}
	for off > 0 && !unicode.IsSpace(text[off]) && !unicode.IsSpace(text[off-1]) {
		off--
	}
	return off
}

// reviewFollow moves the review cursor where the browse mode cursor is
// (followCaret), silently.
func reviewFollow(st *nvdaState) {
	st.review = saveCursor(st)
}

// reviewToFocus moves the review cursor to the focused element
// (followFocus), silently.
func reviewToFocus(a *announcer) {
	withReview(a.nvda, func() { cursorToFocus(a) })
}
