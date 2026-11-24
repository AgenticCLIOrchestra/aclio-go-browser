package session

import (
	"fmt"
	"sync"

	"github.com/agenticcliorchestra/aclio-go-browser/engine"
	"github.com/agenticcliorchestra/aclio-go-browser/preset"
	"github.com/mxschmitt/playwright-go"
)

// announcer records what a screen reader says as it happens, so that every
// focus change and every change to the focused element is heard, not just
// the state the page ends a turn in, and so is what the page announces in
// its live regions. The page queues the element focused at each such event,
// and the text each live region change speaks (announcerJS); Go turns each
// focus entry into an announcement with preset.Announce, against the one
// before, and takes each live entry as spoken. Each item is marked with which
// of the two modelled screen readers says it: both, except for live region
// nodes replaced by nodes with identical text, which only NVDA speaks. The
// queue is processed in the background when the page signals, after each
// action Perform runs, and when the announcements are taken.
type announcer struct {
	mu   sync.Mutex
	lang preset.Language
	page playwright.Page        // the active tab, whose queue is read
	cdp  playwright.CDPSession  // on page
	last *preset.FocusedElement // the element announced last, nil after focus was lost
	said []preset.Announcement  // announced since the last TakeAnnouncements

	signal chan struct{} // the page has queued something
	done   chan struct{} // closed by Close
}

// announcerSignal is the function the page calls to wake the background
// processing.
const announcerSignal = "__aclioAnnouncerSignal"

