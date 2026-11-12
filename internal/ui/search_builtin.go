package ui

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
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
	return globSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}
func globSegments(p, n []string) bool {
	if len(p) == 0 {
		return len(n) == 0
	}
	if p[0] == "**" {
		if globSegments(p[1:], n) {
			return true
		}
		return len(n) > 0 && globSegments(p, n[1:])
	}
	if len(n) == 0 {
		return false
	}
	ok, _ := path.Match(p[0], n[0])
	return ok && globSegments(p[1:], n[1:])
}

type searchIgnore struct {
	base, pattern     string
	negate, directory bool
	// prefix is base plus a separator, so a path below base yields its
	// relative form without filepath.Rel. A pattern without a slash only
	// ever tests the entry's own name; the others are split once here.
	prefix   string
	simple   bool
	name     globPart
	segments []globPart
}

// globPart is one path.Match pattern for a single path element, with the
// common gitignore shapes reduced to string comparisons. An element never
// contains '/', so "*" spans all of it and these forms equal path.Match.
type globPart struct {
	kind    byte // '=' literal, '>' "*suffix", '<' "prefix*", '*' any, 'm' path.Match, '!' "**"
	pattern string
	literal string
}

func compileGlobPart(pattern string) globPart {
	g := globPart{kind: 'm', pattern: pattern}
	meta := func(s string) bool { return strings.ContainsAny(s, "*?[\\") }
	switch {
	case pattern == "**":
		g.kind = '!'
	case pattern == "*":
		g.kind = '*'
	case !meta(pattern):
		g.kind, g.literal = '=', pattern
	case strings.HasPrefix(pattern, "*") && !meta(pattern[1:]):
		g.kind, g.literal = '>', pattern[1:]
	case strings.HasSuffix(pattern, "*") && !meta(pattern[:len(pattern)-1]):
		g.kind, g.literal = '<', pattern[:len(pattern)-1]
	}
	return g
}
func (g globPart) match(element string) bool {
	switch g.kind {
	case '=':
		return element == g.literal
	case '>':
		return strings.HasSuffix(element, g.literal)
	case '<':
		return strings.HasPrefix(element, g.literal)
	case '*':
		return true
	}
	ok, _ := path.Match(g.pattern, element)
	return ok
}
func compiledSegments(p []globPart, n []string) bool {
	if len(p) == 0 {
		return len(n) == 0
	}
	if p[0].kind == '!' {
		if compiledSegments(p[1:], n) {
			return true
		}
		return len(n) > 0 && compiledSegments(p, n[1:])
	}
	return len(n) > 0 && p[0].match(n[0]) && compiledSegments(p[1:], n[1:])
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
			rule := searchIgnore{base: dir, prefix: dir + string(filepath.Separator)}
			if strings.HasPrefix(line, "!") {
				rule.negate = true
				line = line[1:]
			}
			if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
				line = line[1:]
			}
			rule.directory = strings.HasSuffix(line, "/")
			rule.pattern = strings.TrimSuffix(line, "/")
			// The same normalization searchGlob applies on every call.
			pattern := strings.TrimPrefix(filepath.ToSlash(rule.pattern), "./")
			anchored := strings.HasPrefix(pattern, "/")
			pattern = strings.TrimPrefix(pattern, "/")
			rule.simple = !anchored && !strings.Contains(pattern, "/")
			if rule.simple {
				rule.name = compileGlobPart(pattern)
			} else {
				for _, part := range strings.Split(pattern, "/") {
					rule.segments = append(rule.segments, compileGlobPart(part))
				}
			}
			rules = append(rules, rule)
		}
	}
	return rules
}

