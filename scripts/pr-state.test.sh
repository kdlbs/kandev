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

if [[ "${GH_FAIL_PR_VIEW:-0}" == "1" && "$1" == "pr" && "$2" == "view" ]]; then
  echo "pr view failed" >&2
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
      "body": "<!-- walkthrough_start -->",
      "createdAt": "2026-06-01T10:00:00Z"
    },
    {
      "author": { "login": "github-actions" },
      "body": "**Claude finished review**\\n| Blocker | 1 |\\n| Suggestion | 2 |\\n**Verdict:** Ready with suggestions",
      "createdAt": "2026-06-01T13:00:00Z",
      "url": "https://github.com/kdlbs/kandev/pull/123#issuecomment-2"
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
    },
    {
      "__typename": "StatusContext",
      "context": "external pending",
      "state": "PENDING",
      "targetUrl": "https://ci.example.test/build/1"
    }
  ]
}
JSON
  exit 0
fi

if [[ "$1" == "api" && "$2" == "--paginate" && "$3" == "-X" && "$4" == "GET" && "$5" == "repos/kdlbs/kandev/pulls/123/reviews" ]]; then
  printf '%s\n' '[
    {
      "user": { "login": "greptile-apps[bot]" },
      "state": "COMMENTED",
      "submitted_at": "2026-06-01T10:30:00Z"
    }
  ]'
  printf '%s\n' '[
    {
      "user": { "login": "cubic-dev-ai[bot]" },
      "state": "CHANGES_REQUESTED",
      "submitted_at": "2026-06-01T13:30:00Z"
    }
  ]'
  exit 0
fi

