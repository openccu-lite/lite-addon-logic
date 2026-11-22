#!/bin/sh
#
#   The Go test coverage, measured and held to a floor: runs the tests of every package with a
#   coverage profile (the programs in cmd/ and the fake system for the tests are not counted),
#   prints the figure per package and the total, and fails when the total is below the floor in
#   scripts/coverage-floor (percent). COVER_OUT names the profile (build/cover.out by default).
#
set -e
cd "$(dirname "$0")/.."
GO=${GO:-go}
OUT=${COVER_OUT:-build/cover.out}
mkdir -p "$(dirname "$OUT")"
PKGS=$($GO list ./... | grep -v -e '/cmd/' -e '/internal/fakesys$')
$GO test -count=1 -covermode=atomic -coverprofile="$OUT" $PKGS
TOTAL=$($GO tool cover -func="$OUT" | awk '/^total:/ {sub("%","",$3); print $3}')
FLOOR=$(cat scripts/coverage-floor)
echo "Go coverage: $TOTAL % (floor $FLOOR %)"
awk -v t="$TOTAL" -v f="$FLOOR" 'BEGIN { exit (t + 0 < f + 0) ? 1 : 0 }' || { echo "coverage $TOTAL % is below the floor of $FLOOR %"; exit 1; }
