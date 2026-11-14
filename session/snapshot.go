package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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
			dom, err := b.active.Content()
			if err != nil {
				return preset.State{}, fmt.Errorf("session: capturing DOM: %w", err)
			}
			if err := os.WriteFile(st.DOMPath, []byte(formatDOM(dom)), 0o644); err != nil {
				return preset.State{}, fmt.Errorf("session: writing DOM: %w", err)
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

// Focused returns a screen-reader view of the active tab's focused element,
// or nil when nothing (or just the body) is focused.
func Focused(b *Browser) (*preset.FocusedElement, error) {
	if err := checkOpen(b); err != nil {
		return nil, err
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
	return &fe, nil
}

// focusedElementJS reads document.activeElement the way a screen reader would
// announce it: text skips hidden subtrees and uses descendants' aria-label /
// img alt; inline elements join without spaces; aria-labelledby and
// aria-describedby resolve to the referenced text; composite widgets (grid,
// listbox, menu, …) report their aria-activedescendant instead of their own
// content. A field is included only when its source is present, so "" stays
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

  const out = { tag: el.tagName.toLowerCase() };
  const setAttr = (k, n) => { const v = el.getAttribute(n); if (v !== null) out[k] = v; };
  setAttr('role', 'role');
  setAttr('type', 'type');
  setAttr('title', 'title');
  setAttr('placeholder', 'placeholder');
  if (el.value !== undefined && el.value !== null) out.value = String(el.value);

  const role = (el.getAttribute('role') || '').trim().toLowerCase();
  if (!COMPOSITE.has(role)) {
    const text = srText(el);
    if (text) out.text = text;
  }
  const lb = refText(el, 'aria-labelledby');
  if (lb !== null) out.labelledBy = lb;
  const db = refText(el, 'aria-describedby');
  if (db !== null) out.describedBy = db;
  const aria = ariaOf(el);
  if (aria) out.aria = aria;

  const adId = el.getAttribute('aria-activedescendant');
  if (adId) {
    const ad = byId(adId);
    if (!ad) {
      out.activeDescendant = { missing: true };
    } else {
      const a = { tag: ad.tagName.toLowerCase() };
      const r = ad.getAttribute('role'); if (r !== null) a.role = r;
      const t = srText(ad); if (t) a.text = t;
      const alb = refText(ad, 'aria-labelledby'); if (alb !== null) a.labelledBy = alb;
      const adb = refText(ad, 'aria-describedby'); if (adb !== null) a.describedBy = adb;
      const aa = ariaOf(ad); if (aa) a.aria = aa;
      out.activeDescendant = a;
    }
  }
  return out;
}`
