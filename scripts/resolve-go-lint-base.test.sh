#!/usr/bin/env bash
# resolve-go-lint-base.test.sh — regression coverage for the Go lint hook's
# comparison base. A fork's origin can be stale, so the resolver must prefer
# the conventional canonical upstream ref without weakening lint enforcement.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESOLVER="$ROOT_DIR/scripts/resolve-go-lint-base"
status=0

fail() {
	printf 'FAIL  %s\n' "$*"
	status=1
}

expect_eq() {
	local label=$1 want=$2 got=$3
	if [ "$got" = "$want" ]; then
		printf 'ok    %-34s %s\n' "$label" "$got"
	else
		fail "$label (want $want, got $got)"
	fi
}

expect_fail() {
	local label=$1 needle=$2 repo=$3 output
	if output=$(cd "$repo" && "$RESOLVER" 2>&1); then
		fail "$label (unexpected success: $output)"
	elif [[ "$output" == *"$needle"* ]]; then
		printf 'ok    %-34s %s\n' "$label" "$needle"
	else
		fail "$label (expected $needle in: $output)"
	fi
}

new_repo() {
	local repo
	repo=$(mktemp -d)
	git -C "$repo" init -q
	git -C "$repo" config user.email test@example.com
	git -C "$repo" config user.name 'Hook test'
	git -C "$repo" commit --allow-empty -qm initial
	git -C "$repo" branch -M main
	git -C "$repo" remote add origin https://example.test/fork.git
	git -C "$repo" update-ref refs/remotes/origin/main HEAD
	git -C "$repo" symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/main
	echo "$repo"
}

fork_repo="" single_repo="" release_repo="" configured_repo=""
dangling_repo="" ambiguous_repo="" unsafe_repo="" history_repo="" missing_repo=""
full_ref_repo="" collision_repo="" untrusted_repo="" namespace_repo=""
diverged_repo="" merge_repo=""
cleanup() {
	local repo
	for repo in "$fork_repo" "$single_repo" "$release_repo" "$configured_repo" \
		"$dangling_repo" "$ambiguous_repo" "$unsafe_repo" "$history_repo" "$missing_repo" \
		"$full_ref_repo" "$collision_repo" "$untrusted_repo" "$namespace_repo" \
		"$diverged_repo" "$merge_repo"; do
		[ -z "$repo" ] || rm -rf "$repo"
	done
}
trap cleanup EXIT

fork_repo=$(new_repo)
git -C "$fork_repo" remote add upstream git@github.com:kdlbs/kandev.git
git -C "$fork_repo" update-ref refs/remotes/upstream/main HEAD
expect_eq 'fork prefers canonical upstream' 'refs/remotes/upstream/main' "$(cd "$fork_repo" && "$RESOLVER")"

single_repo=$(new_repo)
expect_eq 'single remote uses origin default' 'refs/remotes/origin/main' "$(cd "$single_repo" && "$RESOLVER")"

release_repo=$(new_repo)
git -C "$release_repo" update-ref -d refs/remotes/origin/main
git -C "$release_repo" update-ref refs/remotes/origin/release HEAD
git -C "$release_repo" symbolic-ref refs/remotes/origin/HEAD refs/remotes/origin/release
git -C "$release_repo" remote add upstream https://github.com/kdlbs/kandev.git
git -C "$release_repo" update-ref refs/remotes/upstream/release HEAD
expect_eq 'non-main default follows upstream' 'refs/remotes/upstream/release' "$(cd "$release_repo" && "$RESOLVER")"

configured_repo=$(new_repo)
git -C "$configured_repo" remote add upstream https://github.com/kdlbs/kandev.git
git -C "$configured_repo" update-ref refs/remotes/upstream/release HEAD
git -C "$configured_repo" config kandev.lintBaseRef upstream/release
expect_eq 'configured ref is honored' 'refs/remotes/upstream/release' "$(cd "$configured_repo" && "$RESOLVER")"

