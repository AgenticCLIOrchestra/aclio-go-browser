package session

import (
	"reflect"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

func TestParseKey(t *testing.T) {
	for in, want := range map[string]key{
		"h":                {base: "h"},
		"H":                {base: "h", shift: true},
		"Shift+H":          {base: "h", shift: true},
		"Control+Home":     {base: "Home", ctrl: true},
		"NVDA+Space":       {base: "Space", nvda: true},
		"Insert+ArrowDown": {base: "ArrowDown", nvda: true},
		"CapsLock+Space":   {base: "Space", nvda: true},
		" ":                {base: "Space"},
		"+":                {base: "+"},
		"Shift++":          {base: "+", shift: true},
		"Tab":              {base: "Tab"},
	} {
		if got := parseKey(in); got != want {
			t.Errorf("parseKey(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func axn(id, parent string, role string, internal bool, name string, children ...string) cdpAXNode {
	n := cdpAXNode{NodeID: id, ParentID: parent, ChildIDs: children}
	typ := "role"
	if internal {
		typ = "internalRole"
	}
	n.Role = &struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	}{typ, role}
	if name != "" {
		n.Name = &struct {
			Value   string `json:"value"`
			Sources []struct {
				Type         string `json:"type"`
				Attribute    string `json:"attribute"`
				NativeSource string `json:"nativeSource"`
				Superseded   bool   `json:"superseded"`
				Value        *struct {
					Value string `json:"value"`
				} `json:"value"`
			} `json:"sources"`
		}{Value: name}
	}
	return n
}

// The buffer breaks lines at block elements and keeps inline ones (text, a
// link) on one line; a long text wraps at 100 characters; a list is a
// container the lines inside it are in.
func TestVbufLines(t *testing.T) {
	long := "word "
	for len(long) < 150 {
		long += "word "
	}
	nodes := []cdpAXNode{
		axn("1", "", "RootWebArea", true, "", "2", "5", "7"),
		axn("2", "1", "paragraph", false, "", "3", "4"),
		axn("3", "2", "StaticText", true, "Read the"),
		axn("4", "2", "link", false, "terms", "41"),
		axn("41", "4", "StaticText", true, "terms"),
		axn("5", "1", "list", false, "", "6"),
		axn("6", "5", "listitem", false, "", "61"),
		axn("61", "6", "StaticText", true, "Apples"),
		axn("7", "1", "paragraph", false, "", "71"),
		axn("71", "7", "StaticText", true, long),
	}
	for i := range nodes {
		nodes[i].BackendNodeID = i + 1
	}
	display := map[int]string{2: "block", 5: "inline", 6: "block", 7: "list-item", 8: "inline", 9: "block"}
	buf := vbufFromNodes(nodes, display)
	var lines []string
	for _, l := range buf.lines {
		lines = append(lines, lineText(l))
	}
	if len(lines) != 4 || lines[0] != "Read the terms" || lines[1] != "Apples" {
		t.Fatalf("lines = %q", lines)
	}
	for _, l := range lines[2:] {
		if len([]rune(l)) > vbufMaxLineLength {
			t.Errorf("line longer than %d: %q", vbufMaxLineLength, l)
		}
	}
	if got := buf.nodes[buf.lines[1].segs[0].node].field; got.Role != "" {
		t.Errorf("text segment field = %+v", got)
	}
	if stack := buf.stackOf(buf.lines[1].segs[0].node); len(stack) != 1 || buf.nodes[stack[0]].field.Role != "list" || buf.nodes[stack[0]].field.Items != 1 {
		t.Errorf("Apples' stack = %v", stack)
	}
	items, _ := speakSegs(buf, buf.lines[0].segs, nil, preset.BrowseCaret)
	if want := []string{"Read the", "link", "terms"}; !reflect.DeepEqual(items, want) {
		t.Errorf("line 1 = %q, want %q", items, want)
	}
	items, stack := speakSegs(buf, buf.lines[1].segs, nil, preset.BrowseCaret)
	if want := []string{"list", "with 1 item", "Apples"}; !reflect.DeepEqual(items, want) {
		t.Errorf("line 2 = %q, want %q", items, want)
	}
	if items, _ := speakSegs(buf, buf.lines[2].segs, stack, preset.BrowseCaret); len(items) == 0 || items[0] != "out of list" {
		t.Errorf("line 3 = %q, want out of list first", items)
	}
}