// announcerJS installs the page side. Its queue holds, in the order they
// happen, {kind: "focus", el} entries and {kind: "live", politeness, text,
// nvdaOnly} entries; next() hands over a focus entry's element (the document
// when focus was lost) and a live entry's text behind 'n' (NVDA only) or 'b'
// (both screen readers).
//
// Focus: it queues the focused element on every focusin, on input / change
// events of the focused element, and on changes to the attributes of the
// focused element or its active descendant that a screen reader announces;
// it queues the document when focus is lost.
//
// Live regions (from DOMContentLoaded on, so the parser filling the page is
// silent): it queues what a screen reader speaks for each change inside an
// aria-live / alert / status / log region, coalesced per region over one
// MutationObserver delivery (one microtask checkpoint), following the
// screen-reader rules: only a change aria-relevant covers (additions / text
// changes / removals) that changes what is rendered is spoken, as the whole
// region when aria-atomic, else as the change alone; hidden content, blank
// text, busy regions, aria-live="off" and regions inserted already filled
// (except alerts) stay silent; a busy region is spoken once when it stops
// being busy; replacing text with identical text (compared exactly) is
// silent for VoiceOver, while NVDA, which reads every node inserted into a
// live region, speaks it (an entry flagged nvdaOnly). A removed node can't be
// styled, so stylesheet-hidden text removed from a region that lists
// removals is spoken.
//
// It runs in every new document (an init script) and in the active tab's
// current one.
const announcerJS = `(() => {
  if (window.__aclioAnnouncer) return;
  const MAX = 1000;     // queue entries
  const TEXT_MAX = 1024; // characters of one live announcement
  const q = [];
  const signal = () => {
    try {
      const p = window.` + announcerSignal + ` && window.` + announcerSignal + `();
      if (p && p.catch) p.catch(() => {});
    } catch (e) {}
  };
  const push = (x) => { if (q.length >= MAX) q.shift(); q.push(x); signal(); };
  const focused = () => {
    const el = document.activeElement;
    return el && el !== document.body && el !== document.documentElement ? el : null;
  };
  const record = () => { const el = focused(); if (el) push({ kind: 'focus', el }); };
  document.addEventListener('focusin', record, true);
  document.addEventListener('focusout', (e) => {
    if (!e.relatedTarget) setTimeout(() => { if (!focused()) push({ kind: 'focus', el: document }); }, 0);
  }, true);
  for (const type of ['input', 'change']) {
    document.addEventListener(type, (e) => { if (e.target === document.activeElement) record(); }, true);
  }
  const WATCH = ['aria-checked', 'aria-pressed', 'aria-selected', 'aria-expanded', 'aria-disabled',
    'aria-invalid', 'aria-required', 'aria-readonly', 'aria-activedescendant', 'aria-valuenow',
    'aria-valuetext', 'aria-label', 'aria-labelledby', 'aria-describedby', 'aria-description',
    'aria-hidden', 'aria-level', 'aria-posinset', 'aria-setsize', 'disabled', 'checked',
    'selected', 'value', 'required', 'readonly', 'hidden', 'open', 'title'];
  new MutationObserver((ms) => {
    const el = focused();
    if (!el) return;
    const adId = el.getAttribute('aria-activedescendant');
    if (ms.some((m) => m.target === el || (adId && m.target.id === adId))) record();
  }).observe(document, { attributes: true, subtree: true, attributeFilter: WATCH });

  // Live regions.
  const elOf = (n) => (n && n.nodeType === 1 ? n : n && n.parentElement);
  const roleOf = (el) => (el.getAttribute('role') || '').trim().split(/\s+/)[0].toLowerCase();
  // regionOf is the live region n is in: the nearest element (n or an
  // ancestor) with a valid aria-live, else a live role. An explicit aria-live
  // wins over the role's default.
  const regionOf = (n) => {
    for (let el = elOf(n); el; el = el.parentElement) {
      const live = (el.getAttribute('aria-live') || '').trim().toLowerCase();
      if (live === 'assertive' || live === 'polite' || live === 'off') return { root: el, politeness: live };
      const role = roleOf(el);
      if (role === 'alert') return { root: el, politeness: 'assertive' };
      if (role === 'status' || role === 'log') return { root: el, politeness: 'polite' };
    }
    return null;
  };
  // atomicOf is the element spoken whole for a change at n, or null when only
  // the change is spoken: the nearest aria-atomic up to the region's root
  // decides, else the role's default (true for alert and status).
  const atomicOf = (n, region) => {
    for (let el = elOf(n); el; el = el.parentElement) {
      const a = (el.getAttribute('aria-atomic') || '').trim().toLowerCase();
      if (a === 'true') return el;
      if (a === 'false') return null;
      if (el === region.root) break;
    }
    const role = roleOf(region.root);
    return role === 'alert' || role === 'status' ? region.root : null;
  };
  const DEFAULT_RELEVANT = new Set(['additions', 'text']);
  const relevantOf = (n, region) => {
    for (let el = elOf(n); el; el = el.parentElement) {
      const r = (el.getAttribute('aria-relevant') || '').trim().toLowerCase();
      if (r) {
        const t = r.split(/\s+/);
        return t.includes('all') ? new Set(['additions', 'removals', 'text']) : new Set(t);
      }
      if (el === region.root) break;
    }
    return DEFAULT_RELEVANT;
  };
  const busy = (n) => { const el = elOf(n); return !!(el && el.closest('[aria-busy="true" i]')); };
  // hidden: n isn't rendered, or is left out of the accessibility tree.
  const hidden = (n) => {
    const el = elOf(n);
    if (!el || !el.isConnected) return true;
    if (el.closest('[aria-hidden="true" i], [hidden]')) return true;
    if (el.checkVisibility) return !el.checkVisibility({ visibilityProperty: true });
    const s = getComputedStyle(el);
    return s.display === 'none' || s.visibility === 'hidden';
  };
  const SKIP = new Set(['SCRIPT', 'STYLE', 'TEMPLATE', 'NOSCRIPT']);
  // textOf is n's rendered text as a screen reader reads it, hidden parts
  // left out. It is raw: norm collapses its whitespace for speaking, but
  // comparisons use it as is, so a repeat an app forces with an NBSP or a
  // thin space counts as a change. A removed (detached) node has no
  // rendering, so all its text counts; css=false reads an attached one the
  // same way, so the two sides of a replacement compare alike.
  const textOf = (n, css = true) => {
    const out = [];
    const walk = (n) => {
      const styled = css && n.isConnected;
      if (n.nodeType === 3) {
        const p = n.parentElement;
        if (!(styled && p && getComputedStyle(p).visibility === 'hidden')) out.push(n.data);
        return;
      }
      if (n.nodeType !== 1 || SKIP.has(n.tagName)) return;
      if ((n.getAttribute('aria-hidden') || '').toLowerCase() === 'true' || n.hasAttribute('hidden')) return;
      const display = styled ? getComputedStyle(n).display : 'inline';
      if (display === 'none') return;
      if (n.tagName === 'BR') { out.push(' '); return; }
      if (n.tagName === 'IMG') { out.push(' ' + (n.getAttribute('alt') || '') + ' '); return; }
      const block = !display.startsWith('inline') && display !== 'contents';
      if (block) out.push(' ');
      for (const c of n.childNodes) walk(c);
      if (block) out.push(' ');
    };
    walk(n);
    return out.join('');
  };
  const norm = (s) => s.replace(/\s+/g, ' ').trim();
  const cap = (s) => {
    if (s.length <= TEXT_MAX) return s;
    let end = TEXT_MAX;
    const c = s.charCodeAt(end - 1);
    if (c >= 0xd800 && c <= 0xdbff) end--; // don't split a surrogate pair
    return s.slice(0, end) + '…';
  };
  // nvdaOnly marks what NVDA speaks and VoiceOver doesn't: nodes replaced by
  // nodes with identical text (NVDA reads every node inserted into a live
  // region; VoiceOver skips a change whose text is identical).
  const say = (politeness, raw, nvdaOnly = false) => {
    const text = norm(raw);
    if (text) push({ kind: 'live', politeness, text: cap(text), nvdaOnly });
  };
  // sayInserted speaks the alerts inserted (or revealed) with n: n itself
  // when it is an alert's root, else the alerts inside it. Other regions
  // inserted already filled stay silent.
  const sayInserted = (n) => {
    if (n.nodeType !== 1) return;
    const alerts = roleOf(n) === 'alert' ? [n]
      : Array.from(n.querySelectorAll('[role~="alert" i]')).filter((el) => roleOf(el) === 'alert');
    for (const el of alerts) {
      const r = regionOf(el);
      if (r && r.root === el && r.politeness !== 'off' && !hidden(el) && !busy(el)) say(r.politeness, textOf(el));
    }
  };
  const busyRegions = new Set(); // regions changed while busy, spoken when it ends
  const observeLive = () => new MutationObserver((ms) => {
    const units = new Map(); // per region (or atomic element), in the order first changed
    const unitOf = (n, region) => {
      const atomic = atomicOf(n, region);
      const key = atomic || region.root;
      let u = units.get(key);
      if (!u) {
        // audible: a relevant change to what is rendered, the only kind
        // that is spoken (a removed node can't be styled, so only its
        // attribute-hidden parts are known to be silent).
        u = { region, atomic, pieces: [], added: [], removed: [], textChanged: false, audible: false };
        units.set(key, u);
      }
      return u;
    };
    let released = false;
    // A node added in this delivery is read whole as it is now, so changes
    // made inside it in the same delivery are already in its text.
    const added = new Set();
    for (const m of ms) if (m.type === 'childList') m.addedNodes.forEach((n) => added.add(n));
    const insideAdded = (n) => {
      for (let x = n; x; x = x.parentNode) if (added.has(x)) return true;
      return false;
    };
    for (const m of ms) {
      if (m.type === 'attributes') {
        const el = m.target;
        const was = (m.oldValue || '').trim().toLowerCase();
        const now = (el.getAttribute(m.attributeName) || '').trim().toLowerCase();
        if (m.attributeName === 'aria-busy') {
          if (was === 'true' && now !== 'true') {
            const r = regionOf(el);
            if (r) busyRegions.add(r.root);
            released = true;
          }
          continue;
        }
        const revealed = m.attributeName === 'role' ? was !== 'alert' && roleOf(el) === 'alert'
          : m.attributeName === 'hidden' ? m.oldValue !== null && !el.hasAttribute('hidden')
          : was === 'true' && now !== 'true'; // aria-hidden
        if (!revealed || hidden(el)) continue;
        const r = regionOf(el);
        if (!r || r.root === el || m.attributeName === 'role') { sayInserted(el); continue; }
        if (r.politeness === 'off' || busy(el)) { if (busy(el)) busyRegions.add(r.root); continue; }
        if (relevantOf(el, r).has('additions')) {
          const u = unitOf(el, r);
          const t = textOf(el);
          u.added.push(t);
          u.pieces.push(t);
          if (norm(t)) u.audible = true;
        }
        continue;
      }
      if (insideAdded(m.target)) continue;
      const r = regionOf(m.target);
      if (m.type === 'childList') {
        if (!r) { m.addedNodes.forEach(sayInserted); continue; }
        if (r.politeness === 'off') continue;
        if (busy(m.target)) { busyRegions.add(r.root); continue; }
        const rel = relevantOf(m.target, r);
        const u = unitOf(m.target, r);
        for (const n of m.removedNodes) {
          const t = textOf(n);
          u.removed.push(t);
          if (rel.has('removals')) {
            u.pieces.push(t);
            if (norm(t)) u.audible = true;
          }
        }
        for (const n of m.addedNodes) {
          if (n.nodeType === 1 && regionOf(n).root === n) { sayInserted(n); continue; } // a region inserted into a region
          if (!n.isConnected) continue; // added and removed again
          u.added.push(textOf(n, false)); // compared with the removed nodes, which can't be styled
          if (rel.has('additions') && !hidden(n)) {
            const t = textOf(n);
            u.pieces.push(t);
            if (norm(t)) u.audible = true;
          }
        }
      } else if (m.type === 'characterData') {
        if (!r || r.politeness === 'off' || m.oldValue === m.target.data) continue;
        if (busy(m.target)) { busyRegions.add(r.root); continue; }
        if (hidden(m.target)) continue;
        const u = unitOf(m.target, r);
        u.textChanged = true;
        if (relevantOf(m.target, r).has('text')) {
          u.pieces.push(textOf(m.target));
          u.audible = true;
        }
      }
    }
    // A region that stopped being busy is spoken whole, once, and its other
    // changes in this delivery with it.
    const spoken = new Set();
    if (released) {
      for (const root of Array.from(busyRegions)) {
        if (!root.isConnected) { busyRegions.delete(root); continue; }
        if (busy(root)) continue;
        busyRegions.delete(root);
        spoken.add(root);
        const r = regionOf(root);
        if (r && r.politeness !== 'off' && !hidden(root)) say(r.politeness, textOf(root));
      }
    }
    const join = (a) => a.join(' ');
    for (const u of units.values()) {
      if (spoken.has(u.region.root) || !u.audible) continue;
      // Replacing nodes with ones of identical (raw) text changed nothing
      // VoiceOver hears; NVDA reads the inserted nodes all the same.
      const nvdaOnly = !u.textChanged && join(u.added) === join(u.removed);
      if (u.atomic) {
        if (!hidden(u.atomic)) say(u.region.politeness, textOf(u.atomic), nvdaOnly);
      } else {
        say(u.region.politeness, join(u.pieces), nvdaOnly);
      }
    }
  }).observe(document, {
    childList: true, characterData: true, characterDataOldValue: true, subtree: true,
    attributes: true, attributeOldValue: true, attributeFilter: ['aria-busy', 'hidden', 'aria-hidden', 'role'],
  });
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', observeLive, { once: true });
  else observeLive();

  window.__aclioAnnouncer = {
    q,
    now: record,
    // A live entry comes out as its text behind one character: 'n' when
    // only NVDA speaks it, 'b' when both screen readers do.
    next: () => { const e = q.shift(); return e && (e.kind === 'live' ? (e.nvdaOnly ? 'n' : 'b') + e.text : e.el); },
  };
})()`

