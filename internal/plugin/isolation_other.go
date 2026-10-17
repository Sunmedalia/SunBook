//go:build !darwin

package plugin

import "os/exec"

func IsolationDescription(m Manifest) string {
	return "Process only; network unrestricted"
}

func FileAccessDescription() string {
	return "Files: current user access; no OS filesystem sandbox"
}

func isolatedCommand(entry Entry, executable, scratch string) (*exec.Cmd, error) {
	return exec.Command(executable, entry.Manifest.Command[1:]...), nil
}
