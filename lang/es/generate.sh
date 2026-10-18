#!/bin/sh
# Regenerates words.txt.gz and tiers.txt.gz. Needs hunspell (with unmunch),
# npm, python3 and curl. Usage: ./generate.sh
#
# Words: the RLA-ES es_ES dictionary (npm package dictionary-es), expanded with
# unmunch. unmunch ignores some affix conditions and over-generates, so every
# expanded form is kept only if hunspell itself accepts it.
# Tiers: rank in the OpenSubtitles 2018 Spanish list (hermitdave/FrequencyWords),
# 1 = top 10k, 2 = top 25k, 3 = rest of the 50k list; absent words are tier 5.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
npm pack dictionary-es@4.0.0 >/dev/null
tar xzf dictionary-es-4.0.0.tgz
cp package/index.aff es.aff
cp package/index.dic es.dic
cp package/license "$here/LICENSE.es_ES"
curl -sL https://raw.githubusercontent.com/hermitdave/FrequencyWords/master/content/2018/es/es_50k.txt -o freq.txt

unmunch es.dic es.aff | python3 -c '
import re, sys
ws = set()
for l in sys.stdin:
    w = l.rstrip("\n").split("/")[0]
    if re.fullmatch(r"[^\W\d_]+", w):
        ws.add(w)
print("\n".join(sorted(ws)))' > all.txt
hunspell -d ./package/index -l < all.txt | LC_ALL=C sort -u > bad.txt
LC_ALL=C sort all.txt | LC_ALL=C comm -23 - bad.txt > words.txt
gzip -9 -n -c words.txt > "$here/words.txt.gz"

python3 - <<'PY' | gzip -9 -n -c > "$here/tiers.txt.gz"
words = {l.strip().lower() for l in open("words.txt", encoding="utf8")}
seen = set()
for rank, line in enumerate(open("freq.txt", encoding="utf8"), 1):
    w = line.split()[0]
    if w in seen or w not in words:
        continue
    seen.add(w)
    print(1 if rank <= 10000 else 2 if rank <= 25000 else 3, w)
PY