dangling_repo=$(new_repo)
git -C "$dangling_repo" config kandev.lintBaseRef upstream/main
expect_fail 'missing configured ref fails closed' 'does not resolve' "$dangling_repo"

ambiguous_repo=$(new_repo)
git -C "$ambiguous_repo" config --add kandev.lintBaseRef origin/main
git -C "$ambiguous_repo" config --add kandev.lintBaseRef upstream/main
expect_fail 'multiple configured refs fail closed' 'ambiguous' "$ambiguous_repo"

missing_repo=$(new_repo)
git -C "$missing_repo" update-ref -d refs/remotes/origin/main
expect_fail 'missing default ref fails closed' 'does not resolve' "$missing_repo"

unsafe_repo=$(new_repo)
git -C "$unsafe_repo" config kandev.lintBaseRef 'origin/main --not-a-ref'
expect_fail 'unsafe configured ref is rejected' 'safe ref format' "$unsafe_repo"

history_repo=$(new_repo)
git -C "$history_repo" branch previous
git -C "$history_repo" switch -q previous
git -C "$history_repo" switch -q main
git -C "$history_repo" config kandev.lintBaseRef '@{-1}'
expect_fail 'checkout history is not a base ref' 'remote-tracking' "$history_repo"

full_ref_repo=$(new_repo)
expect_eq 'resolver emits fully qualified ref' 'refs/remotes/origin/main' "$(cd "$full_ref_repo" && "$RESOLVER")"

collision_repo=$(new_repo)
git -C "$collision_repo" tag origin/main
expect_eq 'tag collision still selects remote ref' 'refs/remotes/origin/main' "$(cd "$collision_repo" && "$RESOLVER")"

untrusted_repo=$(new_repo)
git -C "$untrusted_repo" remote add upstream https://example.test/not-kandev.git
git -C "$untrusted_repo" update-ref refs/remotes/upstream/main HEAD
expect_fail 'untrusted upstream fails closed' 'canonical' "$untrusted_repo"

namespace_repo=$(new_repo)
git -C "$namespace_repo" config kandev.lintBaseRef refs/heads/main
expect_fail 'configured local branch is rejected' 'remote-tracking' "$namespace_repo"

diverged_repo=$(new_repo)
git -C "$diverged_repo" branch feature
git -C "$diverged_repo" switch -q feature
git -C "$diverged_repo" commit --allow-empty -qm feature
git -C "$diverged_repo" switch -q main
git -C "$diverged_repo" commit --allow-empty -qm base-advance
diverged_base=$(git -C "$diverged_repo" rev-parse HEAD)
diverged_fork=$(git -C "$diverged_repo" rev-parse HEAD~1)
git -C "$diverged_repo" update-ref refs/remotes/origin/main "$diverged_base"
git -C "$diverged_repo" switch -q feature
expect_eq 'diverged history uses merge base' "$diverged_fork" "$(cd "$diverged_repo" && "$RESOLVER" --comparison-base)"

merge_repo=$(new_repo)
git -C "$merge_repo" branch feature
git -C "$merge_repo" switch -q feature
git -C "$merge_repo" commit --allow-empty -qm feature
git -C "$merge_repo" switch -q main
git -C "$merge_repo" commit --allow-empty -qm base-advance
merge_base_tip=$(git -C "$merge_repo" rev-parse HEAD)
git -C "$merge_repo" update-ref refs/remotes/origin/main "$merge_base_tip"
git -C "$merge_repo" switch -q feature
printf '%s\n' "$merge_base_tip" > "$merge_repo/.git/MERGE_HEAD"
expect_eq 'base merge keeps incoming tip' "$merge_base_tip" "$(cd "$merge_repo" && "$RESOLVER" --comparison-base)"

if [ "$status" -eq 0 ]; then
	echo 'All Go lint base resolver checks passed.'
fi
exit "$status"
