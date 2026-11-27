package session

import (
	"strings"
	"unicode/utf8"

	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/mxschmitt/playwright-go"
)

// vbuf is NVDA's browse mode virtual buffer of a page's main frame: its
// content as lines of segments, built from Chromium's accessibility tree
// (what NVDA reads, through Chrome) and each element's CSS display (where a
// line breaks).
type vbuf struct {
	nodes     []vbufNode
	byBackend map[int]int // backendDOMNodeId -> node index
	lines     []vbufLine
	lineOf    map[int]int // node index -> the first line with content inside it
	order     []int       // node indexes in document order
	orderOf   map[int]int // node index -> its position in order
}

type vbufNode struct {
	ax       cdpAXNode
	parent   int // -1 for the root
	children []int
	display  string
	field    preset.BrowseField
	cat      preset.BrowseCategory
	internal string // Chromium's internal role (StaticText, LineBreak, ListMarker, …), if any
}

// vbufSeg is one piece of a line: a run of text, or a single-line field
// (link, button, form field, …) or marker read as one.
type vbufSeg struct {
	node  int
	text  string
	field bool
}

type vbufLine struct {
	segs []vbufSeg
}

// vbufMaxLineLength is NVDA's default maximum line length in browse mode
// (virtualBuffers.maxLineLength): longer lines wrap at a word boundary.
const vbufMaxLineLength = 100

// lineText is l's text, as character navigation moves through it.
func lineText(l vbufLine) string {
	parts := make([]string, 0, len(l.segs))
	for _, s := range l.segs {
		if s.text != "" {
			parts = append(parts, s.text)
		}
	}
	return strings.Join(parts, " ")
}

// buildVbuf builds the virtual buffer of the page s is attached to.
func buildVbuf(s playwright.CDPSession) (*vbuf, error) {
	var tree struct {
		Nodes []cdpAXNode `json:"nodes"`
	}
	if err := cdpSend(s, "Accessibility.getFullAXTree", map[string]any{}, &tree); err != nil {
		return nil, err
	}
	display, err := displays(s)
	if err != nil {
		return nil, err
	}
	return vbufFromNodes(tree.Nodes, display), nil
}

// displays maps each element's backendDOMNodeId to its CSS display, from a
// DOM snapshot of the main document.
func displays(s playwright.CDPSession) (map[int]string, error) {
	var snap struct {
		Documents []struct {
			Nodes struct {
				BackendNodeID []int `json:"backendNodeId"`
			} `json:"nodes"`
			Layout struct {
				NodeIndex []int   `json:"nodeIndex"`
				Styles    [][]int `json:"styles"`
			} `json:"layout"`
		} `json:"documents"`
		Strings []string `json:"strings"`
	}
	if err := cdpSend(s, "DOMSnapshot.captureSnapshot", map[string]any{"computedStyles": []string{"display"}}, &snap); err != nil {
		return nil, err
	}
	out := map[int]string{}
	if len(snap.Documents) == 0 {
		return out, nil
	}
	doc := snap.Documents[0]
	for i, ni := range doc.Layout.NodeIndex {
		if ni >= len(doc.Nodes.BackendNodeID) || i >= len(doc.Layout.Styles) || len(doc.Layout.Styles[i]) == 0 {
			continue
		}
		if si := doc.Layout.Styles[i][0]; si >= 0 && si < len(snap.Strings) {
			out[doc.Nodes.BackendNodeID[ni]] = snap.Strings[si]
		}
	}
	return out, nil
}

