package ui

// The built-in search engine as of v0.3.0, kept verbatim (renamed) as the
// reference for TestBuiltinSearchMatchesReference.

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type refSearchIgnore struct {
	base, pattern     string
	negate, directory bool
}

func refReadSearchIgnores(dir string, inherited []refSearchIgnore) []refSearchIgnore {
	rules := append([]refSearchIgnore(nil), inherited...)
	for _, name := range []string{".gitignore", ".ignore", ".rgignore"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimRight(line, "\r ")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			rule := refSearchIgnore{base: dir}
			if strings.HasPrefix(line, "!") {
				rule.negate = true
				line = line[1:]
			}
			if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
				line = line[1:]
			}
			rule.directory = strings.HasSuffix(line, "/")
			rule.pattern = strings.TrimSuffix(line, "/")
			rules = append(rules, rule)
		}
	}
	return rules
}

func refSearchIgnored(filename string, directory bool, rules []refSearchIgnore) bool {
	ignored := false
	for _, rule := range rules {
		if rule.directory && !directory {
			continue
		}
		rel, err := filepath.Rel(rule.base, filename)
		if err == nil && searchGlob(rule.pattern, rel) {
			ignored = !rule.negate
		}
	}
	return ignored
}

func refBuiltinSearch(ctx context.Context, root, query string, options searchOptions) ([]searchHit, bool, string) {
	pattern, err := searchPattern(query, options)
	if err != nil {
		return nil, false, err.Error()
	}
	var results []searchHit
	truncated := false
	lastPublish := time.Now()
	var walk func(string, []refSearchIgnore) error
	walk = func(dir string, inherited []refSearchIgnore) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		rules := refReadSearchIgnores(dir, inherited)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Do not follow symlinks or search hidden files (ripgrep defaults).
			if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			filename := filepath.Join(dir, entry.Name())
			if refSearchIgnored(filename, entry.IsDir(), rules) {
				continue
			}
			if entry.IsDir() {
				if err := walk(filename, rules); err != nil {
					return err
				}
				continue
			}
			if !entry.Type().IsRegular() {
				continue
			}
			rel, _ := filepath.Rel(root, filename)
			if !searchPathAllowed(filepath.ToSlash(rel), options) {
				continue
			}
			if err := refScanSearchFile(ctx, filename, pattern, options.Word, func(hit searchHit) bool {
				results = append(results, hit)
				if len(results)%64 == 0 || time.Since(lastPublish) > 100*time.Millisecond {
					lastPublish = time.Now()
				}
				truncated = len(results) >= searchLimit
				return !truncated
			}); err != nil {
				return err
			}
			if truncated {
				return fs.SkipAll
			}
		}
		return nil
	}
	err = walk(root, nil)
	message := ""
	if err != nil && !errors.Is(err, fs.SkipAll) && ctx.Err() == nil {
		message = err.Error()
	}
	return results, truncated, message
}

func refScanSearchFile(ctx context.Context, filename string, pattern *regexp.Regexp, whole bool, emit func(searchHit) bool) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	prefix, _ := reader.Peek(8192)
	if strings.ContainsRune(string(prefix), '\x00') {
		return nil
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 4*1024*1024)
	line := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		source := scanner.Text()
		if strings.ContainsRune(source, '\x00') {
			return nil
		}
		if !utf8.ValidString(source) {
			line++
			continue
		}
		for _, match := range pattern.FindAllStringIndex(source, -1) {
			if whole && !searchWholeWord(source, match[0], match[1]) {
				continue
			}
			if !emit(searchHit{Path: filename, Text: source, Line: line, Column: match[0], End: match[1]}) {
				return nil
			}
		}
		line++
	}
	return scanner.Err()
}

