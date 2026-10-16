package tidespell

import (
	"slices"
	"testing"
	"time"
)

func words(spans []Span) []string {
	out := make([]string, len(spans))
	for i, s := range spans {
		out[i] = s.Word
	}
	return out
}

func TestCheckFlagsMisspellings(t *testing.T) {
	c := New()
	got := words(c.Check("I recieve teh email, but definately not this."))
	want := []string{"recieve", "teh", "definately"}
	if !slices.Equal(got, want) {
		t.Fatalf("Check = %v; want %v", got, want)
	}
}

func TestCheckAcceptsCorrectProse(t *testing.T) {
	c := New()
	text := "Hello Allie, don't forget: the colour and color of Allie's e-mail are fine. It's 5pm on 2026-10-15."
	if got := words(c.Check(text)); len(got) != 0 {
		t.Fatalf("expected no misspellings, got %v", got)
	}
}

func TestCheckSkipsNonProse(t *testing.T) {
	c := New()
	text := "See https://exampel.com/pgae?x=1 or mail jhon@exampel.com, NASA, iPhone, GitHub, and abc123 x."
	if got := words(c.Check(text)); len(got) != 0 {
		t.Fatalf("URLs, emails, acronyms, identifiers and digits must be skipped, got %v", got)
	}
}

func TestSpansAreRuneOffsets(t *testing.T) {
	c := New()
	text := "café résumé — recieve"
	spans := c.Check(text)
	if len(spans) != 1 {
		t.Fatalf("spans = %+v", spans)
	}
	rs := []rune(text)
	if got := string(rs[spans[0].Start:spans[0].End]); got != "recieve" {
		t.Fatalf("span covers %q; want recieve (offsets must index runes)", got)
	}
}

func TestCurlyApostrophe(t *testing.T) {
	c := New()
	if got := words(c.Check("don’t and Allie’s")); len(got) != 0 {
		t.Fatalf("curly apostrophes should be accepted, got %v", got)
	}
}

func TestPersonalWords(t *testing.T) {
	c := New()
	if c.Known("Zorblax") {
		t.Fatal("Zorblax should be unknown")
	}
	c.Add("Zorblax")
	if !c.Known("zorblax") || !c.Known("Zorblax's") {
		t.Fatal("added word should be known case-insensitively, with possessive")
	}
	if !slices.Equal(c.Words(), []string{"zorblax"}) {
		t.Fatalf("Words = %v", c.Words())
	}
	if other := New(); other.Known("Zorblax") {
		t.Fatal("personal words must not leak between Checkers")
	}
}

func TestSuggest(t *testing.T) {
	c := New()
	cases := map[string]string{
		"recieve":    "receive",
		"teh":        "the",
		"definately": "definitely",
		"Recieve":    "Receive",
		"occured":    "occurred",
	}
	for in, want := range cases {
		got := c.Suggest(in, 5)
		if !slices.Contains(got, want) {
			t.Errorf("Suggest(%q) = %v; want it to include %q", in, got, want)
		}
	}
	if got := c.Suggest("zzzzqqqqxxxx", 3); len(got) > 3 {
		t.Errorf("Suggest must honor n, got %v", got)
	}
}

func TestSuggestIsFastEnoughToRunOnKeypress(t *testing.T) {
	c := New()
	c.Known("warm") // load the dictionary outside the timed section
	start := time.Now()
	c.Suggest("incomprehensiblly", 5)
	if d := time.Since(start); d > 750*time.Millisecond {
		t.Fatalf("Suggest took %v", d)
	}
}
