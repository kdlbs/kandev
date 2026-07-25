#!/usr/bin/env python3
"""Contract tests for the scheduled core-agent version update workflow."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/update-agent-versions.yml"
LINT_WORKFLOW = ROOT / ".github/workflows/lint-action-pinning.yml"


def require(content: str, needle: str, message: str) -> None:
    if needle not in content:
        raise AssertionError(message)


if not WORKFLOW.exists():
    raise AssertionError(f"missing workflow: {WORKFLOW}")

workflow = WORKFLOW.read_text()
lint_workflow = LINT_WORKFLOW.read_text()

require(workflow, 'cron: "17 7 * * 3"', "workflow must run weekly off the Monday 06:00 contention window")
require(workflow, "workflow_dispatch: {}", "workflow must support manual dispatch")
require(workflow, "group: update-agent-versions", "workflow must serialize update runs")
require(workflow, "cancel-in-progress: false", "an update run must not cancel another writer")
require(workflow, "permissions:\n  contents: read", "default workflow token must be read-only")
require(
    workflow,
    "persist-credentials: false",
    "read-only checkout must not persist credentials for later writes",
)

require(
    workflow,
    "actions/create-github-app-token@bcd2ba49218906704ab6c1aa796996da409d3eb1 # v3.2.0",
    "workflow must use the repository's SHA-pinned GitHub App token action",
)
require(workflow, "app-id: ${{ vars.AGENT_UPDATE_APP_ID }}", "workflow must use the dedicated App ID")
require(
    workflow,
    "private-key: ${{ secrets.AGENT_UPDATE_APP_PRIVATE_KEY }}",
    "workflow must use the dedicated App private key",
)
require(workflow, "permission-contents: write", "App token must request explicit contents write")
require(
    workflow,
    "permission-pull-requests: write",
    "App token must request explicit pull-request write",
)
require(
    workflow,
    "GH_TOKEN: ${{ steps.app-token.outputs.token }}",
    "git and gh writes must use the dedicated App token",
)
token_step = workflow.index("- name: Create agent-update App token")
for pre_token_step in (
    "- name: Test version updater",
    "- name: Update core agent versions",
    "- name: Test pinned agent command surfaces",
    "- name: Test workflow contract",
):
    if token_step < workflow.index(pre_token_step):
        raise AssertionError(
            f"write-capable App token must be minted after {pre_token_step.removeprefix('- name: ')}"
        )
require(workflow, "gh auth setup-git", "git push must authenticate with the late-minted App token")

require(
    workflow,
    "python3 scripts/update_agent_versions.py",
    "workflow must run the repository-owned updater",
)
require(
    workflow,
    "python3 scripts/update_agent_versions_test.py",
    "workflow must run updater unit tests before writing a branch",
)
require(
    workflow,
    "go test ./internal/agent/agents ./internal/agent/runtime/lifecycle",
    "workflow must run targeted command-surface tests",
)
require(workflow, "automation/agent-version-updates", "workflow must use one stable automation branch")
require(workflow, "git diff --quiet", "workflow must skip empty pull requests")
require(
    workflow,
    '"--force-with-lease=refs/heads/$BRANCH:$remote_sha"',
    "automation branch updates must retain exact-SHA force-with-lease protection",
)
require(workflow, "gh pr create", "workflow must create a pull request when none exists")
require(workflow, "gh pr edit", "workflow must refresh the existing pull request body")
require(
    workflow,
    "head.repo.full_name == $repo",
    "existing PR selection must bind the head repository to the current repository",
)
require(
    workflow,
    'head.ref == $branch',
    "existing PR selection must bind the exact automation branch",
)
if "gh pr list" in workflow:
    raise AssertionError("branch-name-only gh pr list lookup permits fork PR confusion")
require(
    workflow,
    "chore(deps): update pinned agent versions",
    "workflow must use a Conventional Commit PR title",
)

for forbidden in ("npx ", "npm exec", "npm install", "gh pr merge", "--auto"):
    if forbidden in workflow:
        raise AssertionError(f"token-bearing workflow must not contain {forbidden!r}")

require(
    lint_workflow,
    ".github/scripts/update-agent-versions-workflow_test.py",
    "action-pinning CI must run the updater workflow contract test",
)

print("✓ Agent version update workflow contract is intact.")
