#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPT="$ROOT_DIR/scripts/pr-state"

pass() {
  printf 'ok - %s\n' "$1"
}

fail() {
  printf 'not ok - %s\n' "$1" >&2
  exit 1
}

TMP_DIRS=()

cleanup() {
  if [ "${#TMP_DIRS[@]}" -gt 0 ]; then
    rm -rf "${TMP_DIRS[@]}"
  fi
}
trap cleanup EXIT

make_tmp_dir() {
  local __out=$1
  local __tmp
  __tmp="$(mktemp -d)"
  TMP_DIRS+=("$__tmp")
  printf -v "$__out" '%s' "$__tmp"
}

assert_jq() {
  local name=$1
  local expr=$2
  local json=$3
  jq -e "$expr" <<<"$json" >/dev/null || fail "$name"
}

make_mock_gh() {
  local dir=$1
  mkdir -p "$dir"
  cat >"$dir/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${GH_FAIL_REVIEWS:-0}" == "1" && "$*" == *"pulls/123/reviews"* ]]; then
  echo "reviews api failed" >&2
  exit 1
fi

if [[ "${GH_FAIL_GRAPHQL:-0}" == "1" && "$1" == "api" && "$2" == "graphql" ]]; then
  echo "graphql failed" >&2
  exit 1
fi

if [[ "${GH_FAIL_REPO:-0}" == "1" && "$1" == "repo" && "$2" == "view" ]]; then
  echo "repo view failed" >&2
  exit 1
fi

if [[ "$1" == "repo" && "$2" == "view" ]]; then
  printf '{"owner":{"login":"kdlbs"},"name":"kandev"}\n'
  exit 0
fi

if [[ "$1" == "pr" && "$2" == "view" && "$4" == "--json" ]]; then
  if [[ "${GH_NO_PR_URL:-0}" == "1" ]]; then
    pr_url='null'
  else
    pr_url='"https://github.com/kdlbs/kandev/pull/123"'
  fi
  cat <<JSON
{
  "number": 123,
  "headRefName": "feat/pr-state",
  "url": $pr_url,
  "comments": [
    {
      "author": { "login": "coderabbitai" },
      "body": "<!-- walkthrough_start -->"
    },
    {
      "author": { "login": "github-actions" },
      "body": "**Claude finished review**\\n| Blocker | 1 |\\n| Suggestion | 2 |\\n**Verdict:** Ready with suggestions"
    }
  ],
  "statusCheckRollup": [
    {
      "__typename": "CheckRun",
      "name": "web lint",
      "status": "COMPLETED",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000001/job/55150000001"
    },
    {
      "__typename": "CheckRun",
      "name": "e2e",
      "status": "COMPLETED",
      "conclusion": "FAILURE",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000002/job/55150000002",
      "checkSuite": {
        "workflowRun": {
          "workflow": {
            "name": "CI"
          }
        }
      }
    },
    {
      "__typename": "CheckRun",
      "name": "claude-review",
      "status": "IN_PROGRESS",
      "conclusion": "",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000003/job/55150000003"
    }
  ]
}
JSON
  exit 0
fi

if [[ "$1" == "api" && "$2" == "--paginate" && "$3" == "-X" && "$4" == "GET" && "$5" == "repos/kdlbs/kandev/pulls/123/reviews" ]]; then
  printf '%s\n' '[
    { "user": { "login": "greptile-apps[bot]" } }
  ]'
  printf '%s\n' '[
    { "user": { "login": "cubic-dev-ai[bot]" } }
  ]'
  exit 0
fi

if [[ "$1" == "api" && "$2" == "graphql" ]]; then
  if [[ "${GH_GRAPHQL_TWO_PAGES:-0}" == "1" && "$*" != *"cursor=CURSOR1"* ]]; then
    printf '%s\n' '{
      "data": {
        "repository": {
          "pullRequest": {
            "reviewThreads": {
              "pageInfo": {
                "hasNextPage": true,
                "endCursor": "CURSOR1"
              },
              "nodes": [
                {
                  "id": "PRRT_1",
                  "isResolved": false,
                  "path": "apps/web/file.ts",
                  "comments": {
                    "nodes": [
                      {
                        "databaseId": 111,
                        "author": { "login": "greptile-apps[bot]" },
                        "body": "Please rename this helper"
                      }
                    ]
                  }
                }
              ]
            }
          }
        }
      }
    }'
    exit 0
  fi

  if [[ "${GH_GRAPHQL_TWO_PAGES:-0}" == "1" && "$*" == *"cursor=CURSOR1"* ]]; then
    printf '%s\n' '{
      "data": {
        "repository": {
          "pullRequest": {
            "reviewThreads": {
              "pageInfo": {
                "hasNextPage": false,
                "endCursor": null
              },
              "nodes": [
                {
                  "id": "PRRT_2",
                  "isResolved": true,
                  "path": "apps/web/other.ts",
                  "comments": {
                    "nodes": [
                      {
                        "databaseId": 222,
                        "author": { "login": "cubic-dev-ai[bot]" },
                        "body": "resolved"
                      }
                    ]
                  }
                }
              ]
            }
          }
        }
      }
    }'
    exit 0
  fi

  printf '%s\n' '{
    "data": {
      "repository": {
        "pullRequest": {
          "reviewThreads": {
            "pageInfo": {
              "hasNextPage": false,
              "endCursor": null
            },
            "nodes": [
              {
                "id": "PRRT_1",
                "isResolved": false,
                "path": "apps/web/file.ts",
                "comments": {
                  "nodes": [
                    {
                      "databaseId": 111,
                      "author": { "login": "greptile-apps[bot]" },
                      "body": "Please rename this helper"
                    }
                  ]
                }
              },
              {
                "id": "PRRT_2",
                "isResolved": true,
                "path": "apps/web/other.ts",
                "comments": {
                  "nodes": [
                    {
                      "databaseId": 222,
                      "author": { "login": "cubic-dev-ai[bot]" },
                      "body": "resolved"
                    }
                  ]
                }
              }
            ]
          }
        }
      }
    }
  }'
  exit 0
