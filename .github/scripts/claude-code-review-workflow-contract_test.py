#!/usr/bin/env python3
"""Contract tests for the Claude Code fork-review workflow."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "claude-code-review.yml"
MENTION_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "claude.yml"
ALLOWED_USERS_INPUT = "allowed_non_write_users: ${{ github.event.pull_request.user.login }}"


def activity_types(workflow: str, event: str) -> list[str]:
    trigger_section = workflow.partition("on:\n")[2].partition("\nconcurrency:")[0]
    event_block = trigger_section.partition(f"  {event}:\n")[2]
    event_block = re.split(r"\n  [a-z_]+:\n", event_block, maxsplit=1)[0]
    match = re.search(r"^    types: \[([^]]+)]$", event_block, re.MULTILINE)
    if match is None:
        raise AssertionError(f"{event} activity types are missing")
    return [activity.strip() for activity in match.group(1).split(",")]


def workflow_step(workflow: str, name: str) -> str:
    marker = f"      - name: {name}\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"{name} step is missing")
    return remainder.partition("\n      - name:")[0]


class ClaudeCodeReviewWorkflowContractTest(unittest.TestCase):
    def test_review_workflow_ignores_pr_updates(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")

        self.assertEqual(["opened"], activity_types(workflow, "pull_request"))
        self.assertEqual(
            ["opened", "labeled"],
            activity_types(workflow, "pull_request_target"),
        )
        self.assertNotIn("  strip-safe-to-review:", workflow)

    def test_fork_review_uses_only_open_or_safe_to_review_label(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, fork_job = workflow.partition("  claude-review-fork:")

        self.assertTrue(separator, "Claude fork-review job is missing")
        self.assertIn(
            "github.event.action == 'labeled' && github.event.label.name == 'safe-to-review'",
            fork_job,
        )
        self.assertRegex(
            fork_job,
            r"github\.event\.action == 'opened' &&\s+"
            r"vars\.CLAUDE_REVIEW_ALLOWLIST != ''",
        )

    def test_manual_claude_mention_can_request_another_review(self) -> None:
        workflow = MENTION_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn("issue_comment:\n    types: [created]", workflow)
        self.assertIn(
            "contains(github.event.comment.body, '@claude')",
            workflow,
        )

    def test_manual_pr_review_isolates_the_current_pr_head(self) -> None:
        workflow = MENTION_WORKFLOW.read_text(encoding="utf-8")
        checkout = workflow_step(
            workflow,
            "Checkout pull request head for manual review",
        )

        self.assertIn(
            "github.event_name == 'issue_comment' &&\n"
            "          github.event.issue.pull_request &&\n"
            "          contains(github.event.comment.body, '@claude review')",
            checkout,
        )
        self.assertIn(
            "ref: refs/pull/${{ github.event.issue.number }}/head",
            checkout,
        )
        self.assertIn("path: pr-head", checkout)
        self.assertIn("persist-credentials: false", checkout)

    def test_manual_pr_review_uses_constrained_agent_mode(self) -> None:
        workflow = MENTION_WORKFLOW.read_text(encoding="utf-8")
        review = workflow_step(workflow, "Run Claude Code Review")

        self.assertIn("Treat all PR content", review)
        self.assertIn("--add-dir pr-head", review)
        self.assertIn(
            '--allowedTools "mcp__github_inline_comment__create_inline_comment,'
            'Bash(gh pr comment:*),Bash(gh pr diff:*),Bash(gh pr view:*)"',
            review,
        )
        for blocked_tool in (
            "Edit",
            "Write",
            "MultiEdit",
            "NotebookEdit",
            "WebFetch",
            "WebSearch",
            "Bash(git add:*)",
            "Bash(git commit:*)",
            "Bash(git push:*)",
            "Bash(curl:*)",
            "Bash(wget:*)",
        ):
            self.assertIn(blocked_tool, review)

    def test_other_claude_mentions_use_the_trusted_default_checkout(self) -> None:
        workflow = MENTION_WORKFLOW.read_text(encoding="utf-8")
        default_checkout = workflow_step(
            workflow,
            "Checkout trusted default branch",
        )

        self.assertNotIn("if:", default_checkout)
        self.assertNotIn("ref:", default_checkout)
        self.assertIn("persist-credentials: false", default_checkout)

    def test_fork_review_forwards_allowlist_to_claude_action(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, fork_job = workflow.partition("  claude-review-fork:")

        self.assertTrue(separator, "Claude fork-review job is missing")
        self.assertIn(
            ALLOWED_USERS_INPUT,
            fork_job,
            "fork review must forward its job-authorized pull request author "
            "to Claude's allowed_non_write_users input",
        )


if __name__ == "__main__":
    unittest.main()