// vbufFromNodes builds the buffer from accessibility nodes (the root first,
// as Chromium sends them) and the elements' CSS display.
func vbufFromNodes(nodes []cdpAXNode, display map[int]string) *vbuf {
	b := &vbuf{byBackend: map[int]int{}, lineOf: map[int]int{}, orderOf: map[int]int{}}
	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		index[n.NodeID] = i
		vn := vbufNode{ax: n, parent: -1, display: display[n.BackendNodeID]}
		if n.Role != nil && n.Role.Type == "internalRole" {
			vn.internal = n.Role.Value
		}
		b.nodes = append(b.nodes, vn)
		if n.BackendNodeID != 0 {
			if _, dup := b.byBackend[n.BackendNodeID]; !dup {
				b.byBackend[n.BackendNodeID] = i
			}
		}
	}
	for i, n := range nodes {
		for _, c := range n.ChildIDs {
			if ci, ok := index[c]; ok && b.nodes[ci].parent == -1 && ci != i {
				b.nodes[ci].parent = i
				b.nodes[i].children = append(b.nodes[i].children, ci)
			}
		}
	}
	for i := range b.nodes {
		b.nodes[i].field = browseField(b, i)
	}
	// A dialog or application inside another field is a marker; categories
	// are worked out once fields know whether they are nested.
	for i := range b.nodes {
		b.nodes[i].field.Nested = b.hasFieldAncestor(i)
		b.nodes[i].cat = preset.NVDABrowseCategory(b.nodes[i].field)
	}
	root := -1
	for i := range b.nodes {
		if b.nodes[i].parent == -1 {
			root = i
			break
		}
	}
	if root >= 0 {
		w := &vbufWalker{b: b}
		w.visit(root)
		w.flush()
	}
	for li, l := range b.lines {
		for _, s := range l.segs {
			for n := s.node; n >= 0; n = b.nodes[n].parent {
				if _, ok := b.lineOf[n]; !ok {
					b.lineOf[n] = li
				}
			}
		}
	}
	return b
}

func (b *vbuf) hasFieldAncestor(i int) bool {
	for p := b.nodes[i].parent; p >= 0; p = b.nodes[p].parent {
		n := b.nodes[p]
		if n.ax.Ignored || n.ax.Role == nil || n.ax.Role.Type != "role" {
			continue
		}
		switch n.ax.Role.Value {
		case "generic", "none", "paragraph":
			continue
		}
		return true
	}
	return false
}

// browseField is node i as a browse field.
func browseField(b *vbuf, i int) preset.BrowseField {
	n := b.nodes[i].ax
	if n.Ignored {
		return preset.BrowseField{}
	}
	ax := axFromNode(n)
	f := preset.BrowseField{AXState: ax.State}
	if ax.Role != nil {
		f.Role = *ax.Role
	}
	if ax.Name != nil {
		f.Name = strings.TrimSpace(*ax.Name)
	}
	if ax.Value != nil {
		f.Value = *ax.Value
	}
	if ax.Description != nil {
		f.Description = *ax.Description
	}
	for _, p := range n.Properties {
		switch p.Name {
		case "editable":
			s, _ := p.Value.Value.(string)
			f.Editable = s != "" && s != "false"
		case "multiline":
			v, _ := p.Value.Value.(bool)
			f.Multiline = v
		}
	}
	switch f.Role {
	case "list":
		f.Items = b.countDescendants(i, "listitem", "list")
	case "table", "grid", "treegrid":
		for _, r := range b.descendantsWithRole(i, "row", "table") {
			f.Rows++
			f.Columns = max(f.Columns, len(b.descendantsWithRole(r, "cell", "row"))+len(b.descendantsWithRole(r, "gridcell", "row"))+
				len(b.descendantsWithRole(r, "columnheader", "row"))+len(b.descendantsWithRole(r, "rowheader", "row")))
		}
	}
	f.Content = strings.TrimSpace(b.textOf(i))
	return f
}

// countDescendants counts i's descendants with role, not looking inside
// nested elements with role stop.
func (b *vbuf) countDescendants(i int, role, stop string) int {
	return len(b.descendantsWithRole(i, role, stop))
}

func (b *vbuf) descendantsWithRole(i int, role, stop string) []int {
	var out []int
	var walk func(int)
	walk = func(n int) {
		for _, c := range b.nodes[n].children {
			cn := b.nodes[c].ax
			r := ""
			if !cn.Ignored && cn.Role != nil && cn.Role.Type == "role" {
				r = cn.Role.Value
			}
			switch {
			case r == role:
				out = append(out, c)
			case r == stop:
			default:
				walk(c)
			}
		}
	}
	walk(i)
	return out
}

// textOf is the text inside node i: its descendants' static text, joined.
func (b *vbuf) textOf(i int) string {
	var parts []string
	var walk func(int)
	walk = func(n int) {
		vn := b.nodes[n]
		if vn.ax.Ignored {
			for _, c := range vn.children {
				walk(c)
			}
			return
		}
		if vn.internal == "StaticText" && vn.ax.Name != nil {
			if t := strings.TrimSpace(vn.ax.Name.Value); t != "" {
				parts = append(parts, t)
			}
			return
		}
		for _, c := range vn.children {
			walk(c)
		}
	}
	walk(i)
	return strings.Join(parts, " ")
}

