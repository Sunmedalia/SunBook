package ui

import (
	"container/heap"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/text"
)

type openEntry struct {
	Name, Path string
	Dir        bool
}
type openPicker struct {
	Quick                    bool
	Dir, ReadDir, Error      string
	Index                    fileIndex
	Entries                  []openEntry
	Loading                  bool
	Seq                      int
	Path                     string
	PathCursor, PathOffset   int
	EditingPath, SelectPath  bool
	PathRect, UpRect, GoRect rect
}

func (a *App) quickOpen() {
	m := a.openModal(true)
	if a.treeIndexReady {
		m.Open.Index = a.treeFiles
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
	hint := "Ctrl+L edits path · Tab completes · Alt+↑ parent · Ctrl+Enter split · Esc closes"
	if quick {
		title = "Quick Open"
		hint = "Type filename · :line:column jumps · Ctrl+Enter opens in split"
	}
	m := &modal{Title: title, InputEnabled: true, Hint: hint, Placeholder: "File name or path…", Open: &openPicker{Quick: quick, Dir: a.root, Path: a.root, PathCursor: len(a.root)}}
	m.Accept = func(split bool) { a.acceptOpen(m, split) }
	m.Complete = func() { a.completeOpen(m) }
	m.Parent = func() { a.openParent(m) }
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
	return newFileRanker(query).rank(path, filepath.Base(path))
}

// fileRanker prepares a query once so ranking many paths repeats no work.
type fileRanker struct {
	words  []string
	needle [][]rune
}

func newFileRanker(query string) fileRanker {
	var f fileRanker
	f.words = strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	for _, word := range f.words {
		f.needle = append(f.needle, []rune(word))
	}
	return f
}

// rank scores a lower-case, slash-separated path and its filepath.Base.
func (f fileRanker) rank(path, base string) (int, bool) {
	if len(f.words) == 0 {
		return 0, true
	}
	score := 0
	for w, word := range f.words {
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
			needle := f.needle[w]
			j, previous := 0, -2
			part := 0
			i, before := 0, rune(0)
			for _, r := range path {
				if j < len(needle) && r == needle[j] {
					part += 20
					if i == previous+1 {
						part += 30
					}
					if i == 0 || strings.ContainsRune("/_-. ", before) {
						part += 20
					}
					previous = i
					j++
				}
				before = r
				i++
			}
			if j != len(needle) {
				return 0, false
			}
			score += 1000 + part - i
		}
	}
	return score, true
}

type rankedFile struct {
	path, rel string
	score     int
}

// rankedFiles keeps the best files, highest score first and then by path.
// Paths are unique, so this order is total and the kept set is exact.
type rankedFiles []rankedFile

func rankedBefore(x, y rankedFile) bool {
	if x.score != y.score {
		return x.score > y.score
	}
	return x.path < y.path
}
func (h rankedFiles) Len() int           { return len(h) }
func (h rankedFiles) Less(i, j int) bool { return rankedBefore(h[j], h[i]) } // worst on top
func (h rankedFiles) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *rankedFiles) Push(x any)        { *h = append(*h, x.(rankedFile)) }
func (h *rankedFiles) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
func (h *rankedFiles) offer(f rankedFile, limit int) {
	if h.Len() < limit {
		heap.Push(h, f)
	} else if rankedBefore(f, (*h)[0]) {
		(*h)[0] = f
		heap.Fix(h, 0)
	}
}
func (h rankedFiles) sorted() []rankedFile {
	out := append([]rankedFile(nil), h...)
	sort.Slice(out, func(i, j int) bool { return rankedBefore(out[i], out[j]) })
	return out
}

