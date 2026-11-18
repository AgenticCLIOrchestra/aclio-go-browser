package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/mxschmitt/playwright-go"
	"github.com/yosssi/gohtml"
)

// snapshotStampLayout names each turn's files: fixed-width UTC, no colons, so
// they sort chronologically and are valid on every OS.
const snapshotStampLayout = "20060102T150405.000000000Z"

// formatDOM puts each element on its own indented line. Pages often come back
// from Content() as a few enormous lines, and the agent reads the file by line
// range, so a one-line DOM would be unreadable to it. Whitespace inside text
// (e.g. in <pre>) may be re-indented; the structure is unchanged.
func formatDOM(dom string) string {
	return gohtml.Format(dom)
}

// Snapshot captures the parts of the active tab's state that perceive asks
// for. The screenshot and DOM are written into dir (created if needed) as
// {UTC timestamp}.png and {UTC timestamp}.html; their absolute paths go into
// the returned State. Snapshot never changes the page.
func Snapshot(b *Browser, perceive preset.Perception, dir string) (preset.State, error) {
	if err := checkOpen(b); err != nil {
		return preset.State{}, err
	}
	var st preset.State

	if perceive.Screenshot || perceive.DOM {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return preset.State{}, fmt.Errorf("session: snapshot dir: %w", err)
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return preset.State{}, fmt.Errorf("session: creating snapshot dir: %w", err)
		}
		base := filepath.Join(abs, time.Now().UTC().Format(snapshotStampLayout))

		if perceive.Screenshot {
			st.ScreenshotPath = base + ".png"
			if _, err := b.active.Screenshot(playwright.PageScreenshotOptions{
				Path: playwright.String(st.ScreenshotPath),
			}); err != nil {
				return preset.State{}, fmt.Errorf("session: capturing screenshot: %w", err)
			}
		}
		if perceive.DOM {
			st.DOMPath = base + ".html"
			if err := writeDOM(b, st.DOMPath); err != nil {
				return preset.State{}, err
			}
		}
	}

	if perceive.Focused {
		fe, err := Focused(b)
		if err != nil {
			return preset.State{}, err
		}
		st.Focused = fe
	}
	if perceive.Tabs {
		tabs, err := Tabs(b)
		if err != nil {
			return preset.State{}, err
		}
		st.Tabs = tabs
	}
	return st, nil
}

// WriteDOM writes the active tab's DOM to path, formatted exactly as Snapshot
// formats it, creating path's directory if needed. It is for keeping a DOM
// the agent doesn't perceive (e.g. a debug copy for the Blind preset).
// WriteDOM never changes the page.
func WriteDOM(b *Browser, path string) error {
	if err := checkOpen(b); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("session: creating DOM dir: %w", err)
	}
	return writeDOM(b, path)
}

// writeDOM captures the active tab's DOM and writes it, formatted, to path.
func writeDOM(b *Browser, path string) error {
	dom, err := b.active.Content()
	if err != nil {
		return fmt.Errorf("session: capturing DOM: %w", err)
	}
	if err := os.WriteFile(path, []byte(formatDOM(dom)), 0o644); err != nil {
		return fmt.Errorf("session: writing DOM: %w", err)
	}
	return nil
}

// Focused returns a screen-reader view of the active tab's focused element,
// or nil when nothing (or just the body) is focused. It needs Chromium: the
// roles come from Chromium's own accessibility tree, the one a screen reader
// is given, which Firefox doesn't expose.
func Focused(b *Browser) (*preset.FocusedElement, error) {
	if err := checkOpen(b); err != nil {
		return nil, err
	}
	if b.engine != engine.Chromium {
		return nil, fmt.Errorf("session: the focused element needs Chromium, not %s", b.engine)
	}
	s, err := b.context.NewCDPSession(b.active)
	if err != nil {
		return nil, fmt.Errorf("session: reading the focused element: %w", err)
	}
	defer s.Detach()
	id, err := objectID(s, `document.activeElement`)
	if err != nil || id == "" {
		return nil, err
	}
	fe, err := focusedOf(s, id)
	if err != nil {
		return nil, fmt.Errorf("session: reading the focused element: %w", err)
	}
	return fe, nil
}

