package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	raw, err := b.active.Evaluate(focusedElementJS)
	if err != nil {
		return nil, fmt.Errorf("session: reading the focused element: %w", err)
	}
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("session: reading the focused element: %w", err)
	}
	var fe preset.FocusedElement
	if err := json.Unmarshal(data, &fe); err != nil {
		return nil, fmt.Errorf("session: reading the focused element: %w", err)
	}
	if err := addRoles(b, &fe); err != nil {
		return nil, fmt.Errorf("session: reading the focused element's role: %w", err)
	}
	return &fe, nil
}

// addRoles fills in the computed roles of the focused element and of its
// active descendant from Chromium's accessibility tree: the explicit role
// when valid, otherwise the implicit one ("generic" for a plain div). They
// are read in a separate call from focusedElementJS, so a page that moves
// focus in between could mismatch them; snapshots are taken once the page has
// settled.
func addRoles(b *Browser, fe *preset.FocusedElement) error {
	s, err := b.context.NewCDPSession(b.active)
	if err != nil {
		return err
	}
	defer s.Detach()
	if fe.Role, err = axRole(s, `document.activeElement`); err != nil {
		return err
	}
	if ad := fe.ActiveDescendant; ad != nil && !ad.Missing {
		if ad.Role, err = axRole(s, activeDescendantJS); err != nil {
			return err
		}
	}
	return nil
}

// activeDescendantJS finds the focused element's aria-activedescendant the way
// focusedElementJS does: in the element's own tree first, then the document.
const activeDescendantJS = `(() => {
  const el = document.activeElement;
  const id = el && el.getAttribute('aria-activedescendant');
  if (!id) return null;
  const scope = el.getRootNode && el.getRootNode().getElementById ? el.getRootNode() : document;
  return scope.getElementById(id) || document.getElementById(id);
})()`

// axRole evaluates expr to an element and returns its role in Chromium's
// accessibility tree, nil when expr gives no element or the node is ignored
// (not in the tree a screen reader gets).
func axRole(s playwright.CDPSession, expr string) (*string, error) {
	var ev struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
	}
	if err := cdpSend(s, "Runtime.evaluate", map[string]any{"expression": expr}, &ev); err != nil {
		return nil, err
	}
	if ev.Result.ObjectID == "" {
		return nil, nil
	}
	var tree struct {
		Nodes []struct {
			Ignored bool `json:"ignored"`
			Role    *struct {
				Value string `json:"value"`
			} `json:"role"`
		} `json:"nodes"`
	}
	if err := cdpSend(s, "Accessibility.getPartialAXTree", map[string]any{
		"objectId":       ev.Result.ObjectID,
		"fetchRelatives": false,
	}, &tree); err != nil {
		return nil, err
	}
	if len(tree.Nodes) == 0 {
		return nil, nil
	}
	n := tree.Nodes[0]
	if n.Ignored || n.Role == nil || n.Role.Value == "" {
		return nil, nil
	}
	return &n.Role.Value, nil
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

// focusedElementJS reads document.activeElement the way a screen reader would
// announce it: text skips hidden subtrees and uses descendants' aria-label /
// img alt; inline elements join without spaces; aria-labelledby and
// aria-describedby resolve to the referenced text; <label>s (for= or
// wrapping) and the enclosing fieldset's <legend> are read; native states
// (checked, selected, disabled, …) come from the DOM properties; composite
// widgets (grid, listbox, menu, …) report their aria-activedescendant instead
// of their own content. A field is included only when its source is present, so "" stays
// distinct from absent.
const focusedElementJS = `() => {
  const el = document.activeElement;
  if (!el || el === document.body) return null;

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
  // skip, when given, is left out of the text: a <label> wrapping the focused
  // <select> must not read out every option.
  const srText = (root, skip) => {
    const parts = [];
    const walk = (n, isRoot) => {
      if (n === skip) return;
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
  const refText = (n, attr) => {
    const ids = (n.getAttribute(attr) || '').trim();
    if (!ids) return null;
    const texts = ids.split(/\s+/).map((id) => { const t = byId(id); return t ? srText(t) : ''; }).filter(Boolean);
    return texts.length ? texts.join(' ').slice(0, MAX) : '';
  };
  const ariaOf = (n) => {
    const aria = {};
    for (const a of n.attributes) if (a.name.startsWith('aria-')) aria[a.name] = a.value;
    return Object.keys(aria).length ? aria : null;
  };
  // Native states, from the DOM properties so they reflect user interaction.
  // checked / selected only where the element has them (false is reported).
  const addStates = (n, o) => {
    if (n.tagName === 'INPUT' && (n.type === 'checkbox' || n.type === 'radio')) {
      o.checked = n.checked;
      if (n.indeterminate) o.indeterminate = true;
    }
    if (n.tagName === 'OPTION') o.selected = n.selected;
    if (n.matches(':disabled')) o.disabled = true;
    if (n.required === true) o.required = true;
    if (n.readOnly === true) o.readOnly = true;
  };

  const out = {};
  const setAttr = (k, n) => { const v = el.getAttribute(n); if (v !== null) out[k] = v; };
  setAttr('title', 'title');
  setAttr('placeholder', 'placeholder');
  if (el.value !== undefined && el.value !== null) out.value = String(el.value);
  if (el.tagName === 'SELECT') {
    out.selectedText = Array.from(el.selectedOptions).map((o) => o.label.trim()).join(', ').slice(0, MAX);
  }

  const role = (el.getAttribute('role') || '').trim().toLowerCase();
  // A <select>'s options are not its text: selectedText says what is chosen.
  if (!COMPOSITE.has(role) && el.tagName !== 'SELECT') {
    const text = srText(el);
    if (text) out.text = text;
  }
  if (el.labels && el.labels.length) {
    out.label = Array.from(el.labels).map((l) => srText(l, el)).filter(Boolean).join(' ').slice(0, MAX);
  }
  const fieldset = el.parentElement && el.parentElement.closest('fieldset');
  const legend = fieldset && Array.from(fieldset.children).find((c) => c.tagName === 'LEGEND');
  if (legend) out.legend = srText(legend);
  const lb = refText(el, 'aria-labelledby');
  if (lb !== null) out.labelledBy = lb;
  const db = refText(el, 'aria-describedby');
  if (db !== null) out.describedBy = db;
  addStates(el, out);
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
      const alb = refText(ad, 'aria-labelledby'); if (alb !== null) a.labelledBy = alb;
      const adb = refText(ad, 'aria-describedby'); if (adb !== null) a.describedBy = adb;
      addStates(ad, a);
      const aa = ariaOf(ad); if (aa) a.aria = aa;
      out.activeDescendant = a;
    }
  }
  return out;
}`
