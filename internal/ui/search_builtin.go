package ui

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const searchLimit = 2000

// Match path segments independently: * never crosses /, while ** does.
func searchGlob(pattern, name string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	name = filepath.ToSlash(name)
	anchored := strings.HasPrefix(pattern, "/")
	pattern = strings.TrimPrefix(pattern, "/")
	if !anchored && !strings.Contains(pattern, "/") {
		ok, _ := path.Match(pattern, path.Base(name))
		return ok
	}
	var match func([]string, []string) bool
	match = func(p, n []string) bool {
		if len(p) == 0 {
			return len(n) == 0
		}
		if p[0] == "**" {
			if match(p[1:], n) {
				return true
			}
			return len(n) > 0 && match(p, n[1:])
		}
		if len(n) == 0 {
			return false
		}
		ok, _ := path.Match(p[0], n[0])
		return ok && match(p[1:], n[1:])
	}
	return match(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

type searchIgnore struct {
	base, pattern     string
	negate, directory bool
}

func readSearchIgnores(dir string, inherited []searchIgnore) []searchIgnore {
	rules := append([]searchIgnore(nil), inherited...)
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
			rule := searchIgnore{base: dir}
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

func searchIgnored(filename string, directory bool, rules []searchIgnore) bool {
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

func searchPathAllowed(rel string, options searchOptions) bool {
	included := strings.TrimSpace(options.Include) == ""
	for _, g := range strings.Split(options.Include, ",") {
		if g = strings.TrimSpace(g); g != "" && searchGlob(g, rel) {
			included = true
		}
	}
	if !included {
		return false
	}
	for _, g := range strings.Split(options.Exclude, ",") {
		if g = strings.TrimSpace(g); g != "" && searchGlob(strings.TrimSuffix(g, "/"), rel) {
			return false
		}
		// A directory exclusion also excludes all of its children.
		for dir := path.Dir(filepath.ToSlash(rel)); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if g != "" && searchGlob(strings.TrimSuffix(g, "/"), dir) {
				return false
			}
		}
	}
	return true
}

func searchPattern(query string, options searchOptions) (*regexp.Regexp, error) {
	pattern := query
	if !options.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if !options.Case && strings.IndexFunc(query, unicode.IsUpper) < 0 {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

// Like ripgrep's default regex engine, word boundaries use Unicode letters,
// marks, numbers and connector punctuation rather than Go's ASCII-only \b.
func searchWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || unicode.Is(unicode.Pc, r) || r == '\u200c' || r == '\u200d'
}
func searchWholeWord(line string, start, end int) bool {
	before, _ := utf8.DecodeLastRuneInString(line[:start])
	after, _ := utf8.DecodeRuneInString(line[end:])
	return (start == 0 || !searchWordRune(before)) && (end == len(line) || !searchWordRune(after))
}

// The portable engine needs no executable on PATH. Workers only publish immutable
// snapshots, using the same cancellation and stale-result protection as ripgrep.
func (a *App) builtinWorkspaceSearch(ctx context.Context, seq int, root, query string, options searchOptions) {
	pattern, err := searchPattern(query, options)
	if err != nil {
		a.finishSearch(seq, root, nil, err.Error())
		return
	}
	var results []searchHit
	truncated := false
	lastPublish := time.Now()
	var walk func(string, []searchIgnore) error
	walk = func(dir string, inherited []searchIgnore) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		rules := readSearchIgnores(dir, inherited)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Do not follow symlinks or search hidden files (ripgrep defaults).
			if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			filename := filepath.Join(dir, entry.Name())
			if searchIgnored(filename, entry.IsDir(), rules) {
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
			if err := scanSearchFile(ctx, filename, pattern, options.Word, func(hit searchHit) bool {
				results = append(results, hit)
				if len(results)%64 == 0 || time.Since(lastPublish) > 100*time.Millisecond {
					a.publishSearch(seq, root, results, "", false, false)
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
	a.publishSearch(seq, root, results, message, true, truncated)
}

func scanSearchFile(ctx context.Context, filename string, pattern *regexp.Regexp, whole bool, emit func(searchHit) bool) error {
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
