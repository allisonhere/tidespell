package tidespell

import (
	"strings"
	"sync"
)

// metaphone returns a rough phonetic key for an English word (a simplified
// Metaphone): words that sound alike tend to share a key, so "definately"
// and "definitely" or "playwrite" and "playwright" can be matched even when
// they are several edits apart. Input is lowercased letters; anything else is
// dropped.
func metaphone(word string) string {
	var in []rune
	for _, r := range strings.ToLower(word) {
		if r >= 'a' && r <= 'z' {
			in = append(in, r)
		}
	}
	if len(in) == 0 {
		return ""
	}
	at := func(i int) rune {
		if i < 0 || i >= len(in) {
			return 0
		}
		return in[i]
	}
	vowel := func(r rune) bool { return strings.ContainsRune("aeiou", r) }

	i := 0
	var out []rune
	// Initial exceptions.
	switch {
	case (at(0) == 'a' && at(1) == 'e') || (at(0) == 'g' && at(1) == 'n') ||
		(at(0) == 'k' && at(1) == 'n') || (at(0) == 'p' && at(1) == 'n') ||
		(at(0) == 'w' && at(1) == 'r'):
		i = 1
	case at(0) == 'x':
		out = append(out, 's')
		i = 1
	case at(0) == 'w' && at(1) == 'h':
		out = append(out, 'w')
		i = 2
	}
	for ; i < len(in); i++ {
		c := in[i]
		if c == at(i-1) && c != 'c' { // collapse doubles, except "cc"
			continue
		}
		switch c {
		case 'a', 'e', 'i', 'o', 'u':
			if i == 0 {
				out = append(out, c)
			}
		case 'b':
			if at(i-1) != 'm' || i != len(in)-1 {
				out = append(out, 'b')
			}
		case 'c':
			switch {
			case at(i+1) == 'i' && at(i+2) == 'a':
				out = append(out, 'x')
			case at(i+1) == 'h':
				out = append(out, 'x')
				i++
			case strings.ContainsRune("iey", at(i+1)):
				if at(i-1) != 's' {
					out = append(out, 's')
				}
			default:
				out = append(out, 'k')
			}
		case 'd':
			if at(i+1) == 'g' && strings.ContainsRune("eiy", at(i+2)) {
				out = append(out, 'j')
				i++
			} else {
				out = append(out, 't')
			}
		case 'g':
			switch {
			case at(i+1) == 'h' && !vowel(at(i+2)):
				// silent "gh" as in "right", "though"
			case at(i+1) == 'n' && (i+2 == len(in) || (at(i+2) == 'e' && at(i+3) == 'd' && i+4 == len(in))):
				// silent "g" in "sign", "signed"
			case strings.ContainsRune("iey", at(i+1)) && at(i-1) != 'g':
				out = append(out, 'j')
			default:
				out = append(out, 'k')
			}
		case 'h':
			if vowel(at(i+1)) && !strings.ContainsRune("cgpst", at(i-1)) {
				out = append(out, 'h')
			}
		case 'k':
			if at(i-1) != 'c' {
				out = append(out, 'k')
			}
		case 'p':
			if at(i+1) == 'h' {
				out = append(out, 'f')
				i++
			} else {
				out = append(out, 'p')
			}
		case 'q':
			out = append(out, 'k')
		case 's':
			switch {
			case at(i+1) == 'h':
				out = append(out, 'x')
				i++
			case at(i+1) == 'i' && (at(i+2) == 'o' || at(i+2) == 'a'):
				out = append(out, 'x')
			default:
				out = append(out, 's')
			}
		case 't':
			switch {
			case at(i+1) == 'i' && (at(i+2) == 'o' || at(i+2) == 'a'):
				out = append(out, 'x')
			case at(i+1) == 'h':
				out = append(out, '0')
				i++
			case at(i+1) == 'c' && at(i+2) == 'h':
				// silent "t" in "watch"
			default:
				out = append(out, 't')
			}
		case 'v':
			out = append(out, 'f')
		case 'w', 'y':
			if vowel(at(i + 1)) {
				out = append(out, c)
			}
		case 'x':
			out = append(out, 'k', 's')
		case 'z':
			out = append(out, 's')
		default: // f j l m n r
			out = append(out, c)
		}
	}
	return string(out)
}

// phoneticIndex groups dictionary words by metaphone key. It is built on first
// use (a few tens of milliseconds) and shared by every Checker.
var (
	phoneticOnce  sync.Once
	phoneticIndex map[string][]string
)

func phoneticBucket(d *dict, key string) []string {
	phoneticOnce.Do(func() {
		phoneticIndex = make(map[string][]string, 60000)
		for l := range d.lower {
			if k := metaphone(l); k != "" {
				phoneticIndex[k] = append(phoneticIndex[k], l)
			}
		}
	})
	return phoneticIndex[key]
}