// Random workspaces and queries: the parallel, prefiltered engine must return
// exactly the reference's hits in the same order, with the same truncation
// and error. Contents cover case-folding edge cases (KELVIN SIGN, LONG S),
// CRLF, invalid UTF-8, NUL bytes, nested ignore files and path filters.
func TestBuiltinSearchMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 9))
	words := []string{"cat", "CAT", "Cat", "cats", "dog", "k", "K", "K", "s", "S", "ſ", "ks", "KS",
		"中文", "中文字", "straße", "STRASSE", "[x]", "a.b", "needle", "Needle", "foo_bar", " ", "  ", "\t", "é", "é"}
	names := []string{"a.txt", "b.go", "keep.log", "drop.log", "top.txt", "x.md", "deep.txt"}
	dirs := []string{"", "sub", "sub/inner", "build", "a/b", "a/x/b", "vendor"}
	ignores := []string{"*.log", "!keep.log", "build/", "/top.txt", "a/**/deep.txt", "vendor", "inner/", "\\#odd", "*.md"}
	queries := []string{"cat", "CAT", "k", "K", "s", "ks", "Ks", "中文", "straße", "[x]", "a.b", "needle", "e", "zz", "_", "at",
		"^cat", "cat$", "c.t", "(?i)K", "\\bks\\b", "[", "a|k"}
	var hits, truncations, errs int
	defer func() {
		t.Logf("compared %d hits, %d truncated searches, %d errors", hits, truncations, errs)
		if hits == 0 || truncations == 0 || errs == 0 {
			t.Fatal("corpus no longer exercises hits, truncation and errors")
		}
	}()
	for iter := range 40 {
		root := t.TempDir()
		for _, d := range dirs {
			for _, n := range names {
				if rng.IntN(3) == 0 {
					continue
				}
				var sb strings.Builder
				for range rng.IntN(30) {
					for range rng.IntN(8) {
						sb.WriteString(words[rng.IntN(len(words))])
						if rng.IntN(3) == 0 {
							sb.WriteString(" ")
						}
					}
					switch rng.IntN(20) {
					case 0:
						sb.WriteString("\r\n")
					case 1:
						sb.WriteString("\xff\xfe")
					case 2:
						sb.WriteString("\x00")
					}
					sb.WriteString("\n")
				}
				if rng.IntN(8) == 0 {
					sb.WriteString(strings.Repeat("cat k ", 500)) // many hits
				}
				writeOpenFixture(t, filepath.Join(root, d, n), sb.String())
			}
			if rng.IntN(2) == 0 {
				var rules []string
				for range 1 + rng.IntN(3) {
					rules = append(rules, ignores[rng.IntN(len(ignores))])
				}
				writeOpenFixture(t, filepath.Join(root, d, ".gitignore"), strings.Join(rules, "\n")+"\n")
			}
		}
		for range 25 {
			query := queries[rng.IntN(len(queries))]
			options := searchOptions{Regex: rng.IntN(3) == 0, Case: rng.IntN(3) == 0, Word: rng.IntN(3) == 0}
			switch rng.IntN(4) {
			case 0:
				options.Include = "sub/**"
			case 1:
				options.Exclude = "a/,*.go"
			}
			want, wantTrunc, wantErr := refBuiltinSearch(context.Background(), root, query, options)
			got, gotTrunc, err := builtinSearch(context.Background(), root, query, options, func([]searchHit) {})
			gotErr := ""
			if err != nil {
				gotErr = err.Error()
			}
			if gotErr != wantErr || gotTrunc != wantTrunc || !reflect.DeepEqual(got, want) {
				t.Fatalf("iteration %d query %q %+v: got %d hits trunc=%v err=%q, want %d hits trunc=%v err=%q",
					iter, query, options, len(got), gotTrunc, gotErr, len(want), wantTrunc, wantErr)
			}
			hits += len(got)
			if gotTrunc {
				truncations++
			}
			if gotErr != "" {
				errs++
			}
		}
	}
}

func TestContainsFoldASCII(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	alphabet := "abAB c\nC"
	for range 20000 {
		data := make([]byte, rng.IntN(20))
		for i := range data {
			data[i] = alphabet[rng.IntN(len(alphabet))]
		}
		needle := make([]byte, 1+rng.IntN(3))
		for i := range needle {
			needle[i] = lowerASCII(alphabet[rng.IntN(len(alphabet))])
		}
		want := strings.Contains(strings.ToLower(string(data)), string(needle))
		if got := containsFoldASCII(data, needle); got != want {
			t.Fatalf("containsFoldASCII(%q, %q) = %v", data, needle, got)
		}
	}
}

func TestCompiledGlobPartMatchesPathMatch(t *testing.T) {
	patterns := []string{"*", "**", "a", "*.go", "*.", "file*", "f*e", "*a*", "?", "[ab]", "\\*", "[", "a\\", "*.tm[p]", "中*", "*文"}
	names := []string{"", "a", "b", "x.go", "file.go", "file", "fe", ".go", "*", "aa", "中文", "abc", "[", "a\\"}
	for _, p := range patterns {
		g := compileGlobPart(p)
		for _, n := range names {
			want, _ := path.Match(p, n)
			if p == "**" {
				continue // handled by compiledSegments, not as one element
			}
			if got := g.match(n); got != want {
				t.Fatalf("pattern %q name %q: got %v want %v", p, n, got, want)
			}
		}
	}
}
