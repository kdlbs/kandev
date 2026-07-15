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

gh_output="$("$SCRIPT" gh-run -- "$TMP_DIR/success-with-go-failure")"
if grep -q '"Action":"fail"' <<<"$gh_output" && grep -q -- '--- FAIL: TestLeaky' <<<"$gh_output" && grep -q 'goleak:' <<<"$gh_output"; then
  pass "gh-run extracts Go failures from a successful command"
else
  fail "gh-run extracts Go failures from a successful command"
fi

normal_output="$("$SCRIPT" ordinary -- bash -c 'printf "ordinary success\\n"')"
if [[ "$(wc -l <<<"$normal_output" | tr -d ' ')" == "1" ]] && grep -q '^exit=0 log=' <<<"$normal_output"; then
  pass "ordinary successful tags remain compact"
else
  fail "ordinary successful tags remain compact"
fi
