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

if [[ "${GH_FAIL_COMMENT:-0}" == "1" && "$1" == "api" && "$2" == repos/kdlbs/kandev/pulls/comments/* ]]; then
  echo "comment api failed" >&2
  exit 1
fi

if [[ "$1" == "repo" && "$2" == "view" ]]; then
  printf '{"owner":{"login":"kdlbs"},"name":"kandev"}\n'
  exit 0
fi

if [[ "$1" == "api" && "$2" == "repos/kdlbs/kandev/pulls/comments/111" ]]; then
  cat <<'JSON'
{
  "id": 111,
  "user": { "login": "greptile-apps[bot]" },
  "body": "Please rename this helper\n\nFull rationale here.",
  "path": "apps/web/file.ts",
  "line": 42,
  "original_line": 40,
  "commit_id": "abc123",
  "html_url": "https://github.com/kdlbs/kandev/pull/123#discussion_r111",
  "created_at": "2026-06-01T10:00:00Z",
  "updated_at": "2026-06-01T10:01:00Z"
}
JSON
  exit 0
fi

if [[ "$*" == *"actions/workflows/opencode-code-review.yml/runs"* ]]; then
  if [[ "${GH_FAIL_TRUSTED_WORKFLOW:-0}" == "1" ]]; then
    echo "workflow api failed" >&2
    exit 1
  fi
  if [[ "${GH_HEAD_SEQUENCE:-stable}" == "after-workflow" ]]; then
    : >"${GH_HEAD_COUNTER_FILE:?}"
  fi
  if [[ "${GH_TRUSTED_MULTI:-0}" == "1" ]]; then
    cat <<'JSON'
{"workflow_runs":[{"id":901,"path":".github/workflows/opencode-code-review.yml","event":"pull_request_target","status":"completed","conclusion":"failure","run_number":11,"run_attempt":1,"pull_requests":[{"number":123,"head":{"sha":"abc123"}}]},{"id":900,"path":".github/workflows/opencode-code-review.yml","event":"pull_request_target","status":"completed","conclusion":"success","run_number":10,"run_attempt":2,"pull_requests":[{"number":123,"head":{"sha":"abc123"}}]}]}
JSON
    exit 0
  fi
  conclusion="${GH_TRUSTED_WORKFLOW_CONCLUSION:-success}"
  status="${GH_TRUSTED_WORKFLOW_STATUS:-completed}"
  cat <<JSON
{"workflow_runs":[{"id":900,"path":".github/workflows/opencode-code-review.yml","event":"pull_request_target","status":"$status","conclusion":"$conclusion","run_number":10,"run_attempt":2,"pull_requests":[{"number":123,"head":{"sha":"abc123"}}]}]}
JSON
  exit 0
fi

if [[ "$*" == *"actions/runs/900/attempts/2/jobs"* ]]; then
  cat <<'JSON'
{"jobs":[{"name":"opencode-review-same-repo","status":"completed","conclusion":"success"}]}
JSON
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
  "headRefOid": "${GH_CHECKS_HEAD:-abc123}",
  "isCrossRepository": ${GH_CROSS_REPOSITORY:-false},
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
    },
    {
      "author": { "login": "github-actions" },
      "body": "<!-- opencode-review:no-findings -->\\n**OpenCode review complete**\\n\\nOpenCode model: \`opencode-go/minimax-m3\`\\n\\nNo suggestions found for commit \`abc1234\`.",
      "createdAt": "2026-06-01T13:01:00Z",
      "url": "https://github.com/kdlbs/kandev/pull/123#issuecomment-3"
    },
    {
      "author": { "login": "other-bot[bot]" },
      "body": "<!-- opencode-review:no-findings -->\\n**OpenCode review complete**\\n\\nOpenCode model: \`opencode-go/minimax-m3\`\\n\\nNo suggestions found for commit \`abc1234\`.",
      "createdAt": "2026-06-01T13:02:00Z"
    },
    {
      "author": { "login": "github-actions" },
      "body": "Review quotes <!-- opencode-review:no-findings --> later",
      "createdAt": "2026-06-01T13:03:00Z"
    },
    {
      "author": { "login": "github-actions" },
      "body": "<!-- opencode-review:no-findings -->\\n**OpenCode review complete**\\nBlocker: real issue",
      "createdAt": "2026-06-01T13:04:00Z"
    },
    {
      "author": { "login": "github-actions" },
      "body": "<!-- opencode-review:diagnostic --> failure",
      "createdAt": "2026-06-01T13:05:00Z"
    },
    {
      "author": { "login": "github-actions" },
      "body": "<!-- opencode-review:fallback-findings --> finding",
      "createdAt": "2026-06-01T13:06:00Z"
    }
  ],
  "statusCheckRollup": [
    {
      "__typename": "CheckRun",
      "name": "web lint",
      "status": "COMPLETED",
      "conclusion": "CANCELLED",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000000/job/55150000000"
    },
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
      "__typename": "CheckRun",
      "name": "opencode-review-same-repo",
      "workflowName": "OpenCode Code Review",
      "status": "COMPLETED",
      "conclusion": "SKIPPED",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000004/job/55150000004"
    },
    {
      "__typename": "CheckRun",
      "name": "opencode-review-same-repo",
      "workflowName": "OpenCode Code Review",
      "status": "COMPLETED",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000005/job/55150000005"
    },
    {
      "__typename": "CheckRun",
      "name": "opencode-review-fork",
      "workflowName": "OpenCode Code Review",
      "status": "COMPLETED",
      "conclusion": "SKIPPED",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000006/job/55150000006"
    },
    {
      "__typename": "CheckRun",
      "name": "opencode-review-fork",
      "workflowName": "OpenCode Code Review",
      "status": "COMPLETED",
      "conclusion": "FAILURE",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000007/job/55150000007"
    },
    {
      "__typename": "CheckRun",
      "name": "late-cancelled-failure",
      "workflowName": "CI",
      "status": "COMPLETED",
      "conclusion": "FAILURE",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000008/job/55150000008"
    },
    {
      "__typename": "CheckRun",
      "name": "late-cancelled-failure",
      "workflowName": "CI",
      "status": "COMPLETED",
      "conclusion": "CANCELLED",
      "detailsUrl": "https://github.com/kdlbs/kandev/actions/runs/27340000009/job/55150000009"
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
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "old-head" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"APPROVED","commit_id":"old-head-sha","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "exact-head" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"APPROVED","commit_id":"abc123","body":"Reviewed and approved.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "github-actions-clean" ]]; then
    printf '%s\n' '[
      {"user":{"login":"github-actions[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\\n**OpenCode review complete**","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "dedicated-app-clean" ]]; then
    printf '%s\n' '[
      {"user":{"login":"opencode-review-app[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\\n<!-- kandev-review: workflow-run id=900 attempt=2 -->\\n**OpenCode review complete**","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "dedicated-app-wrong-run" ]]; then
    printf '%s\n' '[
      {"user":{"login":"opencode-review-app[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\\n<!-- kandev-review: workflow-run id=899 attempt=1 -->","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "github-actions-old-head" ]]; then
    printf '%s\n' '[
      {"user":{"login":"github-actions[bot]"},"state":"COMMENTED","commit_id":"old-head-sha","body":"<!-- kandev-review: clean -->\\n**OpenCode review complete**","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "exact-dismissed" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"DISMISSED","commit_id":"abc123","body":"Dismissed after update.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "approved-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"APPROVED","commit_id":"abc123","body":"Blocker: the approval does not waive this required fix.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "comment-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blocker: please fix the race before merge.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "plain-selected-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blocker: race condition in the retry loop.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "plain-nonselected-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo blockers.","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"No blockers in unrelated files.\nBlockers: race condition in this path.","submitted_at":"2026-06-01T13:31:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "negated-colon-labels" ]]; then
    printf '%s\n' '[
      {"user":{"login":"one[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blocker: 0","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"two[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blockers: 0","submitted_at":"2026-06-01T13:31:00Z"},
      {"user":{"login":"three[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blocker: none","submitted_at":"2026-06-01T13:32:00Z"},
      {"user":{"login":"four[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Blockers: no issues","submitted_at":"2026-06-01T13:33:00Z"},
      {"user":{"login":"five[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"No blocker: all clear","submitted_at":"2026-06-01T13:34:00Z"},
      {"user":{"login":"six[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"No blockers: all clear","submitted_at":"2026-06-01T13:35:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "negated-then-positive-label" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"No blockers: all clear\nBlocker: race condition","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "formatted-negative-labels" ]]; then
    printf '%s\n' '[
      {"user":{"login":"one[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"**Blocker:** none","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"two[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"> **Blockers:** 0","submitted_at":"2026-06-01T13:31:00Z"},
      {"user":{"login":"three[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"_Blocker:_ no issue","submitted_at":"2026-06-01T13:32:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "formatted-positive-labels" ]]; then
    printf '%s\n' '[
      {"user":{"login":"one[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"**Blocker:** race condition","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"two[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"> **Blockers:** retry race","submitted_at":"2026-06-01T13:31:00Z"},
      {"user":{"login":"three[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"- _Blocker:_ missing lock","submitted_at":"2026-06-01T13:32:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "single-asterisk-labels" ]]; then
    printf '%s\n' '[
      {"user":{"login":"one[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"*Blocker:* race condition","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"two[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"*Blocker:* none","submitted_at":"2026-06-01T13:31:00Z"},
      {"user":{"login":"three[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"*Blockers:* 0","submitted_at":"2026-06-01T13:32:00Z"},
      {"user":{"login":"four[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"* Blocker: race condition","submitted_at":"2026-06-01T13:33:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "comment-clean" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo findings.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "comment-zero-table" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\n| Blocker | 0 |\nNo findings.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "comment-no-blockers" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo blockers. No findings.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "comment-positive-table" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\n| Blocker | 2 |\nMust fix before merge.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "clean-plus-current-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo findings.","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"Must fix this race before merge.","submitted_at":"2026-06-01T13:31:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "clean-plus-old-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo findings.","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"COMMENTED","commit_id":"old-head-sha","body":"Must fix this old issue before merge.","submitted_at":"2026-06-01T13:31:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "changes-dismissed" ]]; then
    printf '%s\n' '[
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"CHANGES_REQUESTED","commit_id":"abc123","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"DISMISSED","commit_id":"abc123","submitted_at":"2026-06-01T13:45:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "changes-current-clean" ]]; then
    printf '%s\n' '[
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"CHANGES_REQUESTED","commit_id":"abc123","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo findings.","submitted_at":"2026-06-01T13:45:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "old-change-current-clean" ]]; then
    printf '%s\n' '[
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"CHANGES_REQUESTED","commit_id":"old-head-sha","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\\nNo findings.","submitted_at":"2026-06-01T13:45:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "current-change-old-approval" ]]; then
    printf '%s\n' '[
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"CHANGES_REQUESTED","commit_id":"abc123","submitted_at":"2026-06-01T13:30:00Z"},
      {"user":{"login":"cubic-dev-ai[bot]"},"state":"APPROVED","commit_id":"old-head-sha","submitted_at":"2026-06-01T13:45:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "quoted-blocker" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\\nPrior review quoted: please fix the race before merge.","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  if [[ "${GH_REVIEW_SCENARIO:-default}" == "fenced-blocker-example" ]]; then
    printf '%s\n' '[
      {"user":{"login":"greptile-apps[bot]"},"state":"COMMENTED","commit_id":"abc123","body":"<!-- kandev-review: clean -->\nNo findings.\n\n```text\nplease fix the example\nBlocker: illustrative only\n```","submitted_at":"2026-06-01T13:30:00Z"}
    ]'
    exit 0
  fi
  printf '%s\n' '[
    {
      "user": { "login": "greptile-apps[bot]" },
      "state": "COMMENTED",
      "commit_id": "old-head-sha",
      "submitted_at": "2026-06-01T10:30:00Z"
    }
  ]'
  printf '%s\n' '[
    {
      "user": { "login": "cubic-dev-ai[bot]" },
      "state": "CHANGES_REQUESTED",
      "commit_id": "abc123",
      "submitted_at": "2026-06-01T13:30:00Z"
    }
  ]'
  exit 0
fi

  if [[ "$1" == "api" && "$2" == "graphql" ]]; then
  if [[ "$*" == *"headRefOid"* ]]; then
    head_oid="abc123"
    if [[ "${GH_HEAD_SEQUENCE:-stable}" == "race" ]]; then
      if [[ -f "${GH_HEAD_COUNTER_FILE:?}" ]]; then
        head_oid="def456"
      else
        : >"$GH_HEAD_COUNTER_FILE"
      fi
    elif [[ "${GH_HEAD_SEQUENCE:-stable}" == "after-workflow" && -f "${GH_HEAD_COUNTER_FILE:?}" ]]; then
      head_oid="def456"
    fi
    printf '%s\n' '{
      "data": {
        "repository": {
          "pullRequest": {
            "headRefOid": "'"$head_oid"'",
            "commits": {
              "nodes": [
                {
                  "commit": {
                    "oid": "'"$head_oid"'",
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
  assert_jq "checks collapse duplicate workflow attempts" '.checks | length < 10' "$json"
  assert_jq "latest duplicate check uses newest attempt" '[.checks[] | select(.name == "web lint")][0] | .conclusion == "success" and .run_id == "27340000001"' "$json"
  assert_jq "failed check preserved" '.checks[] | select(.name == "e2e") | .conclusion == "failure"' "$json"
  assert_jq "check run id" '.checks[] | select(.name == "e2e") | .run_id == "27340000002"' "$json"
  assert_jq "nested workflow name" '.checks[] | select(.name == "e2e") | .workflow == "CI"' "$json"
  assert_jq "duplicate check prefers non-skipped first" '[.checks[] | select(.name == "opencode-review-same-repo")][0] | .conclusion == "success" and .run_id == "27340000005"' "$json"
  assert_jq "duplicate cancellation retains prior failure" '[.checks[] | select(.name == "late-cancelled-failure")][0] | .conclusion == "failure" and .run_id == "27340000008"' "$json"
  assert_jq "pending status context conclusion normalized" '.checks[] | select(.name == "external pending") | .status == "pending" and .conclusion == null' "$json"
  assert_jq "threads count" '.review_threads | length == 1' "$json"
  assert_jq "total unresolved count includes historical unresolved thread" '.unresolved_review_thread_count == 2' "$json"
  assert_jq "filtered thread count" '.filtered_review_thread_count == 1' "$json"
  assert_jq "thread comment id" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_id == 112' "$json"
  assert_jq "thread comment timestamp" '.review_threads[] | select(.thread_id == "PRRT_1") | .comment_created_at == "2026-06-01T13:00:00Z"' "$json"
  assert_jq "reviews count" '.reviews | length == 1' "$json"
  assert_jq "review author" '.reviews[] | select(.author == "cubic-dev-ai[bot]") | .author == "cubic-dev-ai[bot]"' "$json"
  assert_jq "issue comments count" '.issue_comments | length == 7' "$json"
  assert_jq "issue comment author" 'any(.issue_comments[]; .author == "github-actions" and (.body | contains("Verdict")))' "$json"
  assert_jq "only GitHub Actions noncanonical output is actionable" '([.issue_comments[] | select(.actionable == false)] | length) == 2 and .actionable_issue_comment_count == 5' "$json"
  assert_jq "non-GitHub-Actions canonical marker is nonactionable while workflow diagnostic and fallback remain actionable" 'any(.issue_comments[]; .author == "other-bot[bot]" and .actionable == false) and all(.issue_comments[] | select(.body | test("Blocker: real|diagnostic|fallback"; "i")); .actionable == true)' "$json"
  assert_jq "no errors" '.errors == []' "$json"
  assert_jq "raw review evidence is complete" '.review_evidence.complete == true and .review_evidence.current_head_sha == "abc123"' "$json"
  assert_jq "active change request surfaced" '.review_evidence.active_changes_requested_count == 1' "$json"
  pass "snapshot happy path"
}

test_old_head_review_does_not_qualify() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=old-head PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "old head review evidence is complete but not exact" '.review_evidence.complete == true and .review_evidence.exact_current_head_reviews == []' "$json"
  pass "old-head review does not qualify"
}

test_exact_head_selected_review_qualifies() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=exact-head PATH="$tmp/bin:$PATH" "$SCRIPT" --summary 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "summary exact selected review evidence" '.review_evidence.complete == true and .review_evidence.exact_current_head_reviews[0].author == "greptile-apps[bot]" and .review_evidence.exact_current_head_reviews[0].body == "Reviewed and approved." and .review_evidence.exact_current_head_reviews[0].eligibility == "eligible"' "$json"
  pass "exact-head selected review qualifies"
}

test_clean_marker_requires_configured_dedicated_app_provenance() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local exact_json
  GH_REVIEW_SCENARIO=github-actions-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary 123 >"$tmp/unconfigured.json"
  local unconfigured_json
  unconfigured_json="$(<"$tmp/unconfigured.json")"
  assert_jq "unconfigured marker is not trusted evidence" '.review_evidence.exact_current_head_reviews[0].trusted_default_producer == false and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$unconfigured_json"

  GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/exact.json"
  exact_json="$(<"$tmp/exact.json")"
  assert_jq "matching dedicated App has authenticated OpenCode provenance" '.review_evidence.exact_current_head_reviews[0].author == "opencode-review-app[bot]" and .review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.exact_current_head_reviews[0].trusted_default_producer == true and .review_evidence.trusted_workflow_run.id == 900 and .review_evidence.trusted_workflow_run.complete == true and .review_evidence.trusted_producer == "true"' "$exact_json"

  local old_json
  GH_REVIEW_SCENARIO=github-actions-old-head PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'github-actions[bot]' 123 >"$tmp/old.json"
  old_json="$(<"$tmp/old.json")"
  assert_jq "old head actions review never becomes trusted evidence" '.review_evidence.exact_current_head_reviews == [] and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$old_json"
  pass "clean marker requires configured dedicated app provenance"
}

test_trusted_workflow_requires_latest_success_and_matching_marker() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  for conclusion in failure neutral cancelled skipped; do
    GH_TRUSTED_WORKFLOW_CONCLUSION="$conclusion" GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/$conclusion.json"
    json="$(<"$tmp/$conclusion.json")"
    assert_jq "latest $conclusion workflow invalidates old clean review" '.review_evidence.trusted_workflow_run.complete == false and .review_evidence.trusted_producer == "false" and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"
  done

  GH_TRUSTED_WORKFLOW_STATUS=in_progress GH_TRUSTED_WORKFLOW_CONCLUSION=null GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/pending.json"
  json="$(<"$tmp/pending.json")"
  assert_jq "latest pending workflow invalidates old clean review" '.review_evidence.trusted_workflow_run.complete == false and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"

  GH_REVIEW_SCENARIO=dedicated-app-wrong-run PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/wrong-marker.json"
  json="$(<"$tmp/wrong-marker.json")"
  assert_jq "wrong workflow marker is untrusted" '.review_evidence.trusted_workflow_run.complete == true and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"

  GH_TRUSTED_MULTI=1 GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/reversed-generations.json"
  json="$(<"$tmp/reversed-generations.json")"
  assert_jq "newer authentic failure beats older success despite reverse API order" '.review_evidence.trusted_workflow_run.id == 901 and .review_evidence.trusted_workflow_run.complete == false and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"

  GH_FAIL_TRUSTED_WORKFLOW=1 GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/api-failure.json"
  json="$(<"$tmp/api-failure.json")"
  assert_jq "workflow API failure is unknown and untrusted" '.review_evidence.trusted_workflow_run.known == false and .review_evidence.trusted_producer == "unknown" and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"
  pass "trusted workflow requires latest success and matching marker"
}

test_fork_and_unknown_provenance_are_untrusted() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local fork_json unknown_json
  GH_CROSS_REPOSITORY=true GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/fork.json"
  fork_json="$(<"$tmp/fork.json")"
  assert_jq "fork App marker is untrusted" '.pr.is_cross_repository == true and .review_evidence.is_cross_repository == true and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$fork_json"

  GH_CROSS_REPOSITORY=null GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/unknown.json"
  unknown_json="$(<"$tmp/unknown.json")"
  assert_jq "unknown repository provenance is untrusted" '.pr.is_cross_repository == null and .review_evidence.is_cross_repository == null and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$unknown_json"
  pass "fork and unknown provenance are untrusted"
}

test_head_change_during_collection_invalidates_review_evidence() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_HEAD_SEQUENCE=race GH_HEAD_COUNTER_FILE="$tmp/head-calls" GH_REVIEW_SCENARIO=exact-head PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "head race is incomplete and cannot qualify" '.review_evidence.known == false and .review_evidence.complete == false and .review_evidence.opening_head_sha == "abc123" and .review_evidence.closing_head_sha == "def456" and .review_evidence.current_head_sha == "def456" and .review_evidence.evidence_head_sha == null and .review_evidence.exact_current_head_reviews == []' "$json"
  pass "head change during collection invalidates review evidence"
}

test_head_change_during_authenticated_workflow_collection_invalidates_trust() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_HEAD_SEQUENCE=after-workflow GH_HEAD_COUNTER_FILE="$tmp/workflow-called" GH_REVIEW_SCENARIO=dedicated-app-clean PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --trusted-reviewer 'opencode-review-app[bot]' 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "post-workflow head race invalidates authenticated trust" '.review_evidence.complete == false and .review_evidence.opening_head_sha == "abc123" and .review_evidence.closing_head_sha == "def456" and .review_evidence.trusted_default_producer_exact_current_head_reviews == []' "$json"
  pass "head change during authenticated workflow collection invalidates trust"
}

test_checks_head_mismatch_invalidates_combined_evidence() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_CHECKS_HEAD=old-checks-sha GH_REVIEW_SCENARIO=exact-head PATH="$tmp/bin:$PATH" "$SCRIPT" --summary 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "checks and reviews cannot be paired across heads" '.checks_head_sha == "old-checks-sha" and .checks_snapshot_complete == false and .review_evidence.known == false and .review_evidence.complete == false and .review_evidence.checks_head_sha == "old-checks-sha" and .review_evidence.exact_current_head_reviews == []' "$json"
  pass "checks head mismatch invalidates combined evidence"
}

test_exact_head_dismissed_review_is_not_eligible() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=exact-dismissed PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "dismissed exact review is ineligible" '.review_evidence.exact_current_head_reviews[0].state == "dismissed" and .review_evidence.exact_current_head_reviews[0].eligibility == "ineligible"' "$json"
  pass "exact-head dismissed review is not eligible"
}

test_approved_review_with_blocker_is_not_eligible() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=approved-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "approved review body blocker wins" '.review_evidence.exact_current_head_reviews[0].state == "approved" and .review_evidence.exact_current_head_reviews[0].eligibility == "blocked" and .review_evidence.exact_current_head_reviews[0].verdict == "blocker"' "$json"
  pass "approved review with blocker is not eligible"
}

test_commented_blocker_is_not_eligible() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=comment-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "commented blocker is blocked" '(.review_evidence.exact_current_head_reviews[0].body | contains("Blocker")) and .review_evidence.exact_current_head_reviews[0].eligibility == "blocked" and .review_evidence.exact_current_head_reviews[0].verdict == "blocker"' "$json"
  pass "commented blocker is not eligible"
}

test_plain_blocker_labels_are_blocked_and_aggregated() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local selected_json
  GH_REVIEW_SCENARIO=plain-selected-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/selected.json"
  selected_json="$(<"$tmp/selected.json")"
  assert_jq "plain selected blocker is blocked" '.review_evidence.exact_current_head_reviews[0].eligibility == "blocked" and .review_evidence.blocked_exact_current_head_review_count == 1' "$selected_json"

  local nonselected_json
  GH_REVIEW_SCENARIO=plain-nonselected-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/nonselected.json"
  nonselected_json="$(<"$tmp/nonselected.json")"
  assert_jq "plain nonselected blocker aggregates despite no blockers text" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.blocked_exact_current_head_review_count == 1 and .review_evidence.blocked_exact_current_head_reviews[0].author == "cubic-dev-ai[bot]"' "$nonselected_json"
  pass "plain blocker labels are blocked and aggregated"
}

test_colon_blocker_labels_are_line_anchored_and_scan_all_lines() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local negative_json
  GH_REVIEW_SCENARIO=negated-colon-labels PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/negative.json"
  negative_json="$(<"$tmp/negative.json")"
  assert_jq "zero and negated colon labels are nonblocking" '.review_evidence.blocked_exact_current_head_review_count == 0 and ([.review_evidence.exact_current_head_reviews[].eligibility] | all(. != "blocked"))' "$negative_json"

  local mixed_json
  GH_REVIEW_SCENARIO=negated-then-positive-label PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/mixed.json"
  mixed_json="$(<"$tmp/mixed.json")"
  assert_jq "later positive label wins" '.review_evidence.blocked_exact_current_head_review_count == 1 and .review_evidence.exact_current_head_reviews[0].eligibility == "blocked"' "$mixed_json"
  pass "colon blocker labels are line anchored and scan all lines"
}

test_formatted_colon_labels_handle_emphasis_after_colon() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local negative_json
  GH_REVIEW_SCENARIO=formatted-negative-labels PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/negative.json"
  negative_json="$(<"$tmp/negative.json")"
  assert_jq "formatted negative labels are nonblocking" '.review_evidence.blocked_exact_current_head_review_count == 0 and ([.review_evidence.exact_current_head_reviews[].eligibility] | all(. != "blocked"))' "$negative_json"

  local positive_json
  GH_REVIEW_SCENARIO=formatted-positive-labels PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/positive.json"
  positive_json="$(<"$tmp/positive.json")"
  assert_jq "formatted positive labels block" '.review_evidence.blocked_exact_current_head_review_count == 3 and ([.review_evidence.exact_current_head_reviews[].eligibility] | all(. == "blocked"))' "$positive_json"
  pass "formatted colon labels handle emphasis after colon"
}

test_single_asterisk_emphasis_and_list_markers_are_safe() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=single-asterisk-labels PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"
  assert_jq "single asterisk forms classify correctly" '.review_evidence.blocked_exact_current_head_review_count == 2 and .review_evidence.exact_current_head_reviews[0].eligibility == "blocked" and .review_evidence.exact_current_head_reviews[1].eligibility != "blocked" and .review_evidence.exact_current_head_reviews[2].eligibility != "blocked" and .review_evidence.exact_current_head_reviews[3].eligibility == "blocked"' "$json"
  pass "single asterisk emphasis and list markers are safe"
}

test_explicit_clean_commented_review_is_eligible() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=comment-clean PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "explicit clean comment is eligible" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.exact_current_head_reviews[0].verdict == "clean"' "$json"
  pass "explicit clean commented review is eligible"
}

test_zero_blocker_summaries_remain_clean_with_explicit_marker() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local table_json
  GH_REVIEW_SCENARIO=comment-zero-table PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/table.json"
  table_json="$(<"$tmp/table.json")"
  assert_jq "zero blocker table is clean" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.exact_current_head_reviews[0].verdict == "clean"' "$table_json"

  local no_blockers_json
  GH_REVIEW_SCENARIO=comment-no-blockers PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/no-blockers.json"
  no_blockers_json="$(<"$tmp/no-blockers.json")"
  assert_jq "no blockers text is clean" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.exact_current_head_reviews[0].verdict == "clean"' "$no_blockers_json"
  pass "zero blocker summaries remain clean with explicit marker"
}

test_positive_blocker_evidence_overrides_clean_marker() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=comment-positive-table PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "positive blocker overrides clean marker" '.review_evidence.exact_current_head_reviews[0].eligibility == "blocked" and .review_evidence.exact_current_head_reviews[0].verdict == "blocker"' "$json"
  pass "positive blocker evidence overrides clean marker"
}

test_exact_head_blockers_from_any_author_are_aggregated() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local current_json
  GH_REVIEW_SCENARIO=clean-plus-current-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" --summary 123 >"$tmp/current.json"
  current_json="$(<"$tmp/current.json")"
  assert_jq "other author's current blocker is aggregated" '.review_evidence.blocked_exact_current_head_review_count == 1 and .review_evidence.blocked_exact_current_head_reviews[0].author == "cubic-dev-ai[bot]" and .review_evidence.blocked_exact_current_head_reviews[0].eligibility == "blocked"' "$current_json"

  local old_json
  GH_REVIEW_SCENARIO=clean-plus-old-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/old.json"
  old_json="$(<"$tmp/old.json")"
  assert_jq "old-head blocker is excluded" '.review_evidence.blocked_exact_current_head_review_count == 0 and .review_evidence.exact_current_head_reviews[0].eligibility == "eligible"' "$old_json"
  pass "exact-head blockers from any author are aggregated"
}

test_latest_decisive_review_state_controls_active_change_requests() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local blocked_json
  PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/blocked.json"
  blocked_json="$(<"$tmp/blocked.json")"
  assert_jq "latest change request blocks" '.review_evidence.active_changes_requested_count == 1 and .review_evidence.active_changes_requested[0].author == "cubic-dev-ai[bot]"' "$blocked_json"

  local cleared_json
  GH_REVIEW_SCENARIO=changes-dismissed PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/cleared.json"
  cleared_json="$(<"$tmp/cleared.json")"
  assert_jq "later decisive dismissal clears change request" '.review_evidence.active_changes_requested_count == 0' "$cleared_json"

  local clean_json
  GH_REVIEW_SCENARIO=changes-current-clean PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/clean.json"
  clean_json="$(<"$tmp/clean.json")"
  assert_jq "later clean current-head review clears change request" '.review_evidence.active_changes_requested_count == 0' "$clean_json"

  local old_change_current_clean_json
  GH_REVIEW_SCENARIO=old-change-current-clean PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/old-change-current-clean.json"
  old_change_current_clean_json="$(<"$tmp/old-change-current-clean.json")"
  assert_jq "old-head change request does not block a clean current head" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.active_changes_requested_count == 0' "$old_change_current_clean_json"

  local current_change_old_approval_json
  GH_REVIEW_SCENARIO=current-change-old-approval PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/current-change-old-approval.json"
  current_change_old_approval_json="$(<"$tmp/current-change-old-approval.json")"
  assert_jq "old-head approval cannot clear a current-head change request" '.review_evidence.active_changes_requested_count == 1 and .review_evidence.active_changes_requested[0].commit_id == "abc123"' "$current_change_old_approval_json"
  pass "latest decisive review state controls active change requests"
}

test_quoted_blocker_language_does_not_block_clean_review() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=quoted-blocker PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "quoted blocker text preserves explicit clean review" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.blocked_exact_current_head_review_count == 0' "$json"
  pass "quoted blocker language does not block clean review"
}

test_fenced_blocker_examples_do_not_block_clean_review() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_REVIEW_SCENARIO=fenced-blocker-example PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "fenced blocker examples preserve explicit clean review" '.review_evidence.exact_current_head_reviews[0].eligibility == "eligible" and .review_evidence.blocked_exact_current_head_review_count == 0' "$json"
  pass "fenced blocker examples do not block clean review"
}

test_partial_failure_records_error_but_keeps_other_data() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  GH_FAIL_REVIEWS=1 PATH="$tmp/bin:$PATH" "$SCRIPT" 123 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "reviews empty on failure" '.reviews == []' "$json"
  assert_jq "checks still present" '.checks | length < 9' "$json"
  assert_jq "new issue comments still present" '.issue_comments | length == 7' "$json"
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

  assert_jq "checks still present on repo failure" '.checks | length < 9' "$json"
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

  assert_jq "checks still present on graphql failure" '.checks | length < 9' "$json"
  assert_jq "reviews still present on graphql failure" '.reviews | length == 2' "$json"
  assert_jq "review threads empty on graphql failure" '.review_threads == []' "$json"
  assert_jq "unresolved count unknown on graphql failure" '.unresolved_review_thread_count == null' "$json"
  assert_jq "graphql failure records opening and closing head errors" '.errors | length == 3' "$json"
  assert_jq "graphql failure records since error" '.errors[] | select(.source == "since") | .message == "gh api graphql head commit failed; including historical comments"' "$json"
  assert_jq "graphql failure records review_threads error" '.errors[] | select(.source == "review_threads") | .message == "gh api graphql reviewThreads failed"' "$json"
  assert_jq "graphql failure records closing head error" '.errors[] | select(.source == "closing_head") | .message == "gh api graphql closing head commit failed"' "$json"
  assert_jq "since fallback includes all historical comments" '.issue_comments | length == 8' "$json"
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
  assert_jq "all issue comments present" '.issue_comments | length == 8' "$json"
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
  assert_jq "summary failed check count" '.failed_checks | length == 3' "$json"
  assert_jq "summary failed check" '.failed_checks[] | select(.name == "e2e") | .conclusion == "failure" and .run_id == "27340000002"' "$json"
  assert_jq "summary reports failed duplicate over skipped" '.failed_checks[] | select(.name == "opencode-review-fork") | .conclusion == "failure" and .run_id == "27340000007"' "$json"
  assert_jq "summary retains failure before a duplicate cancellation" '.failed_checks[] | select(.name == "late-cancelled-failure") | .conclusion == "failure" and .run_id == "27340000008"' "$json"
  assert_jq "summary ignores cancelled duplicate check" '[.failed_checks[] | select(.name == "web lint")] | length == 0' "$json"
  assert_jq "summary dedupes skipped duplicate check" '[.failed_checks[], .pending_checks[] | select(.name == "opencode-review-same-repo")] | length == 0' "$json"
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

test_comment_mode_returns_full_review_comment() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  local json
  PATH="$tmp/bin:$PATH" "$SCRIPT" --comment 111 >"$tmp/out.json"
  json="$(<"$tmp/out.json")"

  assert_jq "comment id" '.comment_id == 111' "$json"
  assert_jq "comment body is full" '.body | contains("Full rationale here.")' "$json"
  assert_jq "comment path" '.path == "apps/web/file.ts"' "$json"
  assert_jq "comment line" '.line == 42' "$json"
  assert_jq "comment author" '.author == "greptile-apps[bot]"' "$json"
  pass "--comment returns full review comment"
}

test_comment_mode_reports_fetch_failure() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  if GH_FAIL_COMMENT=1 PATH="$tmp/bin:$PATH" "$SCRIPT" --comment 999 >"$tmp/out.json" 2>"$tmp/err.log"; then
    fail "--comment failure exits non-zero"
  fi

  if [ -s "$tmp/out.json" ]; then
    fail "--comment failure does not emit JSON"
  fi
  if ! grep -q "failed to fetch PR comment 999" "$tmp/err.log"; then
    fail "--comment failure reports clear error"
  fi

  pass "--comment reports fetch failure"
}

test_comment_mode_rejects_incompatible_flags() {
  local tmp
  make_tmp_dir tmp
  make_mock_gh "$tmp/bin"

  if PATH="$tmp/bin:$PATH" "$SCRIPT" --summary --comment 111 >"$tmp/out.log" 2>&1; then
    fail "--comment rejects --summary"
  fi
  if ! grep -q "scripts/pr-state --comment <comment_id>" "$tmp/out.log"; then
    fail "--comment --summary prints usage"
  fi

  if PATH="$tmp/bin:$PATH" "$SCRIPT" --all --comment 111 >"$tmp/out.log" 2>&1; then
    fail "--comment rejects --all"
  fi
  if ! grep -q "scripts/pr-state --comment <comment_id>" "$tmp/out.log"; then
    fail "--comment --all prints usage"
  fi

  if PATH="$tmp/bin:$PATH" "$SCRIPT" --comment --summary >"$tmp/out.log" 2>&1; then
    fail "--comment rejects flag-shaped id"
  fi
  if ! grep -q "scripts/pr-state --comment <comment_id>" "$tmp/out.log"; then
    fail "--comment flag-shaped id prints usage"
  fi

  if PATH="$tmp/bin:$PATH" "$SCRIPT" --comment abc >"$tmp/out.log" 2>&1; then
    fail "--comment rejects non-numeric id"
  fi
  if ! grep -q "scripts/pr-state --comment <comment_id>" "$tmp/out.log"; then
    fail "--comment non-numeric id prints usage"
  fi

  pass "--comment rejects incompatible flags"
}

test_snapshot_happy_path
test_old_head_review_does_not_qualify
test_exact_head_selected_review_qualifies
test_clean_marker_requires_configured_dedicated_app_provenance
test_trusted_workflow_requires_latest_success_and_matching_marker
test_fork_and_unknown_provenance_are_untrusted
test_head_change_during_collection_invalidates_review_evidence
test_head_change_during_authenticated_workflow_collection_invalidates_trust
test_checks_head_mismatch_invalidates_combined_evidence
test_exact_head_dismissed_review_is_not_eligible
test_approved_review_with_blocker_is_not_eligible
test_commented_blocker_is_not_eligible
test_plain_blocker_labels_are_blocked_and_aggregated
test_colon_blocker_labels_are_line_anchored_and_scan_all_lines
test_formatted_colon_labels_handle_emphasis_after_colon
test_single_asterisk_emphasis_and_list_markers_are_safe
test_explicit_clean_commented_review_is_eligible
test_zero_blocker_summaries_remain_clean_with_explicit_marker
test_positive_blocker_evidence_overrides_clean_marker
test_exact_head_blockers_from_any_author_are_aggregated
test_fenced_blocker_examples_do_not_block_clean_review
test_latest_decisive_review_state_controls_active_change_requests
test_quoted_blocker_language_does_not_block_clean_review
test_partial_failure_records_error_but_keeps_other_data
test_pr_view_failure_with_non_numeric_ref_keeps_schema
test_repo_failure_skips_review_threads
test_graphql_failure_records_error_but_keeps_other_data
test_graphql_pagination_collects_all_threads
test_all_flag_includes_historical_comments_and_reviews
test_summary_mode_returns_compact_fixup_state
test_summary_all_flag_includes_historical_unresolved_threads
test_comment_mode_returns_full_review_comment
test_comment_mode_reports_fetch_failure
test_comment_mode_rejects_incompatible_flags
