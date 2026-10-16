package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

type searchOptions struct {
	Regex, Case, Word bool
	Include, Exclude  string
}

type searchHit struct {
	Path, Text   string
	Line, Column int
	End          int
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
	a.searchTruncated = false
	a.searchCollapsed = nil
}
func (a *App) setSearchQuery(query string) {
	a.stopWorkspaceSearch()
	a.searchQuery = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(query)
	a.searchCursor = len(a.searchQuery)
	a.searchResults = nil
	a.searchSelected = 0
	a.searchTop = 0
	a.searchError = ""
	a.searchTruncated = false
	a.searchCollapsed = nil
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
	a.searchTruncated = false
	a.searchError = ""
	seq := a.searchSeq
	root, query := a.root, a.searchQuery
	options := a.searchOptions
	go func() {
		defer cancel()
		args := []string{"--no-config", "--json"}
		if options.Case {
			args = append(args, "--case-sensitive")
		} else {
			args = append(args, "--smart-case")
		}
		if !options.Regex {
			args = append(args, "--fixed-strings")
		}
		if options.Word {
			args = append(args, "--word-regexp")
		}
		for _, g := range strings.Split(options.Include, ",") {
			if g = strings.TrimSpace(g); g != "" {
				args = append(args, "--glob", g)
			}
		}
		for _, g := range strings.Split(options.Exclude, ",") {
			if g = strings.TrimSpace(g); g != "" {
				args = append(args, "--glob", "!"+g)
			}
		}
		args = append(args, "--", query, ".")
		cmd := exec.CommandContext(ctx, "rg", args...)
		cmd.Dir = root
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
		truncated := false
		lastPublish := time.Now()
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
						End   int `json:"end"`
					} `json:"submatches"`
				} `json:"data"`
			}
			if json.Unmarshal(scanner.Bytes(), &row) != nil || row.Type != "match" || row.Data.Path.Text == "" {
				continue
			}
			for _, match := range row.Data.Submatches {
				results = append(results, searchHit{Path: filepath.Join(root, row.Data.Path.Text), Text: row.Data.Lines.Text, Line: row.Data.Line - 1, Column: match.Start, End: match.End})
				if len(results) >= 2000 {
					break
				}
			}
			if len(results)%64 == 0 || time.Since(lastPublish) > 100*time.Millisecond {
				a.publishSearch(seq, root, results, "", false, false)
				lastPublish = time.Now()
			}
			if len(results) >= 2000 {
				truncated = true
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
		a.publishSearch(seq, root, results, message, true, truncated)
	}()
}
func (a *App) finishSearch(seq int, root string, results []searchHit, message string) {
	a.publishSearch(seq, root, results, message, true, false)
}
func (a *App) publishSearch(seq int, root string, results []searchHit, message string, done, truncated bool) {
	snapshot := append([]searchHit(nil), results...)
	sort.SliceStable(snapshot, func(i, j int) bool {
		a, b := snapshot[i], snapshot[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	a.post(func() {
		if seq != a.searchSeq || root != a.root {
			return
		}
		var selected searchHit
		had := a.searchSelected >= 0 && a.searchSelected < len(a.searchResults)
		if had {
			selected = a.searchResults[a.searchSelected]
		}
		a.searchResults = snapshot
		a.searchRunning = !done
		a.searchTruncated = truncated
		a.searchError = message
		a.searchSelected = max(0, min(a.searchSelected, len(snapshot)-1))
		if had {
			for i, h := range snapshot {
				if h.Path == selected.Path && h.Line == selected.Line && h.Column == selected.Column {
					a.searchSelected = i
					break
				}
			}
		}
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
		a.moveSearchSelection(-max(1, a.sideRect.Y+a.sideRect.H-a.searchBodyStart()))
	case "pagedown":
		a.moveSearchSelection(max(1, a.sideRect.Y+a.sideRect.H-a.searchBodyStart()))
	case "enter":
		a.openSearchResult()
	case "home":
		a.searchSelected = 0
		a.moveSearchSelection(0)
	case "end":
		a.searchSelected = len(a.searchResults) - 1
		a.moveSearchSelection(0)
	case "tab", "ctrl+f":
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
