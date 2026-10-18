package tidespell

import (
	"slices"
	"testing"
	"time"
)

func TestMetaphoneGroupsSoundAlikes(t *testing.T) {
	same := [][2]string{
		{"playwrite", "playwright"},
		{"phone", "fone"},
		{"knight", "night"},
		{"wrap", "rap"},
		{"nation", "nashun"},
	}
	for _, p := range same {
		if a, b := metaphone(p[0]), metaphone(p[1]); a != b || a == "" {
			t.Errorf("metaphone(%q)=%q, metaphone(%q)=%q; want equal and non-empty", p[0], a, p[1], b)
		}
	}
	if metaphone("cat") == metaphone("dog") {
		t.Error("unrelated words should not share a key")
	}
	if metaphone("123 !!") != "" {
		t.Error("non-letters should produce no key")
	}
}

func TestEditDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"teh", "the", 1}, // adjacent swap
		{"cat", "cart", 1},
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"same", "same", 0},
	}
	for _, c := range cases {
		if got := editDistance([]rune(c.a), []rune(c.b)); got != c.want {
			t.Errorf("editDistance(%q,%q) = %d; want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestTypingPatterns(t *testing.T) {
	if !isTransposition([]rune("teh"), []rune("the")) || isTransposition([]rune("tea"), []rune("the")) {
		t.Error("isTransposition")
	}
	if !isDoubledLetterFix([]rune("occured"), []rune("occurred")) || !isDoubledLetterFix([]rune("comming"), []rune("coming")) {
		t.Error("isDoubledLetterFix should work in both directions")
	}
	if isDoubledLetterFix([]rune("cat"), []rune("cart")) {
		t.Error("an inserted non-duplicate letter is not a doubled-letter fix")
	}
	d := New().d
	if !d.isNeighbourSlip([]rune("wirk"), []rune("work")) || d.isNeighbourSlip([]rune("wxrk"), []rune("work")) {
		t.Error("isNeighbourSlip uses QWERTY adjacency (i is next to o; x is not)")
	}
}

func TestSuggestRanksPhoneticAndFrequency(t *testing.T) {
	c := New()
	if got := c.Suggest("teh", 3); len(got) == 0 || got[0] != "the" {
		t.Errorf("Suggest(teh) = %v; the commonest typo should give 'the' first", got)
	}
	if got := c.Suggest("playwrite", 3); !slices.Contains(got, "playwright") {
		t.Errorf("Suggest(playwrite) = %v; a phonetic match should be reachable", got)
	}
	if got := c.Suggest("finaly", 2); len(got) == 0 || got[0] != "finally" {
		t.Errorf("Suggest(finaly) = %v; want 'finally' ahead of rare proper nouns", got)
	}
}

func TestSuggestWorstCaseIsInteractive(t *testing.T) {
	c := New()
	c.Suggest("warm", 1) // load dictionary and phonetic index outside the timing
	for _, w := range []string{"incomprehensiblly", "internationalizaton", "x"} {
		start := time.Now()
		c.Suggest(w, 5)
		if d := time.Since(start); d > time.Second {
			t.Errorf("Suggest(%q) took %v", w, d)
		}
	}
}
