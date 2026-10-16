package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

type searchHit struct {
	Path, Text   string
	Line, Column int
}

func (a *App) workspaceSearch() { a.showSidebar("Search") }
func (a *App) stopWorkspaceSearch() {
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchSeq++
	a.searchRunning = false
	a.searchDue = zeroTime
}
func (a *App) clearWorkspaceSearch() {
	a.stopWorkspaceSearch()
	a.searchQuery = ""
	a.searchCursor = 0
	a.searchOffset = 0
	a.searchResults = nil
	a.searchSelected = 0
	a.searchTop = 0
	a.searchError = ""
}
func (a *App) setSearchQuery(query string) {
	a.stopWorkspaceSearch()
	a.searchQuery = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(query)
	a.searchCursor = len(a.searchQuery)
	a.searchResults = nil
	a.searchSelected = 0
	a.searchTop = 0
	a.searchError = ""
	if a.searchQuery != "" {
		a.searchDue = time.Now().Add(250 * time.Millisecond)
	}
}
func (a *App) insertSearchQuery(s string) {
	p := a.searchCursor
	tail := len(a.searchQuery) - p
	a.setSearchQuery(a.searchQuery[:p] + s + a.searchQuery[p:])
	a.searchCursor = text.Snap(a.searchQuery, len(a.searchQuery)-tail)
}
func (a *App) startWorkspaceSearch() {
	a.stopWorkspaceSearch()
	if a.searchQuery == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.searchCancel = cancel
	a.searchRunning = true
	a.searchError = ""
	seq := a.searchSeq
	root, query := a.root, a.searchQuery
	go func() {
		defer cancel()
		cmd := exec.CommandContext(ctx, "rg", "--json", "--smart-case", "--", query, root)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			a.finishSearch(seq, root, nil, err.Error())
			return
		}
		if err = cmd.Start(); err != nil {
			a.finishSearch(seq, root, nil, "ripgrep unavailable: "+err.Error())
			return
		}
		scanner := bufio.NewScanner(pipe)
		scanner.Buffer(make([]byte, 4096), 4*1024*1024)
		var results []searchHit
		for scanner.Scan() {
			var row struct {
				Type string `json:"type"`
				Data struct {
					Path struct {
						Text string `json:"text"`
					} `json:"path"`
					Lines struct {
						Text string `json:"text"`
					} `json:"lines"`
					Line       int `json:"line_number"`
					Submatches []struct {
						Start int `json:"start"`
					} `json:"submatches"`
				} `json:"data"`
			}
			if json.Unmarshal(scanner.Bytes(), &row) != nil || row.Type != "match" || row.Data.Path.Text == "" {
				continue
			}
			for _, match := range row.Data.Submatches {
				results = append(results, searchHit{Path: row.Data.Path.Text, Text: row.Data.Lines.Text, Line: row.Data.Line - 1, Column: match.Start})
				if len(results) >= 2000 {
					break
				}
			}
			if len(results) >= 2000 {
				cancel()
				break
			}
		}
		scanErr := scanner.Err()
		if scanErr != nil {
			cancel()
		}
		err = cmd.Wait()
		message := ""
		if scanErr != nil {
			message = scanErr.Error()
		} else if err != nil && ctx.Err() == nil {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				message = strings.TrimSpace(stderr.String())
				if message == "" {
					message = err.Error()
				}
			}
		}
		a.finishSearch(seq, root, results, message)
	}()
}
func (a *App) finishSearch(seq int, root string, results []searchHit, message string) {
	a.post(func() {
		if seq != a.searchSeq || root != a.root {
			return
		}
		a.searchRunning = false
		a.searchResults = results
		a.searchSelected = max(0, min(a.searchSelected, len(results)-1))
		a.searchTop = 0
		a.searchError = message
		if message != "" {
			a.notify("Search: " + message)
		}
	})
}
func (a *App) searchInputKey(e *tcell.EventKey, name string) {
	switch name {
	case "escape":
		a.focus = "editor"
	case "enter":
		a.startWorkspaceSearch()
		a.focus = "search"
	case "down":
		a.focus = "search"
		a.moveSearchSelection(0)
	case "left":
		a.searchCursor = text.Prev(a.searchQuery, a.searchCursor)
	case "right":
		a.searchCursor = text.Next(a.searchQuery, a.searchCursor)
	case "home", "ctrl+a":
		a.searchCursor = 0
	case "end", "ctrl+e":
		a.searchCursor = len(a.searchQuery)
	case "ctrl+u":
		a.clearWorkspaceSearch()
	case "ctrl+v":
		a.readClipboard()
	case "backspace", "backspace2":
		p := text.Prev(a.searchQuery, a.searchCursor)
		head := a.searchCursor
		a.setSearchQuery(a.searchQuery[:p] + a.searchQuery[head:])
		a.searchCursor = p
	case "delete":
		head := a.searchCursor
		p := text.Next(a.searchQuery, head)
		a.setSearchQuery(a.searchQuery[:head] + a.searchQuery[p:])
		a.searchCursor = head
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.insertSearchQuery(e.Str())
		}
	}
}
func (a *App) searchResultsKey(e *tcell.EventKey, name string) {
	switch name {
	case "up":
		a.moveSearchSelection(-1)
	case "down":
		a.moveSearchSelection(1)
	case "pageup":
		a.moveSearchSelection(-max(1, a.sideRect.H-5))
	case "pagedown":
		a.moveSearchSelection(max(1, a.sideRect.H-5))
	case "enter":
		a.openSearchResult()
	case "ctrl+f":
		a.focus = "searchInput"
		a.searchCursor = len(a.searchQuery)
	case "escape":
		a.focus = "editor"
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.focus = "searchInput"
			a.insertSearchQuery(e.Str())
		}
	}
}
