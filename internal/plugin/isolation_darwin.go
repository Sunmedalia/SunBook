package plugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// sandbox-exec is deprecated by Apple. Until a signed helper/App Sandbox
// distribution exists, require this backend and fail closed if it stops working.
// A plugin and its descendants inherit the Seatbelt restrictions at exec time.
const sandboxExecutable = "/usr/bin/sandbox-exec"

func IsolationDescription(m Manifest) string {
	network := "network blocked"
	if m.Allows(Network) {
		network = "network allowed"
	}
	return "macOS sandbox; " + network
}

func FileAccessDescription() string {
	return "Files: plugin/runtime reads; private temporary writes"
}

func isolatedCommand(entry Entry, executable, scratch string) (*exec.Cmd, error) {
	return macSandboxCommand(entry, executable, scratch, sandboxExecutable)
}

func macSandboxCommand(entry Entry, executable, scratch, backend string) (*exec.Cmd, error) {
	if _, err := exec.LookPath(backend); err != nil {
		return nil, fmt.Errorf("macOS plugin sandbox unavailable; refusing unsandboxed execution: %w", err)
	}
	profile := sandboxProfile(entry, executable, scratch)
	args := append([]string{"-p", profile, executable}, entry.Manifest.Command[1:]...)
	return exec.Command(backend, args...), nil
}

func sandboxProfile(entry Entry, executable, scratch string) string {
	var p strings.Builder
	p.WriteString("(version 1)\n(deny default)\n(allow process-exec)\n(allow process-fork)\n")
	// Runtime hardware/OS discovery is needed by Go/Python/Node. Do not grant
	// kern.procargs or process inspection that could expose the editor's secrets.
	p.WriteString("(allow sysctl-read (sysctl-name-regex #\"^hw[.]\" #\"^kern[.](os|version|boottime|hostname|argmax|maxfiles|maxproc)\") (sysctl-name \"vm.loadavg\"))\n")
	// Metadata permits the runtime to inspect parent directories, but grants no
	// document contents. The root literal is needed by dyld on current macOS.
	p.WriteString("(allow file-read-metadata)\n(allow file-read* (literal \"/\")")
	// Never allow all of /System: /System/Volumes/Data aliases user files.
	for _, dir := range append([]string{"/System/Library", "/System/Volumes/Preboot/Cryptexes/OS", "/usr", "/bin", "/sbin", "/Library/Apple", "/Library/Frameworks", "/Library/Developer", "/Applications/Xcode.app/Contents/Developer", "/opt/homebrew", "/private/etc/ssl", "/private/var/db/timezone", entry.Directory, scratch}, pythonRuntimePaths(executable)...) {
		fmt.Fprintf(&p, " (subpath %s)", strconv.Quote(dir))
	}
	fmt.Fprintf(&p, " (literal %s)", strconv.Quote(executable))
	if resolved, err := filepath.EvalSymlinks(executable); err == nil && resolved != executable {
		fmt.Fprintf(&p, " (literal %s)", strconv.Quote(resolved))
	}
	for _, file := range []string{"/dev/null", "/dev/random", "/dev/urandom", "/private/etc/localtime"} {
		fmt.Fprintf(&p, " (literal %s)", strconv.Quote(file))
	}
	p.WriteString(")\n")
	fmt.Fprintf(&p, "(allow file-write* (subpath %s) (literal \"/dev/null\"))\n", strconv.Quote(scratch))
	if entry.Manifest.Allows(Network) {
		// Native TLS clients use trustd for certificate validation. This grants
		// trust evaluation, not arbitrary Keychain/SecurityServer access.
		p.WriteString("(allow network*)\n(allow system-socket)\n(allow mach-lookup (global-name \"com.apple.system.config.network_change\") (global-name \"com.apple.SystemConfiguration.configd\") (global-name \"com.apple.mDNSResponder\") (global-name \"com.apple.trustd.agent\") (global-name \"com.apple.trustd\"))\n")
		p.WriteString("(allow file-read* (literal \"/private/etc/resolv.conf\") (literal \"/private/var/run/resolv.conf\") (literal \"/private/etc/hosts\"))\n")
	}
	return p.String()
}

var pythonLibrary = regexp.MustCompile(`^(python[0-9]+\.[0-9]+|libpython[0-9]+\.[0-9]+.*\.dylib)$`)

// Support user-installed CPython/pyenv/uv runtimes without making the user's
// home or the interpreter's entire parent tree readable. No discovery code is
// executed outside the sandbox.
func pythonRuntimePaths(executable string) []string {
	executables := []string{executable}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil && resolved != executable {
		executables = append(executables, resolved)
	}
	var paths []string
	for _, exe := range executables {
		bin := filepath.Dir(exe)
		if filepath.Base(bin) != "bin" || !strings.HasPrefix(filepath.Base(exe), "python") {
			continue
		}
		root := filepath.Dir(bin)
		config := filepath.Join(root, "pyvenv.cfg")
		if canonical, err := filepath.EvalSymlinks(config); err == nil {
			paths = append(paths, canonical)
		}
		lib := filepath.Join(root, "lib")
		entries, _ := os.ReadDir(lib)
		for _, e := range entries {
			if pythonLibrary.MatchString(e.Name()) {
				path := filepath.Join(lib, e.Name())
				if canonical, err := filepath.EvalSymlinks(path); err == nil {
					paths = append(paths, canonical)
				}
			}
		}
	}
	return paths
}