// focusedOf builds the screen-reader view of the element with CDP object id
// id: focusedElementJS run on it, then addAX. nil when it is the body or the
// document element (nothing meaningful focused).
func focusedOf(s playwright.CDPSession, id string) (*preset.FocusedElement, error) {
	var res struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails any `json:"exceptionDetails"`
	}
	if err := cdpSend(s, "Runtime.callFunctionOn", map[string]any{
		"objectId":            id,
		"functionDeclaration": focusedElementJS,
		"returnByValue":       true,
	}, &res); err != nil {
		return nil, err
	}
	if res.ExceptionDetails != nil {
		return nil, fmt.Errorf("focusedElementJS: %v", res.ExceptionDetails)
	}
	if len(res.Result.Value) == 0 || string(res.Result.Value) == "null" {
		return nil, nil
	}
	var fe preset.FocusedElement
	if err := json.Unmarshal(res.Result.Value, &fe); err != nil {
		return nil, err
	}
	if err := addAX(s, id, &fe); err != nil {
		return nil, fmt.Errorf("accessibility node: %w", err)
	}
	return &fe, nil
}

// objectID evaluates expr in the page and returns the CDP object id of the
// element it gives, "" when it gives none.
func objectID(s playwright.CDPSession, expr string) (string, error) {
	var ev struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := cdpSend(s, "Runtime.evaluate", map[string]any{"expression": expr}, &ev); err != nil {
		return "", err
	}
	return ev.Result.ObjectID, nil
}

// addAX fills in the role, the accessible name and its sources, the value, the
// description and the states and properties of the element with object id
// id and of its active descendant, and the element's context (its announced
// ancestors), from Chromium's accessibility tree. An element the tree ignores
// is reduced to that fact and its reasons.
func addAX(s playwright.CDPSession, id string, fe *preset.FocusedElement) error {
	ax, err := axNode(s, id, true)
	if err != nil {
		return err
	}
	if ax.State.Ignored {
		// A screen reader gets nothing for it, so nothing else is reported.
		*fe = preset.FocusedElement{AXState: ax.State, NodeID: ax.NodeID}
		return nil
	}
	// The DOM's required (see focusedElementJS) stands in when the tree leaves
	// it out, as it does for a required checkbox.
	domRequired := fe.Required
	fe.Role, fe.Name, fe.NameSources, fe.Value, fe.Description, fe.AXState = ax.Role, ax.Name, ax.NameSources, ax.Value, ax.Description, ax.State
	fe.Required = fe.Required || domRequired
	fe.Context, fe.NodeID = ax.Context, ax.NodeID
	if ad := fe.ActiveDescendant; ad != nil && !ad.Missing {
		adID, err := callForObject(s, id, activeDescendantJS)
		if err != nil || adID == "" {
			return err
		}
		if ax, err = axNode(s, adID, false); err != nil {
			return err
		}
		if ax.State.Ignored {
			*ad = preset.ActiveDescendant{AXState: ax.State, NodeID: ax.NodeID}
			return nil
		}
		ad.Role, ad.Name, ad.NameSources, ad.Value, ad.Description, ad.AXState = ax.Role, ax.Name, ax.NameSources, ax.Value, ax.Description, ax.State
		ad.NodeID = ax.NodeID
	}
	return nil
}

// callForObject calls fn on the object with id and returns the object id of
// the element it returns, "" when it returns none.
func callForObject(s playwright.CDPSession, id, fn string) (string, error) {
	var res struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := cdpSend(s, "Runtime.callFunctionOn", map[string]any{
		"objectId":            id,
		"functionDeclaration": fn,
	}, &res); err != nil {
		return "", err
	}
	return res.Result.ObjectID, nil
}

