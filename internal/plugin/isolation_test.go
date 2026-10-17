package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func probe(t *testing.T, p *process, command, path string) Result {
	t.Helper()
	var result Result
	if err := p.call(context.Background(), "execute", Invocation{Command: command, Document: &Document{Path: path}}, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestManifestSpecialFilesAreRejectedWithoutBlocking(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "demo")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(dir, "plugin.json")
			var err error
			if kind == "symlink" {
				err = os.Symlink("/etc/passwd", manifest)
			} else {
				err = syscall.Mkfifo(manifest, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { _, err := ReadManifest(dir); done <- err }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("special manifest was accepted")
				}
			case <-time.After(time.Second):
				t.Fatal("plugin discovery blocked on a special file")
			}
		})
	}
}

func TestInstallPreservesExecutableApprovalWithPrivateUmask(t *testing.T) {
	_, entry := fixture(t, "ok", nil)
	executable := filepath.Join(entry.Directory, "local-tool")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0755); err != nil {
		t.Fatal(err)
	}
	// Umask is process-wide; this package deliberately has no parallel tests.
	previous := syscall.Umask(0077)
	defer syscall.Umask(previous)
	m := NewManager(context.Background(), filepath.Join(t.TempDir(), "plugins"))
	defer m.Close()
	if _, err := m.Install(entry.Directory); err != nil {
		t.Fatal("private umask broke plugin installation", err)
	}
	info, err := os.Stat(filepath.Join(m.Directory, "demo", "local-tool"))
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("executable mode changed during installation", info, err)
	}
}

func TestPluginEnvironmentAndTemporaryHome(t *testing.T) {
	t.Setenv("SUNBOOK_TEST_SECRET", "do-not-pass-this-token")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "do-not-pass-this-token")
	t.Setenv("NODE_OPTIONS", "--require=host-loader")
	t.Setenv("PYTHONPATH", "/host/private-modules")
	_, entry := fixture(t, "isolation", nil)
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.stop)
	result := probe(t, p, "environment", "")
	for _, secret := range []string{"do-not-pass-this-token", "NODE_OPTIONS=", "PYTHONPATH=", "AWS_SECRET_ACCESS_KEY=", "SUNBOOK_TEST_SECRET="} {
		if strings.Contains(result.Body, secret) {
			t.Fatalf("host environment leaked %q", secret)
		}
	}
	if result.Message != p.scratch || result.Message == os.Getenv("HOME") {
		t.Fatal("plugin did not receive a private home", result.Message)
	}
	if result := probe(t, p, "write", filepath.Join(p.scratch, "working-file")); result.Message != "allowed" {
		t.Fatal("private temporary files unusable", result)
	}
	if err := p.ping(context.Background()); err != nil {
		t.Fatal("process failed after normal probes", err)
	}
	p.stop()
	if _, err := os.Stat(p.scratch); !os.IsNotExist(err) {
		t.Fatal("temporary plugin home was not removed", err)
	}
}

func TestCodeChangesInvalidateApprovalAndPreventStaleLaunch(t *testing.T) {
	m, entry := fixture(t, "ok", nil)
	path := filepath.Join(entry.Directory, "implementation.txt")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := ReadManifest(entry.Directory)
	if err != nil {
		t.Fatal(err)
	}
	enable(t, m, entry)
	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if p, err := startProcess(entry); err == nil {
		p.stop()
		t.Fatal("code change ran with obsolete approval")
	}
	if err := m.Refresh(map[string]Preference{"demo": {Enabled: true, Fingerprint: entry.Fingerprint}}); err != nil {
		t.Fatal(err)
	}
	if status := m.Snapshot()[0]; status.State != Review || status.Entry.Fingerprint == entry.Fingerprint {
		t.Fatal("code change did not stop the old runtime and require review", status)
	}
	if _, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err == nil {
		t.Fatal("unreviewed code is callable")
	}
	updated, err := ReadManifest(entry.Directory)
	if err != nil {
		t.Fatal(err)
	}
	enable(t, m, updated)
	if _, err := m.Invoke(context.Background(), "demo", Invocation{Command: "run"}); err != nil {
		t.Fatal("reapproval did not restore availability", err)
	}
}
