package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type storeTransport func(*http.Request) (*http.Response, error)

func (f storeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureStore(t *testing.T) (Store, StorePlugin, []byte) {
	t.Helper()
	data, err := os.ReadFile("../../marketplace/packages/text-tools-1.0.0.zip")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	p := StorePlugin{ID: "text-tools", Name: "Text Tools", Version: "1.0.0", Description: "Text tools", Author: "Sunmedalia", Repository: "https://github.com/Sunmedalia/SunBook-plugins", Runtime: "Python 3.9+", URL: "https://raw.githubusercontent.com/Sunmedalia/SunBook-plugins/main/packages/text-tools-1.0.0.zip", SHA256: hex.EncodeToString(digest[:])}
	catalog, _ := json.Marshal(Catalog{SchemaVersion: 1, Plugins: []StorePlugin{p}})
	store := Store{Client: &http.Client{Transport: storeTransport(func(r *http.Request) (*http.Response, error) {
		body := data
		if strings.HasSuffix(r.URL.Path, "index.json") {
			body = catalog
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})}}
	return store, p, data
}
func TestStoreInstallAndCatalog(t *testing.T) {
	store, p, _ := fixtureStore(t)
	catalog, err := store.Fetch(context.Background(), DefaultCatalogURL)
	if err != nil || len(catalog.Plugins) != 1 {
		t.Fatal(catalog, err)
	}
	m := NewManager(context.Background(), filepath.Join(t.TempDir(), "plugins"))
	defer m.Close()
	id, err := store.Install(context.Background(), m, p)
	if err != nil || id != p.ID {
		t.Fatal(id, err)
	}
	if err = m.Refresh(nil); err != nil {
		t.Fatal(err)
	}
	snapshot := m.Snapshot()
	if len(snapshot) != 1 || snapshot[0].State != Disabled {
		t.Fatal(snapshot)
	}
	if _, err = store.Install(context.Background(), m, p); err == nil {
		t.Fatal("overwrote installed package")
	}
}
func TestStoreRejectsTamperedAndMismatchedPackages(t *testing.T) {
	for _, name := range []string{"hash", "version", "name", "cancel"} {
		t.Run(name, func(t *testing.T) {
			store, p, _ := fixtureStore(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "hash":
				p.SHA256 = strings.Repeat("0", 64)
			case "version":
				p.Version = "9.0.0"
			case "name":
				p.Name = "Other plugin"
			case "cancel":
				cancel()
			}
			m := NewManager(context.Background(), filepath.Join(t.TempDir(), "plugins"))
			defer m.Close()
			if _, err := store.Install(ctx, m, p); err == nil {
				t.Fatal("accepted invalid installation")
			}
			entries, _ := os.ReadDir(m.Directory)
			if len(entries) != 0 {
				t.Fatal("partial installation left behind", entries)
			}
		})
	}
}
func TestStoreZipRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"../escape", "text-tools/../../escape", "/text-tools/main.py", "text-tools\\main.py", "other/main.py", "text-tools/a/../main.py", "text-tools/link"} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			h := &zip.FileHeader{Name: name}
			if strings.HasSuffix(name, "link") {
				h.SetMode(os.ModeSymlink | 0777)
			}
			f, _ := w.CreateHeader(h)
			f.Write([]byte("outside"))
			w.Close()
			if err := extractPluginZip(context.Background(), b.Bytes(), filepath.Join(t.TempDir(), "text-tools"), "text-tools"); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for i := 0; i < 2; i++ {
		f, _ := w.Create("text-tools/main.py")
		f.Write([]byte("test"))
	}
	w.Close()
	if err := extractPluginZip(context.Background(), b.Bytes(), filepath.Join(t.TempDir(), "text-tools"), "text-tools"); err == nil {
		t.Fatal("accepted duplicate files")
	}
}
func TestStoreDownloadErrors(t *testing.T) {
	store, _, _ := fixtureStore(t)
	for _, raw := range []string{"http://github.com/file", "https://evil.example/file", "https://github.com:443/file", "https://user@github.com/file"} {
		if _, err := store.Fetch(context.Background(), raw); err == nil {
			t.Fatal("accepted URL", raw)
		}
	}
	for _, status := range []int{404, 429, 500} {
		store.Client.Transport = storeTransport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("error")), Header: make(http.Header)}, nil
		})
		if _, err := store.Fetch(context.Background(), DefaultCatalogURL); err == nil {
			t.Fatal("accepted HTTP error", status)
		}
	}
	store.Client.Transport = storeTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://evil.example/payload"}}}, nil
	})
	if _, err := store.Fetch(context.Background(), DefaultCatalogURL); err == nil {
		t.Fatal("accepted unsafe redirect")
	}
}
func TestPublishedCatalog(t *testing.T) {
	data, err := os.ReadFile("../../marketplace/index.json")
	if err != nil {
		t.Fatal(err)
	}
	var c Catalog
	if err = json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	for _, p := range c.Plugins {
		if err = p.validate(); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(filepath.Join("../../marketplace/packages", filepath.Base(p.URL)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != p.SHA256 {
			t.Fatal("published hash mismatch", p.ID)
		}
		dir := filepath.Join(t.TempDir(), p.ID)
		if err = extractPluginZip(context.Background(), data, dir, p.ID); err != nil {
			t.Fatal(err)
		}
		entry, err := ReadManifest(dir)
		if err != nil || entry.Manifest.Version != p.Version {
			t.Fatal(entry, err)
		}
		err = filepath.WalkDir(dir, func(file string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, file)
			actual, e := os.ReadFile(file)
			if e != nil {
				return e
			}
			expected, e := os.ReadFile(filepath.Join("../../examples", p.ID, rel))
			if e != nil {
				return e
			}
			if !bytes.Equal(actual, expected) {
				t.Fatalf("published file %s differs from example; run scripts/package_plugins.py", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestStoreProxyCatalogAndInstall(t *testing.T) {
	store, p, data := fixtureStore(t)
	catalog, _ := json.Marshal(Catalog{SchemaVersion: 1, Plugins: []StorePlugin{p}})
	var requests []string
	store.UseGHProxy = true
	store.Client.Transport = storeTransport(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.URL.String())
		if !strings.HasPrefix(r.URL.String(), GHProxyPrefix) {
			t.Fatalf("proxy bypass: %s", r.URL)
		}
		body := data
		if strings.HasSuffix(r.URL.Path, "index.json") {
			body = catalog
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	if _, err := store.Fetch(context.Background(), DefaultCatalogURL); err != nil {
		t.Fatal(err)
	}
	m := NewManager(context.Background(), filepath.Join(t.TempDir(), "plugins"))
	defer m.Close()
	if _, err := store.Install(context.Background(), m, p); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0] != GHProxyPrefix+DefaultCatalogURL || requests[1] != GHProxyPrefix+p.URL {
		t.Fatal(requests)
	}
	p.SHA256 = strings.Repeat("0", 64)
	if _, err := store.Install(context.Background(), m, p); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatal("proxy bypassed hash verification", err)
	}
}
func TestStoreProxyRedirects(t *testing.T) {
	for _, target := range []string{
		"https://raw.githubusercontent.com/owner/repo/main/index.json",
		GHProxyPrefix + "https://github.com/owner/repo/raw/main/index.json",
		"https://evil.example/file",
		"https://gh-proxy.com/https://evil.example/file",
		"http://gh-proxy.com/https://github.com/owner/repo/file",
		"https://gh-proxy.com.evil.example/https://github.com/owner/repo/file",
	} {
		t.Run(target, func(t *testing.T) {
			store, _, _ := fixtureStore(t)
			store.UseGHProxy = true
			calls := 0
			store.Client.Transport = storeTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{target}}, Request: r}, nil
				}
				expected := target
				if githubURL(expected) {
					expected = GHProxyPrefix + expected
				}
				if r.URL.String() != expected {
					t.Fatal("redirect bypassed proxy", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"schemaVersion":1,"plugins":[]}`)), Header: make(http.Header), Request: r}, nil
			})
			_, err := store.Fetch(context.Background(), DefaultCatalogURL)
			valid := githubURL(target) || strings.HasPrefix(target, GHProxyPrefix) && githubURL(strings.TrimPrefix(target, GHProxyPrefix))
			if valid && err != nil {
				t.Fatal(err)
			}
			if !valid && (err == nil || calls != 1) {
				t.Fatal("accepted unsafe redirect", err, calls)
			}
		})
	}
}