// activeDescendantJS finds an element's aria-activedescendant (this is the
// element) the way focusedElementJS does: in the element's own tree first,
// then the document.
const activeDescendantJS = `function () {
  const el = this;
  const id = el.getAttribute('aria-activedescendant');
  if (!id) return null;
  const scope = el.getRootNode && el.getRootNode().getElementById ? el.getRootNode() : document;
  return scope.getElementById(id) || document.getElementById(id);
}`

// cdpAXNode is the part of a CDP Accessibility.AXNode we read.
type cdpAXNode struct {
	NodeID         string `json:"nodeId"`
	BackendNodeID  int    `json:"backendDOMNodeId"`
	ParentID       string `json:"parentId"`
	Ignored        bool   `json:"ignored"`
	IgnoredReasons []struct {
		Name string `json:"name"`
	} `json:"ignoredReasons"`
	Role *struct {
		Type  string `json:"type"` // "role", or "internalRole" for Chromium's own
		Value string `json:"value"`
	} `json:"role"`
	Name *struct {
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
	} `json:"name"`
	Value *struct {
		Value any `json:"value"` // a string, or a number for e.g. a slider
	} `json:"value"`
	Description *struct {
		Value string `json:"value"`
	} `json:"description"`
	Properties []struct {
		Name  string `json:"name"`
		Value struct {
			Value any `json:"value"`
		} `json:"value"`
	} `json:"properties"`
}

// axInfo is what we take from an accessibility node. The zero value means no
// node; one left out of the tree a screen reader gets has only State.Ignored
// and State.IgnoredReasons set.
type axInfo struct {
	Role        *string
	Name        *string
	NameSources []preset.NameSource
	Value       *string
	Description *string
	State       preset.AXState
	Context     []preset.ContextItem
	NodeID      int // Chromium's DOM node id (backendDOMNodeId)
}

// axFromNode picks the role, name, name sources, value, description and states
// out of n. Only sources that had a value are kept, once each, in Chromium's
// (precedence) order. A numeric value (a slider, a spinbutton) is written out
// plainly.
func axFromNode(n cdpAXNode) axInfo {
	if n.Ignored {
		st := preset.AXState{Ignored: true}
		for _, r := range n.IgnoredReasons {
			st.IgnoredReasons = append(st.IgnoredReasons, r.Name)
		}
		return axInfo{State: st, NodeID: n.BackendNodeID}
	}
	ax := axInfo{NodeID: n.BackendNodeID}
	ax.Role = ariaRole(n)
	if n.Value != nil {
		switch v := n.Value.Value.(type) {
		case string:
			ax.Value = &v
		case float64:
			s := strconv.FormatFloat(v, 'f', -1, 64)
			ax.Value = &s
		}
	}
	if n.Description != nil && n.Description.Value != "" {
		ax.Description = &n.Description.Value
	}
	ax.State = axState(n)
	if n.Name == nil {
		return ax
	}
	ax.Name = &n.Name.Value
	seen := map[preset.NameSource]bool{}
	for _, src := range n.Name.Sources {
		if src.Value == nil {
			continue
		}
		ns := preset.NameSource{From: src.Attribute, Value: src.Value.Value, Superseded: src.Superseded}
		if ns.From == "" {
			ns.From = src.NativeSource
		}
		if ns.From == "" {
			ns.From = src.Type
		}
		key := preset.NameSource{From: ns.From, Value: ns.Value}
		if seen[key] {
			continue
		}
		seen[key] = true
		ax.NameSources = append(ax.NameSources, ns)
	}
	return ax
}

// internalRoleARIA maps the internal roles Chromium gives some elements (no
// ARIA role, Chromium's own name instead) to the ARIA role reported in their
// place. Most have no corresponding ARIA role (per HTML-AAM) and become
// "generic"; the date and time inputs, which take typed text, become
// "textbox" (as Playwright's role computation has them); <summary> and the
// colour input, which act as buttons from the keyboard (toggle open, open a
// picker), become "button".
var internalRoleARIA = map[string]string{
	"Date":               "textbox", // <input type=date>
	"DateTime":           "textbox", // <input type=datetime-local|month|week>
	"InputTime":          "textbox", // <input type=time>
	"ColorWell":          "button",  // <input type=color>
	"DisclosureTriangle": "button",  // <summary>
	"LabelText":          "generic", // <label>
	"Abbr":               "generic", // <abbr>
	"Video":              "generic", // <video>
	"Audio":              "generic", // <audio>
	"Canvas":             "generic", // <canvas>
	"Iframe":             "generic", // <iframe>
	"PluginObject":       "generic", // <object>
	"EmbeddedObject":     "generic", // <embed>
}