func (a *App) updateOpenModal(m *modal) {
	if a.modal != m || m.Open == nil {
		return
	}
	s := m.Open
	query, _, _ := splitOpenLocation(strings.TrimSpace(m.Input))
	if s.Quick {
		m.Subtitle = "Recent files and workspace · " + a.root
		// Recent and open files come first; the index supplies precomputed
		// relative and lower-case paths for everything else.
		var extra []string
		extra = append(extra, a.recentFiles...)
		for _, d := range a.docs {
			if d.Path != "" {
				extra = append(extra, d.Path)
			}
		}
		index := s.Index
		exact := func(path, rel, name string) bool {
			return m.Input == rel || m.Input == name || m.Input == path
		}
		matched := false
		for _, path := range extra {
			rel, _ := filepath.Rel(a.root, path)
			if exact(path, rel, filepath.Base(path)) {
				matched = true
				break
			}
		}
		for i := 0; !matched && i < len(index.Paths); i++ {
			matched = exact(index.Paths[i], index.Rel[i], index.Names[i])
		}
		if matched {
			query = m.Input
		}
		recent := make(map[string]int, len(a.recentFiles))
		for i, path := range a.recentFiles {
			if _, ok := recent[path]; !ok {
				recent[path] = i
			}
		}
		ranker := newFileRanker(query)
		top := &rankedFiles{}
		seen := make(map[string]bool, len(extra))
		consider := func(path, rel, key, base string) {
			score, ok := ranker.rank(key, base)
			if !ok {
				return
			}
			if i, ok := recent[path]; ok {
				score += max(1, 100-i)
			}
			top.offer(rankedFile{path, rel, score}, 200)
		}
		for _, path := range extra {
			if seen[path] {
				continue
			}
			seen[path] = true
			rel, _ := filepath.Rel(a.root, path)
			key := strings.ToLower(filepath.ToSlash(rel))
			consider(path, rel, key, filepath.Base(key))
		}
		for i, path := range index.Paths {
			if seen[path] || i > 0 && index.Paths[i-1] == path {
				continue
			}
			consider(path, index.Rel[i], index.Keys[i], index.Bases[i])
		}
		best := top.sorted()
		m.Visible = nil
		for _, r := range best {
			detail := filepath.ToSlash(filepath.Dir(r.rel))
			if detail == "." {
				detail = "workspace"
			}
			if _, ok := recent[r.path]; ok {
				detail = "recent · " + detail
			}
			m.Visible = append(m.Visible, item{Label: filepath.Base(r.path), Detail: detail, Path: r.path})
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
		ranker := newFileRanker(leaf)
		rank := func(name string) (int, bool) {
			name = strings.ToLower(filepath.ToSlash(name))
			return ranker.rank(name, filepath.Base(name))
		}
		var scores []int
		for _, entry := range s.Entries {
			if _, ok := rank(entry.Name); !ok {
				continue
			}
			label, detail := entry.Name, "file"
			if entry.Dir {
				detail = "folder"
			}
			score, _ := rank(filepath.Base(entry.Path))
			m.Visible = append(m.Visible, item{Label: label, Detail: detail, Path: entry.Path, Dir: entry.Dir})
			scores = append(scores, score)
		}
		// Rank each entry once instead of twice per comparison.
		order := make([]int, len(m.Visible))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool {
			x, y := m.Visible[order[i]], m.Visible[order[j]]
			if xs, ys := scores[order[i]], scores[order[j]]; leaf != "" && xs != ys {
				return xs > ys
			}
			if x.Dir != y.Dir {
				return x.Dir
			}
			return x.Path < y.Path
		})
		if len(order) > 0 {
			sorted := make([]item, len(order))
			for i, k := range order {
				sorted[i] = m.Visible[k]
			}
			m.Visible = sorted
		}
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
	if !s.EditingPath {
		s.Path = dir
		s.PathCursor = len(dir)
	}
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
		a.enterOpenDirectory(m, it.Path)
		return
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
				a.enterOpenDirectory(m, candidate)
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
		a.enterOpenDirectory(m, path)
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

func (a *App) enterOpenDirectory(m *modal, dir string) {
	s := m.Open
	s.Dir = dir
	s.Path = dir
	s.PathCursor = len(dir)
	s.EditingPath = false
	s.SelectPath = false
	m.Input = ""
	m.Cursor = 0
	m.Hint = "Ctrl+L edits path · Tab completes · Alt+↑ parent · Ctrl+Enter split · Esc closes"
	a.updateOpenModal(m)
}
func (a *App) openParent(m *modal) {
	s := m.Open
	if s.Quick {
		return
	}
	dir := s.ReadDir
	if dir == "" {
		dir = s.Dir
	}
	if parent := filepath.Dir(dir); parent != dir {
		a.enterOpenDirectory(m, parent)
	}
}
func (a *App) submitOpenPath(m *modal) {
	s := m.Open
	base := s.ReadDir
	if base == "" {
		base = s.Dir
	}
	dir := openPath(base, s.Path)
	st, err := os.Stat(dir)
	if err != nil {
		m.Hint = err.Error()
		s.EditingPath = true
		return
	}
	if !st.IsDir() {
		m.Hint = "Choose a folder for the path; use the file field to open a file"
		s.EditingPath = true
		return
	}
	a.enterOpenDirectory(m, dir)
}
func (a *App) insertOpenPath(m *modal, value string) {
	s := m.Open
	if s.SelectPath {
		s.Path = ""
		s.PathCursor = 0
		s.SelectPath = false
	}
	value = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value)
	s.Path = s.Path[:s.PathCursor] + value + s.Path[s.PathCursor:]
	s.PathCursor += len(value)
}
func (a *App) openPathKey(m *modal, e *tcell.EventKey, name string) bool {
	s := m.Open
	if s == nil || s.Quick {
		return false
	}
	switch name {
	case "ctrl+l":
		s.EditingPath = true
		s.SelectPath = true
		s.PathCursor = len(s.Path)
		return true
	case "alt+up":
		a.openParent(m)
		return true
	}
	if !s.EditingPath {
		if name == "shift+tab" {
			s.EditingPath = true
			s.SelectPath = true
			s.PathCursor = len(s.Path)
			return true
		}
		return false
	}
	switch name {
	case "enter", "ctrl+enter":
		a.submitOpenPath(m)
	case "escape":
		s.EditingPath = false
		s.SelectPath = false
		s.Path = s.ReadDir
		s.PathCursor = len(s.Path)
	case "tab", "shift+tab":
		s.EditingPath = false
		s.SelectPath = false
	case "ctrl+v":
		a.readClipboard()
	case "ctrl+a":
		s.SelectPath = true
		s.PathCursor = len(s.Path)
	case "ctrl+u":
		s.Path = ""
		s.PathCursor = 0
		s.SelectPath = false
	case "left":
		if s.SelectPath {
			s.PathCursor = 0
		} else {
			s.PathCursor = text.Prev(s.Path, s.PathCursor)
		}
		s.SelectPath = false
	case "right":
		if s.SelectPath {
			s.PathCursor = len(s.Path)
		} else {
			s.PathCursor = text.Next(s.Path, s.PathCursor)
		}
		s.SelectPath = false
	case "home":
		s.PathCursor = 0
		s.SelectPath = false
	case "end", "ctrl+e":
		s.PathCursor = len(s.Path)
		s.SelectPath = false
	case "backspace", "backspace2", "delete":
		if s.SelectPath {
			s.Path = ""
			s.PathCursor = 0
			s.SelectPath = false
			break
		}
		if name == "delete" {
			end := text.Next(s.Path, s.PathCursor)
			s.Path = s.Path[:s.PathCursor] + s.Path[end:]
		} else {
			start := text.Prev(s.Path, s.PathCursor)
			s.Path = s.Path[:start] + s.Path[s.PathCursor:]
			s.PathCursor = start
		}
	default:
		if e.Str() != "" && e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt|tcell.ModMeta) == 0 {
			a.insertOpenPath(m, e.Str())
		}
	}
	return true
}
func (a *App) openNavigationMouse(m *modal, x, y int, fresh bool) bool {
	s := m.Open
	if s == nil || s.Quick || !fresh {
		return false
	}
	switch {
	case s.UpRect.contains(x, y):
		a.openParent(m)
	case s.GoRect.contains(x, y):
		a.submitOpenPath(m)
	case s.PathRect.contains(x, y):
		s.EditingPath = true
		s.SelectPath = false
		offset := text.Snap(s.Path, min(s.PathOffset, len(s.Path)))
		s.PathCursor = offset + text.Offset(s.Path[offset:], x-s.PathRect.X, a.cfg.TabSize)
	case m.InputRect.contains(x, y):
		s.EditingPath = false
		s.SelectPath = false
		m.Cursor = m.InputOffset + text.Offset(m.Input[m.InputOffset:], x-m.InputRect.X, a.cfg.TabSize)
	default:
		return false
	}
	return true
}
func (a *App) drawOpenNavigation(m *modal, r rect) {
	s := m.Open
	s.UpRect = rect{}
	s.GoRect = rect{}
	s.PathRect = rect{}
	if r.W < 14 {
		return
	}
	upWidth := 6
	if r.W < 28 {
		upWidth = 3
	}
	s.UpRect = rect{r.X, r.Y, upWidth, 1}
	s.GoRect = rect{r.X + r.W - 4, r.Y, 4, 1}
	s.PathRect = rect{r.X + upWidth + 1, r.Y, max(1, r.W-upWidth-6), 1}
	for _, b := range []struct {
		r        rect
		label    string
		disabled bool
	}{{s.UpRect, "↑ Up", filepath.Dir(s.ReadDir) == s.ReadDir}, {s.GoRect, "Go", false}} {
		label := b.label
		if b.r.W == 3 {
			label = "↑"
		}
		st := a.base.Background(a.colors.raised)
		if b.disabled {
			st = a.chrome.Foreground(a.colors.disabled)
		}
		a.fill(b.r, st)
		a.put(b.r.X+1, b.r.Y, b.r.W-2, label, st.Bold(!b.disabled))
		if !b.disabled {
			a.put(b.r.X, b.r.Y, 1, "▏", st.Foreground(mix(st.GetBackground(), a.base.GetForeground(), 26)))
			a.put(b.r.X+b.r.W-1, b.r.Y, 1, "▕", st.Foreground(a.colors.shadow))
		}
	}
	st := a.inputStyle(s.EditingPath)
	if s.EditingPath && s.SelectPath {
		st = st.Background(a.colors.selection)
	}
	a.fill(s.PathRect, st)
	offset := 0
	if s.EditingPath {
		if col := text.Width(s.Path[:s.PathCursor], a.cfg.TabSize); col >= s.PathRect.W {
			offset = text.Offset(s.Path, col-s.PathRect.W+1, a.cfg.TabSize)
		}
	} else if text.Width(s.Path, a.cfg.TabSize) > s.PathRect.W {
		offset = text.Offset(s.Path, text.Width(s.Path, a.cfg.TabSize)-s.PathRect.W, a.cfg.TabSize)
	}
	s.PathOffset = offset
	a.put(s.PathRect.X, s.PathRect.Y, s.PathRect.W, s.Path[offset:], st)
	if s.EditingPath {
		a.screen.ShowCursor(s.PathRect.X+text.Width(s.Path[offset:s.PathCursor], a.cfg.TabSize), s.PathRect.Y)
	}
}
