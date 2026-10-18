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

// tiersGz maps lowercase words to a frequency tier ("1 the"): 1 is everyday
// vocabulary (SCOWL size 35), 2 and 3 progressively rarer (sizes 40 and 50).
// Words absent from it are tier 5. Suggest prefers lower tiers so "the"
// outranks rare entries like "Te" when the word list carries no frequencies.
//
//go:embed tiers.txt.gz
var tiersGz []byte

// Language is everything the checker needs to know about one language. English
// is built in; other languages live in sub-packages (tidespell/lang/...) so
// importers only embed the word lists they use.
type Language struct {
	Code     string   // BCP 47 tag, e.g. "en"
	Alphabet string   // letters (plus apostrophe) that edits may insert or substitute
	Keyboard []string // key rows, for scoring mistypes of neighbouring keys

	// Phonetic returns a key shared by words that sound alike; nil disables
	// phonetic candidates.
	Phonetic func(string) string

	// Words is a gzipped word list, one entry per line. Tiers is a gzipped
	// "<tier> <word>" list ("1 the"); words absent from it are tier 5.
	Words, Tiers []byte
}

// English is the default language.
var English = &Language{
	Code:     "en",
	Alphabet: "abcdefghijklmnopqrstuvwxyz'",
	Keyboard: []string{"qwertyuiop", "asdfghjkl", "zxcvbnm"},
	Phonetic: metaphone,
	Words:    wordsGz,
	Tiers:    tiersGz,
}

// Option configures New.
type Option func(*Checker)

// WithLanguage selects the dictionary and typing model; the default is English.
func WithLanguage(l *Language) Option {
	return func(c *Checker) { c.lang = l }
}

// Span is one misspelled word. Start and End are rune offsets into the text
// passed to Check (End is exclusive).
type Span struct {
	Start, End int
	Word       string
}

type dict struct {
	lang  *Language
	exact map[string]struct{} // entries as written ("Allie", "NASA", "color")
	lower map[string]string   // lowercase form -> preferred written form
	tier  map[string]uint8    // lowercase word -> frequency tier (1 = everyday)

	neighbours map[rune]map[rune]bool // keys adjacent on lang.Keyboard

	phonOnce sync.Once
	phon     map[string][]string // phonetic key -> lowercase words
}

// dicts caches one parsed dictionary per language: *Language -> *dictEntry.
var dicts sync.Map

type dictEntry struct {
	once sync.Once
	d    *dict
}

func loadDict(l *Language) *dict {
	v, _ := dicts.LoadOrStore(l, &dictEntry{})
	e := v.(*dictEntry)
	e.once.Do(func() {
		d := &dict{
			lang:       l,
			exact:      make(map[string]struct{}, 120000),
			lower:      make(map[string]string, 120000),
			tier:       make(map[string]uint8, 90000),
			neighbours: keyNeighbours(l.Keyboard),
		}
		readLines(l.Words, func(w string) {
			d.exact[w] = struct{}{}
			l := strings.ToLower(w)
			// Prefer the all-lowercase spelling so suggestions for a common
			// word are not capitalized because a proper noun shares it.
			if prev, ok := d.lower[l]; !ok || (w == l && prev != l) {
				d.lower[l] = w
			}
		})
		readLines(l.Tiers, func(line string) {
			if len(line) > 2 && line[1] == ' ' && line[0] >= '1' && line[0] <= '9' {
				d.tier[line[2:]] = line[0] - '0'
			}
		})
		e.d = d
	})
	return e.d
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
	lang     *Language
	d        *dict
	mu       sync.RWMutex
	personal map[string]struct{} // lowercase
}

// New returns a Checker backed by an embedded dictionary (English unless
// WithLanguage says otherwise). Each language's dictionary is parsed once per
// process, on first use.
func New(opts ...Option) *Checker {
	c := &Checker{lang: English, personal: map[string]struct{}{}}
	for _, o := range opts {
		o(c)
	}
	c.d = loadDict(c.lang)
	return c
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

// matchCase gives suggestion the capitalization pattern of word.
func matchCase(word, suggestion string) string {
	r, _ := utf8.DecodeRuneInString(word)
	if unicode.IsUpper(r) {
		first, size := utf8.DecodeRuneInString(suggestion)
		return string(unicode.ToUpper(first)) + suggestion[size:]
	}
	return suggestion
}

// edits returns every string one edit (delete, transpose, replace, insert)
// away from w.
func edits(w, alphabet string) []string {
	rs := []rune(w)
	out := make([]string, 0, 2*len(alphabet)*len(rs)+len(alphabet))
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