// ariaRole is n's role as an ARIA role: Chromium's role when it is one,
// otherwise its internal role mapped by internalRoleARIA, and nil for an
// internal role the map doesn't know.
func ariaRole(n cdpAXNode) *string {
	if n.Role == nil || n.Role.Value == "" {
		return nil
	}
	if n.Role.Type != "internalRole" {
		return &n.Role.Value
	}
	if r, ok := internalRoleARIA[n.Role.Value]; ok {
		return &r
	}
	return nil
}

// axState reads the states and properties we report out of n's property
// list. Properties Chromium leaves out stay unset; so do "false" for invalid
// and an empty valuetext. Properties a screen reader doesn't announce
// (focusable, editable, settable, live, …) are skipped.
func axState(n cdpAXNode) preset.AXState {
	var st preset.AXState
	for _, p := range n.Properties {
		v := p.Value.Value
		str, isStr := v.(string)
		b, isBool := v.(bool)
		num, isNum := v.(float64)
		switch p.Name {
		case "checked":
			if isStr {
				st.Checked = &str
			}
		case "pressed":
			if isStr {
				st.Pressed = &str
			}
		case "selected":
			if isBool {
				st.Selected = &b
			}
		case "expanded":
			if isBool {
				st.Expanded = &b
			}
		case "disabled":
			st.Disabled = isBool && b
		case "readonly":
			st.ReadOnly = isBool && b
		case "required":
			st.Required = isBool && b
		case "invalid":
			if isStr && str != "false" {
				st.Invalid = &str
			}
		case "level":
			if isNum {
				l := int(num)
				st.Level = &l
			}
		case "hasPopup":
			if isStr && str != "false" {
				st.HasPopup = &str
			}
		case "multiselectable":
			st.Multiselectable = isBool && b
		case "orientation":
			if isStr {
				st.Orientation = &str
			}
		case "valuemin":
			if isNum {
				st.ValueMin = &num
			}
		case "valuemax":
			if isNum {
				st.ValueMax = &num
			}
		case "valuetext":
			if isStr && str != "" {
				st.ValueText = &str
			}
		case "autocomplete":
			if isStr && str != "none" {
				st.Autocomplete = &str
			}
		case "keyshortcuts":
			if isStr && str != "" {
				st.KeyShortcuts = &str
			}
		case "roledescription":
			if isStr && str != "" {
				st.RoleDescription = &str
			}
		case "modal":
			st.Modal = isBool && b
		}
	}
	return st
}

// axContext walks from the node with id up through its ancestors in nodes and
// returns the ones a screen reader may announce as context, outermost first:
// not ignored, not a generic wrapper or role "none", and not one of
// Chromium's internal roles (which include the document root).
func axContext(nodes []cdpAXNode, id string) []preset.ContextItem {
	byID := make(map[string]cdpAXNode, len(nodes))
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	var ctx []preset.ContextItem
	seen := map[string]bool{id: true}
	for n, ok := byID[byID[id].ParentID]; ok && !seen[n.NodeID]; n, ok = byID[n.ParentID] {
		seen[n.NodeID] = true
		if n.Ignored || n.Role == nil || n.Role.Type != "role" || n.Role.Value == "generic" || n.Role.Value == "none" {
			continue
		}
		item := preset.ContextItem{Role: n.Role.Value}
		if n.Name != nil {
			item.Name = n.Name.Value
		}
		ctx = append(ctx, item)
	}
	slices.Reverse(ctx)
	return ctx
}

