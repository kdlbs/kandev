#!/usr/bin/env python3
"""Contract tests for the prebuilt desktop E2E image path."""

import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
DOCKERFILE = REPO_ROOT / ".github" / "docker" / "ci-base" / "Dockerfile"
IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
E2E_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "e2e-tests.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
IMAGE_DIGEST_RESOLVER = REPO_ROOT / ".github" / "scripts" / "resolve-image-digest.sh"


def job_block(workflow: str, job: str, next_job: str) -> str:
    """Return one workflow job block without parsing YAML anchors or expressions."""
    marker = f"  {job}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"Workflow has no {job} job")
    return remainder.partition(f"\n  {next_job}:\n")[0]


class DesktopE2EWorkflowContractTest(unittest.TestCase):
    def run_image_digest_resolver(
        self,
        *,
        manifest: bytes,
        failures_before_success: int,
        stalls_before_success: int = 0,
        invalid_manifest: bytes = b"",
        invalid_responses_before_success: int = 0,
    ) -> tuple[subprocess.CompletedProcess[str], int]:
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            bin_dir = temp_path / "bin"
            bin_dir.mkdir()
            count_file = temp_path / "docker-count"
            manifest_file = temp_path / "manifest.json"
            manifest_file.write_bytes(manifest)
            invalid_manifest_file = temp_path / "invalid-manifest.json"
            invalid_manifest_file.write_bytes(invalid_manifest)

            docker = bin_dir / "docker"
            docker.write_text(
                """#!/usr/bin/env bash
set -euo pipefail
count=0
if [[ -f "${FAKE_DOCKER_COUNT}" ]]; then
  count="$(cat "${FAKE_DOCKER_COUNT}")"
fi
count="$((count + 1))"
printf '%s' "${count}" > "${FAKE_DOCKER_COUNT}"
if (( count <= FAKE_DOCKER_FAILURES )); then
  echo "transient registry failure" >&2
  exit 1
fi
if (( count <= FAKE_DOCKER_FAILURES + FAKE_DOCKER_STALLS )); then
  echo "stalled registry lookup" >&2
  /bin/sleep 1
  exit 1
fi
if (( count <= FAKE_DOCKER_FAILURES + FAKE_DOCKER_STALLS + FAKE_DOCKER_INVALID_RESPONSES )); then
  cat "${FAKE_DOCKER_INVALID_MANIFEST}"
  exit 0
fi
cat "${FAKE_DOCKER_MANIFEST}"
""",
                encoding="utf-8",
            )
            docker.chmod(0o755)

            sleep = bin_dir / "sleep"
            sleep.write_text("#!/usr/bin/env bash\nexit 0\n", encoding="utf-8")
            sleep.chmod(0o755)

            env = os.environ.copy()
            env.update(
                {
                    "PATH": f"{bin_dir}:{env['PATH']}",
                    "FAKE_DOCKER_COUNT": str(count_file),
                    "FAKE_DOCKER_FAILURES": str(failures_before_success),
                    "FAKE_DOCKER_STALLS": str(stalls_before_success),
                    "FAKE_DOCKER_INVALID_MANIFEST": str(invalid_manifest_file),
                    "FAKE_DOCKER_INVALID_RESPONSES": str(
                        invalid_responses_before_success
                    ),
                    "FAKE_DOCKER_MANIFEST": str(manifest_file),
                    "IMAGE_RESOLVE_ATTEMPT_TIMEOUT": "0.1s",
                }
            )
            result = subprocess.run(
                ["bash", str(IMAGE_DIGEST_RESOLVER), "ghcr.io/kdlbs/kandev-ci:runtime-latest"],
                check=False,
                capture_output=True,
                env=env,
                text=True,
            )
            attempts = int(count_file.read_text(encoding="utf-8")) if count_file.exists() else 0
            return result, attempts

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
            ".github/scripts/resolve-image-digest.sh",
            ".github/workflows/ci-base-image.yml",
        ):
            self.assertIn(pattern, changes_job)

    def test_normal_shard_has_queue_safe_timeout(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        normal_job = job_block(workflow, "e2e", "playwright_image")

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
        resolver_job = job_block(workflow, "playwright_image", "e2e-containers")
        container_job = job_block(workflow, "e2e-containers", "e2e-kubernetes-compatibility")

        self.assertIn(
            "uses: actions/cache/restore@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
            container_job,
        )
        self.assertIn(
            "uses: actions/cache/save@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
            container_job,
        )
        self.assertIn("needs: [changes, build, playwright_image]", container_job)
        self.assertNotIn("docker buildx imagetools inspect", container_job)
        self.assertIn(
            "if: github.event_name != 'pull_request' || github.event.pull_request.head.repo.full_name == github.repository\n        uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f",
            resolver_job,
        )
        self.assertIn(
            ".github/scripts/resolve-image-digest.sh \"$image\"",
            resolver_job,
        )
        self.assertIn(
            "digest: ${{ steps.resolve.outputs.digest }}",
            resolver_job,
        )
        gate_job = workflow.partition("  e2e-gate:\n")[2]
        self.assertIn("PLAYWRIGHT_IMAGE_RESULT", gate_job)
        self.assertIn('"playwright-image:${PLAYWRIGHT_IMAGE_RESULT}"', gate_job)
        self.assertIn(
            "key: e2e-playwright-${{ runner.os }}-v1.61.1-noble-${{ needs.playwright_image.outputs.digest }}-${{ github.run_id }}-${{ github.run_attempt }}",
            container_job,
        )
        self.assertIn("path: /tmp/ms-playwright", container_job)
        self.assertIn(
            "restore-keys: e2e-playwright-${{ runner.os }}-v1.61.1-noble-${{ needs.playwright_image.outputs.digest }}-",
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
            "PLAYWRIGHT_RUNTIME_REF: ${{ needs.playwright_image.outputs.ref }}",
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

    def test_image_digest_resolver_hashes_raw_manifest_bytes(self) -> None:
        self.assertTrue(IMAGE_DIGEST_RESOLVER.exists())
        resolver = IMAGE_DIGEST_RESOLVER.read_text(encoding="utf-8")
        self.assertIn('imagetools inspect --raw "$image"', resolver)
        manifest = b'{"schemaVersion":2,"config":{},"layers":[]}'

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest, failures_before_success=0
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout.strip(), f"sha256:{hashlib.sha256(manifest).hexdigest()}"
        )
        self.assertEqual(attempts, 1)

    def test_image_digest_resolver_retries_transient_registry_failures(self) -> None:
        self.assertTrue(IMAGE_DIGEST_RESOLVER.exists())
        manifest = b'{"schemaVersion":2,"manifests":[]}'

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest, failures_before_success=2
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(attempts, 3)
        self.assertIn("attempt 1/5", result.stderr)
        self.assertIn("transient registry failure", result.stderr)

    def test_image_digest_resolver_times_out_stalled_registry_lookup(self) -> None:
        manifest = b'{"schemaVersion":2,"manifests":[]}'

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest,
            failures_before_success=0,
            stalls_before_success=1,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(attempts, 2)
        self.assertIn("timed out after 0.1s", result.stderr)

    def test_image_digest_resolver_retries_invalid_manifest_then_succeeds(self) -> None:
        self.assertTrue(IMAGE_DIGEST_RESOLVER.exists())
        manifest = b'{"schemaVersion":2,"manifests":[]}'

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest,
            failures_before_success=0,
            invalid_manifest=b'{"schemaVersion":1}',
            invalid_responses_before_success=2,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout.strip(), f"sha256:{hashlib.sha256(manifest).hexdigest()}"
        )
        self.assertEqual(attempts, 3)
        self.assertIn("attempt 1/5", result.stderr)

    def test_image_digest_resolver_rejects_invalid_manifest_responses(self) -> None:
        for name, invalid_manifest in (
            ("empty", b""),
            ("malformed", b"not-json"),
            ("wrong-schema", b'{"schemaVersion":1}'),
            ("schema-only", b'{"schemaVersion":2}'),
        ):
            with self.subTest(name=name):
                result, attempts = self.run_image_digest_resolver(
                    manifest=b'{"schemaVersion":2,"manifests":[]}',
                    failures_before_success=0,
                    invalid_manifest=invalid_manifest,
                    invalid_responses_before_success=5,
                )

                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(attempts, 5)
                self.assertEqual(result.stdout, "")
                self.assertIn("Could not resolve an immutable digest", result.stderr)

    def test_image_digest_resolver_fails_closed_after_bounded_retries(self) -> None:
        self.assertTrue(IMAGE_DIGEST_RESOLVER.exists())

        result, attempts = self.run_image_digest_resolver(
            manifest=b'{"schemaVersion":2,"manifests":[]}', failures_before_success=5
        )

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(attempts, 5)
        self.assertIn("Could not resolve an immutable digest", result.stderr)


if __name__ == "__main__":
    unittest.main()
