#!/usr/bin/env python3
"""Contract tests for the fork preview authorization workflow."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "preview-env.yml"
PREVIEW_BUILD = REPO_ROOT / "apps" / "backend" / "cmd" / "preview" / "build.go"


def workflow_job(workflow: str, name: str) -> str:
    marker = f"  {name}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"workflow job is missing: {name}")
    next_job = re.search(r"^  [A-Za-z0-9_-]+:\n", remainder, re.MULTILINE)
    return remainder[: next_job.start()] if next_job else remainder


class PreviewEnvironmentWorkflowContractTest(unittest.TestCase):
    def test_safe_to_review_and_allowlists_authorize_fork_preview_deployment(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, deploy_job = workflow.partition("  deploy-fork:")

        self.assertTrue(separator, "Fork preview deploy job is missing")
        self.assertIn("github.event_name == 'pull_request_target'", deploy_job)
        self.assertIn(
            "github.event.pull_request.head.repo.full_name != github.repository",
            deploy_job,
        )
        self.assertIn(
            "contains(github.event.pull_request.labels.*.name, 'safe-to-review')",
            deploy_job,
        )
        self.assertIn("vars.PREVIEW_ENV_ALLOWLIST != ''", deploy_job)
        self.assertIn("vars.CLAUDE_REVIEW_ALLOWLIST != ''", deploy_job)
        self.assertIn(
            "contains(fromJSON(vars.CLAUDE_REVIEW_ALLOWLIST), github.event.pull_request.user.login)",
            deploy_job,
        )
        self.assertEqual(workflow.count("persist-credentials: false"), 7)

    def test_safe_to_review_approval_survives_follow_up_pushes(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        _, separator, deploy_job = workflow.partition("  deploy-fork:")

        self.assertTrue(separator, "Fork preview deploy job is missing")
        self.assertIn(
            "((contains(github.event.pull_request.labels.*.name, 'safe-to-review')) ||",
            deploy_job,
        )
        self.assertNotIn("safe-to-test", deploy_job)
        self.assertNotIn("  strip-safe-to-test:", workflow)
        self.assertNotIn("github.rest.issues.removeLabel", workflow)

    def test_description_mutation_isolated_from_preview_lifecycle_jobs(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        for name in ("deploy-same-repo", "deploy-fork", "cleanup-preview"):
            job = workflow_job(workflow, name)
            self.assertNotIn("concurrency:", job)

        for name, command in (
            ("update-description-same-repo", "update-description"),
            ("update-description-fork", "update-description"),
            ("cleanup-preview-description", "remove-description"),
        ):
            job = workflow_job(workflow, name)
            self.assertIn("concurrency:", job)
            self.assertIn(
                "group: pr-description-${{ github.event.pull_request.number }}",
                job,
            )
            self.assertIn("cancel-in-progress: false", job)
            self.assertIn(f"go run ./cmd/preview {command}", job)

        for name in ("deploy-same-repo", "deploy-fork"):
            job = workflow_job(workflow, name)
            self.assertIn("--skip-description", job)
            self.assertIn("preview_url=", job)
        self.assertIn("--skip-description", workflow_job(workflow, "cleanup-preview"))

    def test_fork_deploy_uses_the_trusted_preview_command(self) -> None:
        workflow = WORKFLOW.read_text(encoding="utf-8")
        deploy_job = workflow_job(workflow, "deploy-fork")

        self.assertIn("path: .preview-cli", deploy_job)
        self.assertIn("ref: ${{ github.workflow_sha }}", deploy_job)
        self.assertIn("go build -o \"$RUNNER_TEMP/kandev-preview-deploy\" ./cmd/preview", deploy_job)
        self.assertIn(
            'deploy_output="$(\"$RUNNER_TEMP/kandev-preview-deploy\" deploy',
            deploy_job,
        )
        self.assertNotIn("go run ./cmd/preview deploy", deploy_job)

    def test_fork_build_subprocesses_cannot_inherit_deployment_credentials(self) -> None:
        build_source = PREVIEW_BUILD.read_text(encoding="utf-8")

        self.assertIn("func untrustedBuildEnv", build_source)
        for credential in (
            "SPRITES_API_TOKEN",
            "GH_TOKEN",
            "GITHUB_TOKEN",
            "ACTIONS_ID_TOKEN_REQUEST_TOKEN",
            "ACTIONS_RUNTIME_TOKEN",
        ):
            self.assertIn(credential, build_source)
        self.assertEqual(build_source.count("untrustedBuildEnv(os.Environ())"), 3)


if __name__ == "__main__":
    unittest.main()
