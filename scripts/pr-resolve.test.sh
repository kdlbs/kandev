#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/pr-resolve"

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

mkdir -p "$TMP_DIR/bin"
printf '#!/usr/bin/env bash\necho "gh should not be called" >&2\nexit 99\n' >"$TMP_DIR/bin/gh"
chmod +x "$TMP_DIR/bin/gh"

run_invalid_reply() {
  PATH="$TMP_DIR/bin:$PATH" "$SCRIPT" reply 123 "$@" 2>"$TMP_DIR/stderr"
}

if PATH="$TMP_DIR/bin:$PATH" "$SCRIPT" --help >"$TMP_DIR/help" 2>"$TMP_DIR/stderr"; then
  if grep -q "scripts/pr-resolve list <PR>" "$TMP_DIR/help"; then
    pass "--help prints usage without gh"
  else
    fail "--help prints usage without gh"
  fi
else
  fail "--help exits zero"
fi

if run_invalid_reply PRRT_bad 456 body; then
  fail "thread id in comment position fails"
fi
if grep -q "review thread ID in the comment_id position" "$TMP_DIR/stderr"; then
  pass "thread id in comment position has clear error"
else
  fail "thread id in comment position has clear error"
fi

if run_invalid_reply 456 789 body; then
  fail "non-thread id in thread position fails"
fi
if grep -q "non-review-thread ID in the thread_id position" "$TMP_DIR/stderr"; then
  pass "non-thread id in thread position has clear error"
else
  fail "non-thread id in thread position has clear error"
fi

empty_file="$TMP_DIR/empty.txt"
: >"$empty_file"
if run_invalid_reply 456 PRRT_xyz --body-file "$empty_file"; then
  fail "empty body file fails"
fi
if grep -q "body file is empty" "$TMP_DIR/stderr"; then
  pass "empty body file has clear error"
else
  fail "empty body file has clear error"
fi
