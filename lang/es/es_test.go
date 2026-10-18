package es

import (
	"slices"
	"testing"

	"github.com/allisonhere/tidespell"
)

func TestKnown(t *testing.T) {
	c := tidespell.New(tidespell.WithLanguage(Language))
	for _, w := range []string{"información", "niño", "pingüe", "España", "canciones", "hablábamos", "cómprame"} {
		if !c.Known(w) {
			t.Errorf("Known(%q) = false", w)
		}
	}
	for _, w := range []string{"informcaión", "ninio", "the", "hablabamoz"} {
		if c.Known(w) {
			t.Errorf("Known(%q) = true", w)
		}
	}
}

func TestSuggest(t *testing.T) {
	c := tidespell.New(tidespell.WithLanguage(Language))
	for typo, want := range map[string]string{
		"informcaión": "información",
		"informacion": "información",
		"ablar":       "hablar",
		"bamos":       "vamos",
		"jirafa":      "jirafa",
		"girafa":      "jirafa",
		"cancion":     "canción",
		"español":     "español",
		"espanol":     "español",
	} {
		if got := c.Suggest(typo, 5); typo != want && !slices.Contains(got, want) {
			t.Errorf("Suggest(%q) = %v; want %q among them", typo, got, want)
		}
	}
}

func TestPhonetic(t *testing.T) {
	for _, p := range [][2]string{{"vaca", "baca"}, {"hola", "ola"}, {"jirafa", "girafa"}, {"cerveza", "servesa"}, {"llave", "yabe"}, {"queso", "keso"}} {
		if a, b := phonetic(p[0]), phonetic(p[1]); a != b || a == "" {
			t.Errorf("phonetic(%q)=%q, phonetic(%q)=%q; want equal", p[0], a, p[1], b)
		}
	}
	if phonetic("casa") == phonetic("cosa") {
		t.Error("different vowels should differ")
	}
}
