#!/usr/bin/env bash
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp_dir="$(mktemp -d)"
trap 'rm -rf -- "$tmp_dir"' EXIT

fail() {
  printf 'go-cache-reuse: %s\n' "$1" >&2
  exit 1
}

make_plan() (
  cd "$1"
  unset GOFLAGS
  make -s -n "$2"
)

has_trimpath() {
  case " $1 " in
    *" -trimpath "*) return 0 ;;
    *) return 1 ;;
  esac
}

root_build_plan="$(make_plan "$repo_root" build-backend)"
backend_build_plan="$(make_plan "$repo_root/apps/backend" build)"
has_trimpath "$root_build_plan" || fail "root backend build must delegate to trimmed Go build targets"
has_trimpath "$backend_build_plan" || fail "backend build targets must pass -trimpath explicitly"

cache_dir="$tmp_dir/cache"
source_dir_a="$tmp_dir/source-a"
source_dir_b="$tmp_dir/source-b"
mkdir -p "$cache_dir" "$source_dir_a" "$source_dir_b"

write_fixture() {
  local source_dir="$1"
  cat >"$source_dir/go.mod" <<'EOF'
module example.com/cachefixture

go 1.24
EOF
  cat >"$source_dir/main.go" <<'EOF'
package main

func value() string { return "same" }

func main() { println(value()) }
EOF
  cat >"$source_dir/main_test.go" <<'EOF'
package main

import "testing"

func TestValue(t *testing.T) {
	if value() != "same" {
		t.Fatal(value())
	}
}
EOF
}

write_fixture "$source_dir_a"
write_fixture "$source_dir_b"

build_artifact() (
  cd "$1"
  GOCACHE="$cache_dir" GOFLAGS=-trimpath go list -export -buildvcs=false -f '{{.Export}} {{.BuildID}}' .
)

test_artifact() (
  cd "$1"
  GOCACHE="$cache_dir" GOFLAGS=-trimpath go list -test -export -buildvcs=false \
    -f '{{if .ForTest}}{{.Export}} {{.BuildID}}{{end}}' .
)

assert_same_artifact() {
  local label="$1" expected="$2" actual="$3"
  [[ "$expected" == "$actual" ]] || fail "$label did not reuse the same cache artifact: '$expected' != '$actual'"
}

assert_changed_artifact() {
  local label="$1" before="$2" after="$3"
  [[ "$before" != "$after" ]] || fail "$label did not invalidate after source changed: '$before'"
}

build_a="$(build_artifact "$source_dir_a")"
build_b="$(build_artifact "$source_dir_b")"
test_a="$(test_artifact "$source_dir_a")"
test_b="$(test_artifact "$source_dir_b")"
[[ -n "$test_a" && -n "$test_b" ]] || fail "test package compilation artifact was not reported"
assert_same_artifact "build" "$build_a" "$build_b"
assert_same_artifact "test compilation" "$test_a" "$test_b"

(cd "$source_dir_a" && GOCACHE="$cache_dir" GOFLAGS=-trimpath go test -buildvcs=false ./...)
(cd "$source_dir_b" && GOCACHE="$cache_dir" GOFLAGS=-trimpath go test -buildvcs=false ./...)

cat >"$source_dir_b/main_test.go" <<'EOF'
package main

import "testing"

func TestValue(t *testing.T) {
	got := value()
	if got != "same" {
		t.Fatal(got)
	}
}
EOF
test_b_after_test_edit="$(test_artifact "$source_dir_b")"
build_b_after_test_edit="$(build_artifact "$source_dir_b")"
assert_changed_artifact "test compilation" "$test_b" "$test_b_after_test_edit"
assert_same_artifact "non-test build after test-only edit" "$build_b" "$build_b_after_test_edit"
(cd "$source_dir_b" && GOCACHE="$cache_dir" GOFLAGS=-trimpath go test -buildvcs=false ./...)

cat >"$source_dir_b/main.go" <<'EOF'
package main

func value() string { return "changed" }

func main() { println(value()) }
EOF
cat >"$source_dir_b/main_test.go" <<'EOF'
package main

import "testing"

func TestValue(t *testing.T) {
	got := value()
	if got != "changed" {
		t.Fatal(got)
	}
}
EOF
build_b_after_source_edit="$(build_artifact "$source_dir_b")"
test_b_after_source_edit="$(test_artifact "$source_dir_b")"
assert_changed_artifact "build" "$build_b" "$build_b_after_source_edit"
assert_changed_artifact "test compilation after source edit" "$test_b_after_test_edit" "$test_b_after_source_edit"
(cd "$source_dir_b" && GOCACHE="$cache_dir" GOFLAGS=-trimpath go test -buildvcs=false ./...)

printf 'Go cache trimpath reuse and source invalidation checks passed.\n'
