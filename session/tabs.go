package session

import (
	"fmt"

	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// Tabs lists every open tab in browser order, with the active one flagged.
// Titles are read best-effort.
//
// Tabs the page opens by itself (e.g. a link with target=_blank) show up here
// but do not become active; the agent switches to them with tabSwitch.
func Tabs(b *Browser) ([]preset.Tab, error) {
	if err := checkOpen(b); err != nil {
		return nil, err
	}
	pages := b.context.Pages()
	tabs := make([]preset.Tab, len(pages))
	for i, p := range pages {
		title, _ := p.Title()
		tabs[i] = preset.Tab{Title: title, URL: p.URL(), Active: p == b.active}
	}
	return tabs, nil
}

func openTab(b *Browser) error {
	p, err := b.context.NewPage()
	if err != nil {
		return err
	}
	if err := p.BringToFront(); err != nil {
		return err
	}
	setActive(b, p)
	return nil
}

func switchTab(b *Browser, index int) error {
	pages := b.context.Pages()
	if index < 1 || index > len(pages) {
		return fmt.Errorf("index %d out of range (1..%d)", index, len(pages))
	}
	p := pages[index-1]
	if err := p.BringToFront(); err != nil {
		return err
	}
	setActive(b, p)
	return nil
}

// closeTab refuses to close the only tab. When the active tab is closed, its
// neighbour (the next one, else the previous) becomes active.
func closeTab(b *Browser, index int) error {
	pages := b.context.Pages()
	if len(pages) <= 1 {
		return fmt.Errorf("cannot close the only open tab")
	}
	if index < 1 || index > len(pages) {
		return fmt.Errorf("index %d out of range (1..%d)", index, len(pages))
	}
	target := pages[index-1]
	wasActive := target == b.active
	if err := target.Close(); err != nil {
		return err
	}
	if !wasActive {
		return nil
	}
	remaining := b.context.Pages()
	next := min(index-1, len(remaining)-1)
	p := remaining[next]
	if err := p.BringToFront(); err != nil {
		return err
	}
	setActive(b, p)
	return nil
}
