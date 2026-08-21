#!/usr/bin/env bash
# dev-prod-db-path.test.sh — assert the root Makefile's `dev-prod-db` target
# exports the same production database path the launcher would resolve on its
# own: an environment-provided KANDEV_DATABASE_PATH wins, then KANDEV_HOME_DIR,
# then $HOME/.kandev (resolveDatabasePath/resolveHomeDir in
# apps/backend/internal/launcher/constants.go).
#
# The pre-fix target assigned with `:=`, which outranks the environment in GNU
# Make. On an install whose KANDEV_HOME_DIR points outside $HOME, that silently
# pointed dev mode at the untouched $HOME/.kandev default while the launcher
# still printed "backed up production db", so the warning named a database the
# run never opened.
#
# Each case is a dry run. `-n` alone is not enough: make executes any recipe
# line mentioning $(MAKE) even under -n, so `@$(MAKE) dev` would recurse into a
# real build. Overriding MAKE swaps that line for a probe that prints the
# exported value and swallows the leftover `dev` argument on a `:` builtin. That
# neutralises the recursion and asserts the variable as the child process
# actually receives it rather than as the sibling echo renders it. `:` is used
# rather than a trailing `#` so the value carries nothing a make implementation
# might re-read as a comment.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROBE_MAKE='printenv KANDEV_DATABASE_PATH || echo UNSET; :'
status=0

# resolve <env-assignment>... — the KANDEV_DATABASE_PATH dev-prod-db exports.
resolve() {
	env "$@" make -C "$ROOT_DIR" --no-print-directory MAKE="$PROBE_MAKE" \
		-n dev-prod-db 2>/dev/null | tail -n 1
}

resolve_make_home() {
	env -u KANDEV_DATABASE_PATH make -C "$ROOT_DIR" --no-print-directory MAKE="$PROBE_MAKE" \
		-n dev-prod-db "KANDEV_HOME_DIR=$1" 2>/dev/null | tail -n 1
}

expect_eq() {
	local label=$1 want=$2 got=$3
	if [ "$got" = "$want" ]; then
		printf 'ok    %-29s %s\n' "$label" "$got"
	else
		printf 'FAIL  %-29s\n        want: %s\n        got:  %s\n' "$label" "$want" "$got"
		status=1
	fi
}

expect_suffix() {
	local label=$1 want=$2 got=$3
	# $HOME is asserted by suffix: Git Bash/MSYS rewrites HOME on its way to
	# native make, so the absolute prefix is not reproducible across shells.
	case "$got" in
	UNSET | "") ;;
	*"$want")
		printf 'ok    %-29s %s\n' "$label" "$got"
		return
		;;
	esac
	printf 'FAIL  %-29s\n        want suffix: %s\n        got:         %s\n' \
		"$label" "$want" "$got"
	status=1
}

# 1. Clean environment — the historical default under $HOME.
expect_suffix "default home" "/.kandev/data/kandev.db" \
	"$(resolve -u KANDEV_DATABASE_PATH -u KANDEV_HOME_DIR)"

# 2. Relocated home, no explicit db path — derived from KANDEV_HOME_DIR. The
#    backslash value mirrors the Windows install that surfaced the bug; make
#    joins with a forward slash, which Go accepts alongside backslashes.
expect_eq "KANDEV_HOME_DIR" 'D:\kandev-probe/data/kandev.db' \
	"$(resolve -u KANDEV_DATABASE_PATH 'KANDEV_HOME_DIR=D:\kandev-probe')"

# 3. Explicit db path — forwarded unchanged, and it outranks KANDEV_HOME_DIR.
expect_eq "KANDEV_DATABASE_PATH" 'D:\kandev-probe\data\custom.db' \
	"$(resolve 'KANDEV_DATABASE_PATH=D:\kandev-probe\data\custom.db' 'KANDEV_HOME_DIR=D:\other')"

# 4. A whitespace-only KANDEV_HOME_DIR is not a relocated install. resolveHomeDir
#    trims before testing for empty, so the Makefile must fall back to $HOME
#    rather than derive "   /data/kandev.db" from it.
expect_suffix "blank KANDEV_HOME_DIR" "/.kandev/data/kandev.db" \
	"$(resolve -u KANDEV_DATABASE_PATH 'KANDEV_HOME_DIR=   ')"

# 5. An exported-but-empty KANDEV_DATABASE_PATH must not be forwarded. Make
#    counts it as defined, so `?=` would keep the blank; the launcher trims it,
#    finds nothing, and drops to the isolated .kandev-dev db — dev-prod-db would
#    then announce production while opening the dev database.
expect_eq "empty KANDEV_DATABASE_PATH" 'D:\kandev-probe/data/kandev.db' \
	"$(resolve 'KANDEV_DATABASE_PATH=' 'KANDEV_HOME_DIR=D:\kandev-probe')"

# 6. Same for a whitespace-only override, which the launcher also trims away.
expect_eq "blank KANDEV_DATABASE_PATH" 'D:\kandev-probe/data/kandev.db' \
	"$(resolve 'KANDEV_DATABASE_PATH=   ' 'KANDEV_HOME_DIR=D:\kandev-probe')"

# 7. Internal whitespace is part of the path. $(strip …) collapses runs of
#    spaces, so it may gate the emptiness test but must not supply the value.
expect_eq "spaces inside KANDEV_HOME_DIR" 'D:\kandev  probe/data/kandev.db' \
	"$(resolve -u KANDEV_DATABASE_PATH 'KANDEV_HOME_DIR=D:\kandev  probe')"

# 8. Leading and trailing padding around a nonblank home must be removed
#    before the database suffix is appended. Otherwise the trailing spaces
#    become internal path characters and survive the launcher's TrimSpace.
expect_eq "padded KANDEV_HOME_DIR" 'D:\kandev-probe/data/kandev.db' \
	"$(resolve -u KANDEV_DATABASE_PATH 'KANDEV_HOME_DIR=  D:\kandev-probe   ')"

# 9. A Make command-line assignment must use the same trimmed home path. The
#    explicit environment database is cleared so the home remains the source.
expect_eq "Make variable home" 'D:\kandev-probe/data/kandev.db' \
	"$(resolve_make_home 'D:\kandev-probe   ')"

if [ "$status" -eq 0 ]; then
	echo "All dev-prod-db path checks passed (env db path > KANDEV_HOME_DIR > \$HOME/.kandev)."
fi
exit "$status"
