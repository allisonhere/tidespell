# tidespell

An offline English spell checker for Go, built for terminal editors. No cgo, no
dictionary files to install: the word list is embedded in the binary (about
410 KB gzipped), so it works in a single static build.

It is UI-free on purpose. `Check` returns misspelled spans as **rune offsets**,
which is what editors such as
[ripple](https://github.com/allisonhere/ripple) index by, and `Suggest` ranks
corrections for one word.

## Install

```bash
go get github.com/allisonhere/tidespell
```

## Usage

```go
c := tidespell.New()              // parses the embedded dictionary once per process
c.Add("Zorblax")                  // personal words: names, jargon (case-insensitive)

for _, s := range c.Check("I recieve teh email") {
	fmt.Println(s.Word, s.Start, s.End) // recieve 2 9, teh 10 13
}

fmt.Println(c.Suggest("recieve", 4))    // [receive relieve recede received]
fmt.Println(c.Known("colour"))          // true (US and GB spellings both accepted)
c.Words()                               // personal words, sorted, for you to persist
```

### With ripple

Build a rune-offset lookup from `Check` and hand it to ripple's `StyleKey`
and `Style` options (ripple v0.3.0+) to underline misspellings:

```go
spans := c.Check(editor.Value())
bad := make([]bool, len([]rune(editor.Value())))
for _, s := range spans {
	for i := s.Start; i < s.End; i++ {
		bad[i] = true
	}
}
opts.StyleKey = func(off int) string { if off < len(bad) && bad[off] { return "typo" }; return "" }
opts.Style = func(key, text string) string {
	if key != "typo" { // ripple calls Style for every run, flagged or not
		return text
	}
	return lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("203")).Render(text)
}
```

Recompute `spans` after edits (debounced), not on every render.

## What it ignores

URLs, email addresses, tokens containing digits, ALL-CAPS acronyms, single
letters, and mixed-case identifiers such as `iPhone` or `GitHub`. Matching is
lenient about case, accepts typographic apostrophes, and accepts possessives
when the base word is known.

## Limits

- English only (US and GB spellings). No grammar checking and no per-message
  language detection.
- No word frequencies: suggestions are ranked by edit distance, transposition,
  membership in a built-in common-words list, and capitalization fit.
- Names and jargon will be flagged until added with `Add`.

## License

Code is MIT (see `LICENSE`). The embedded word lists come from the English
Speller Database (SCOWL) and are covered by the notice in `NOTICE`, which must
accompany any copy of them.
