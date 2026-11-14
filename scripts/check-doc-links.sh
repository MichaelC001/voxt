#!/usr/bin/env bash
# Checks the relative links of the repository's Markdown files (README*,
# CONTRIBUTING, CHANGELOG, AGENTS, docs/**, the components' READMEs): the
# file must exist, and a #fragment must name a heading of the target (GitHub's
# anchors) or an <a name>. External links (http, mailto) are not fetched.
# Prints one line per broken link and exits 1 if there is any.
# Usage: scripts/check-doc-links.sh [file.md ...]
set -euo pipefail
cd "$(dirname "$0")/.."
if (($# == 0)); then
  mapfile -t files < <(git ls-files '*.md' ':!:third_party/**' ':!:sglang_omni_mlx/**' ':!:testdata/**')
else
  files=("$@")
fi
python3 -I - "${files[@]}" <<'EOF'
import os, re, sys, unicodedata

def anchors(path, cache={}):
    if path in cache:
        return cache[path]
    out, seen, fence = set(), {}, False
    try:
        text = open(path, encoding="utf-8").read()
    except OSError:
        cache[path] = out
        return out
    for line in text.splitlines():
        if line.lstrip().startswith(("```", "~~~")):
            fence = not fence
            continue
        if fence:
            continue
        for name in re.findall(r'<a\s+(?:name|id)="([^"]+)"', line):
            out.add(name)
        m = re.match(r"^(#{1,6})\s+(.*?)\s*#*\s*$", line)
        if not m:
            continue
        title = re.sub(r"`([^`]*)`", r"\1", m.group(2))
        title = re.sub(r"\[([^\]]*)\]\([^)]*\)", r"\1", title)
        slug = "".join(c for c in title.lower()
                       if c in " -_" or unicodedata.category(c)[0] in "LN")
        slug = slug.replace(" ", "-")
        n = seen.get(slug, 0)
        seen[slug] = n + 1
        out.add(slug if n == 0 else f"{slug}-{n}")
    cache[path] = out
    return out

link = re.compile(r"!?\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)|^\[[^\]]+\]:\s*(\S+)|<img[^>]*\ssrc=\"([^\"]+)\"")
bad = 0
for f in sys.argv[1:]:
    fence = False
    for no, line in enumerate(open(f, encoding="utf-8"), 1):
        if line.lstrip().startswith(("```", "~~~")):
            fence = not fence
            continue
        if fence:
            continue
        line = re.sub(r"`[^`]*`", "", line)
        for m in link.finditer(line):
            target = next(g for g in m.groups() if g)
            if re.match(r"^[a-z][a-z0-9+.-]*:", target, re.I):
                continue
            path, _, frag = target.partition("#")
            dest = os.path.normpath(os.path.join(os.path.dirname(f), path)) if path else f
            if not os.path.exists(dest):
                print(f"{f}:{no}: missing {target}")
                bad += 1
            elif frag and dest.endswith(".md") and frag not in anchors(dest):
                print(f"{f}:{no}: no heading #{frag} in {dest}")
                bad += 1
sys.exit(1 if bad else 0)
EOF
