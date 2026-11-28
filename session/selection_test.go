package session

import (
	"reflect"
	"testing"

	"github.com/agenticcliorchestra/aclio-go-browser/preset"
)

// speakSelectionChange words selection changes as NVDA's
// speech.speakSelectionChange does.
func TestSpeakSelectionChange(t *testing.T) {
	text := []rune("Read the terms first.")
	for _, c := range []struct {
		name     string
		old, new selection
		want     []string
	}{
		{"nothing selected", selection{3, 3}, selection{5, 5}, nil},
		{"one character", selection{0, 0}, selection{0, 1}, []string{"R selected"}},
		{"a space is spelled", selection{4, 4}, selection{4, 5}, []string{"space selected"}},
		{"extended", selection{0, 1}, selection{0, 5}, []string{"ead selected"}},
		{"shrunk", selection{0, 5}, selection{0, 2}, []string{"ad unselected"}},
		{"collapsed", selection{0, 5}, selection{5, 5}, []string{"Read unselected"}},
		{"backwards", selection{9, 9}, selection{9, 5}, []string{"the selected"}},
		{"replaced", selection{0, 4}, selection{9, 14}, []string{"terms selected", "Read unselected"}},
		{"crossed over", selection{5, 9}, selection{5, 2}, []string{"ad selected", "the unselected"}},
	} {
		b := &Browser{}
		a := &announcer{nvda: nvdaOf(b)}
		speakSelectionChange(a, text, c.old, c.new)
		if got := preset.NVDASaid(a.said); !reflect.DeepEqual(got, c.want) && !(len(got) == 0 && c.want == nil) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSelectionWords(t *testing.T) {
	text := []rune("Read the  terms")
	if got := nextWordStart(text, 1); got != 5 {
		t.Errorf("next word from 1 = %d, want 5", got)
	}
	if got := nextWordStart(text, 5); got != 10 {
		t.Errorf("next word from 5 = %d, want 10", got)
	}
	if got := prevWordStart(text, 10); got != 5 {
		t.Errorf("previous word from 10 = %d, want 5", got)
	}
	if got := prevWordStart(text, 3); got != 0 {
		t.Errorf("previous word from 3 = %d, want 0", got)
	}
	if got := selectionText([]rune("  \n ")); got != "blank" {
		t.Errorf("blank = %q", got)
	}
	if got := selectionText([]rune("Apples\nPears")); got != "Apples Pears" {
		t.Errorf("across lines = %q", got)
	}
}