if [[ "$1" == "api" && "$2" == "graphql" ]]; then
  if [[ "$*" == *"headRefOid"* ]]; then
    printf '%s\n' '{
      "data": {
        "repository": {
          "pullRequest": {
            "headRefOid": "abc123",
            "commits": {
              "nodes": [
                {
                  "commit": {
                    "oid": "abc123",
                    "committedDate": "2026-06-01T12:00:00Z"
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
                        "createdAt": "2026-06-01T13:00:00Z",
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
                        "createdAt": "2026-06-01T14:00:00Z",
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
                        "createdAt": "2026-06-01T10:00:00Z",
                        "author": { "login": "greptile-apps[bot]" },
                        "body": "Please rename this helper"
                      },
                      {
                        "databaseId": 112,
                        "createdAt": "2026-06-01T13:00:00Z",
                        "author": { "login": "greptile-apps[bot]" },
                        "body": "Please rename this helper after the latest commit"
                      }
                    ]
                  }
              },
              {
                "id": "PRRT_2",
                "isResolved": false,
                "path": "apps/web/other.ts",
                "comments": {
                  "nodes": [
                      {
                        "databaseId": 222,
                        "createdAt": "2026-06-01T11:00:00Z",
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
  assert_jq "since timestamp" '.since.committed_at == "2026-06-01T12:00:00Z"' "$json"
  assert_jq "checks count" '.checks | length == 4' "$json"
  assert_jq "failed check preserved" '.checks[] | select(.name == "e2e") | .conclusion == "failure"' "$json"
  assert_jq "check run id" '.checks[] | select(.name == "e2e") | .run_id == "27340000002"' "$json"
  assert_jq "nested workflow name" '.checks[] | select(.name == "e2e") | .workflow == "CI"' "$json"
  assert_jq "pending status context conclusion normalized" '.checks[] | select(.name == "external pending") | .status == "pending" and .conclusion == null' "$json"
  assert_jq "threads count" '.review_threads | length == 1' "$json"
  assert_jq "total unresolved count includes historical unresolved thread" '.unresolved_review_thread_count == 2' "$json"
  assert_jq "filtered thread count" '.filtered_review_thread_count == 1' "$json"
  assert_jq "thread comment id" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_id == 112' "$json"
  assert_jq "thread comment timestamp" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_created_at == "2026-06-01T13:00:00Z"' "$json"
  assert_jq "reviews count" '.reviews | length == 1' "$json"
  assert_jq "review author" '.reviews[] | select(.author == "cubic-dev-ai[bot]") | .author == "cubic-dev-ai[bot]"' "$json"
  assert_jq "issue comments count" '.issue_comments | length == 1' "$json"
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
  assert_jq "checks still present" '.checks | length == 4' "$json"
  assert_jq "new issue comments still present" '.issue_comments | length == 1' "$json"
  assert_jq "partial failure recorded" '.errors | length == 1' "$json"
  assert_jq "partial failure source" '.errors[0].source == "reviews"' "$json"
  pass "partial failure records error but keeps other data"
}

test_pr_view_failure_with_non_numeric_ref_keeps_schema() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_PR_VIEW=1 PATH="$tmp/bin:$PATH" "$SCRIPT" feat/pr-state >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "pr number is null on ref-based invocation" '.pr.number == null' "$json"
  assert_jq "pr branch preserved" '.pr.branch == "feat/pr-state"' "$json"
  assert_jq "pr_view error recorded" '.errors[] | select(.source == "pr_view") | .message == "gh pr view failed"' "$json"
  pass "pr_view failure keeps schema stable for non-numeric refs"
}

test_repo_failure_skips_review_threads() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_REPO=1 GH_NO_PR_URL=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "checks still present on repo failure" '.checks | length == 4' "$json"
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

  assert_jq "checks still present on graphql failure" '.checks | length == 4' "$json"
  assert_jq "reviews still present on graphql failure" '.reviews | length == 2' "$json"
  assert_jq "review threads empty on graphql failure" '.review_threads == []' "$json"
  assert_jq "unresolved count unknown on graphql failure" '.unresolved_review_thread_count == null' "$json"
  assert_jq "graphql failure records both errors" '.errors | length == 2' "$json"
  assert_jq "graphql failure records since error" '.errors[] | select(.source == "since") | .message == "gh api graphql head commit failed; including historical comments"' "$json"
  assert_jq "graphql failure records review_threads error" '.errors[] | select(.source == "review_threads") | .message == "gh api graphql reviewThreads failed"' "$json"
  assert_jq "since fallback includes all historical comments" '.issue_comments | length == 2' "$json"
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
  assert_jq "paginated filtered thread count" '.filtered_review_thread_count == 2' "$json"
  assert_jq "paginated no errors" '.errors == []' "$json"
  pass "graphql pagination collects all threads"
}

test_all_flag_includes_historical_comments_and_reviews() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  PATH="$tmp/bin:$PATH" "$SCRIPT" --all 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "since omitted in all mode" '.since == null' "$json"
  assert_jq "all issue comments present" '.issue_comments | length == 2' "$json"
  assert_jq "all reviews present" '.reviews | length == 2' "$json"
  assert_jq "all review threads present" '.review_threads | length == 2' "$json"
  assert_jq "all mode keeps historical thread comment" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_id == 111' "$json"
  pass "--all includes historical comments and reviews"
}

test_summary_mode_returns_compact_fixup_state() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  PATH="$tmp/bin:$PATH" "$SCRIPT" --summary 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "summary keeps pr" '.pr.number == 123' "$json"
  assert_jq "summary keeps since" '.since.committed_at == "2026-06-01T12:00:00Z"' "$json"
  assert_jq "summary failed check count" '.failed_checks | length == 1' "$json"
  assert_jq "summary failed check" '.failed_checks[0] | .name == "e2e" and .conclusion == "failure" and .run_id == "27340000002"' "$json"
  assert_jq "summary pending check count" '.pending_checks | length == 2' "$json"
  assert_jq "summary pending check" '.pending_checks[0] | .name == "claude-review" and .status == "in_progress" and .run_id == "27340000003"' "$json"
  assert_jq "summary pending status context keeps target url" '.pending_checks[] | select(.name == "external pending") | .status == "pending" and .details_url == null and .target_url == "https://ci.example.test/build/1"' "$json"
  assert_jq "summary unresolved count" '.unresolved_review_thread_count == 2' "$json"
  assert_jq "summary filtered thread count" '.filtered_review_thread_count == 1' "$json"
  assert_jq "summary unresolved threads" '.unresolved_threads | length == 1' "$json"
  assert_jq "summary unresolved thread fields" '.unresolved_threads[0] | .thread_id == "PRRT_1" and .comment_id == 112 and .author == "greptile-apps[bot]"' "$json"
  assert_jq "summary hidden unresolved threads" '.hidden_unresolved_threads | length == 1' "$json"
  assert_jq "summary hidden unresolved thread fields" '.hidden_unresolved_threads[0] | .thread_id == "PRRT_2" and .comment_id == 222 and .author == "cubic-dev-ai[bot]"' "$json"
  assert_jq "summary omits raw arrays" 'has("checks") | not' "$json"
  assert_jq "summary no errors" '.errors == []' "$json"
  pass "--summary returns compact fixup state"
}

test_summary_all_flag_includes_historical_unresolved_threads() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --all 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "summary all since omitted" '.since == null' "$json"
  assert_jq "summary all unresolved count" '.unresolved_review_thread_count == 2' "$json"
  assert_jq "summary all filtered thread count" '.filtered_review_thread_count == 2' "$json"
  assert_jq "summary all keeps historical first comment" '.unresolved_threads[] | select(.thread_id == "PRRT_1") | .comment_id == 111' "$json"
  assert_jq "summary all unresolved threads count" '.unresolved_threads | length == 2' "$json"
  assert_jq "summary all has no hidden unresolved threads" '.hidden_unresolved_threads == []' "$json"
  pass "--summary --all includes historical unresolved thread comments"
}

test_snapshot_happy_path
test_partial_failure_records_error_but_keeps_other_data
test_pr_view_failure_with_non_numeric_ref_keeps_schema
test_repo_failure_skips_review_threads
test_graphql_failure_records_error_but_keeps_other_data
test_graphql_pagination_collects_all_threads
test_all_flag_includes_historical_comments_and_reviews
test_summary_mode_returns_compact_fixup_state
test_summary_all_flag_includes_historical_unresolved_threads
