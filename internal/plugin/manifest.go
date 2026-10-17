// Package plugin implements the versioned SunBook local process extension API.
// macOS additionally requires a restricted OS sandbox for each plugin process.
package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ProtocolVersion = 1
const MaxMessage = 4 * 1024 * 1024
const ReadDocument = "document.read"
const EditDocument = "document.edit"
const Network = "network"

type Command struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type Manifest struct {
	Schema      string    `json:"$schema,omitempty"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	APIVersion  int       `json:"apiVersion"`
	Description string    `json:"description,omitempty"`
	Command     []string  `json:"command"`
	Permissions []string  `json:"permissions"`
	Commands    []Command `json:"commands"`
}
type Preference struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint"`
}
type Entry struct {
	Directory, Fingerprint string
	Manifest               Manifest
	Error                  string
}

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

func label(s string, max int) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= max && strings.IndexFunc(s, unicode.IsControl) < 0
}
func (m Manifest) Allows(permission string) bool {
	for _, p := range m.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}
func ReadManifest(dir string) (Entry, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return Entry{Directory: dir}, err
	}
	dir = absolute
	entry := Entry{Directory: dir}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return entry, err
	}
	entry.Directory = dir
	f, err := openPluginFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return entry, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil {
		return entry, err
	}
	if !utf8.Valid(raw) {
		return entry, errors.New("manifest must be UTF-8")
	}
	if len(raw) > 64*1024 {
		return entry, errors.New("manifest exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&entry.Manifest); err != nil {
		return entry, fmt.Errorf("manifest: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return entry, errors.New("manifest must contain one JSON object")
	}
	m := entry.Manifest
	if !identifier.MatchString(m.ID) || m.ID != filepath.Base(dir) {
		return entry, errors.New("plugin id must match its folder name and use lowercase letters, numbers, dots or hyphens")
	}
	if !label(m.Name, 80) || !version.MatchString(m.Version) || utf8.RuneCountInString(m.Description) > 500 {
		return entry, errors.New("invalid plugin name, version or description")
	}
	if m.APIVersion != ProtocolVersion {
		return entry, fmt.Errorf("unsupported plugin API %d; host supports %d", m.APIVersion, ProtocolVersion)
	}
	if len(m.Command) == 0 || len(m.Command) > 32 {
		return entry, errors.New("command must be an argv array of 1–32 entries")
	}
	for _, arg := range m.Command {
		if arg == "" || len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return entry, errors.New("invalid process argument")
		}
	}
	permissions := map[string]bool{}
	for _, p := range m.Permissions {
		if p != ReadDocument && p != EditDocument && p != Network {
			return entry, fmt.Errorf("unknown permission %q", p)
		}
		if permissions[p] {
			return entry, errors.New("duplicate permission")
		}
		permissions[p] = true
	}
	if permissions[EditDocument] && !permissions[ReadDocument] {
		return entry, errors.New("document.edit also requires document.read")
	}
	if len(m.Commands) == 0 || len(m.Commands) > 64 {
		return entry, errors.New("declare 1–64 commands")
	}
	seen := map[string]bool{}
	for _, c := range m.Commands {
		if !identifier.MatchString(c.ID) || !label(c.Title, 100) || seen[c.ID] {
			return entry, errors.New("invalid or duplicate contributed command")
		}
		seen[c.ID] = true
	}
	entry.Fingerprint, err = fingerprintPackage(dir, raw)
	return entry, err
}
func Discover(dir string) ([]Entry, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Entry
	for _, d := range entries {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		entry, err := ReadManifest(filepath.Join(dir, d.Name()))
		if err != nil {
			entry.Manifest.ID = d.Name()
			entry.Manifest.Name = d.Name()
			entry.Error = err.Error()
		}
		found = append(found, entry)
	}
	return found, nil
}
