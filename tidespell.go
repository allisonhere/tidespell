// Package tidespell is an offline English spell checker with no cgo and no
// external dictionary files: the word list is embedded in the binary.
//
// It is deliberately UI-free. Check turns text into misspelled spans (rune
// offsets, so they line up with editors that index by rune, such as ripple's
// StyleKey hook), and Suggest ranks corrections for one word.
package tidespell

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed words.txt.gz
var wordsGz []byte

// commonGz is the everyday-vocabulary subset; Suggest prefers it so "the"
// outranks rare entries like "Te" when the word list carries no frequencies.
//
//go:embed common.txt.gz
var commonGz []byte

// Span is one misspelled word. Start and End are rune offsets into the text
// passed to Check (End is exclusive).
type Span struct {
	Start, End int
	Word       string
}

type dict struct {
	exact  map[string]struct{} // entries as written ("Allie", "NASA", "color")
	lower  map[string]string   // lowercase form -> preferred written form
	common map[string]struct{} // lowercase everyday words
}

var (
	defaultDict     *dict
	defaultDictOnce sync.Once
)

func loadDict() *dict {
	defaultDictOnce.Do(func() {
		d := &dict{
			exact:  make(map[string]struct{}, 120000),
			lower:  make(map[string]string, 120000),
			common: make(map[string]struct{}, 50000),
		}
		readLines(wordsGz, func(w string) {
			d.exact[w] = struct{}{}
			l := strings.ToLower(w)
			// Prefer the all-lowercase spelling so suggestions for a common
			// word are not capitalized because a proper noun shares it.
			if prev, ok := d.lower[l]; !ok || (w == l && prev != l) {
				d.lower[l] = w
			}
		})
		readLines(commonGz, func(w string) { d.common[strings.ToLower(w)] = struct{}{} })
		defaultDict = d
	})
	return defaultDict
}

func readLines(gz []byte, fn func(string)) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return
	}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		if w := sc.Text(); w != "" {
			fn(w)
		}
	}
}

// Checker checks words against the built-in dictionary plus a personal
// word set. A Checker is safe for concurrent use.
type Checker struct {
	d        *dict
	mu       sync.RWMutex
	personal map[string]struct{} // lowercase
}

// New returns a Checker backed by the embedded dictionary. The dictionary is
// parsed once per process, on first use.
func New() *Checker {
	return &Checker{d: loadDict(), personal: map[string]struct{}{}}
}

// Add teaches the checker words (a user's names, jargon). Matching is
// case-insensitive.
func (c *Checker) Add(words ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, w := range words {
		if w = normalize(strings.TrimSpace(w)); w != "" {
			c.personal[strings.ToLower(w)] = struct{}{}
		}
	}
}

