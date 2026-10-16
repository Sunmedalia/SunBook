package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	gitservice "sunbook/internal/git"
)

type gitLogRow struct {
	Commit           int
	Kind, Path, Name string
	Depth            int
	File             gitservice.CommitFile
}

func (r gitLogRow) node() string {
	if r.Kind == "file" || r.Kind == "folder" {
		return r.Kind + ":" + r.Path
	}
	return ""
}
func (r gitLogRow) selectable() bool {
	return r.Kind == "commit" || r.Kind == "file" || r.Kind == "folder"
}
func (a *App) gitLogRowKey(row gitLogRow) string {
	return a.gitLog[row.Commit].Hash + "\x00" + row.Kind + "\x00" + row.Path
}

type logTreeNode struct {
	Name, Path string
	File       *gitservice.CommitFile
	Children   map[string]*logTreeNode
}

func (a *App) gitLogRows() []gitLogRow {
	var rows []gitLogRow
	for index, revision := range a.gitLog {
		rows = append(rows, gitLogRow{Commit: index, Kind: "commit"}, gitLogRow{Commit: index, Kind: "meta"})
		if !a.gitLogOpen[revision.Hash] {
			continue
		}
		root := &logTreeNode{Children: map[string]*logTreeNode{}}
		for _, file := range revision.Files {
			node := root
			parts := strings.Split(file.Path, "/")
			for depth, part := range parts {
				child := node.Children[part]
				if child == nil {
					child = &logTreeNode{Name: part, Path: strings.Join(parts[:depth+1], "/"), Children: map[string]*logTreeNode{}}
					node.Children[part] = child
				}
				if depth == len(parts)-1 {
					copy := file
					child.File = &copy
				}
				node = child
			}
		}
		var walk func(*logTreeNode, int)
		walk = func(parent *logTreeNode, depth int) {
			children := make([]*logTreeNode, 0, len(parent.Children))
			for _, child := range parent.Children {
				children = append(children, child)
			}
			sort.Slice(children, func(i, j int) bool {
				if (children[i].File == nil) != (children[j].File == nil) {
					return children[i].File == nil
				}
				return children[i].Name < children[j].Name
			})
			for _, node := range children {
				row := gitLogRow{Commit: index, Kind: "folder", Path: node.Path, Name: node.Name, Depth: depth}
				if node.File != nil {
					row.Kind, row.File = "file", *node.File
				}
				rows = append(rows, row)
				if node.File == nil && !a.gitLogCollapsed[revision.Hash+"\x00"+node.Path] {
					walk(node, depth+1)
				}
			}
		}
		walk(root, 0)
		if len(revision.Files) == 0 {
			rows = append(rows, gitLogRow{Commit: index, Kind: "empty", Name: "No changed files"})
		}
	}
	return rows
}

func (a *App) drawGitLogRows() {
	r := a.gitLogRect
	rows := a.gitLogRows()
	a.gitLogTop = max(0, min(a.gitLogTop, max(0, len(rows)-1)))
	for i, y := a.gitLogTop, r.Y; i < len(rows) && y < r.Y+r.H; i, y = i+1, y+1 {
		row := rows[i]
		revision := a.gitLog[row.Commit]
		style := a.chrome
		if row.Commit == a.gitLogSelected && row.node() == a.gitLogNode && row.Kind != "empty" {
			style = a.drawFileSelection(rect{r.X, y, r.W, 1}, a.focus == "gitLog")
		}
		switch row.Kind {
		case "commit":
			arrow := "▸"
			if a.gitLogOpen[revision.Hash] {
				arrow = "▾"
			}
			a.put(r.X+1, y, 1, arrow, style.Foreground(a.colors.muted))
			hash := revision.Hash[:min(7, len(revision.Hash))]
			a.put(r.X+3, y, len(hash), hash, style.Foreground(a.accent.GetForeground()))
			a.put(r.X+len(hash)+4, y, r.W-len(hash)-5, fitLabel(revision.Subject, r.W-len(hash)-5), style)
		case "meta":
			count := fmt.Sprintf("%d files", len(revision.Files))
			if len(revision.Files) == 1 {
				count = "1 file"
			}
			countX := a.sidebarCount(r.X+r.W-1, y, count, style)
			a.put(r.X+3, y, countX-r.X-4, fitLabel(revision.Author+" · "+revision.Age, countX-r.X-4), style.Foreground(a.colors.muted))
		case "empty":
			a.put(r.X+3, y, r.W-4, fitLabel(row.Name, r.W-4), style.Foreground(a.colors.muted))
		default:
			x := r.X + 2 + min(row.Depth*2, max(0, r.W-14))
			end := r.X + r.W - 3
			for depth := 0; depth < row.Depth && r.X+2+depth*2 < x; depth++ {
				a.put(r.X+2+depth*2, y, 1, "│", style.Foreground(a.colors.border))
			}
			folder := row.Kind == "folder"
			open := folder && !a.gitLogCollapsed[revision.Hash+"\x00"+row.Path]
			prefix := "  "
			if folder {
				prefix = "▸ "
				if open {
					prefix = "▾ "
				}
			}
			a.put(x, y, min(2, end-x), prefix, style.Foreground(a.colors.muted))
			x += 2
			x += a.drawFileIcon(x, y, end-x, row.Path, folder, open, style)
			a.put(x, y, end-x, fitLabel(row.Name, end-x), style)
			if !folder {
				color := a.accent.GetForeground()
				if row.File.Status == "A" {
					color = a.colors.success
				} else if row.File.Status == "D" {
					color = a.colors.danger
				}
				a.put(r.X+r.W-2, y, 1, row.File.Status, style.Foreground(color))
			}
		}
	}
	a.sidebarScroll(r, a.gitLogTop, len(rows))
}

