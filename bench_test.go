package tidespell

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"
)

// typoBenchmark returns the (typo, correct) pairs from testdata/typos.txt.
func typoBenchmark(t testing.TB) [][2]string {
	t.Helper()
	f, err := os.Open("testdata/typos.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var pairs [][2]string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 2 {
			t.Fatalf("bad benchmark line %q", line)
		}
		pairs = append(pairs, [2]string{f[0], f[1]})
	}
	return pairs
}

// TestSuggestionQuality measures how often the intended word is the first
// suggestion (top-1) or among the first three (top-3). The floors guard
// against ranking regressions; raise them as the ranking improves.
func TestSuggestionQuality(t *testing.T) {
	c := New()
	pairs := typoBenchmark(t)
	var top1, top3, found, unknown int
	var missed []string
	for _, p := range pairs {
		if !c.Known(p[1]) {
			unknown++ // the dictionary lacks the target; not a ranking problem
			continue
		}
		got := c.Suggest(p[0], 5)
		if len(got) > 0 && got[0] == p[1] {
			top1++
		} else if len(missed) < 40 {
			missed = append(missed, p[0]+"→"+p[1]+" got "+strings.Join(got[:min(3, len(got))], ","))
		}
		if slices.Contains(got[:min(3, len(got))], p[1]) {
			top3++
		}
		if slices.Contains(got, p[1]) {
			found++
		}
	}
	n := len(pairs) - unknown
	t.Logf("benchmark: %d typos (%d skipped, target not in dictionary): top-1 %d (%.0f%%), top-3 %d (%.0f%%), in top-5 %d (%.0f%%)",
		n, unknown, top1, pct(top1, n), top3, pct(top3, n), found, pct(found, n))
	for _, m := range missed {
		t.Log("  miss:", m)
	}
	if pct(top1, n) < minTop1 || pct(top3, n) < minTop3 {
		t.Fatalf("suggestion quality fell below the floor (top-1 %.0f%% < %.0f%% or top-3 %.0f%% < %.0f%%)",
			pct(top1, n), minTop1, pct(top3, n), minTop3)
	}
}

// Floors, in percent. v0.1.0 scored 83% / 96% on this list; the scored
// ranking in v0.2.0 reaches 93% / 99%. Ratchet up with the ranking.
const (
	minTop1 = 90.0
	minTop3 = 97.0
)

func pct(a, n int) float64 {
	if n == 0 {
		return 0
	}
	return 100 * float64(a) / float64(n)
}
