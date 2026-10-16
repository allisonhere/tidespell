package tidespell

import (
	"sort"
	"strings"
)

// Suggest returns up to n corrections for word, best first. Candidates come
// from every word within two edits plus words that sound alike (so "playwrite"
// can reach "playwright"). Each is scored by its true edit distance, adjusted
// for the way people actually mistype: swapped neighbouring letters, a
// neighbouring key hit by mistake, a doubled letter added or dropped, a
// matching pronunciation, and how everyday the word is. The result keeps the
// capitalization of word.
func (c *Checker) Suggest(word string, n int) []string {
	word = normalize(word)
	l := strings.ToLower(word)
	if n <= 0 || l == "" {
		return nil
	}
	typo := []rune(l)

	cands := map[string]bool{}
	e1 := edits(l)
	for _, w := range e1 {
		cands[w] = true
	}
	if len(typo) <= 18 { // two edits is quadratic in length; skip for very long words
		for _, w1 := range e1 {
			for _, w2 := range edits(w1) {
				cands[w2] = true
			}
		}
	}
	key := metaphone(l)
	if key != "" {
		for _, w := range phoneticBucket(c.d, key) {
			cands[w] = true
		}
	}
	delete(cands, l)

	lowerInput := word == l
	type scored struct {
		w     string
		score float64
	}
	var out []scored
	for w := range cands {
		written, ok := c.d.lower[w]
		if !ok {
			c.mu.RLock()
			_, ok = c.personal[w]
			c.mu.RUnlock()
			if !ok {
				continue
			}
			written = w
		}
		rs := []rune(w)
		d := editDistance(typo, rs)
		if d > 3 {
			continue
		}
		s := float64(d)
		switch {
		case d == 1 && isTransposition(typo, rs):
			s -= 0.5
		case d == 1 && len(rs) == len(typo)+1:
			s -= 0.15 // a dropped letter is the commonest single-letter slip
		case d == 1 && isNeighbourSlip(typo, rs):
			s -= 0.1
		}
		if isDoubledLetterFix(typo, rs) {
			s -= 0.25
		}
		if key != "" && metaphone(w) == key {
			// Sounding alike matters most when the spelling is far off.
			if d >= 2 {
				s -= 0.4
			} else {
				s -= 0.15
			}
		}
		tier := c.d.tier[w]
		if tier == 0 {
			tier = 5
		}
		s += float64(tier-1) * 0.12
		if lowerInput && written != w {
			s += 0.3 // a proper noun or abbreviation offered for a lowercase word
		}
		if rs[0] != typo[0] {
			s += 0.15
		}
		out = append(out, scored{w: written, score: s})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].w < out[j].w
	})
	res := make([]string, 0, n)
	for _, sc := range out {
		if len(res) == n {
			break
		}
		res = append(res, matchCase(word, sc.w))
	}
	return res
}

// editDistance is the optimal-string-alignment distance: insertions,
// deletions, substitutions, and swaps of adjacent letters each cost 1.
func editDistance(a, b []rune) int {
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

// isTransposition reports whether b is a with two adjacent letters swapped.
func isTransposition(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i+1 < len(a); i++ {
		if a[i] != b[i] {
			return a[i] == b[i+1] && a[i+1] == b[i] && string(a[i+2:]) == string(b[i+2:])
		}
	}
	return false
}

// isNeighbourSlip reports whether b differs from a by one letter and the two
// letters sit next to each other on a QWERTY keyboard.
func isNeighbourSlip(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return string(a[i+1:]) == string(b[i+1:]) && keyNeighbours[a[i]][b[i]]
		}
	}
	return false
}

// isDoubledLetterFix reports whether b is a with one letter doubled, or the
// reverse: "occured"/"occurred", "comming"/"coming".
func isDoubledLetterFix(a, b []rune) bool {
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b) != len(a)+1 {
		return false
	}
	for i := 1; i < len(b); i++ {
		if b[i] == b[i-1] {
			rest := append(append([]rune{}, b[:i]...), b[i+1:]...)
			if string(rest) == string(a) {
				return true
			}
		}
	}
	return false
}

var keyNeighbours = func() map[rune]map[rune]bool {
	rows := []string{"qwertyuiop", "asdfghjkl", "zxcvbnm"}
	m := map[rune]map[rune]bool{}
	link := func(x, y rune) {
		if m[x] == nil {
			m[x] = map[rune]bool{}
		}
		m[x][y] = true
	}
	for r, row := range rows {
		for i, x := range row {
			if i+1 < len(row) {
				link(x, rune(row[i+1]))
				link(rune(row[i+1]), x)
			}
			if r+1 < len(rows) {
				for _, j := range []int{i - 1, i, i + 1} { // keys diagonally/directly below
					if j >= 0 && j < len(rows[r+1]) {
						link(x, rune(rows[r+1][j]))
						link(rune(rows[r+1][j]), x)
					}
				}
			}
		}
	}
	return m
}()
