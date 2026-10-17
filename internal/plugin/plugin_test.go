package plugin

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPluginHelper(t *testing.T) {
	if os.Getenv("SUNBOOK_PLUGIN_API") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "inspection-target" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	for scanner.Scan() {
		var req struct {
			ID     uint64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialize":
			if mode == "bad-version" {
				result = map[string]int{"protocolVersion": 99}
			} else {
				result = map[string]int{"protocolVersion": 1}
			}
		case "ping":
			if mode == "bad-ping" {
				fmt.Fprintln(os.Stdout, "bad health response")
				continue
			}
		case "execute":
			switch mode {
			case "isolation":
				var inv Invocation
				json.Unmarshal(req.Params, &inv)
				var probeErr error
				switch inv.Command {
				case "environment":
					data, _ := json.Marshal(os.Environ())
					result = Result{Body: string(data), Message: os.Getenv("HOME")}
				case "read":
					_, probeErr = os.ReadFile(inv.Document.Path)
				case "write":
					probeErr = os.WriteFile(inv.Document.Path, []byte("plugin write"), 0600)
				case "child-read":
					probeErr = exec.Command("/bin/cat", inv.Document.Path).Run()
				case "symlink-read", "hardlink-read":
					link := filepath.Join(os.Getenv("HOME"), inv.Command)
					if inv.Command == "symlink-read" {
						probeErr = os.Symlink(inv.Document.Path, link)
					} else {
						probeErr = os.Link(inv.Document.Path, link)
					}
					if probeErr == nil {
						_, probeErr = os.ReadFile(link)
					}
				case "network", "unix", "udp":
					network := "tcp"
					if inv.Command != "network" {
						network = inv.Command
					}
					var c net.Conn
					c, probeErr = net.DialTimeout(network, inv.Document.Path, 300*time.Millisecond)
					if c != nil {
						c.Close()
					}
				case "lookup":
					_, probeErr = net.LookupHost(inv.Document.Path)
				case "inspect-process":
					data, err := exec.Command("/bin/ps", "eww", "-p", inv.Document.Path).CombinedOutput()
					probeErr = err
					result = Result{Body: string(data)}
				case "certificate":
					var certificate *x509.Certificate
					var data []byte
					data, probeErr = base64.StdEncoding.DecodeString(inv.Document.Path)
					if probeErr == nil {
						certificate, probeErr = x509.ParseCertificate(data)
					}
					if probeErr == nil {
						_, probeErr = certificate.Verify(x509.VerifyOptions{})
						var untrusted x509.UnknownAuthorityError
						if errors.As(probeErr, &untrusted) {
							probeErr = nil // Native trust evaluation worked and rejected the test CA.
						} else if probeErr == nil {
							probeErr = errors.New("unexpectedly trusted test CA")
						}
					}
				}
				if inv.Command != "environment" {
					message := "allowed"
					if probeErr != nil {
						message = "denied: " + probeErr.Error()
					}
					got, _ := result.(Result)
					got.Message = message
					result = got
				}
			case "crash", "bad-ping":
				os.Exit(3)
			case "bad-json":
				fmt.Fprintln(os.Stdout, "broken protocol")
				continue
			case "descendant":
				child := exec.Command("/bin/sh", "-c", "sleep 60; exit 0")
				if err := child.Start(); err != nil {
					fmt.Fprintln(os.Stderr, "child start failed:", err)
				} else {
					fmt.Fprintln(os.Stderr, "child-pid:", child.Process.Pid)
				}
				time.Sleep(time.Minute)
			case "stall":
				time.Sleep(time.Minute)
			case "edits":
				result = Result{Edits: []Edit{{Start: 0, End: 1, Text: "X"}}}
			case "preview", "bad-preview", "large-preview":
				preview := &Preview{Kind: "markdown", Text: "# Heading"}
				if mode == "bad-preview" {
					preview.Kind = "html"
				}
				if mode == "large-preview" {
					preview.Text = strings.Repeat("x", 1024*1024+1)
				}
				result = Result{Preview: preview}
			case "log-flood":
				fmt.Fprint(os.Stderr, strings.Repeat("x", 2*1024*1024))
				result = Result{Message: "should be rejected"}
			case "huge-log":
				fmt.Fprint(os.Stderr, strings.Repeat("log", 20000))
				result = Result{Message: "ok"}
			default:
				var inv Invocation
				json.Unmarshal(req.Params, &inv)
				message := "no document"
				if inv.Document != nil {
					message = inv.Document.Text
				}
				result = Result{Message: message}
			}
		}
		data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		fmt.Fprintln(os.Stdout, string(data))
		if req.Method == "initialize" && mode == "no-read" {
			time.Sleep(time.Minute)
		}
	}
	os.Exit(0)
}
func fixture(t *testing.T, mode string, permissions []string) (*Manager, Entry) {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "demo")
	if err := os.Mkdir(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{ID: "demo", Name: "Demo", Version: "1.0.0", APIVersion: 1, Command: []string{exe, "-test.run=TestPluginHelper", "--", mode}, Permissions: permissions, Commands: []Command{{ID: "run", Title: "Run demo"}}}
	raw, _ := json.Marshal(manifest)
	if err = os.WriteFile(filepath.Join(pluginDir, "plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := ReadManifest(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(context.Background(), dir)
	t.Cleanup(m.Close)
	return m, entry
}
func waitState(t *testing.T, m *Manager, state State) Status {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		for _, s := range m.Snapshot() {
			if s.State == state {
				return s
			}
		}
		select {
		case <-m.Changes:
		case <-timer.C:
			t.Fatalf("state %s not reached: %+v", state, m.Snapshot())
		}
	}
}
func enable(t *testing.T, m *Manager, entry Entry) {
	t.Helper()
	if err := m.Refresh(map[string]Preference{entry.Manifest.ID: {Enabled: true, Fingerprint: entry.Fingerprint}}); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, Ready)
}
func TestLifecyclePermissionsManifestReviewAndRestart(t *testing.T) {
	m, entry := fixture(t, "ok", nil)
	if err := m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot()[0].State != Disabled {
		t.Fatal("new plugin auto-enabled")
	}
	enable(t, m, entry)
	result, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run", Document: &Document{Text: "secret"}})
	if err != nil || result.Message != "no document" {
		t.Fatal("read permission leak", result, err)
	}
	if err = m.Restart("demo"); err != nil {
		t.Fatal(err)
	}
	waitState(t, m, Ready)
	if _, err = m.Invoke(context.Background(), "demo", Invocation{Command: "unknown"}); err == nil {
		t.Fatal("undeclared command accepted")
	}
	path := filepath.Join(entry.Directory, "plugin.json")
	raw, _ := os.ReadFile(path)
	raw = append(raw, '\n')
	os.WriteFile(path, raw, 0600)
	if err = m.Refresh(map[string]Preference{"demo": {Enabled: true, Fingerprint: entry.Fingerprint}}); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot()[0].State != Review {
		t.Fatal("updated manifest did not require review")
	}
	if _, err = m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err == nil {
		t.Fatal("unreviewed plugin ran")
	}
}
func TestEditsPermissionAndApplicationFailuresKeepPluginAvailable(t *testing.T) {
	m, entry := fixture(t, "edits", []string{ReadDocument})
	enable(t, m, entry)
	if _, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err == nil || !strings.Contains(err.Error(), "permission") {
		t.Fatal(err)
	}
	waitState(t, m, Ready)
	if m.Snapshot()[0].Failures != 0 {
		t.Fatal("permission rejection crashed plugin")
	}
}
func TestCrashRecoveryAndCircuitBreaker(t *testing.T) {
	m, entry := fixture(t, "bad-version", nil)
	if err := m.Refresh(map[string]Preference{"demo": {Enabled: true, Fingerprint: entry.Fingerprint}}); err != nil {
		t.Fatal(err)
	}
	s := waitState(t, m, Quarantined)
	if s.Failures != 4 || !strings.Contains(s.Error, "protocol") {
		t.Fatal(s)
	}
	if err := m.Restart("demo"); err != nil {
		t.Fatal(err)
	}
	s = waitState(t, m, Restarting)
	if s.Failures != 1 {
		t.Fatal("restart did not reset failure circuit", s)
	}
}
func TestDeadlineCrashAndMalformedOutputAreIsolated(t *testing.T) {
	for _, mode := range []string{"stall", "crash", "bad-json", "no-read", "log-flood"} {
		t.Run(mode, func(t *testing.T) {
			m, entry := fixture(t, mode, []string{ReadDocument})
			enable(t, m, entry)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, err := m.Invoke(ctx, "demo", Invocation{Command: "run", Document: &Document{Text: strings.Repeat("x", 256*1024)}})
			if err == nil {
				t.Fatal("bad plugin succeeded")
			}
			waitState(t, m, Restarting)
			if err = m.Refresh(nil); err != nil {
				t.Fatal(err)
			}
			if m.Snapshot()[0].State != Disabled {
				t.Fatal("disable failed")
			}
		})
	}
}
func TestLogsAreBoundedAndHealthyPluginSurvives(t *testing.T) {
	m, entry := fixture(t, "huge-log", nil)
	enable(t, m, entry)
	if _, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err != nil {
		t.Fatal(err)
	}
	s := waitState(t, m, Ready)
	if len(s.Log) > 16384 {
		t.Fatal("unbounded stderr")
	}
}
func TestInstallValidationAndRemoval(t *testing.T) {
	_, entry := fixture(t, "ok", nil)
	m := NewManager(context.Background(), filepath.Join(t.TempDir(), "plugins"))
	defer m.Close()
	if _, err := m.Install(entry.Directory); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(entry.Directory); err == nil {
		t.Fatal("overwrote installed plugin")
	}
	if err := m.Remove("../outside"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := m.Remove("demo"); err != nil {
		t.Fatal(err)
	}
	if len(m.Snapshot()) != 0 {
		t.Fatal("plugin still installed")
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(entry.Directory, "bad-link")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(entry.Directory); err == nil {
		t.Fatal("symlink package accepted")
	}
}
func TestManifestValidationAndPythonExampleConformance(t *testing.T) {
	_, entry := fixture(t, "ok", nil)
	manifest := entry.Manifest
	for _, change := range []func(*Manifest){func(m *Manifest) { m.APIVersion = 2 }, func(m *Manifest) { m.ID = "../bad" }, func(m *Manifest) { m.Permissions = []string{EditDocument} }, func(m *Manifest) { m.Commands = append(m.Commands, m.Commands[0]) }, func(m *Manifest) { m.Command = nil }} {
		m := manifest
		change(&m)
		raw, _ := json.Marshal(m)
		os.WriteFile(filepath.Join(entry.Directory, "plugin.json"), raw, 0600)
		if _, err := ReadManifest(entry.Directory); err == nil {
			t.Fatal("invalid manifest accepted", m)
		}
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("Python example requires python3")
	}
	root, err := filepath.Abs("../../examples/text-tools")
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	sdk, _ := os.ReadFile("../../sdk/python/sunbook_plugin.py")
	vendored, _ := os.ReadFile(filepath.Join(root, "sunbook_plugin.py"))
	if string(sdk) != string(vendored) {
		t.Fatal("example SDK differs from canonical SDK")
	}
	sample, err := ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(context.Background(), filepath.Dir(root))
	defer m.Close()
	enable(t, m, sample)
	result, err := m.Invoke(context.Background(), "text-tools", Invocation{Command: "word-count", Document: &Document{Text: "hello 世界\n"}})
	if err != nil || !strings.Contains(result.Body, "Words: 2") {
		t.Fatal(result, err)
	}
	result, err = m.Invoke(context.Background(), "text-tools", Invocation{Command: "uppercase-selection", Document: &Document{Text: "中hello", Selections: []Position{{Anchor: 3, Head: 8}}}})
	if err != nil || len(result.Edits) != 1 || result.Edits[0].Text != "HELLO" || result.Edits[0].Start != 3 {
		t.Fatal(result, err)
	}
}

func TestHealthFailureAndOtherPluginAvailability(t *testing.T) {
	m, entry := fixture(t, "bad-ping", nil)
	changes := make(chan struct{}, 1)
	r := newRuntimeWithHealth(context.Background(), entry, func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	}, 20*time.Millisecond)
	t.Cleanup(func() { stopRuntime(r) })
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for r.snapshot().State != Restarting {
		select {
		case <-changes:
		case <-timer.C:
			t.Fatal("health failure was not detected", r.snapshot())
		}
	}
	// A failing neighbor must not prevent a healthy extension from executing.
	exe, _ := os.Executable()
	good := entry.Manifest
	good.ID = "good"
	good.Name = "Healthy"
	good.Command = []string{exe, "-test.run=TestPluginHelper", "--", "ok"}
	dir := filepath.Join(m.Directory, "good")
	os.Mkdir(dir, 0700)
	raw, _ := json.Marshal(good)
	os.WriteFile(filepath.Join(dir, "plugin.json"), raw, 0600)
	healthy, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Refresh(map[string]Preference{"demo": {Enabled: true, Fingerprint: entry.Fingerprint}, "good": {Enabled: true, Fingerprint: healthy.Fingerprint}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		ready := false
		badReady := false
		for _, s := range m.Snapshot() {
			if s.Entry.Manifest.ID == "good" && s.State == Ready {
				ready = true
			}
			if s.Entry.Manifest.ID == "demo" && s.State == Ready {
				badReady = true
			}
		}
		if ready && badReady {
			break
		}
		select {
		case <-m.Changes:
		case <-deadline.C:
			t.Fatal("healthy neighbor failed to start")
		}
	}
	if _, err = m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err == nil {
		t.Fatal("neighbor did not crash")
	}
	waitState(t, m, Restarting)
	if _, err = m.Invoke(context.Background(), "good", Invocation{Command: "run"}); err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutTerminatesPluginDescendants(t *testing.T) {
	m, entry := fixture(t, "descendant", nil)
	enable(t, m, entry)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := m.Invoke(ctx, "demo", Invocation{Command: "run"}); err == nil {
		t.Fatal("timeout missing")
	}
	status := waitState(t, m, Restarting)
	fields := strings.Fields(status.Log)
	if len(fields) < 2 || fields[0] != "child-pid:" {
		t.Fatal("descendant was not actually started", status)
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil || pid <= 0 {
		t.Fatal("invalid descendant pid", status.Log)
	}
	if err := m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		state := strings.TrimSpace(string(out))
		if err != nil || state == "" || strings.HasPrefix(state, "Z") {
			break // An unreaped zombie has terminated and cannot execute code.
		}
		if time.Now().After(deadline) {
			t.Fatal("plugin descendant survived process-group termination", pid, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestMissingEnabledPluginIsVisibleAndCanBeDisabled(t *testing.T) {
	m := NewManager(context.Background(), t.TempDir())
	defer m.Close()
	if err := m.Refresh(map[string]Preference{"missing": {Enabled: true, Fingerprint: "old"}}); err != nil {
		t.Fatal(err)
	}
	states := m.Snapshot()
	if len(states) != 1 || states[0].State != Invalid || !strings.Contains(states[0].Error, "missing") {
		t.Fatal(states)
	}
	if err := m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	if len(m.Snapshot()) != 0 {
		t.Fatal("missing preference wasn't cleared")
	}
}

func TestPackageByteLimitAndInvalidFolderRemoval(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.WriteFile(source, []byte("elevenbytes"), 0600)
	if _, err := copyPluginFile(source, filepath.Join(root, "copy"), 0600, 4); err == nil {
		t.Fatal("actual byte limit wasn't enforced")
	}
	m := NewManager(context.Background(), filepath.Join(root, "plugins"))
	defer m.Close()
	invalid := filepath.Join(m.Directory, "Invalid Name")
	if err := os.MkdirAll(invalid, 0700); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	if states := m.Snapshot(); len(states) != 1 || states[0].State != Invalid {
		t.Fatal(states)
	}
	if err := m.Remove("Invalid Name"); err != nil {
		t.Fatal("invalid local package couldn't be removed", err)
	}
}

// Every checked-in example participates in CI's API compatibility gate. New
// examples cannot silently bypass manifest validation or the health contract.
func TestCheckedInPluginExamplesConform(t *testing.T) {
	root, err := filepath.Abs("../../examples")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no installable plugin example")
	}
	for _, entry := range entries {
		t.Run(entry.Manifest.ID, func(t *testing.T) {
			if entry.Error != "" {
				t.Fatal(entry.Error)
			}
			if err := Check(context.Background(), entry.Directory); err != nil {
				t.Fatal("example violates API v1 handshake or health contract:", err)
			}
		})
	}
}
