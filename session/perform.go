package session

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agenticcliorchestra/aclio-go-browser/action"
	"github.com/mxschmitt/playwright-go"
)

// Perform applies actions to b in order and reports the outcome of each.
// Every action is checked with action.Validate before it runs. It does not
// check what an agent was allowed to do: run preset.Check on each action
// first.
//
// The first failure stops the run: that action carries the Reason, and the
// ones after it stay Attempted=false. Failures are reported in the result,
// not as an error, so they can go straight back to the agent. After each
// action that touches the page, Perform waits for the page's load event so
// the next action (and the next Snapshot) sees a settled page.
func Perform(b *Browser, actions []action.Action) []action.Performed {
	performed := make([]action.Performed, len(actions))
	for i, a := range actions {
		performed[i].Action = a
	}
	if err := checkOpen(b); err != nil {
		if len(performed) > 0 {
			performed[0].Attempted = true
			setReason(&performed[0], err)
		}
		return performed
	}

	// What the screen reader said before this batch is heard first; then,
	// after each action, what it set off, before the next action can move on.
	processAnnouncements(b.announcer)
	for i := range performed {
		p := &performed[i]
		p.Attempted = true

		if err := action.Validate(p.Action); err != nil {
			setReason(p, err)
			break
		}
		result, err := perform(b, p.Action)
		if err == nil && touchesPage(p.Type) {
			err = waitForLoad(b)
		}
		processAnnouncements(b.announcer)
		if err != nil {
			setReason(p, err)
			break
		}
		p.Succeeded = true
		p.Result = result
	}
	return performed
}

func setReason(p *action.Performed, err error) {
	msg := err.Error()
	p.Reason = &msg
}

// touchesPage reports whether t acts on the page, so Perform should wait for
// it to settle afterwards. The artifact actions only touch the disk
// (screenshot reads the page but doesn't change it).
func touchesPage(t action.Type) bool {
	return !action.IsArtifact(t)
}

// waitForLoad waits for the "load" event. Not "networkidle": apps with
// analytics or websockets never go network-idle, so that wait would time out.
// On an in-page (SPA) update without navigation it returns at once.
func waitForLoad(b *Browser) error {
	if err := b.active.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateLoad,
	}); err != nil {
		return fmt.Errorf("waiting for page load: %w", err)
	}
	return nil
}

// perform runs one validated action. A non-nil string is data returned to the
// agent in Performed.Result.
func perform(b *Browser, a action.Action) (*string, error) {
	page := b.active
	mouse := page.Mouse()
	keyboard := page.Keyboard()

	switch d := a.Data.(type) {
	case action.Click:
		if d.Target.Selector != nil {
			return nil, page.Locator(*d.Target.Selector).Click()
		}
		return nil, mouse.Click(d.Target.Point.X, d.Target.Point.Y)
	case action.DoubleClick:
		if d.Target.Selector != nil {
			return nil, page.Locator(*d.Target.Selector).Dblclick()
		}
		return nil, mouse.Dblclick(d.Target.Point.X, d.Target.Point.Y)
	case action.Hover:
		return nil, moveTo(page, d.Target)
	case action.MouseDown:
		if d.Target != nil {
			if err := moveTo(page, *d.Target); err != nil {
				return nil, err
			}
		}
		return nil, mouse.Down()
	case action.MouseUp:
		if d.Target != nil {
			if err := moveTo(page, *d.Target); err != nil {
				return nil, err
			}
		}
		return nil, mouse.Up()
	case action.Scroll:
		return nil, mouse.Wheel(d.DeltaX, d.DeltaY)
	case action.Typing:
		if d.Target != nil {
			if err := focus(page, *d.Target); err != nil {
				return nil, err
			}
		}
		return nil, keyboard.Type(d.Text)
	case action.Fill:
		if d.Target.Selector != nil {
			return nil, page.Locator(*d.Target.Selector).Fill(d.Value)
		}
		el, err := elementAt(page, *d.Target.Point)
		if err != nil {
			return nil, err
		}
		return nil, el.Fill(d.Value)
	case action.Press:
		return nil, keyboard.Press(d.Key)
	case action.Wait:
		page.WaitForTimeout(float64(d.Milliseconds))
		return nil, nil
	case action.GoTo:
		target, err := resolveGoTo(page.URL(), d.URL)
		if err != nil {
			return nil, err
		}
		_, err = page.Goto(target)
		return nil, err
	case action.TabOpen:
		return nil, openTab(b)
	case action.TabSwitch:
		return nil, switchTab(b, d.Index)
	case action.TabClose:
		return nil, closeTab(b, d.Index)
	case action.Javascript:
		return evaluate(page, d.Code)
	case action.Screenshot:
		png, err := screenshot(page, d)
		if err != nil {
			return nil, err
		}
		return nil, createArtifact(b.artifactsDir, action.ScreenshotFileName(d), png)
	case action.CreateArtifact:
		return nil, createArtifact(b.artifactsDir, d.Name, []byte(d.Content))
	case action.ReadArtifact:
		content, err := readArtifact(b.artifactsDir, d.Name)
		if err != nil {
			return nil, err
		}
		return &content, nil
	case action.ListArtifacts:
		names, err := listArtifacts(b.artifactsDir)
		if err != nil {
			return nil, err
		}
		return &names, nil
	default:
		return nil, fmt.Errorf("unsupported action data %T", a.Data)
	}
}

