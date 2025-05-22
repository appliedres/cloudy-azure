#!/usr/bin/env bash
set -euo pipefail

# ─── CONFIG ────────────────────────────────────────────────────────────────
# Add folder names here (leave the array empty to skip nothing)
EXCLUDE_DIRS=()

# ─── BUILD THE FIND EXPRESSION ─────────────────────────────────────────────
#   find . \( -path './dir1' -o -path './dir2' \) -prune -o -name '*.go' -type f -print
find_expr=(find .)

if ((${#EXCLUDE_DIRS[@]})); then
  find_expr+=( \( )
  first=true
  for d in "${EXCLUDE_DIRS[@]}"; do
    $first || find_expr+=( -o )
    find_expr+=( -path "./$d" )
    first=false
  done
  find_expr+=( \) -prune -o )
fi

find_expr+=( -type f -name '*.go' -print )

# ─── HELPERS ───────────────────────────────────────────────────────────────
get_unformatted() {
  "${find_expr[@]}" | xargs --no-run-if-empty gofmt -l
}

# ─── MAIN ──────────────────────────────────────────────────────────────────
UNFMT=$(get_unformatted)
COUNT=$(echo "$UNFMT" | grep -c . || true)

if ((COUNT)); then
  echo "gofmt found $COUNT unformatted file(s):"
  echo "$UNFMT"
  read -p "Fix them now? (y/n): " ans
  if [[ $ans =~ ^[Yy]$ ]]; then
    while [ -n "$UNFMT" ]; do
      echo "$UNFMT" | xargs --no-run-if-empty gofmt -w
      UNFMT=$(get_unformatted)
    done
    echo "All Go files are properly formatted."
  else
    echo "No changes made."
    exit 1
  fi
else
  echo "All Go files are properly formatted."
fi