func (a *App) selectedGitLogRow() (gitLogRow, int) {
	rows := a.gitLogRows()
	for i, row := range rows {
		if row.selectable() && row.Commit == a.gitLogSelected && row.node() == a.gitLogNode {
			return row, i
		}
	}
	return gitLogRow{Commit: a.gitLogSelected, Kind: "commit"}, -1
}

func (a *App) selectGitLogRow(row gitLogRow, index int) {
	a.gitLogSelected, a.gitLogNode = row.Commit, row.node()
	if index < a.gitLogTop {
		a.gitLogTop = index
	}
	if index >= a.gitLogTop+max(1, a.gitLogRect.H) {
		a.gitLogTop = index - max(1, a.gitLogRect.H) + 1
	}
}

func (a *App) moveGitLog(delta int) {
	rows := a.gitLogRows()
	var indices []int
	selected := 0
	for i, row := range rows {
		if row.selectable() {
			if row.Commit == a.gitLogSelected && row.node() == a.gitLogNode {
				selected = len(indices)
			}
			indices = append(indices, i)
		}
	}
	if len(indices) > 0 {
		index := indices[max(0, min(selected+delta, len(indices)-1))]
		a.selectGitLogRow(rows[index], index)
	}
}

func (a *App) activateGitLogRow() {
	row, index := a.selectedGitLogRow()
	if index < 0 || row.Commit < 0 || row.Commit >= len(a.gitLog) {
		return
	}
	hash := a.gitLog[row.Commit].Hash
	switch row.Kind {
	case "commit":
		if a.gitLogOpen == nil {
			a.gitLogOpen = map[string]bool{}
		}
		a.gitLogOpen[hash] = !a.gitLogOpen[hash]
	case "folder":
		if a.gitLogCollapsed == nil {
			a.gitLogCollapsed = map[string]bool{}
		}
		key := hash + "\x00" + row.Path
		a.gitLogCollapsed[key] = !a.gitLogCollapsed[key]
	case "file":
		a.readGitComparison(a.git.Root, func(ctx context.Context, root string) (gitservice.Comparison, error) {
			return gitservice.CompareRevision(ctx, root, hash, row.File)
		})
	}
}

func (a *App) gitLogTreeArrow(right bool) {
	row, index := a.selectedGitLogRow()
	if index < 0 {
		return
	}
	hash := a.gitLog[row.Commit].Hash
	open := a.gitLogOpen[hash]
	if row.Kind == "folder" {
		open = !a.gitLogCollapsed[hash+"\x00"+row.Path]
	}
	if row.Kind != "file" && open != right {
		a.activateGitLogRow()
		return
	}
	if right {
		if row.Kind != "file" {
			a.moveGitLog(1)
		}
		return
	}
	parent := ""
	if slash := strings.LastIndex(row.Path, "/"); slash >= 0 {
		parent = row.Path[:slash]
	}
	for i, candidate := range a.gitLogRows() {
		if candidate.Commit == row.Commit && (parent == "" && candidate.Kind == "commit" || parent != "" && candidate.Kind == "folder" && candidate.Path == parent) {
			a.selectGitLogRow(candidate, i)
			return
		}
	}
}