// axNode returns what the node in Chromium's accessibility tree of the
// element with object id id says. With withContext, it also fetches the
// node's ancestors for axInfo.Context.
func axNode(s playwright.CDPSession, id string, withContext bool) (axInfo, error) {
	var tree struct {
		Nodes []cdpAXNode `json:"nodes"`
	}
	if err := cdpSend(s, "Accessibility.getPartialAXTree", map[string]any{
		"objectId":       id,
		"fetchRelatives": false,
	}, &tree); err != nil {
		return axInfo{}, err
	}
	if len(tree.Nodes) == 0 {
		return axInfo{}, nil
	}
	ax := axFromNode(tree.Nodes[0])
	if !withContext || ax.State.Ignored {
		return ax, nil
	}
	// Fetched separately: with relatives, which node is the requested one
	// isn't specified, so it is found by the id from the call above.
	var rel struct {
		Nodes []cdpAXNode `json:"nodes"`
	}
	if err := cdpSend(s, "Accessibility.getPartialAXTree", map[string]any{
		"objectId":       id,
		"fetchRelatives": true,
	}, &rel); err != nil {
		return axInfo{}, err
	}
	ax.Context = axContext(rel.Nodes, tree.Nodes[0].NodeID)
	return ax, nil
}

// cdpSend sends a CDP command and decodes its result into out.
func cdpSend(s playwright.CDPSession, method string, params map[string]any, out any) error {
	res, err := s.Send(method, params)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// focusedElementJS reads what Chromium's accessibility tree doesn't give us
// (the role, name, value, states and properties come from there, see addAX)
// off the element it is called on (this; null for the body or the document
// element): text skips hidden subtrees and uses descendants' aria-label / img
// alt; inline elements join without spaces; required stands in for the
// tree's when it leaves it out; the position in a set is counted; only the
// aria-* attributes no other field covers are kept; composite widgets (grid,
// listbox, menu, …) report their aria-activedescendant instead of their own
// content. A field is included only when its source is present, so "" stays
// distinct from absent.
const focusedElementJS = `function () {
  const el = this;
  if (!el || el.nodeType !== Node.ELEMENT_NODE || el === document.body || el === document.documentElement) return null;

  const COMPOSITE = new Set(['grid', 'treegrid', 'listbox', 'menu', 'menubar', 'tree', 'tablist', 'radiogroup']);
  const MAX = 500;

  const isHidden = (n) => {
    if (n.getAttribute('aria-hidden') === 'true' || n.hidden) return true;
    const cs = window.getComputedStyle(n);
    return cs.display === 'none' || cs.visibility === 'hidden';
  };
  const isInline = (n) => {
    const d = window.getComputedStyle(n).display;
    return d === 'inline' || d === 'contents';
  };
  const srText = (root) => {
    const parts = [];
    const walk = (n, isRoot) => {
      if (n.nodeType === Node.TEXT_NODE) { parts.push(n.data); return; }
      if (n.nodeType !== Node.ELEMENT_NODE) return;
      if (!isRoot && isHidden(n)) return;
      if (!isRoot) {
        const label = n.getAttribute('aria-label');
        if (label && label.trim()) { parts.push(' ' + label + ' '); return; }
        if (n.tagName === 'IMG') { const alt = n.getAttribute('alt'); if (alt) parts.push(' ' + alt + ' '); return; }
        if (n.tagName === 'BR') { parts.push(' '); return; }
      }
      const sep = isRoot || isInline(n) ? '' : ' ';
      parts.push(sep);
      for (const c of n.childNodes) walk(c, false);
      parts.push(sep);
    };
    walk(root, true);
    return parts.join('').replace(/\s+/g, ' ').trim().slice(0, MAX);
  };

  const scope = el.getRootNode && el.getRootNode().getElementById ? el.getRootNode() : document;
  const byId = (id) => scope.getElementById(id) || document.getElementById(id);
  // aria-* attributes another field already covers: the name and its sources,
  // the value, the states and properties from the accessibility tree (incl.
  // ignored, which is what aria-hidden does when it takes effect),
  // the description and activeDescendant. The rest (aria-current, aria-valuetext,
  // aria-setsize, aria-controls, …) the tree doesn't report, so they stay.
  const COVERED = new Set([
    'aria-label', 'aria-labelledby', 'aria-describedby', 'aria-activedescendant',
    'aria-valuenow', 'aria-checked', 'aria-pressed', 'aria-selected', 'aria-expanded',
    'aria-disabled', 'aria-readonly', 'aria-required', 'aria-invalid', 'aria-level',
    'aria-haspopup', 'aria-multiselectable', 'aria-orientation', 'aria-valuemin',
    'aria-valuemax', 'aria-autocomplete', 'aria-keyshortcuts', 'aria-roledescription',
    'aria-modal', 'aria-hidden', 'aria-description', 'aria-posinset', 'aria-setsize',
  ]);
  // Position in a set ("2 of 5"), which the accessibility tree doesn't give:
  // the page's aria-posinset / aria-setsize, otherwise counted the way
  // Chromium does: radios sharing a name in the same form, or visible
  // siblings with the same role.
  const SET_ROLES = new Set(['radio', 'option', 'tab', 'menuitem', 'menuitemcheckbox', 'menuitemradio', 'treeitem', 'listitem']);
  const roleOf = (n) => {
    const r = (n.getAttribute('role') || '').trim().split(/\s+/)[0];
    if (r) return r;
    if (n.tagName === 'LI') return 'listitem';
    if (n.tagName === 'OPTION') return 'option';
    if (n.tagName === 'INPUT' && n.type === 'radio') return 'radio';
    return '';
  };
  const addPos = (n, o) => {
    const role = roleOf(n);
    let set = null;
    if (n.tagName === 'INPUT' && n.type === 'radio' && n.name) {
      const root = n.form || n.getRootNode();
      set = Array.from(root.querySelectorAll('input[type="radio"]'))
        .filter((r) => r.name === n.name && r.form === n.form && !isHidden(r));
    } else if (SET_ROLES.has(role) && n.parentElement) {
      set = Array.from(n.parentElement.children).filter((c) => roleOf(c) === role && !isHidden(c));
    }
    const ap = parseInt(n.getAttribute('aria-posinset'), 10);
    const as = parseInt(n.getAttribute('aria-setsize'), 10);
    const i = set ? set.indexOf(n) : -1;
    const pos = ap > 0 ? ap : i + 1;
    const size = as > 0 ? as : set ? set.length : 0;
    if (pos > 0 && size > 0) { o.posInSet = pos; o.setSize = size; }
  };
  const ariaOf = (n) => {
    const aria = {};
    for (const a of n.attributes) if (a.name.startsWith('aria-') && !COVERED.has(a.name)) aria[a.name] = a.value;
    return Object.keys(aria).length ? aria : null;
  };

  const out = {};
  // A list-style <select> has no value in the accessibility tree (a
  // drop-down's value is its selected text), so its selection is read here.
  if (el.tagName === 'SELECT' && (el.multiple || el.size > 1)) {
    out.selectedText = Array.from(el.selectedOptions).map((o) => o.label.trim()).join(', ').slice(0, MAX);
  }

  const role = (el.getAttribute('role') || '').trim().toLowerCase();
  // A <select>'s options are not its text: value / selectedText say what is
  // chosen.
  if (!COMPOSITE.has(role) && el.tagName !== 'SELECT') {
    const text = srText(el);
    if (text) out.text = text;
  }
  // Chromium's accessibility tree leaves required out for some controls (e.g.
  // a required checkbox); this stands in for it then (see addAX).
  if (el.required === true || el.getAttribute('aria-required') === 'true') out.required = true;
  addPos(el, out);
  const aria = ariaOf(el);
  if (aria) out.aria = aria;

  const adId = el.getAttribute('aria-activedescendant');
  if (adId) {
    const ad = byId(adId);
    if (!ad) {
      out.activeDescendant = { missing: true };
    } else {
      const a = {};
      const t = srText(ad); if (t) a.text = t;
      addPos(ad, a);
      const aa = ariaOf(ad); if (aa) a.aria = aa;
      out.activeDescendant = a;
    }
  }
  return out;
}`