// announcerGroup is the CDP object group the announcer's element references
// are held in, released after each processing.
const announcerGroup = "aclio-announcer"

// StartAnnouncing makes b record what a screen reader says, in lang (the zero
// value meaning preset.DefaultLanguage), as it happens: every focus change,
// every change to the focused element or its active descendant, and what
// the live regions (aria-live, alert, status, log) announce, in the active
// tab's main frame, including what the page's own scripts do. Take what was said
// with TakeAnnouncements. The element focused when it starts is announced
// first. Calling it again only changes the language. It needs Chromium, like
// Focused.
func StartAnnouncing(b *Browser, lang preset.Language) error {
	if err := checkOpen(b); err != nil {
		return err
	}
	if b.engine != engine.Chromium {
		return fmt.Errorf("session: announcing needs Chromium, not %s", b.engine)
	}
	lang, err := preset.ResolveLanguage(lang)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	if a := b.announcer; a != nil {
		a.mu.Lock()
		a.lang = lang
		a.mu.Unlock()
		return nil
	}
	a := &announcer{lang: lang, signal: make(chan struct{}, 1), done: make(chan struct{})}
	script := announcerJS
	if err := b.context.AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("session: installing the announcer: %w", err)
	}
	if err := b.context.ExposeBinding(announcerSignal, func(*playwright.BindingSource, ...any) any {
		select {
		case a.signal <- struct{}{}:
		default: // a wake-up is already pending
		}
		return nil
	}); err != nil {
		return fmt.Errorf("session: installing the announcer: %w", err)
	}
	if err := attachAnnouncer(a, b.active); err != nil {
		return err
	}
	b.announcer = a
	go runAnnouncer(a)
	return nil
}

