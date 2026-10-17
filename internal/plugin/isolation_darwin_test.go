package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMacSandboxFileBoundariesAndInheritance(t *testing.T) {
	_, entry := fixture(t, "isolation", nil)
	outside := filepath.Join(t.TempDir(), "host-document")
	if err := os.WriteFile(outside, []byte("private host content"), 0600); err != nil {
		t.Fatal(err)
	}
	// Both quoting and canonical path handling must survive real plugin paths.
	special := filepath.Join(filepath.Dir(entry.Directory), "中文 \"(path)\\")
	if err := os.Mkdir(special, 0700); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(special, "demo")
	if err := os.Rename(entry.Directory, newDir); err != nil {
		t.Fatal(err)
	}
	var err error
	entry, err = ReadManifest(newDir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	for _, command := range []string{"read", "write", "child-read", "symlink-read", "hardlink-read"} {
		t.Run(command, func(t *testing.T) {
			if result := probe(t, p, command, outside); !strings.HasPrefix(result.Message, "denied:") {
				t.Fatal("host file access escaped sandbox", result)
			}
		})
	}
	alias := "/System/Volumes/Data" + outside
	if _, err := os.Stat(alias); err == nil {
		if result := probe(t, p, "read", alias); !strings.HasPrefix(result.Message, "denied:") {
			t.Fatal("Data volume alias bypassed the sandbox", result)
		}
	}
	if result := probe(t, p, "read", filepath.Join(entry.Directory, "plugin.json")); result.Message != "allowed" {
		t.Fatal("plugin cannot read its own package", result)
	}
	if result := probe(t, p, "write", filepath.Join(entry.Directory, "tampered")); !strings.HasPrefix(result.Message, "denied:") {
		t.Fatal("plugin can overwrite its installed package", result)
	}
	if result := probe(t, p, "write", filepath.Join(p.scratch, "allowed")); result.Message != "allowed" {
		t.Fatal("private writable directory unavailable", result)
	}
	neighbor, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer neighbor.stop()
	if result := probe(t, neighbor, "read", filepath.Join(p.scratch, "allowed")); !strings.HasPrefix(result.Message, "denied:") {
		t.Fatal("plugin can read a neighboring process's temporary home", result)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "private host content" {
		t.Fatal("host file was modified", string(data), err)
	}
	if err := p.ping(context.Background()); err != nil {
		t.Fatal("denied operations broke plugin availability", err)
	}
}

func TestMacSandboxNetworkRequiresPermission(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	for _, allowed := range []bool{false, true} {
		name := "blocked"
		var permissions []string
		if allowed {
			name, permissions = "permitted", []string{Network}
		}
		t.Run(name, func(t *testing.T) {
			_, entry := fixture(t, "isolation", permissions)
			p, err := startProcess(entry)
			if err != nil {
				t.Fatal(err)
			}
			defer p.stop()
			for _, address := range []struct{ command, path string }{{"network", listener.Addr().String()}, {"udp", udp.LocalAddr().String()}} {
				result := probe(t, p, address.command, address.path)
				if (result.Message == "allowed") != allowed {
					t.Fatal("network permission not enforced", address.command, result)
				}
				if !allowed && !strings.Contains(result.Message, "operation not permitted") {
					t.Fatal("expected OS denial rather than unreachable network", address.command, result)
				}
			}
			if allowed {
				if result := probe(t, p, "lookup", "localhost"); result.Message != "allowed" {
					t.Fatal("permitted plugin cannot resolve hostnames", result)
				}
			}
		})
	}
}

func TestMacSandboxCannotInspectHostProcessCredentials(t *testing.T) {
	const credential = "sunbook-fake-credential-for-sandbox-test"
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Apple's protected system binaries hide their environment even outside a
	// sandbox. Use our own executable so the positive control proves the guard.
	host := exec.Command(executable, "-test.run=TestPluginHelper", "--", "inspection-target")
	host.Env = []string{"SUNBOOK_PLUGIN_API=1", "SUNBOOK_HOST_SECRET=" + credential}
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { host.Process.Kill(); host.Wait() }()
	pid := strconv.Itoa(host.Process.Pid)
	control, err := exec.Command("/bin/ps", "eww", "-p", pid).Output()
	if err != nil || !strings.Contains(string(control), credential) {
		t.Fatal("unsandboxed process inspection control did not work", err)
	}
	_, entry := fixture(t, "isolation", nil)
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	result := probe(t, p, "inspect-process", pid)
	if strings.Contains(result.Body, credential) {
		t.Fatal("plugin escaped the environment filter through host process inspection")
	}
	if !strings.HasPrefix(result.Message, "denied:") {
		t.Fatal("host process inspection was not denied", result)
	}
}

func TestMacSandboxPermitsNativeTLSCertificateValidation(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	_, entry := fixture(t, "isolation", []string{Network})
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	// A native TLS client must reach trustd and reject an untrusted CA normally,
	// rather than fail with an OSStatus sandbox error or skip verification.
	result := probe(t, p, "certificate", base64.StdEncoding.EncodeToString(server.Certificate().Raw))
	if result.Message != "allowed" {
		t.Fatal("native certificate validation is broken inside network-enabled sandbox", result)
	}
}

func TestMacSandboxUnavailableFailsClosed(t *testing.T) {
	_, entry := fixture(t, "isolation", nil)
	cmd, err := macSandboxCommand(entry, entry.Manifest.Command[0], t.TempDir(), filepath.Join(t.TempDir(), "missing-sandbox"))
	if err == nil || cmd != nil || !strings.Contains(err.Error(), "refusing unsandboxed") {
		t.Fatal("missing backend allowed unrestricted fallback", cmd, err)
	}
}

func TestMacSandboxSupportsExternalPythonVirtualenv(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("Python is required by the checked-in example", err)
	}
	venv := filepath.Join(t.TempDir(), "runtime")
	if output, err := exec.Command(python, "-m", "venv", "--without-pip", venv).CombinedOutput(); err != nil {
		t.Fatal("create isolated test runtime:", err, string(output))
	}
	venv, err = filepath.EvalSymlinks(venv)
	if err != nil {
		t.Fatal(err)
	}
	libs, err := filepath.Glob(filepath.Join(venv, "lib", "python*", "site-packages"))
	if err != nil || len(libs) != 1 {
		t.Fatal("virtualenv package path", libs, err)
	}
	if err := os.WriteFile(filepath.Join(libs[0], "sunbook_test_module.py"), []byte("VALUE = 'venv dependency loaded'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, entry := fixture(t, "isolation", nil)
	manifest := entry.Manifest
	manifest.Command = []string{filepath.Join(venv, "bin", "python3"), "main.py"}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(entry.Directory, "plugin.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	sdk, err := os.ReadFile("../../sdk/python/sunbook_plugin.py")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Directory, "sunbook_plugin.py"), sdk, 0600); err != nil {
		t.Fatal(err)
	}
	source := "import sys\nfrom sunbook_plugin import Plugin\nfrom sunbook_test_module import VALUE\nPlugin({'run': lambda doc: {'body': sys.prefix, 'message': VALUE}}).run()\n"
	if err := os.WriteFile(filepath.Join(entry.Directory, "main.py"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err = ReadManifest(entry.Directory)
	if err != nil {
		t.Fatal(err)
	}
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var hello map[string]any
	if err := p.call(ctx, "initialize", map[string]int{"protocolVersion": 1}, &hello); err != nil {
		t.Fatal(err, p.log.String())
	}
	var result Result
	if err := p.call(ctx, "execute", Invocation{Command: "run"}, &result); err != nil {
		t.Fatal(err, p.log.String())
	}
	if result.Body != venv || result.Message != "venv dependency loaded" {
		t.Fatal("launch discarded virtualenv or failed to load its packages", result)
	}
}

func TestMacSandboxBlocksHostUnixSockets(t *testing.T) {
	_, entry := fixture(t, "isolation", nil)
	// A short path avoids the macOS Unix socket name length limit.
	dir, err := os.MkdirTemp("/tmp", "sb-sock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "socket")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	p, err := startProcess(entry)
	if err != nil {
		t.Fatal(err)
	}
	defer p.stop()
	result := probe(t, p, "unix", path)
	if !strings.Contains(result.Message, "operation not permitted") {
		t.Fatal("host local service accessible without network permission", result)
	}
}
