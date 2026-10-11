#!/usr/bin/env python3
"""Contract tests for the E2E workflow and prebuilt images."""

import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import tempfile
import textwrap
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
DOCKERFILE = REPO_ROOT / ".github" / "docker" / "ci-base" / "Dockerfile"
IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
E2E_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "e2e-tests.yml"
SESSION_ACCEPTANCE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "kubernetes-session-acceptance.yml"
DOWNLOAD_ARTIFACT_RETRY_ACTION = (
    REPO_ROOT / ".github" / "actions" / "download-artifact-retry" / "action.yml"
)
BACKEND_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "backend-tests.yml"
FRONTEND_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "frontend-tests.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
IMAGE_DIGEST_RESOLVER = REPO_ROOT / ".github" / "scripts" / "resolve-image-digest.sh"
VALID_IMAGE_INDEX = (
    b'{"schemaVersion":2,"manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json",'
    b'"digest":"sha256:' + b"1" * 64 + b'","size":2}]}'
)
VALID_IMAGE_MANIFEST = (
    b'{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json",'
    b'"digest":"sha256:' + b"0" * 64 + b'","size":2},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip",'
    b'"digest":"sha256:' + b"2" * 64 + b'","size":2}]}'
)


def job_block(workflow: str, job: str, next_job: str) -> str:
    """Return one workflow job block without parsing YAML anchors or expressions."""
    marker = f"  {job}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"Workflow has no {job} job")
    return remainder.partition(f"\n  {next_job}:\n")[0]


