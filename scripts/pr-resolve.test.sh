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

run_invalid_reopen() {
  PATH="$TMP_DIR/bin:$PATH" "$SCRIPT" reopen 123 "$@" 2>"$TMP_DIR/stderr"
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

if PATH="$TMP_DIR/bin:$PATH" "$SCRIPT" --help >"$TMP_DIR/help" 2>"$TMP_DIR/stderr"; then
  if grep -q "scripts/pr-resolve reopen <PR> <thread_id>" "$TMP_DIR/help"; then
    pass "--help documents authorized thread reopening"
  else
    fail "--help documents authorized thread reopening"
  fi
else
  fail "--help exits zero for reopen documentation"
fi

if run_invalid_reopen 456; then
  fail "non-thread id in reopen position fails"
fi
if grep -q "review thread ID as its second argument" "$TMP_DIR/stderr"; then
  pass "reopen validates the thread id"
else
  fail "reopen validates the thread id"
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

NULL_GH_DIR="$TMP_DIR/null-gh"
mkdir -p "$NULL_GH_DIR"
cat >"$NULL_GH_DIR/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "repo" && "$2" == "view" ]]; then
  printf 'kdlbs kandev\n'
  exit 0
fi
if [[ "$1" == "api" && "$2" == "graphql" ]]; then
  printf '%s\n' '{"data":{"node":null}}'
  exit 0
fi
echo "unexpected gh arguments: $*" >&2
exit 1
EOF
chmod +x "$NULL_GH_DIR/gh"

if PATH="$NULL_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_missing 2>"$TMP_DIR/stderr"; then
  fail "missing review thread fails clearly"
fi
if grep -q "review thread PRRT_missing not found on PR #123" "$TMP_DIR/stderr"; then
  pass "missing review thread includes its id and PR"
else
  fail "missing review thread includes its id and PR"
fi

REOPEN_GH_DIR="$TMP_DIR/reopen-gh"
mkdir -p "$REOPEN_GH_DIR"
cat >"$REOPEN_GH_DIR/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "repo" && "$2" == "view" ]]; then
  printf 'kdlbs kandev\n'
  exit 0
fi
if [[ "$1" == "api" && "$2" == "graphql" ]]; then
  if [[ "$*" == *"unresolveReviewThread"* ]]; then
    case "${REOPEN_SCENARIO:-resolved}" in
      mutation-failed)
        echo "mutation failed" >&2
        exit 1
        ;;
      mutation-malformed)
        printf '%s\n' '{"data":{"unresolveReviewThread":{"thread":null}}}'
        exit 0
        ;;
      *)
        printf '%s\n' '{"data":{"unresolveReviewThread":{"thread":{"isResolved":false}}}}'
        exit 0
        ;;
    esac
  fi
  case "${REOPEN_SCENARIO:-resolved}" in
    unresolved)
      printf '%s\n' '{"data":{"node":{"id":"PRRT_target","isResolved":false,"pullRequest":{"number":123,"repository":{"nameWithOwner":"kdlbs/kandev"}}}}}'
      ;;
    foreign)
      printf '%s\n' '{"data":{"node":{"id":"PRRT_target","isResolved":true,"pullRequest":{"number":123,"repository":{"nameWithOwner":"other/kandev"}}}}}'
      ;;
    *)
      printf '%s\n' '{"data":{"node":{"id":"PRRT_target","isResolved":true,"pullRequest":{"number":123,"repository":{"nameWithOwner":"kdlbs/kandev"}}}}}'
      ;;
  esac
  exit 0
fi
echo "unexpected gh arguments: $*" >&2
exit 1
EOF
chmod +x "$REOPEN_GH_DIR/gh"

if REOPEN_SCENARIO=resolved PATH="$REOPEN_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_target >"$TMP_DIR/out" 2>"$TMP_DIR/stderr" \
  && grep -q "ok PRRT_target" "$TMP_DIR/out"; then
  pass "reopen succeeds for a resolved same-repository thread"
else
  fail "reopen succeeds for a resolved same-repository thread"
fi

if REOPEN_SCENARIO=unresolved PATH="$REOPEN_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_target >"$TMP_DIR/out" 2>"$TMP_DIR/stderr"; then
  fail "reopen rejects an unresolved thread"
fi
grep -q "thread is not resolved" "$TMP_DIR/stderr" \
  && pass "reopen rejects an unresolved thread" \
  || fail "reopen rejects an unresolved thread"

if REOPEN_SCENARIO=foreign PATH="$REOPEN_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_target >"$TMP_DIR/out" 2>"$TMP_DIR/stderr"; then
  fail "reopen rejects a same-number thread from another repository"
fi
grep -q "thread belongs to repository other/kandev" "$TMP_DIR/stderr" \
  && pass "reopen rejects a same-number thread from another repository" \
  || fail "reopen rejects a same-number thread from another repository"

if REOPEN_SCENARIO=mutation-failed PATH="$REOPEN_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_target >"$TMP_DIR/out" 2>"$TMP_DIR/stderr"; then
  fail "reopen reports a failed mutation"
fi
grep -q "failed to reopen review thread PRRT_target on PR #123" "$TMP_DIR/stderr" \
  && pass "reopen reports a failed mutation" \
  || fail "reopen reports a failed mutation"

if REOPEN_SCENARIO=mutation-malformed PATH="$REOPEN_GH_DIR:$PATH" "$SCRIPT" reopen 123 PRRT_target >"$TMP_DIR/out" 2>"$TMP_DIR/stderr"; then
  fail "reopen reports a malformed mutation response"
fi
grep -q "invalid reopen response for review thread PRRT_target on PR #123" "$TMP_DIR/stderr" \
  && pass "reopen reports a malformed mutation response" \
  || fail "reopen reports a malformed mutation response"

if run_invalid_reply 456 PRRT_xyz --body-file; then
  fail "missing body file path fails"
fi
if grep -q "requires a path" "$TMP_DIR/stderr"; then
  pass "missing body file path has clear error"
else
  fail "missing body file path has clear error"
fi
