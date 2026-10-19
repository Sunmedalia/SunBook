package ui

import (
	"fmt"
	"sort"
	"strings"

	"sunbook/internal/text"
)

// File find searches the current buffer, including unsaved edits.
func fileMatches(source, query string) []int {
	if query == "" {
		return nil
	}
	var matches []int
	for start := 0; start <= len(source)-len(query); {
		i := strings.Index(source[start:], query)
		if i < 0 {
			break
		}
		start += i
		matches = append(matches, start)
		start += len(query)
	}
	return matches
}

// inclusive is used while typing so a match at the original cursor is included.
func (a *App) selectFileMatch(query string, previous, inclusive bool) int {
	v := a.view()
	source := v.Doc.Buffer.String()
	matches := fileMatches(source, query)
	if len(matches) == 0 {
		return -1
	}
	lo, hi := v.Selections[0].Range()
	position := hi
	if inclusive || previous {
		position = lo
	}
	index := sort.SearchInts(matches, position)
	if previous {
		index--
		if index < 0 {
			index = len(matches) - 1
		}
	} else if index == len(matches) {
		index = 0
	}
	start, end := matches[index], matches[index]+len(query)
	// A substring of a combining grapheme must still select the entire grapheme.
	anchor := text.Snap(source, start)
	head := text.Snap(source, end)
	if head < end {
		head = text.Next(source, head)
	}
	v.Selections = []text.Selection{{Anchor: anchor, Head: head}}
	a.reveal()
	return index
}

func (a *App) repeatFind(previous bool) {
	if a.findQuery == "" {
		a.find(false)
		return
	}
	a.focus = "editor"
	if a.selectFileMatch(a.findQuery, previous, false) < 0 {
		a.notify("No match")
	}
}

func (a *App) showFind(initial string) {
	original := append([]text.Selection(nil), a.view().Selections...)
	m := &modal{Title: "Find in File", Input: initial, InputEnabled: true, Compact: true, Placeholder: "Find text in this file…"}
	update := func(previous, typing bool) {
		a.findQuery = m.Input
		if typing {
			a.view().Selections = append([]text.Selection(nil), original...)
		}
		index := a.selectFileMatch(m.Input, previous, typing)
		status := "Type to find"
		if m.Input != "" {
			count := len(fileMatches(a.view().Doc.Buffer.String(), m.Input))
			if index < 0 {
				status = "No matches"
			} else {
				status = fmt.Sprintf("%d/%d", index+1, count)
			}
		}
		m.Hint = status + " · Enter/⇧Enter next/prev · Esc closes"
	}
	m.Changed = func(string) { update(false, true) }
	m.FindNavigate = func(previous bool) { update(previous, false) }
	a.setModal(m)
}
