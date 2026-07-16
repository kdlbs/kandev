#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/run-quiet"
TMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

cat >"$TMP_DIR/success-with-go-failure" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '{"Time":"2026-07-15T19:00:00Z","Action":"fail","Package":"example.com/project/pkg","Test":"TestLeaky"}'
printf '%s\n' '--- FAIL: TestLeaky (0.00s)'
printf '%s\n' 'goleak: Errors on successful test run: found unexpected goroutines'
EOF
chmod +x "$TMP_DIR/success-with-go-failure"

for tag in gh-run ci gh-run-view; do
  gh_output="$("$SCRIPT" "$tag" -- "$TMP_DIR/success-with-go-failure")"
  if grep -q '"Action":"fail"' <<<"$gh_output" && grep -q -- '--- FAIL: TestLeaky' <<<"$gh_output" && grep -q 'goleak:' <<<"$gh_output"; then
    pass "$tag extracts Go failures from a successful command"
  else
    fail "$tag extracts Go failures from a successful command"
  fi
done

clean_output="$("$SCRIPT" gh-run -- bash -c 'printf "everything is fine\\n"')"
if [[ "$(wc -l <<<"$clean_output" | tr -d ' ')" == "1" ]] && grep -q '^exit=0 log=' <<<"$clean_output"; then
  pass "gh-run keeps clean successful output compact"
else
  fail "gh-run keeps clean successful output compact"
fi

if failing_output="$("$SCRIPT" gh-run -- bash -c 'printf "%s\\n" "--- FAIL: TestBroken"; exit 1')"; then
  fail "gh-run preserves a nonzero command exit"
elif grep -q -- '--- FAIL: TestBroken' <<<"$failing_output"; then
  pass "gh-run extracts failures from a nonzero command"
else
  fail "gh-run extracts failures from a nonzero command"
fi

normal_output="$("$SCRIPT" ordinary -- bash -c 'printf "ordinary success\\n"')"
if [[ "$(wc -l <<<"$normal_output" | tr -d ' ')" == "1" ]] && grep -q '^exit=0 log=' <<<"$normal_output"; then
  pass "ordinary successful tags remain compact"
else
  fail "ordinary successful tags remain compact"
fi
