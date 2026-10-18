// Package es is the Spanish dictionary for tidespell:
//
//	c := tidespell.New(tidespell.WithLanguage(es.Language))
//
// It is a separate package so programs that only check English do not embed
// the Spanish word list.
package es

import (
	_ "embed"
	"strings"
	"unicode"

	"github.com/allisonhere/tidespell"
)

//go:embed words.txt.gz
var wordsGz []byte

//go:embed tiers.txt.gz
var tiersGz []byte

// Language is Spanish (es_ES word list; seseo-neutral phonetic matching).
var Language = &tidespell.Language{
	Code:     "es",
	Alphabet: "abcdefghijklmnopqrstuvwxyzáéíóúüñ",
	Keyboard: []string{"qwertyuiop", "asdfghjklñ", "zxcvbnm"},
	Phonetic: phonetic,
	Words:    wordsGz,
	Tiers:    tiersGz,
}

var unaccent = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "ny")

// phonetic returns a key shared by words that sound alike in Spanish, so that
// spelling slips such as "baca"/"vaca", "hola"/"ola" or "jirafa"/"girafa"
// reach each other. It follows seseo (c/z/s alike), the pronunciation of most
// speakers, and ignores accents.
func phonetic(word string) string {
	w := unaccent.Replace(strings.ToLower(word))

	rs := []rune(w)
	at := func(i int) rune {
		if i < 0 || i >= len(rs) {
			return 0
		}
		return rs[i]
	}
	front := func(r rune) bool { return r == 'e' || r == 'i' }

	var out []rune
	emit := func(r rune) {
		if n := len(out); n > 0 && out[n-1] == r && r != 'r' {
			return
		}
		out = append(out, r)
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 'h':
			if at(i-1) == 'c' {
				continue // handled with c
			}
		case r == 'c' && at(i+1) == 'h':
			emit('C')
			i++
		case r == 'c' && front(at(i+1)), r == 'z', r == 's':
			emit('s')
		case r == 'c', r == 'k', r == 'q':
			emit('k')
			if r == 'q' && at(i+1) == 'u' {
				i++
			}
		case r == 'g' && front(at(i+1)), r == 'j':
			emit('x')
		case r == 'g' && at(i+1) == 'u' && front(at(i+2)):
			emit('g')
			i++
		case r == 'b', r == 'v', r == 'w':
			emit('b')
		case r == 'y', r == 'l' && at(i+1) == 'l':
			emit('y')
			if r == 'l' {
				i++
			}
		case r == 'x':
			emit('k')
			emit('s')
		case unicode.IsLetter(r):
			emit(r)
		}
	}
	return string(out)
}