// Words returns the personal words, sorted, for persisting.
func (c *Checker) Words() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.personal))
	for w := range c.personal {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// Known reports whether word is spelled correctly. It is lenient about case:
// "allie" passes because "Allie" is in the dictionary, and "Hello" passes at a
// sentence start. Possessives ("Allie's") pass when the base word does.
func (c *Checker) Known(word string) bool {
	word = normalize(word)
	if word == "" {
		return true
	}
	if _, ok := c.d.exact[word]; ok {
		return true
	}
	l := strings.ToLower(word)
	if _, ok := c.d.lower[l]; ok {
		return true
	}
	c.mu.RLock()
	_, ok := c.personal[l]
	c.mu.RUnlock()
	if ok {
		return true
	}
	if base, cut := strings.CutSuffix(l, "'s"); cut && base != "" {
		return c.Known(base)
	}
	if base, cut := strings.CutSuffix(l, "s'"); cut && base != "" {
		return c.Known(base) || c.Known(base+"s")
	}
	return false
}

// normalize folds typographic apostrophes to ASCII.
func normalize(w string) string {
	return strings.NewReplacer("’", "'", "‘", "'").Replace(w)
}

// skipRe matches runs that are never prose: URLs and email addresses.
var skipRe = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|[\w.+-]+@[\w-]+(?:\.[\w-]+)+`)

// Check returns the misspelled words in text, in order. It ignores URLs,
// email addresses, tokens containing digits, ALL-CAPS tokens (acronyms),
// single letters, and mixed-case identifiers such as "iPhone" or "GitHub".
func (c *Checker) Check(text string) []Span {
	runes := []rune(text)
	skip := make([]bool, len(runes))
	for _, loc := range skipRe.FindAllStringIndex(text, -1) {
		s := utf8.RuneCountInString(text[:loc[0]])
		e := s + utf8.RuneCountInString(text[loc[0]:loc[1]])
		for i := s; i < e && i < len(skip); i++ {
			skip[i] = true
		}
	}

	var spans []Span
	i := 0
	for i < len(runes) {
		if !isWordRune(runes[i]) || skip[i] {
			i++
			continue
		}
		start := i
		for i < len(runes) && !skip[i] && (isWordRune(runes[i]) || (isApostrophe(runes[i]) && i+1 < len(runes) && isLetter(runes[i+1]) && i > start)) {
			i++
		}
		tok := string(runes[start:i])
		if shouldSkip(tok) || c.Known(tok) {
			continue
		}
		spans = append(spans, Span{Start: start, End: i, Word: tok})
	}
	return spans
}

func isLetter(r rune) bool     { return unicode.IsLetter(r) }
func isApostrophe(r rune) bool { return r == '\'' || r == '’' }
func isWordRune(r rune) bool   { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func shouldSkip(tok string) bool {
	var letters, upper int
	var prevLower bool
	for _, r := range tok {
		switch {
		case unicode.IsDigit(r):
			return true
		case unicode.IsLetter(r):
			letters++
			if unicode.IsUpper(r) {
				upper++
				if prevLower {
					return true // camelCase / iPhone
				}
			}
			prevLower = unicode.IsLower(r)
		}
	}
	if letters < 2 {
		return true
	}
	return upper == letters // ACRONYM
}

// Suggest returns up to n corrections for word, best first: candidates within
// one edit, then two, with transpositions and a matching first letter ranked
// ahead. The result keeps the capitalization of word.
func (c *Checker) Suggest(word string, n int) []string {
	word = normalize(word)
	l := strings.ToLower(word)
	if n <= 0 || l == "" {
		return nil
	}
	type cand struct {
		w          string
		dist       int
		transposed bool
		uncommon   bool
		oddCase    bool // capitalized/abbreviation entry offered for a lowercase word
		otherFirst bool
	}
	lowerInput := word == l
	seen := map[string]bool{l: true}
	var cands []cand
	add := func(w string, dist int, transposed bool) {
		if seen[w] {
			return
		}
		seen[w] = true
		written, ok := c.d.lower[w]
		if !ok {
			c.mu.RLock()
			_, ok = c.personal[w]
			c.mu.RUnlock()
			if !ok {
				return
			}
			written = w
		}
		_, common := c.d.common[w]
		cands = append(cands, cand{
			w: written, dist: dist, transposed: transposed, uncommon: !common,
			oddCase:    lowerInput && written != w,
			otherFirst: !strings.HasPrefix(w, l[:1]),
		})
	}

	rs := []rune(l)
	e1 := edits(l)
	for i := 0; i+1 < len(rs); i++ {
		add(string(rs[:i])+string(rs[i+1])+string(rs[i])+string(rs[i+2:]), 1, true)
	}
	for _, w := range e1 {
		add(w, 1, false)
	}
	if len(cands) < n {
		for _, w1 := range e1 {
			for _, w2 := range edits(w1) {
				add(w2, 2, false)
			}
		}
	}
	less := func(a, b cand) int {
		for _, k := range []struct{ x, y bool }{
			{a.transposed, b.transposed}, {!a.uncommon, !b.uncommon},
			{!a.oddCase, !b.oddCase}, {!a.otherFirst, !b.otherFirst},
		} {
			if k.x != k.y {
				if k.x {
					return -1
				}
				return 1
			}
		}
		return strings.Compare(a.w, b.w)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return less(cands[i], cands[j]) < 0
	})
	out := make([]string, 0, n)
	for _, cd := range cands {
		if len(out) == n {
			break
		}
		out = append(out, matchCase(word, cd.w))
	}
	return out
}

// matchCase gives suggestion the capitalization pattern of word.
func matchCase(word, suggestion string) string {
	r, _ := utf8.DecodeRuneInString(word)
	if unicode.IsUpper(r) {
		first, size := utf8.DecodeRuneInString(suggestion)
		return string(unicode.ToUpper(first)) + suggestion[size:]
	}
	return suggestion
}

const alphabet = "abcdefghijklmnopqrstuvwxyz'"

// edits returns every string one edit (delete, transpose, replace, insert)
// away from w.
func edits(w string) []string {
	rs := []rune(w)
	out := make([]string, 0, 54*len(rs)+27)
	for i := 0; i <= len(rs); i++ {
		if i < len(rs) {
			out = append(out, string(rs[:i])+string(rs[i+1:])) // delete
		}
		if i+1 < len(rs) {
			out = append(out, string(rs[:i])+string(rs[i+1])+string(rs[i])+string(rs[i+2:])) // transpose
		}
		for _, a := range alphabet {
			if i < len(rs) && a != rs[i] {
				out = append(out, string(rs[:i])+string(a)+string(rs[i+1:])) // replace
			}
			out = append(out, string(rs[:i])+string(a)+string(rs[i:])) // insert
		}
	}
	return out
}
