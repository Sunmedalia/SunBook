package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const DefaultCatalogURL = "https://raw.githubusercontent.com/Sunmedalia/SunBook-plugins/main/index.json"
const GHProxyPrefix = "https://gh-proxy.com/"
const maxDownload = 64 * 1024 * 1024

type Catalog struct {
	SchemaVersion int           `json:"schemaVersion"`
	Plugins       []StorePlugin `json:"plugins"`
}
type StorePlugin struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Repository  string `json:"repository"`
	Runtime     string `json:"runtime"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
}

// Store downloads data only. Installation never starts a plugin or a shell.
type Store struct {
	Client     *http.Client
	UseGHProxy bool
}

func githubURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	switch u.Host {
	case "github.com", "raw.githubusercontent.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	}
	return false
}
func (p StorePlugin) validate() error {
	digest, err := hex.DecodeString(p.SHA256)
	if !identifier.MatchString(p.ID) || !label(p.Name, 80) || !version.MatchString(p.Version) || !label(p.Description, 500) || !label(p.Author, 100) || !label(p.Runtime, 200) || !githubURL(p.Repository) || !githubURL(p.URL) || err != nil || len(digest) != 32 {
		return errors.New("invalid store plugin metadata or GitHub URL")
	}
	return nil
}
func (s Store) download(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if !githubURL(raw) {
		return nil, errors.New("store sources must use HTTPS GitHub URLs")
	}
	if s.UseGHProxy {
		raw = GHProxyPrefix + raw
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SunBook-Plugin-Store/1")
	client := http.Client{Timeout: 30 * time.Second}
	if s.Client != nil {
		client = *s.Client
	}
	// Validate every redirect, including release asset redirects.
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many store redirects")
		}
		target := req.URL.String()
		if s.UseGHProxy {
			if githubURL(target) {
				// Keep GitHub redirects on the selected proxy route.
				req.URL, _ = url.Parse(GHProxyPrefix + target)
				req.Host = ""
				return nil
			}
			if strings.HasPrefix(target, GHProxyPrefix) && githubURL(strings.TrimPrefix(target, GHProxyPrefix)) {
				return nil
			}
		} else if githubURL(target) {
			return nil
		}
		return errors.New("unsafe store redirect")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub download: HTTP %d", resp.StatusCode)
	}
	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(rawBytes)) > limit {
		return nil, errors.New("store download exceeds size limit")
	}
	return rawBytes, nil
}
func (s Store) Fetch(ctx context.Context, source string) (Catalog, error) {
	data, err := s.download(ctx, source, 1024*1024)
	if err != nil {
		return Catalog{}, err
	}
	var catalog Catalog
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	if decoder.Decode(new(any)) != io.EOF || catalog.SchemaVersion != 1 || len(catalog.Plugins) > 500 {
		return Catalog{}, errors.New("unsupported or invalid plugin catalog")
	}
	seen := map[string]bool{}
	for _, p := range catalog.Plugins {
		if err = p.validate(); err != nil {
			return Catalog{}, err
		}
		if seen[p.ID] {
			return Catalog{}, errors.New("duplicate catalog plugin")
		}
		seen[p.ID] = true
	}
	return catalog, nil
}
func (s Store) Install(ctx context.Context, manager *Manager, p StorePlugin) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	data, err := s.download(ctx, p.URL, maxDownload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), p.SHA256) {
		return "", errors.New("plugin download SHA-256 mismatch")
	}
	root, err := os.MkdirTemp("", "sunbook-download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(root)
	dir := filepath.Join(root, p.ID)
	if err = extractPluginZip(ctx, data, dir, p.ID); err != nil {
		return "", err
	}
	entry, err := ReadManifest(dir)
	if err != nil {
		return "", err
	}
	if entry.Manifest.ID != p.ID || entry.Manifest.Version != p.Version || entry.Manifest.Name != p.Name {
		return "", errors.New("downloaded plugin does not match catalog identity/version")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return manager.Install(dir)
}

// Packages have exactly one top-level directory named after the plugin ID.
func extractPluginZip(ctx context.Context, data []byte, dir, id string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if len(reader.File) > 2000 {
		return errors.New("plugin archive exceeds 2000 entries")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	seen := map[string]bool{}
	files := 0
	var size int64
	for _, f := range reader.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(f.Name, "/")
		if strings.ContainsAny(name, "\\\x00") || path.Clean(name) != name || !strings.HasPrefix(name, id+"/") && name != id {
			return errors.New("unsafe plugin archive path")
		}
		if seen[name] {
			return errors.New("duplicate plugin archive path")
		}
		seen[name] = true
		if f.Mode()&os.ModeSymlink != 0 || !f.FileInfo().IsDir() && !f.Mode().IsRegular() {
			return errors.New("plugin archive contains a special file")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(name, id), "/")
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return err
			}
			continue
		}
		files++
		if files > 1000 || f.UncompressedSize64 > uint64(maxDownload-size) {
			return errors.New("plugin archive exceeds 1000 files or 64 MiB")
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		in, e := f.Open()
		if e != nil {
			return e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return e
		}
		n, e := io.Copy(out, io.LimitReader(in, maxDownload-size+1))
		in.Close()
		if e == nil {
			e = out.Chmod(0600 | f.Mode().Perm()&0111)
		}
		closeErr := out.Close()
		size += n
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if size > maxDownload {
			return errors.New("plugin archive exceeds 64 MiB")
		}
	}
	return nil
}