// TakeAnnouncements returns everything the screen readers have said since
// the last call (or since StartAnnouncing), one spoken item each, in order,
// and forgets it. Each item says whether VoiceOver, NVDA or both spoke it:
// both, except for what a live region re-inserted with identical text, which
// only NVDA reads (preset.VoiceOverSaid / preset.NVDASaid give each feed).
// It is empty when nothing was said, and nil when b isn't announcing.
func TakeAnnouncements(b *Browser) []preset.Announcement {
	if b == nil || b.announcer == nil {
		return nil
	}
	a := b.announcer
	a.mu.Lock()
	defer a.mu.Unlock()
	processLocked(a)
	said := a.said
	if said == nil {
		said = []preset.Announcement{}
	}
	a.said = nil
	return said
}

// runAnnouncer processes the queue whenever the page signals, until Close.
func runAnnouncer(a *announcer) {
	for {
		select {
		case <-a.done:
			return
		case <-a.signal:
			processAnnouncements(a)
		}
	}
}

// processAnnouncements turns what the page has queued into announcements.
func processAnnouncements(a *announcer) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	processLocked(a)
}

// processLocked does processAnnouncements with a.mu held. Failures (a page
// mid-navigation, a closed tab) end the round; what is still queued is read
// next time, and a queue lost with its document is lost with it.
func processLocked(a *announcer) {
	if a.cdp == nil {
		return
	}
	defer a.cdp.Send("Runtime.releaseObjectGroup", map[string]any{"objectGroup": announcerGroup}) //nolint:errcheck // best-effort cleanup
	for range 1000 {
		var ev struct {
			Result struct {
				Type      string `json:"type"`
				Value     any    `json:"value"` // a live entry's text
				ClassName string `json:"className"`
				ObjectID  string `json:"objectId"`
			} `json:"result"`
		}
		if err := cdpSend(a.cdp, "Runtime.evaluate", map[string]any{
			"expression":  `window.__aclioAnnouncer ? window.__aclioAnnouncer.next() : undefined`,
			"objectGroup": announcerGroup,
		}, &ev); err != nil {
			return
		}
		switch {
		case ev.Result.Type == "string": // a live region spoke, which leaves focus as it was
			if v, ok := ev.Result.Value.(string); ok && len(v) > 1 {
				a.said = append(a.said, preset.Announcement{Text: v[1:], VoiceOver: v[0] != 'n', NVDA: true})
			}
			continue
		case ev.Result.Type == "undefined" || ev.Result.ObjectID == "":
			return
		case ev.Result.ClassName == "HTMLDocument": // focus was lost
			a.last = nil
			continue
		}
		fe, err := focusedOf(a.cdp, ev.Result.ObjectID)
		if err != nil || fe == nil {
			continue
		}
		for _, text := range preset.Announce(fe, a.last, a.lang) {
			a.said = append(a.said, preset.Announcement{Text: text, VoiceOver: true, NVDA: true})
		}
		a.last = fe
	}
}

