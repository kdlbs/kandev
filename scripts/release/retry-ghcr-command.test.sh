#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/release/retry-ghcr-command.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

cat >"$TMP_DIR/transient-command" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

COUNT_FILE="${COUNT_FILE:?}"
count=0
if [[ -f "$COUNT_FILE" ]]; then
  count="$(<"$COUNT_FILE")"
fi
count=$((count + 1))
printf '%s\n' "$count" >"$COUNT_FILE"

if ((count < 3)); then
  echo 'denied: Error from intermediary with HTTP status code 403 "Forbidden"'
  echo 'You have exceeded a secondary rate limit.'
  exit 17
fi

echo 'success'
EOF

cat >"$TMP_DIR/permanent-command" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo 'permission_denied: insufficient package permission'
exit 23
EOF

cat >"$TMP_DIR/persistent-command" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo 'You have exceeded a secondary rate limit.'
exit 31
EOF

cat >"$TMP_DIR/sleep" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$1" >>"${SLEEP_LOG:?}"
EOF

chmod +x \
  "$TMP_DIR/transient-command" \
  "$TMP_DIR/permanent-command" \
  "$TMP_DIR/persistent-command" \
  "$TMP_DIR/sleep"

COUNT_FILE="$TMP_DIR/transient-count" \
SLEEP_LOG="$TMP_DIR/transient-sleeps" \
RETRY_GHCR_SLEEP_COMMAND="$TMP_DIR/sleep" \
RETRY_GHCR_MAX_ATTEMPTS=4 \
RETRY_GHCR_INITIAL_DELAY_SECONDS=60 \
  bash "$SCRIPT" "$TMP_DIR/transient-command" >"$TMP_DIR/transient-output"

grep -q '^success$' "$TMP_DIR/transient-output" || \
  fail "transient failure was not retried to success"
[[ "$(<"$TMP_DIR/transient-count")" == "3" ]] || \
  fail "expected two failed attempts before success"
diff -u <(printf '60\n120\n') "$TMP_DIR/transient-sleeps" || \
  fail "retry delays did not use exponential backoff"

set +e
RETRY_GHCR_MAX_ATTEMPTS=4 \
RETRY_GHCR_INITIAL_DELAY_SECONDS=0 \
  bash "$SCRIPT" "$TMP_DIR/persistent-command" >"$TMP_DIR/persistent-output" 2>"$TMP_DIR/persistent-error"
persistent_status="$?"
set -e
[[ "$persistent_status" -eq 31 ]] || \
  fail "persistent transient failure did not preserve the command status"
[[ "$(wc -l <"$TMP_DIR/persistent-output")" -eq 4 ]] || \
  fail "persistent transient failure did not use all four attempts"

set +e
RETRY_GHCR_SLEEP_COMMAND="$TMP_DIR/sleep" \
RETRY_GHCR_MAX_ATTEMPTS=4 \
RETRY_GHCR_INITIAL_DELAY_SECONDS=0 \
  bash "$SCRIPT" "$TMP_DIR/permanent-command" >"$TMP_DIR/permanent-output" 2>"$TMP_DIR/permanent-error"
permanent_status="$?"
set -e
if [[ "$permanent_status" -ne 23 ]]; then
  fail "permanent failure did not preserve the command status"
fi

if grep -q 'retrying' "$TMP_DIR/permanent-error"; then
  fail "permanent failure unexpectedly succeeded"
fi

echo "PASS: GHCR retry helper retries secondary limits with bounded backoff"