fi

echo "unexpected gh args: $*" >&2
exit 1
EOF
  chmod +x "$dir/gh"
}

test_snapshot_happy_path() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "pr number" '.pr.number == 123' "$json"
  assert_jq "branch" '.pr.branch == "feat/pr-state"' "$json"
  assert_jq "checks count" '.checks | length == 3' "$json"
  assert_jq "failed check preserved" '.checks[] | select(.name == "e2e") | .conclusion == "failure"' "$json"
  assert_jq "check run id" '.checks[] | select(.name == "e2e") | .run_id == "27340000002"' "$json"
  assert_jq "nested workflow name" '.checks[] | select(.name == "e2e") | .workflow == "CI"' "$json"
  assert_jq "threads count" '.review_threads | length == 2' "$json"
  assert_jq "unresolved count" '.unresolved_review_thread_count == 1' "$json"
  assert_jq "thread comment id" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_id == 111' "$json"
  assert_jq "thread resolved field" '.review_threads[] | select(.thread_id == "PRRT_2") | .is_resolved == true' "$json"
  assert_jq "reviews count" '.reviews | length == 2' "$json"
  assert_jq "review author" '.reviews[] | select(.author == "greptile-apps[bot]") | .author == "greptile-apps[bot]"' "$json"
  assert_jq "issue comments count" '.issue_comments | length == 2' "$json"
  assert_jq "issue comment author" '.issue_comments[] | select(.author == "github-actions") | .body | contains("Verdict")' "$json"
  assert_jq "no errors" '.errors == []' "$json"
  pass "snapshot happy path"
}

test_partial_failure_records_error_but_keeps_other_data() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_REVIEWS=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "reviews empty on failure" '.reviews == []' "$json"
  assert_jq "checks still present" '.checks | length == 3' "$json"
  assert_jq "issue comments still present" '.issue_comments | length == 2' "$json"
  assert_jq "partial failure recorded" '.errors | length == 1' "$json"
  assert_jq "partial failure source" '.errors[0].source == "reviews"' "$json"
  pass "partial failure records error but keeps other data"
}

test_repo_failure_skips_review_threads() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_REPO=1 GH_NO_PR_URL=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "checks still present on repo failure" '.checks | length == 3' "$json"
  assert_jq "review threads empty on repo failure" '.review_threads == []' "$json"
  assert_jq "unresolved count unknown on repo failure" '.unresolved_review_thread_count == null' "$json"
  assert_jq "repo failure recorded" '.errors[] | select(.source == "repo") | .message == "gh repo view failed"' "$json"
  assert_jq "review thread skip recorded" '.errors[] | select(.source == "review_threads") | .message == "skipped: repo lookup failed"' "$json"
  pass "repo failure skips review threads"
}

test_graphql_failure_records_error_but_keeps_other_data() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_GRAPHQL=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "checks still present on graphql failure" '.checks | length == 3' "$json"
  assert_jq "reviews still present on graphql failure" '.reviews | length == 2' "$json"
  assert_jq "review threads empty on graphql failure" '.review_threads == []' "$json"
  assert_jq "unresolved count unknown on graphql failure" '.unresolved_review_thread_count == null' "$json"
  assert_jq "graphql failure recorded" '.errors[] | select(.source == "review_threads") | .message == "gh api graphql reviewThreads failed"' "$json"
  pass "graphql failure records error but keeps other data"
}

test_graphql_pagination_collects_all_threads() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_GRAPHQL_TWO_PAGES=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "paginated threads count" '.review_threads | length == 2' "$json"
  assert_jq "paginated first thread" '.review_threads[] | select(.thread_id == "PRRT_1") | .is_resolved == false' "$json"
  assert_jq "paginated second thread" '.review_threads[] | select(.thread_id == "PRRT_2") | .is_resolved == true' "$json"
  assert_jq "paginated unresolved count" '.unresolved_review_thread_count == 1' "$json"
  assert_jq "paginated no errors" '.errors == []' "$json"
  pass "graphql pagination collects all threads"
}

test_snapshot_happy_path
test_partial_failure_records_error_but_keeps_other_data
test_repo_failure_skips_review_threads
test_graphql_failure_records_error_but_keeps_other_data
test_graphql_pagination_collects_all_threads
