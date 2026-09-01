#!/usr/bin/env python3
"""Contract tests for the prebuilt desktop E2E image path."""

from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
DOCKERFILE = REPO_ROOT / ".github" / "docker" / "ci-base" / "Dockerfile"
IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
E2E_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "e2e-tests.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"


def job_block(workflow: str, job: str, next_job: str) -> str:
    """Return one workflow job block without parsing YAML anchors or expressions."""
    marker = f"  {job}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"Workflow has no {job} job")
    return remainder.partition(f"\n  {next_job}:\n")[0]


class DesktopE2EWorkflowContractTest(unittest.TestCase):
    def test_desktop_image_contains_pinned_toolchain_and_system_dependencies(self) -> None:
        dockerfile = DOCKERFILE.read_text(encoding="utf-8")

        self.assertIn("FROM runtime AS desktop", dockerfile)
        self.assertIn("ARG RUST_VERSION=1.97.1", dockerfile)
        self.assertIn("rustup toolchain install \"${RUST_VERSION}\" --profile minimal", dockerfile)

        for package in (
            "build-essential",
            "pkg-config",
            "libglib2.0-dev",
            "libwebkit2gtk-4.1-dev",
            "libgtk-3-dev",
            "libayatana-appindicator3-dev",
            "librsvg2-dev",
            "patchelf",
            "rpm",
            "xvfb",
        ):
            self.assertIn(package, dockerfile)

        for smoke_command in (
            "rustc --version",
            "cargo --version",
            "pkg-config --exists webkit2gtk-4.1",
            "command -v patchelf",
            "command -v xvfb-run",
        ):
            self.assertIn(smoke_command, dockerfile)

    def test_image_workflow_publishes_desktop_tags(self) -> None:
        workflow = IMAGE_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn("target: desktop", workflow)
        self.assertIn("desktop-sha-${{ steps.tag.outputs.image_tag }}", workflow)
        self.assertIn("${{ env.IMAGE_NAME }}:desktop-latest", workflow)
        self.assertIn("type=gha,scope=desktop", workflow)
        self.assertIn("type=gha,scope=runtime", workflow)
        self.assertIn("desktop-latest", workflow)

    def test_desktop_job_uses_image_without_live_bootstrap_downloads(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        desktop_job = job_block(workflow, "desktop-e2e", "e2e-report")

        self.assertIn("image: ghcr.io/kdlbs/kandev-ci:desktop-latest", desktop_job)
        self.assertIn("options: --ipc=host", desktop_job)
        self.assertIn("git config --global --add safe.directory", desktop_job)
        self.assertIn("path: ~/.local/share/pnpm/store", desktop_job)
        self.assertIn("pnpm install --frozen-lockfile", desktop_job)
        self.assertIn("pnpm --filter @kandev/desktop e2e", desktop_job)

        for forbidden in (
            "pnpm/action-setup",
            "actions/setup-node",
            "rustup toolchain install",
            "apt-get",
            "sudo",
        ):
            self.assertNotIn(forbidden, desktop_job)

        changes_job = job_block(workflow, "changes", "build")
        for pattern in (
            ".github/docker/ci-base/**",
            ".github/workflows/ci-base-image.yml",
        ):
            self.assertIn(pattern, changes_job)

    def test_normal_shard_has_queue_safe_timeout(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        normal_job = job_block(workflow, "e2e", "e2e-containers")

        self.assertIn(
            "# 35 min covers the serial count-fallback tail and setup overhead",
            normal_job,
        )
        self.assertIn("timeout-minutes: 35", normal_job)
        self.assertNotIn("timeout-minutes: 25", normal_job)

    def test_contract_runs_in_the_unfiltered_required_workflow(self) -> None:
        workflow = LINT_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn(
            "python3 .github/scripts/e2e-tests-workflow-contract_test.py",
            workflow,
        )
        for trigger in ("push", "pull_request", "merge_group"):
            trigger_marker = f"  {trigger}:"
            _, separator, trigger_block_text = workflow.partition(trigger_marker)
            self.assertTrue(separator, f"Lint workflow has no {trigger} trigger")
            self.assertNotIn("    paths:", trigger_block_text.split("\n  ", 1)[0])

    def test_timing_lookup_requires_a_profile_artifact(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")

        self.assertIn("/actions/runs/", workflow)
        self.assertIn("candidate.id", workflow)
        self.assertIn("/artifacts?per_page=100", workflow)
        self.assertIn('artifact.name === "e2e-timing-profile"', workflow)
        self.assertIn("!artifact.expired", workflow)

    # @covers AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.1
    # @covers AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-002.2
    def test_container_job_reuses_verified_browser_cache_with_fallback(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        container_job = job_block(workflow, "e2e-containers", "desktop-e2e")

        self.assertIn(
            "uses: actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
            container_job,
        )
        self.assertIn(
            "uses: actions/cache/save@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
            container_job,
        )
        self.assertIn("id: playwright_image", container_job)
        self.assertLess(
            container_job.index("- name: Log in to GHCR"),
            container_job.index("- name: Resolve immutable Playwright runtime image"),
        )
        self.assertIn(
            "if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository\n        uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f",
            container_job,
        )
        self.assertIn(
            "image=ghcr.io/kdlbs/kandev-ci:runtime-latest",
            container_job,
        )
        self.assertIn(
            'docker buildx imagetools inspect "$image"',
            container_job,
        )
        self.assertIn("for attempt in 1 2 3; do", container_job)
        self.assertIn(
            'if candidate="$(docker buildx imagetools inspect "$image"',
            container_job,
        )
        self.assertIn('sleep "$((attempt * 2))"', container_job)
        self.assertIn("path: /tmp/ms-playwright", container_job)
        self.assertIn(
            "key: e2e-playwright-${{ runner.os }}-v1.61.1-noble-${{ steps.playwright_image.outputs.digest }}-${{ github.run_id }}-${{ github.run_attempt }}",
            container_job,
        )
        self.assertIn(
            "restore-keys: e2e-playwright-${{ runner.os }}-v1.61.1-noble-${{ steps.playwright_image.outputs.digest }}-",
            container_job,
        )
        self.assertIn("id: playwright_cache", container_job)
        self.assertIn("id: playwright_cache_verify", container_job)
        self.assertIn("PLAYWRIGHT_BROWSERS_PATH=/tmp/ms-playwright", container_job)
        self.assertIn(
            "if: steps.playwright_cache.outputs.cache-hit != ''",
            container_job,
        )
        self.assertIn(
            "if: steps.playwright_cache.outputs.cache-hit == '' || steps.playwright_cache_verify.outcome != 'success'",
            container_job,
        )
        self.assertIn(
            "github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository",
            container_job,
        )
        self.assertIn(
            "PLAYWRIGHT_RUNTIME_REF: ${{ steps.playwright_image.outputs.ref }}",
            container_job,
        )
        self.assertIn('docker pull "$PLAYWRIGHT_RUNTIME_REF"', container_job)
        self.assertIn('"$PLAYWRIGHT_RUNTIME_REF" \\', container_job)
        self.assertNotIn('docker pull "${{ steps.playwright_image.outputs.ref }}"', container_job)
        self.assertIn(
            "if: success() && (steps.playwright_cache.outputs.cache-hit == '' || steps.playwright_cache_verify.outcome != 'success')",
            container_job,
        )
        self.assertIn("GITHUB_STEP_SUMMARY", container_job)
        self.assertIn("id: browser_setup_timer", container_job)
        self.assertIn('echo "started_at=$(date +%s)" >> "$GITHUB_OUTPUT"', container_job)
        self.assertIn('setup_mode="cache-hit"', container_job)
        self.assertIn('setup_mode="image-fallback"', container_job)
        self.assertGreaterEqual(container_job.count("continue-on-error: true"), 2)


if __name__ == "__main__":
    unittest.main()