// searchIgnored applies rules to filename, whose last element is name.
func searchIgnored(filename, name string, directory bool, rules []searchIgnore) bool {
	ignored := false
	// Rules from one ignore file share a base, so split the path once per base.
	base, segments, relOK := "", []string(nil), false
	for _, rule := range rules {
		if rule.directory && !directory || rule.negate != ignored {
			// A rule can only change the answer when it would flip it.
			continue
		}
		var ok bool
		if rule.simple {
			ok = rule.name.match(name)
		} else {
			if segments == nil || rule.base != base {
				base = rule.base
				rel, found := strings.CutPrefix(filename, rule.prefix)
				relOK = true
				if !found {
					var err error
					rel, err = filepath.Rel(rule.base, filename)
					relOK = err == nil
				}
				segments = strings.Split(filepath.ToSlash(rel), "/")
			}
			ok = relOK && compiledSegments(rule.segments, segments)
		}
		if ok {
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
	results, truncated, err := builtinSearch(ctx, root, query, options, func(partial []searchHit) {
		a.publishSearch(seq, root, partial, "", false, false)
	})
	message := ""
	if err != nil && ctx.Err() == nil {
		message = err.Error()
	}
	a.publishSearch(seq, root, results, message, true, truncated)
}

type searchJob struct {
	filename string
	err      error // a directory that could not be read, reported in walk order
	done     chan searchFileResult
}
type searchFileResult struct {
	hits []searchHit
	err  error
}

// builtinSearch walks root depth-first in directory order, scanning files on
// several workers. Results are committed strictly in walk order, so the hits,
// the truncation point and the first error match a sequential scan.
func builtinSearch(ctx context.Context, root, query string, options searchOptions, publish func([]searchHit)) ([]searchHit, bool, error) {
	pattern, err := searchPattern(query, options)
	if err != nil {
		return nil, false, err
	}
	prefilter := newSearchPrefilter(query, options, pattern)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := min(runtime.GOMAXPROCS(0), 8)
	jobs := make(chan searchJob, 4*workers)
	ordered := make(chan searchJob, 4*workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				var r searchFileResult
				r.err = scanSearchFile(ctx, job.filename, pattern, prefilter, options.Word, func(hit searchHit) bool {
					r.hits = append(r.hits, hit)
					return len(r.hits) < searchLimit
				})
				job.done <- r
			}
		}()
	}
	go func() {
		defer close(ordered)
		defer close(jobs)
		submit := func(job searchJob) bool {
			job.done = make(chan searchFileResult, 1)
			if job.err != nil {
				job.done <- searchFileResult{err: job.err}
			} else {
				select {
				case jobs <- job:
				case <-ctx.Done():
					return false
				}
			}
			select {
			case ordered <- job:
				return true
			case <-ctx.Done():
				return false
			}
		}
		var walk func(string, []searchIgnore) bool
		walk = func(dir string, inherited []searchIgnore) bool {
			if ctx.Err() != nil {
				return false
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				submit(searchJob{err: err})
				return false
			}
			rules := readSearchIgnores(dir, inherited)
			for _, entry := range entries {
				if ctx.Err() != nil {
					return false
				}
				// Do not follow symlinks or search hidden files (ripgrep defaults).
				name := entry.Name()
				if strings.HasPrefix(name, ".") || entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				filename := filepath.Join(dir, name)
				if searchIgnored(filename, name, entry.IsDir(), rules) {
					continue
				}
				if entry.IsDir() {
					if !walk(filename, rules) {
						return false
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
				if !submit(searchJob{filename: filename}) {
					return false
				}
			}
			return true
		}
		walk(root, nil)
	}()
	defer wg.Wait()
	var results []searchHit
	lastPublish := time.Now()
	for job := range ordered {
		r := <-job.done
		for _, hit := range r.hits {
			results = append(results, hit)
			if len(results)%64 == 0 || time.Since(lastPublish) > 100*time.Millisecond {
				publish(results)
				lastPublish = time.Now()
			}
			if len(results) >= searchLimit {
				cancel()
				return results, true, nil
			}
		}
		if r.err != nil {
			cancel()
			return results, false, r.err
		}
	}
	return results, false, ctx.Err()
}

// A prefilter reports whether a file can contain a match at all, so most files
// skip line splitting and per-line regexp evaluation. It may say yes wrongly,
// never no. Regex queries have none: ^, $ and \A mean something else in a
// whole file than in one line.
type searchPrefilter func([]byte) bool

func newSearchPrefilter(query string, options searchOptions, pattern *regexp.Regexp) searchPrefilter {
	if options.Regex || query == "" {
		return nil
	}
	if options.Case || strings.IndexFunc(query, unicode.IsUpper) >= 0 {
		// A case-sensitive literal: the regexp engine already runs at memchr speed.
		return pattern.Match
	}
	// Case-insensitive literal. Go folds with Unicode simple case folding, so an
	// ASCII letter also matches a few non-ASCII runes (k: KELVIN SIGN, s: LONG S).
	// Every other rune must fold only to itself for a byte search to be exact.
	needle := []byte(query)
	var exotic [][]byte
	for _, r := range query {
		ascii := r < utf8.RuneSelf && unicode.IsLetter(r)
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if !ascii {
				return pattern.Match
			}
			if f >= utf8.RuneSelf {
				exotic = append(exotic, []byte(string(f)))
			}
		}
	}
	for i, c := range needle {
		needle[i] = lowerASCII(c)
	}
	return func(data []byte) bool {
		for _, e := range exotic {
			if bytes.Contains(data, e) {
				return pattern.Match(data)
			}
		}
		return containsFoldASCII(data, needle)
	}
}
func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// containsFoldASCII reports whether data contains needle (lower case) with
// ASCII letters compared case-insensitively. Candidates come from memchr on
// the needle's rarest byte, in both cases.
func containsFoldASCII(data, needle []byte) bool {
	at := rareByte(needle)
	lo := needle[at]
	up := lo
	if 'a' <= lo && lo <= 'z' {
		up = lo - 'a' + 'A'
	}
	nextLo, nextUp := -1, -1
	for i := at; i+len(needle)-at <= len(data); {
		if nextLo < i {
			if nextLo = bytes.IndexByte(data[i:], lo); nextLo >= 0 {
				nextLo += i
			} else {
				nextLo = len(data)
			}
		}
		if up == lo {
			nextUp = len(data)
		} else if nextUp < i {
			if nextUp = bytes.IndexByte(data[i:], up); nextUp >= 0 {
				nextUp += i
			} else {
				nextUp = len(data)
			}
		}
		j := min(nextLo, nextUp)
		if j == len(data) {
			return false
		}
		if start := j - at; start >= 0 && start+len(needle) <= len(data) && equalFoldASCII(data[start:start+len(needle)], needle) {
			return true
		}
		i = j + 1
	}
	return false
}
func equalFoldASCII(s, lower []byte) bool {
	for i, c := range lower {
		if lowerASCII(s[i]) != c {
			return false
		}
	}
	return true
}

// rareByte picks the needle byte least likely to occur in source text.
func rareByte(needle []byte) int {
	const common = " etaoinsrhldcumfpgwybvk\n\t()=;.,_\"/:-0123456789{}"
	best, rank := 0, -1
	for i, c := range needle {
		r := strings.IndexByte(common, lowerASCII(c))
		if r < 0 {
			return i
		}
		if rank < 0 || r > rank {
			best, rank = i, r
		}
	}
	return best
}

// Files larger than this are streamed line by line without a prefilter.
const searchWholeFileLimit = 64 << 20

func scanSearchFile(ctx context.Context, filename string, pattern *regexp.Regexp, prefilter searchPrefilter, whole bool, emit func(searchHit) bool) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	var reader *bufio.Reader
	if st, err := file.Stat(); err == nil && prefilter != nil && st.Size() <= searchWholeFileLimit {
		data, err := io.ReadAll(file)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 || !prefilter(data) {
			return nil
		}
		reader = bufio.NewReader(bytes.NewReader(data))
	} else {
		reader = bufio.NewReader(file)
		prefix, _ := reader.Peek(8192)
		if bytes.IndexByte(prefix, 0) >= 0 {
			return nil
		}
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
