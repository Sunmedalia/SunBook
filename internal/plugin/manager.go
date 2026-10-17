package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type managed struct {
	entry   Entry
	state   State
	runtime *runtime
}
type Manager struct {
	Directory string
	Changes   chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	control   sync.Mutex
	mu        sync.Mutex
	plugins   map[string]*managed
	closed    bool
}

func NewManager(parent context.Context, dir string) *Manager {
	ctx, cancel := context.WithCancel(parent)
	if absolute, err := filepath.Abs(dir); err == nil {
		dir = absolute
	}
	return &Manager{Directory: dir, Changes: make(chan struct{}, 1), ctx: ctx, cancel: cancel, plugins: make(map[string]*managed)}
}
func (m *Manager) changed() {
	select {
	case m.Changes <- struct{}{}:
	default:
	}
}
func stopRuntime(r *runtime) {
	if r == nil {
		return
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(time.Second):
	}
}
func (m *Manager) Refresh(preferences map[string]Preference) error {
	m.control.Lock()
	defer m.control.Unlock()
	return m.refresh(preferences)
}
func (m *Manager) refresh(preferences map[string]Preference) error {
	entries, err := Discover(m.Directory)
	if err != nil {
		return err
	}
	next := make(map[string]*managed)
	var stop []*runtime
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("plugin manager stopped")
	}
	for _, entry := range entries {
		id := entry.Manifest.ID
		pref := preferences[id]
		state := Disabled
		if entry.Error != "" {
			state = Invalid
		} else if pref.Enabled && pref.Fingerprint != entry.Fingerprint {
			state = Review
		} else if pref.Enabled {
			state = Starting
		}
		old := m.plugins[id]
		if old != nil && old.runtime != nil && state == Starting && old.entry.Fingerprint == entry.Fingerprint {
			next[id] = old
			continue
		}
		next[id] = &managed{entry: entry, state: state}
	}
	for id, pref := range preferences {
		if pref.Enabled && next[id] == nil && identifier.MatchString(id) {
			next[id] = &managed{entry: Entry{Directory: filepath.Join(m.Directory, id), Manifest: Manifest{ID: id, Name: id}, Error: "Enabled plugin folder is missing; reinstall or disable it"}, state: Invalid}
		}
	}
	for id, old := range m.plugins {
		if old.runtime != nil && next[id] != old {
			stop = append(stop, old.runtime)
		}
	}
	m.plugins = next
	m.mu.Unlock()
	m.changed()
	for _, r := range stop {
		stopRuntime(r)
	}
	m.mu.Lock()
	if !m.closed {
		for _, p := range m.plugins {
			if p.state == Starting && p.runtime == nil {
				p.runtime = newRuntime(m.ctx, p.entry, m.changed)
			}
		}
	}
	m.mu.Unlock()
	m.changed()
	return nil
}
func (m *Manager) Snapshot() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Status, 0, len(m.plugins))
	for _, p := range m.plugins {
		status := Status{Entry: p.entry, State: p.state, Error: p.entry.Error}
		if p.runtime != nil {
			status = p.runtime.snapshot()
		}
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Entry.Manifest.ID < result[j].Entry.Manifest.ID })
	return result
}
func (m *Manager) Invoke(ctx context.Context, id string, inv Invocation) (Result, error) {
	m.mu.Lock()
	p := m.plugins[id]
	var r *runtime
	if p != nil {
		r = p.runtime
	}
	m.mu.Unlock()
	if r == nil {
		return Result{}, errors.New("plugin is not enabled or requires review")
	}
	declared := false
	for _, c := range r.entry.Manifest.Commands {
		if c.ID == inv.Command {
			declared = true
			break
		}
	}
	if !declared {
		return Result{}, errors.New("plugin command is not declared")
	}
	if !r.entry.Manifest.Allows(ReadDocument) {
		inv.Document = nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.invoke(ctx, inv)
}
func (m *Manager) Restart(id string) error {
	m.control.Lock()
	defer m.control.Unlock()
	m.mu.Lock()
	p := m.plugins[id]
	if m.closed || p == nil || p.runtime == nil {
		m.mu.Unlock()
		return errors.New("enable and review the plugin before restarting")
	}
	old := p.runtime
	p.runtime = nil
	p.state = Starting
	m.mu.Unlock()
	stopRuntime(old)
	m.mu.Lock()
	if !m.closed {
		p.runtime = newRuntime(m.ctx, p.entry, m.changed)
	}
	m.mu.Unlock()
	m.changed()
	return nil
}
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	var all []*runtime
	for _, p := range m.plugins {
		if p.runtime != nil {
			all = append(all, p.runtime)
		}
	}
	m.mu.Unlock()
	for _, r := range all {
		r.cancel()
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for _, r := range all {
		select {
		case <-r.done:
		case <-deadline.C:
			return
		}
	}
}

// Check starts a local plugin only when explicitly invoked by the developer.
// It validates the API handshake and health response without executing commands.
func Check(ctx context.Context, dir string) error {
	entry, err := ReadManifest(dir)
	if err != nil {
		return err
	}
	p, err := startProcess(entry)
	if err != nil {
		return err
	}
	defer p.stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var hello struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err = p.call(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion, "host": "SunBook"}, &hello); err != nil {
		return err
	}
	if hello.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("API mismatch: got %d, expected %d", hello.ProtocolVersion, ProtocolVersion)
	}
	return p.ping(ctx)
}
func (m *Manager) Install(source string) (string, error) {
	m.control.Lock()
	defer m.control.Unlock()
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	entry, err := ReadManifest(source)
	if err != nil {
		return "", err
	}
	if rel, e := filepath.Rel(source, m.Directory); e == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return "", errors.New("plugin source cannot contain its installation directory")
	}
	if err = os.MkdirAll(m.Directory, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(m.Directory, entry.Manifest.ID)
	if _, err = os.Lstat(target); !os.IsNotExist(err) {
		return "", errors.New("plugin already installed; disable and remove it before installing an update")
	}
	temp, err := os.MkdirTemp(m.Directory, ".install-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	count := 0
	var size int64
	err = filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if m.ctx.Err() != nil {
			return m.ctx.Err()
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("plugin package must contain only regular files and directories")
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(temp, rel), 0700)
		}
		count++
		if count > 1000 || info.Size() > 64*1024*1024-size {
			return errors.New("plugin package exceeds 1000 files or 64 MiB")
		}
		copied, err := copyPluginFile(path, filepath.Join(temp, rel), info.Mode().Perm(), 64*1024*1024-size)
		size += copied
		return err
	})
	if err != nil {
		return "", err
	}
	// Reject a source that changed while copying. Directory access bits are not
	// part of approval, so the private installation root can remain mode 0700.
	fingerprint, err := packageFingerprint(temp)
	if err != nil {
		return "", err
	}
	if fingerprint != entry.Fingerprint {
		return "", errors.New("plugin package changed during installation; try again")
	}
	if err = os.Rename(temp, target); err != nil {
		return "", err
	}
	return entry.Manifest.ID, nil
}
func (m *Manager) Remove(id string) error {
	m.control.Lock()
	defer m.control.Unlock()
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsRune(id, 0) {
		return errors.New("invalid plugin id")
	}
	m.mu.Lock()
	p := m.plugins[id]
	if p == nil {
		m.mu.Unlock()
		return errors.New("plugin not installed")
	}
	if p.runtime != nil {
		m.mu.Unlock()
		return errors.New("disable the plugin before removing it")
	}
	m.mu.Unlock()
	target := filepath.Join(m.Directory, id)
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("plugin directory is not a regular directory")
	}
	if err = os.RemoveAll(target); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.plugins, id)
	m.mu.Unlock()
	m.changed()
	return nil
}
