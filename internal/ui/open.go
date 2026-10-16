package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sunbook/internal/text"
)

type openEntry struct {
	Name, Path string
	Dir        bool
}
type openPicker struct {
	Quick               bool
	Dir, ReadDir, Error string
	Paths               []string
	Entries             []openEntry
	Loading             bool
	Seq                 int
}

func (a *App) quickOpen() {
	m := a.openModal(true)
	if a.treeIndexReady {
		m.Open.Paths = a.treeFiles
		a.updateOpenModal(m)
	} else {
		m.Open.Loading = true
		a.updateOpenModal(m)
		a.loadTreeIndex()
	}
}
func (a *App) fileOpen() { a.openModal(false) }
func (a *App) openModal(quick bool) *modal {
	title := "Open File"
	hint := "Tab completes · Alt+↑ parent · Ctrl+Enter opens in split · Esc closes"
	if quick {
		title = "Quick Open"
		hint = "Type filename · :line:column jumps · Ctrl+Enter opens in split"
	}
	m := &modal{Title: title, InputEnabled: true, Hint: hint, Placeholder: "File name or path…", Open: &openPicker{Quick: quick, Dir: a.root}}
	m.Accept = func(split bool) { a.acceptOpen(m, split) }
	m.Complete = func() { a.completeOpen(m) }
	m.Parent = func() {
		if !m.Open.Quick {
			dir := m.Open.ReadDir
			if dir == "" {
				dir = m.Open.Dir
			}
			m.Open.Dir = filepath.Dir(dir)
			m.Input = ""
			m.Cursor = 0
			a.updateOpenModal(m)
		}
	}
	a.setModal(m)
	return m
}
func splitOpenLocation(s string) (string, int, int) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return s, 0, 0
	}
	last, e := strconv.Atoi(parts[len(parts)-1])
	if e != nil || last < 1 {
		return s, 0, 0
	}
	if len(parts) >= 3 {
		if line, e := strconv.Atoi(parts[len(parts)-2]); e == nil && line > 0 {
			return strings.Join(parts[:len(parts)-2], ":"), line, last
		}
	}
	return strings.Join(parts[:len(parts)-1], ":"), last, 1
}
func openPath(base, path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, e := os.UserHomeDir(); e == nil {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}
func fileRank(path, query string) (int, bool) {
	path = strings.ToLower(filepath.ToSlash(path))
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0, true
	}
	base := filepath.Base(path)
	score := 0
	for _, word := range strings.Fields(query) {
		switch {
		case base == word:
			score += 10000
		case strings.HasPrefix(base, word):
			score += 8000
		case strings.Contains(base, word):
			score += 6000
		case strings.Contains(path, word):
			score += 4000
		default:
			hay, needle := []rune(path), []rune(word)
			j, previous := 0, -2
			part := 0
			for i, r := range hay {
				if j < len(needle) && r == needle[j] {
					part += 20
					if i == previous+1 {
						part += 30
					}
					if i == 0 || strings.ContainsRune("/_-. ", hay[max(0, i-1)]) {
						part += 20
					}
					previous = i
					j++
				}
			}
			if j != len(needle) {
				return 0, false
			}
			score += 1000 + part - len(hay)
		}
	}
	return score, true
}
func (a *App) updateOpenModal(m *modal) {
	if a.modal != m || m.Open == nil {
		return
	}
	s := m.Open
	query, _, _ := splitOpenLocation(strings.TrimSpace(m.Input))
	if s.Quick {
		m.Subtitle = "Recent files and workspace · " + a.root
		paths := append([]string(nil), a.recentFiles...)
		for _, d := range a.docs {
			if d.Path != "" {
				paths = append(paths, d.Path)
			}
		}
		paths = append(paths, s.Paths...)
		for _, path := range paths {
			rel, _ := filepath.Rel(a.root, path)
			if m.Input == rel || m.Input == filepath.Base(path) || m.Input == path {
				query = m.Input
				break
			}
		}
		type candidate struct {
			item
			score int
		}
		var ranked []candidate
		seen := map[string]bool{}
		for _, path := range paths {
			if seen[path] {
				continue
			}
			seen[path] = true
			rel, _ := filepath.Rel(a.root, path)
			score, ok := fileRank(rel, query)
			if !ok {
				continue
			}
			detail := filepath.ToSlash(filepath.Dir(rel))
			if detail == "." {
				detail = "workspace"
			}
			for i, recent := range a.recentFiles {
				if recent == path {
					score += max(1, 100-i)
					detail = "recent · " + detail
					break
				}
			}
			ranked = append(ranked, candidate{item: item{Label: filepath.Base(path), Detail: detail, Path: path}, score: score})
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			if ranked[i].score != ranked[j].score {
				return ranked[i].score > ranked[j].score
			}
			return ranked[i].Path < ranked[j].Path
		})
		m.Visible = nil
		for i, r := range ranked {
			if i >= 200 {
				break
			}
			m.Visible = append(m.Visible, r.item)
		}
		m.Empty = "No matching files"
		if s.Loading {
			m.Empty = "Indexing workspace…"
		}
		if s.Error != "" {
			m.Empty = s.Error
		}
	} else {
		path := query
		expanded := openPath(s.Dir, path)
		dir, leaf := filepath.Dir(expanded), filepath.Base(expanded)
		if path == "" {
			dir, leaf = s.Dir, ""
		} else if strings.HasSuffix(path, "/") || path == "~" {
			dir, leaf = expanded, ""
		}
		m.Subtitle = dir
		if s.ReadDir != dir {
			a.readOpenDirectory(m, dir)
			return
		}
		m.Visible = nil
		for _, entry := range s.Entries {
			if _, ok := fileRank(entry.Name, leaf); !ok {
				continue
			}
			label, detail := entry.Name, "file"
			if entry.Dir {
				label = "▸ " + label
				detail = "folder"
			}
			m.Visible = append(m.Visible, item{Label: label, Detail: detail, Path: entry.Path, Dir: entry.Dir})
		}
		sort.SliceStable(m.Visible, func(i, j int) bool {
			x, y := m.Visible[i], m.Visible[j]
			xs, _ := fileRank(filepath.Base(x.Path), leaf)
			ys, _ := fileRank(filepath.Base(y.Path), leaf)
			if leaf != "" && xs != ys {
				return xs > ys
			}
			if x.Dir != y.Dir {
				return x.Dir
			}
			return x.Path < y.Path
		})
		m.Empty = "Folder is empty"
		if s.Loading {
			m.Empty = "Reading folder…"
		} else if s.Error != "" {
			m.Empty = s.Error
		} else if len(m.Visible) == 0 && leaf != "" {
			m.Visible = []item{{Label: "New file: " + leaf, Detail: "unsaved buffer", Path: expanded}}
		}
	}
	m.Selected = 0
	m.Top = 0
}
func (a *App) readOpenDirectory(m *modal, dir string) {
	s := m.Open
	s.ReadDir = dir
	s.Seq++
	seq := s.Seq
	s.Loading = true
	s.Error = ""
	s.Entries = nil
	m.Visible = nil
	m.Empty = "Reading folder…"
	go func() {
		entries, e := os.ReadDir(dir)
		var files []openEntry
		if e == nil {
			for _, d := range entries {
				isDir := d.IsDir()
				if d.Type()&os.ModeSymlink != 0 {
					if st, e := os.Stat(filepath.Join(dir, d.Name())); e == nil {
						isDir = st.IsDir()
					}
				}
				files = append(files, openEntry{d.Name(), filepath.Join(dir, d.Name()), isDir})
			}
		}
		a.post(func() {
			if a.modal != m || seq != s.Seq {
				return
			}
			s.Loading = false
			s.Entries = files
			if e != nil {
				s.Error = e.Error()
			}
			a.updateOpenModal(m)
		})
	}()
}
func (a *App) completeOpen(m *modal) {
	if m.Selected < 0 || m.Selected >= len(m.Visible) {
		return
	}
	it := m.Visible[m.Selected]
	if !m.Open.Quick && it.Dir {
		m.Open.Dir = it.Path
		m.Input = ""
	} else {
		_, line, col := splitOpenLocation(m.Input)
		base := m.Open.Dir
		if m.Open.Quick {
			base = a.root
		}
		rel, _ := filepath.Rel(base, it.Path)
		m.Input = filepath.ToSlash(rel)
		if line > 0 {
			m.Input += fmt.Sprintf(":%d:%d", line, col)
		}
	}
	m.Cursor = len(m.Input)
	a.updateOpenModal(m)
}
func (a *App) acceptOpen(m *modal, split bool) {
	s := m.Open
	path := ""
	if m.Selected >= 0 && m.Selected < len(m.Visible) {
		path = m.Visible[m.Selected].Path
	}
	query, line, col := splitOpenLocation(strings.TrimSpace(m.Input))
	// A literal filename ending in :digits takes precedence over the jump suffix.
	rel, _ := filepath.Rel(s.Dir, path)
	if path != "" && (m.Input == path || m.Input == filepath.Base(path) || m.Input == rel) {
		line, col = 0, 0
	}
	literal := openPath(s.Dir, strings.TrimSpace(m.Input))
	if st, e := os.Stat(literal); m.Input != "" && e == nil && !st.IsDir() {
		path = literal
		line, col = 0, 0
	}
	if path == "" {
		candidate := openPath(s.Dir, query)
		if st, e := os.Stat(candidate); query != "" && e == nil {
			if st.IsDir() {
				s.Dir = candidate
				m.Input = ""
				m.Cursor = 0
				a.updateOpenModal(m)
				return
			}
			path = candidate
		}
	}
	if path == "" {
		m.Hint = "Choose a file or enter a path · Esc closes"
		return
	}
	if st, e := os.Stat(path); e == nil && st.IsDir() {
		s.Dir = path
		m.Input = ""
		m.Cursor = 0
		a.updateOpenModal(m)
		return
	}
	if st, e := os.Stat(filepath.Dir(path)); e != nil {
		m.Hint = e.Error()
		return
	} else if !st.IsDir() {
		m.Hint = "Parent path is not a folder"
		return
	}
	if s.Quick {
		if _, err := os.Stat(path); err != nil {
			openBuffer := false
			for _, d := range a.docs {
				if d.Path == path {
					openBuffer = true
					break
				}
			}
			if !openBuffer {
				m.Hint = err.Error()
				return
			}
		}
	}
	oldViews, oldActive, oldSplit, oldFocus := a.views, a.active, a.split, a.focus
	wasComparing := a.comparing()
	if split {
		if !a.split {
			a.splitEditor()
		} else {
			a.active = 1 - a.active
		}
	}
	if e := a.open(path); e != nil {
		a.views, a.active, a.split, a.focus = oldViews, oldActive, oldSplit, oldFocus
		if a.comparison != nil {
			a.comparison.Active = wasComparing
		}
		m.Hint = e.Error()
		return
	}
	a.modal = nil
	if line > 0 {
		b := a.view().Doc.Buffer
		row := min(line-1, b.LineCount()-1)
		offset := text.Offset(b.Line(row), max(0, col-1), a.cfg.TabSize)
		a.view().Set(b.LineStart(row)+offset, false)
	}
	a.reveal()
}
func (a *App) rememberFile(path string) {
	if path == "" {
		return
	}
	files := []string{path}
	for _, old := range a.recentFiles {
		if old != path {
			files = append(files, old)
		}
		if len(files) >= 30 {
			break
		}
	}
	a.recentFiles = files
}