// attachAnnouncer points a at page p, the new active tab: its own queue from
// now on, the page side installed in its current document if missing, and
// its focused element queued, so a screen reader's "where am I" after the
// switch is heard.
func attachAnnouncer(a *announcer, p playwright.Page) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cdp != nil {
		a.cdp.Detach() //nolint:errcheck // the old tab may already be closed
		a.cdp = nil
	}
	cdp, err := p.Context().NewCDPSession(p)
	if err != nil {
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	if _, err := p.Evaluate(announcerJS); err != nil {
		cdp.Detach() //nolint:errcheck
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	if _, err := p.Evaluate(`() => window.__aclioAnnouncer.now()`); err != nil {
		cdp.Detach() //nolint:errcheck
		return fmt.Errorf("session: attaching the announcer: %w", err)
	}
	a.page, a.cdp, a.last = p, cdp, nil
	return nil
}

// setActive makes p b's active tab, records it in the debug logs, and makes
// it the announcer's, when b announces. An announcer that can't follow (rare:
// the tab closing under it) stays quiet rather than failing the tab action.
func setActive(b *Browser, p playwright.Page) {
	b.active = p
	logTab(b.logs, "active", p)
	if b.announcer != nil {
		_ = attachAnnouncer(b.announcer, p)
	}
}

// stopAnnouncer ends the background processing, for Close.
func stopAnnouncer(b *Browser) {
	a := b.announcer
	if a == nil {
		return
	}
	close(a.done)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cdp != nil {
		a.cdp.Detach() //nolint:errcheck
		a.cdp = nil
	}
}
