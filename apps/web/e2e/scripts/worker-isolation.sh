#!/usr/bin/env bash
# Called before managed build or raw Playwright preparation.
worker_isolation_dispatch() {
  local entry=$1
  shift
  local arg next= expect_one=0 expect_project=0
  [[ -z ${KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM:-} ]] || { echo 'Unsafe browser parallelism is unsupported in isolation' >&2; return 2; }
  for arg in "$@"; do
    if [[ $expect_one == 1 ]]; then
      [[ $arg == 1 ]] || { echo 'Isolated browser checks require one worker and shard' >&2; return 2; }
      expect_one=0
    fi
    if [[ $expect_project == 1 ]]; then
      case "$arg" in containers|docker|kubernetes-compat) echo 'Daemon-dependent project cannot run in validation isolation' >&2; return 2;; esac
      expect_project=0
    fi
    case "$arg" in
      --workers|--shards|-j) expect_one=1;;
      --workers=*|--shards=*|-j=*) [[ ${arg#*=} == 1 ]] || { echo 'Isolated browser checks require one worker and shard' >&2; return 2; };;
      -j?*) [[ $arg == -j1 ]] || { echo 'Isolated browser checks require one worker' >&2; return 2; };;
      --project) expect_project=1;;
      --project=containers|--project=docker|--project=kubernetes-compat|--docker|tests/docker/*|tests/ssh/*|tests/kubernetes/*|e2e/tests/docker/*|e2e/tests/ssh/*|e2e/tests/kubernetes/*)
        echo 'Daemon-dependent browser checks cannot run in validation isolation' >&2; return 2;;
    esac
  done
  [[ $expect_one == 0 && $expect_project == 0 ]] || { echo 'Missing browser option value' >&2; return 2; }
  [[ ${FULL_WORKER_CHECK_INSIDE:-} != 1 ]] || return 0
  local root
  root=$(cd "$SCRIPT_DIR/../../../.." && pwd)
  if [[ $entry == run-e2e.sh ]]; then
    if [[ ${npm_lifecycle_event:-} == e2e:run && ${1:-} == -- ]]; then shift; fi
    exec "$root/scripts/worker-check" --kind browser -- bash "$SCRIPT_DIR/$entry" --host "$@"
  fi
  exec "$root/scripts/worker-check" --kind browser -- bash "$SCRIPT_DIR/$entry" "$@"
}