// moveTo puts the pointer over target: hovering a selector, or moving to a
// point.
func moveTo(page playwright.Page, t action.Target) error {
	if t.Selector != nil {
		return page.Locator(*t.Selector).Hover()
	}
	return page.Mouse().Move(t.Point.X, t.Point.Y)
}

// focus gives target keyboard focus: focusing a selector, or clicking a point
// (the only way to focus whatever sits there).
func focus(page playwright.Page, t action.Target) error {
	if t.Selector != nil {
		return page.Locator(*t.Selector).Focus()
	}
	return page.Mouse().Click(t.Point.X, t.Point.Y)
}

// elementAt is the element a click at p would hit.
func elementAt(page playwright.Page, p action.Point) (playwright.ElementHandle, error) {
	handle, err := page.EvaluateHandle("([x, y]) => document.elementFromPoint(x, y)", []any{p.X, p.Y})
	if err != nil {
		return nil, fmt.Errorf("finding the element at (%v, %v): %w", p.X, p.Y, err)
	}
	el := handle.AsElement()
	if el == nil {
		return nil, fmt.Errorf("no element at (%v, %v); the point may be outside the viewport", p.X, p.Y)
	}
	return el, nil
}

// screenshot captures the PNG a Screenshot action asks for: one element, the
// whole page, or the viewport.
func screenshot(page playwright.Page, s action.Screenshot) ([]byte, error) {
	switch {
	case s.Target != nil && s.Target.Selector != nil:
		return page.Locator(*s.Target.Selector).Screenshot()
	case s.Target != nil:
		el, err := elementAt(page, *s.Target.Point)
		if err != nil {
			return nil, err
		}
		return el.Screenshot()
	default:
		return page.Screenshot(playwright.PageScreenshotOptions{FullPage: playwright.Bool(s.FullPage)})
	}
}

// maxScriptResult caps the JSON returned from a javascript action.
const maxScriptResult = 64 << 10

// evaluate runs code in the page and returns its result as JSON (nil when the
// script returns nothing), truncated to maxScriptResult.
func evaluate(page playwright.Page, code string) (*string, error) {
	v, err := page.Evaluate(code)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, errors.New("the script's result can't be returned as JSON")
	}
	s := string(raw)
	if len(s) > maxScriptResult {
		s = s[:maxScriptResult] + "…(truncated)"
	}
	return &s, nil
}