class E2EWorkflowContractTest(unittest.TestCase):
    def test_session_browser_setup_uses_bounded_https_mirror_before_install(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        step = workflow.split('      - name: Prepare host Chromium\n', 1)[1].split('      - name:', 1)[0]
        self.assertIn('timeout-minutes: 10', step)
        self.assertIn('run: |', step)
        script = textwrap.dedent(step.split('run: |\n', 1)[1])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = {
                'sudo': 'exec "$@"',
                'tee': '''case "$1" in
/etc/apt/apt-mirrors.txt|/etc/apt/apt.conf.d/99kandev-acceptance-timeouts)
  exec /usr/bin/tee "$APT_ROOT/$(basename "$1")";;
*) exit 2;;
esac''',
                'pnpm': '''test "$*" = 'exec playwright install --with-deps chromium' || exit 2
test -s "$APT_ROOT/apt-mirrors.txt" && test -s "$APT_ROOT/99kandev-acceptance-timeouts" || exit 3
echo installed''',
            }
            for name, body in commands.items():
                command = root/name
                command.write_text('#!/bin/sh\n' + body + '\n')
                command.chmod(0o755)
            result = subprocess.run(['bash', '-ceu', script], capture_output=True, text=True,
                timeout=5, env={**os.environ, 'PATH': f"{root}:{os.environ['PATH']}", 'APT_ROOT': str(root)})
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn('installed', result.stdout)
            self.assertEqual((root/'apt-mirrors.txt').read_text(), 'https://archive.ubuntu.com/ubuntu/\n')
            config = root/'99kandev-acceptance-timeouts'
            parsed = subprocess.run(['apt-config', '-c', str(config), 'dump'],
                capture_output=True, text=True, timeout=5, check=True).stdout
            for key, value in (('http::Timeout', '30'), ('https::Timeout', '30'), ('Retries', '2')):
                self.assertIn(f'Acquire::{key} "{value}";', parsed)

    def test_session_acceptance_uses_one_read_only_draft_job(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        self.assertIn("pull_request:", workflow)
        self.assertIn("github.event.pull_request.draft == true", workflow)
        self.assertNotIn("needs:", workflow)
        self.assertEqual(re.findall(r"^  ([a-z][a-z0-9_-]*):$", workflow.split("jobs:\n", 1)[1], re.MULTILINE), ["acceptance"])
        self.assertIn("contents: read", workflow)
        self.assertIn("packages: read", workflow)
        self.assertNotIn("packages: write", workflow)
        self.assertNotIn("docker push", workflow)

    def test_session_acceptance_runs_exact_controls_without_retries(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        self.assertLess(workflow.index("prepare-full-worker-acceptance.sh"), workflow.index("pnpm e2e:run"))
        for spec in ("kubernetes-session-resilience", "kubernetes-failure-recovery", "kubernetes-task-pod"):
            self.assertIn(f"tests/kubernetes/{spec}.spec.ts", workflow)
        self.assertIn("--host --shards 1 --project containers", workflow)
        self.assertIn("--retries 0", workflow)
        self.assertIn('MAKEFLAGS: "GOFLAGS=-p=1"', workflow)
        self.assertIn('GOMAXPROCS: "2"', workflow)
        self.assertIn("verify-kubernetes-session-acceptance.cjs", workflow)

    def test_session_acceptance_scopes_runner_paths_to_steps(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        job_env = workflow.split("    env:\n", 1)[1].split("    steps:\n", 1)[0]
        self.assertNotIn("runner.", job_env, "runner context is unavailable in job env")
        execute = workflow.split("      - name: Run five real Kubernetes acceptance scenarios\n", 1)[1]
        self.assertIn("PLAYWRIGHT_JSON_OUTPUT_NAME: ${{ runner.temp }}", execute)
        self.assertIn('verify-kubernetes-session-acceptance.cjs "${{ runner.temp }}', execute)

    def test_session_acceptance_preserves_exact_cleanup_and_failure_artifacts(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        self.assertIn("if: always()", workflow)
        self.assertIn('"$owned_name" == "$cluster_name"', workflow)
        self.assertIn('delete cluster --name "$cluster_name"', workflow)
        self.assertIn("apps/web/e2e/blob-report/", workflow)
        self.assertIn("kandev-full-worker-acceptance-build.log", workflow)
        self.assertIn("kubernetes-session-acceptance-results.json", workflow)

    def test_session_acceptance_build_identity_is_valid_for_compact_runtime(self):
        workflow = SESSION_ACCEPTANCE_WORKFLOW.read_text()
        execute = workflow.split("      - name: Run five real Kubernetes acceptance scenarios\n", 1)[1].split("      - name:", 1)[0]
        configured = re.search(r"^\s+VERSION: (.+)$", execute, re.MULTILINE)
        self.assertIsNotNone(configured, "shallow checkout needs an explicit release-compatible app version")
        commit = "a" * 40
        version = configured.group(1).strip('"').replace("${{ github.sha }}", commit)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            helpers = root / "bin"
            helpers.mkdir()
            for platform in ("linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64"):
                helper = helpers / f"agentctl-{platform}"
                helper.write_bytes(b"fixture helper\n")
                helper.chmod(0o755)
            result = subprocess.run([
                "node", str(REPO_ROOT / "scripts/release/remote-helper-assets.mjs"), "build",
                "--bin-dir", str(helpers), "--output-dir", str(root / "artifact"),
                "--version", version, "--commit", commit, "--stable", "true",
            ], capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)

    def test_session_acceptance_requires_five_executed_passes(self):
        verify = REPO_ROOT / ".github/scripts/verify-kubernetes-session-acceptance.cjs"
        tests = [{"expectedStatus": "passed", "results": [{"status": "passed", "retry": 0}]} for _ in range(5)]
        report = {"stats": {"expected": 5, "unexpected": 0, "skipped": 0, "flaky": 0},
                  "errors": [], "suites": [{"suites": [{"specs": [{"tests": tests}]}]}]}
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "report.json"
            for case in ("passed", "zero", "skipped", "retried", "missing_results", "failed", "wrong_count", "setup_error"):
                current = json.loads(json.dumps(report))
                if case == "zero":
                    current["suites"] = []
                elif case == "skipped":
                    current["stats"]["skipped"] = 1
                elif case == "retried":
                    current["suites"][0]["suites"][0]["specs"][0]["tests"][0]["results"][0]["retry"] = 1
                elif case == "failed":
                    current["suites"][0]["suites"][0]["specs"][0]["tests"][0]["results"][0]["status"] = "failed"
                elif case == "wrong_count":
                    current["stats"]["expected"] = 4
                elif case == "setup_error":
                    current["errors"] = [{"message": "fixture setup failed"}]
                elif case == "missing_results":
                    current["suites"][0]["suites"][0]["specs"][0]["tests"][0]["results"] = []
                output.write_text(json.dumps(current))
                result = subprocess.run(["node", str(verify), str(output)], capture_output=True, text=True, timeout=10)
                if case == "passed":
                    self.assertEqual(result.returncode, 0, result.stderr)
                else:
                    self.assertNotEqual(result.returncode, 0, case)

    def test_full_worker_acceptance_is_prepared_before_its_container_shard(self):
        workflow = E2E_WORKFLOW.read_text()
        job = job_block(workflow, "e2e-containers", "e2e-kubernetes-compatibility")
        self.assertIn('shard.files.includes("tests/kubernetes/kubernetes-session-resilience.spec.ts")', job)
        self.assertIn("steps.kubernetes.outputs.has_full_worker == 'true'", job)
        prepare = job.index("bash .github/scripts/prepare-full-worker-acceptance.sh")
        execute = job.index("run: bash e2e/scripts/run-planned-shard.sh")
        self.assertLess(prepare, execute)
        self.assertIn("kandev-full-worker-acceptance-build.log", job)
        self.assertIn("prepare-full-worker-acceptance_test.py", LINT_WORKFLOW.read_text())

    def run_image_digest_resolver(
        self,
        *,
        manifest: bytes,
        failures_before_success: int,
        attempt_timeout: str = "5s",
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
                    "IMAGE_RESOLVE_ATTEMPT_TIMEOUT": attempt_timeout,
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

    def test_container_jobs_use_the_baked_corepack_cache(self) -> None:
        dockerfile = DOCKERFILE.read_text(encoding="utf-8")

        self.assertIn("COREPACK_HOME=/root/.cache/node/corepack", dockerfile)
        for workflow_path in (E2E_WORKFLOW, BACKEND_WORKFLOW, FRONTEND_WORKFLOW):
            workflow = workflow_path.read_text(encoding="utf-8")
            self.assertIn(
                "COREPACK_HOME: /root/.cache/node/corepack",
                workflow,
                workflow_path.name,
            )

    def test_desktop_job_uses_image_without_live_bootstrap_downloads(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        desktop_job = job_block(workflow, "desktop-e2e", "e2e-report")

        self.assertIn("image: ghcr.io/kdlbs/kandev-ci:desktop-latest", desktop_job)
        self.assertIn("options: --ipc=host", desktop_job)
        self.assertIn("git config --global --add safe.directory", desktop_job)
        self.assertIn("path: ~/.local/share/pnpm/store", desktop_job)
        self.assertIn("actions/setup-go@", desktop_job)
        self.assertIn("go-version-file: apps/backend/go.mod", desktop_job)
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
            "# 45 min covers the serial count-fallback tail and setup overhead",
            normal_job,
        )
        self.assertIn("timeout-minutes: 45", normal_job)
        self.assertNotIn("timeout-minutes: 25", normal_job)

    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.1
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.2
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.3
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.4
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.5
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.6
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1
    # @covers AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.2
    def test_external_runner_tiers_are_toggleable_for_eligible_jobs(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")

        changes = job_block(workflow, "changes", "build")
        self.assertIn("runs-on: ubuntu-latest", changes)
        self.assertNotIn("    needs:", changes)
        self.assertNotIn("  e2e-gate:\n", workflow)
        e2e_job = job_block(workflow, "e2e", "playwright_image")
        self.assertIn("runs-on: ${{ matrix.runner }}", e2e_job)
        self.assertIn(
            "matrix: ${{ fromJSON(needs.changes.outputs.plan).e2e_matrix }}",
            e2e_job,
        )

        protected_jobs = {
            "build": "e2e",
            "e2e-report": "e2e-gate",
            "playwright_image": "e2e-containers",
            "e2e-containers": "e2e-kubernetes-compatibility",
            "e2e-kubernetes-compatibility": "desktop-e2e",
            "desktop-e2e": "e2e-report",
        }
        for job, next_job in protected_jobs.items():
            protected_text = job_block(workflow, job, next_job)
            self.assertIn("runs-on: ubuntu-latest", protected_text)
            self.assertNotIn("runner_plan", protected_text)
            self.assertNotIn("KANDEV_CI_EXTERNAL", protected_text)
            self.assertNotIn("KANDEV_CI_RUNNER_", protected_text)

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

    def test_e2e_artifact_downloads_retry_transient_service_failures(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        self.assertTrue(DOWNLOAD_ARTIFACT_RETRY_ACTION.exists())
        self.assertNotIn("uses: actions/download-artifact@", workflow)
        self.assertGreaterEqual(
            workflow.count("uses: ./.github/actions/download-artifact-retry"),
            12,
        )

        action = DOWNLOAD_ARTIFACT_RETRY_ACTION.read_text(encoding="utf-8")
        self.assertEqual(action.count("uses: actions/download-artifact@"), 3)
        self.assertEqual(action.count("continue-on-error: true"), 3)
        self.assertIn('default: "false"', action)
        for input_name in (
            "name",
            "path",
            "pattern",
            "merge-multiple",
            "repository",
            "run-id",
            "github-token",
        ):
            self.assertEqual(
                action.count(f"{input_name}: ${{{{ inputs.{input_name} }}}}"),
                3,
                input_name,
            )
        self.assertIn("sleep 10", action)
        self.assertIn("sleep 30", action)
        self.assertIn('rm -rf -- "$DOWNLOAD_PATH"', action)
        self.assertIn("exit 1", action)

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
        gate_job = workflow.partition("  e2e-report:\n")[2]
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
        manifest = VALID_IMAGE_MANIFEST

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
        manifest = VALID_IMAGE_INDEX

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest, failures_before_success=2
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(attempts, 3)
        self.assertIn("attempt 1/5", result.stderr)
        self.assertIn("transient registry failure", result.stderr)

    def test_image_digest_resolver_times_out_stalled_registry_lookup(self) -> None:
        manifest = VALID_IMAGE_INDEX

        result, attempts = self.run_image_digest_resolver(
            manifest=manifest,
            failures_before_success=0,
            attempt_timeout="0.5s",
            stalls_before_success=1,
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(attempts, 2)
        self.assertIn("timed out after 0.5s", result.stderr)

    def test_image_digest_resolver_retries_invalid_manifest_then_succeeds(self) -> None:
        self.assertTrue(IMAGE_DIGEST_RESOLVER.exists())
        manifest = VALID_IMAGE_INDEX

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
            ("empty-index", b'{"schemaVersion":2,"manifests":[]}'),
            ("missing-descriptor-fields", b'{"schemaVersion":2,"manifests":[{}]}'),
            (
                "empty-layers",
                b'{"schemaVersion":2,"config":{"mediaType":"application/vnd.oci.image.config.v1+json",'
                b'"digest":"sha256:' + b"0" * 64 + b'","size":2},"layers":[]}',
            ),
        ):
            with self.subTest(name=name):
                result, attempts = self.run_image_digest_resolver(
                    manifest=VALID_IMAGE_INDEX,
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
            manifest=VALID_IMAGE_INDEX, failures_before_success=5
        )

        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(attempts, 5)
        self.assertIn("Could not resolve an immutable digest", result.stderr)


if __name__ == "__main__":
    unittest.main()