// stackOf is node i's ancestors that NVDA announces as context when the
// cursor enters them (containers and markers), outermost first.
func (b *vbuf) stackOf(i int) []int {
	var out []int
	for p := b.nodes[i].parent; p >= 0; p = b.nodes[p].parent {
		if c := b.nodes[p].cat; c == preset.BrowseContainer || c == preset.BrowseMarker {
			out = append(out, p)
		}
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out
}

// vbufWalker lays the tree out in lines.
type vbufWalker struct {
	b   *vbuf
	cur vbufLine
	n   int // characters on cur
}

func (w *vbufWalker) flush() {
	if len(w.cur.segs) > 0 {
		w.b.lines = append(w.b.lines, w.cur)
	}
	w.cur, w.n = vbufLine{}, 0
}

func (w *vbufWalker) add(s vbufSeg) {
	if s.field || utf8.RuneCountInString(s.text) <= vbufMaxLineLength-w.n {
		w.cur.segs = append(w.cur.segs, s)
		w.n += utf8.RuneCountInString(s.text) + 1
		if w.n > vbufMaxLineLength {
			w.flush()
		}
		return
	}
	// Wrap long text at a word boundary, as NVDA does past its maximum line
	// length.
	words := strings.Fields(s.text)
	var run []string
	runLen := 0
	for _, word := range words {
		l := utf8.RuneCountInString(word)
		if w.n+runLen+l > vbufMaxLineLength && (runLen > 0 || w.n > 0) {
			if runLen > 0 {
				w.cur.segs = append(w.cur.segs, vbufSeg{node: s.node, text: strings.Join(run, " ")})
			}
			w.flush()
			run, runLen = nil, 0
		}
		run = append(run, word)
		runLen += l + 1
	}
	if len(run) > 0 {
		w.cur.segs = append(w.cur.segs, vbufSeg{node: s.node, text: strings.Join(run, " ")})
		w.n += runLen
	}
}

// isBlock reports whether node i starts and ends a line: an element whose
// CSS display isn't inline, or a container or marker NVDA announces.
func (w *vbufWalker) isBlock(i int) bool {
	n := w.b.nodes[i]
	if n.cat == preset.BrowseContainer || n.cat == preset.BrowseMarker {
		return true
	}
	switch n.internal {
	case "StaticText", "InlineTextBox", "ListMarker":
		return false // text and markers take their parent's display, but flow inline
	}
	d := n.display
	return d != "" && !strings.HasPrefix(d, "inline") && d != "contents" && d != "none"
}

func (w *vbufWalker) visit(i int) {
	b := w.b
	b.orderOf[i] = len(b.order)
	b.order = append(b.order, i)
	n := b.nodes[i]
	if n.ax.Ignored {
		for _, c := range n.children {
			w.visit(c)
		}
		return
	}
	block := w.isBlock(i)
	if block {
		w.flush()
	}
	switch {
	case n.cat == preset.BrowseSingleLine:
		text := n.field.Content
		if text == "" {
			text = n.field.Name
		}
		w.add(vbufSeg{node: i, text: text, field: true})
		w.markVisited(i)
	case n.cat == preset.BrowseMarker && len(n.children) == 0:
		w.add(vbufSeg{node: i, field: true})
	case n.internal == "StaticText":
		if n.ax.Name != nil {
			if t := strings.TrimSpace(n.ax.Name.Value); t != "" {
				w.add(vbufSeg{node: i, text: t})
			}
		}
	case n.internal == "LineBreak":
		w.flush()
	case n.internal == "ListMarker":
		if n.ax.Name != nil {
			if t := strings.TrimSpace(n.ax.Name.Value); t != "" {
				w.add(vbufSeg{node: i, text: t}) // spoken as its symbol name, see symbolName
			}
		}
		w.markVisited(i)
	default:
		for _, c := range n.children {
			w.visit(c)
		}
	}
	if block {
		w.flush()
	}
}

// markVisited puts i's descendants in document order without laying them
// out: a field read as one piece.
func (w *vbufWalker) markVisited(i int) {
	for _, c := range w.b.nodes[i].children {
		w.b.orderOf[c] = len(w.b.order)
		w.b.order = append(w.b.order, c)
		w.markVisited(c)
	}
}

// symbolName is how NVDA says a list marker's symbol: a bullet as "bullet"
// (its symbol name), anything else (a number) as it is.
func symbolName(t string) string {
	switch t {
	case "•":
		return "bullet"
	case "◦":
		return "white bullet"
	case "▪", "■":
		return "black square"
	}
	return t
}
