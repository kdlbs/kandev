#!/usr/bin/env python3
"""Contract tests for the E2E workflow and prebuilt images."""

import hashlib
import json
import re
import shutil
import textwrap
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
DOCKERFILE = REPO_ROOT / ".github" / "docker" / "ci-base" / "Dockerfile"
IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
E2E_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "e2e-tests.yml"
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
    def test_required_report_reconciles_the_build_catalog(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        report = workflow.partition("  e2e-report:\n")[2]
        collection = report.partition("      - name: Collect timing and retry diagnostics\n")[2].partition("      - name:")[0]
        self.assertIn('--catalog "$GITHUB_WORKSPACE/e2e-manifests/native-catalog.json"', collection)
        manifest_download = report.partition("      - name: Download E2E shard manifests\n")[2].partition("      - name:")[0]
        self.assertNotIn("continue-on-error", manifest_download)
        for job in (job_block(workflow, "e2e", "playwright_image"), job_block(workflow, "e2e-containers", "e2e-kubernetes-compatibility"), report):
            self.assertIn("KANDEV_E2E_CATALOG_PRODUCER_ATTEMPT: ${{ needs.build.outputs.catalog_attempt }}", job)
        build = job_block(workflow, "build", "e2e")
        self.assertIn("catalog_attempt: ${{ steps.e2e_catalog.outputs.attempt }}", build)
        self.assertIn('"$GITHUB_RUN_ATTEMPT" >> "$GITHUB_OUTPUT"', build)

    def test_browser_free_jobs_bootstrap_host_tools_and_share_one_store_writer(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        build = job_block(workflow, "build", "e2e")
        report = workflow.partition("  e2e-report:\n")[2]
        for job in (build, report):
            self.assertNotIn("COREPACK_HOME: ${{ runner.", job.partition("    steps:\n")[0])
            self.assertNotRegex(job, r"(?m)^    container:")
            self.assertIn("runs-on: ubuntu-latest", job)
            self.assertIn("uses: pnpm/action-setup@", job)
            self.assertIn("version: 9.15.9", job)
            self.assertIn("uses: actions/setup-node@", job)
            self.assertIn("node-version: '24'", job)
            self.assertIn('pnpm store path --silent', job)
            self.assertIn("path: ${{ steps.pnpm_store.outputs.path }}", job)
            self.assertIn("key: pnpm-e2e-host-${{ runner.os }}-${{ hashFiles('apps/pnpm-lock.yaml') }}", job)
            self.assertLess(job.index("uses: pnpm/action-setup@"), job.index("pnpm store path --silent"))
            self.assertLess(job.index("uses: actions/setup-node@"), job.index("pnpm install --frozen-lockfile"))
            self.assertNotIn("apt-get", job)
        self.assertIn("uses: actions/setup-go@", build)
        self.assertIn("go-version-file: apps/backend/go.mod", build)
        self.assertIn("cache: false", build)
        self.assertIn("uses: actions/cache@", build)
        self.assertIn("uses: actions/cache/restore@", report)
        self.assertNotIn("uses: actions/cache@", report)
        self.assertNotIn("uses: actions/setup-go@", report)
        self.assertNotIn("if: steps.pnpm_store", build)
        self.assertNotIn("if: steps.pnpm_store", report)

    def test_normal_matrix_matches_the_planner_and_keeps_guarded_processes(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        planner = (REPO_ROOT / "apps/web/e2e/scripts/plan-shards.ts").read_text()
        count = int(re.search(r"NORMAL_SHARD_COUNT = (\d+)", planner).group(1))
        self.assertEqual(count, 28)
        families = workflow.split("families: >-\n", 1)[1].split("burst:", 1)[0]
        family = next(f for f in json.loads(families) if f["name"] == "e2e")
        self.assertEqual(family["instances"], [{"shard": n} for n in range(1, count + 1)])
        self.assertIn("name: E2E Shard ${{ matrix.shard }}/28", workflow)

    def test_independent_helper_cache_snapshots_refresh_after_source_changes(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        dependency = "${{ runner.os }}-${{ hashFiles('apps/backend/go.mod', 'apps/backend/go.sum') }}"
        block = job_block(workflow, "helper_compile", "build")
        prefix = f"go-e2e-helpers-{dependency}"
        self.assertIn(f"key: {prefix}-${{{{ github.sha }}}}", block)
        self.assertIn(f"restore-keys: |\n            {prefix}-\n", block)

    def test_build_reuses_optional_daily_cache_without_conditioning_verification(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        build = job_block(workflow, "build", "e2e")
        self.assertIn("uses: ./.github/actions/cache-go", build)
        self.assertIn("family: e2e-build", build)
        self.assertNotIn("key: go-e2e-build-", build)
        self.assertNotIn("steps.cache", build)
        self.assertIn(".github/actions/cache-go/**", workflow)
        self.assertIn(".github/scripts/go-cache-identity.*", workflow)
        self.assertIn("make -C apps/backend build-kandev", build)
        self.assertIn("make build-web-e2e", build)
        action = (REPO_ROOT / ".github/actions/cache-go/action.yml").read_text()
        self.assertEqual(action.count("continue-on-error: true"), 4)
        self.assertIn("key: ${{ steps.identity.outputs.buildKey }}", action)
        self.assertIn("restore-keys: ${{ steps.identity.outputs.buildPrefix }}", action)
        lint = LINT_WORKFLOW.read_text()
        self.assertIn("node --test .github/scripts/go-cache-identity.test.mjs", lint)

    def test_parallel_build_joins_both_children_and_preserves_each_failure(self) -> None:
        source = E2E_WORKFLOW.read_text().split("      - name: Build backend and web\n", 1)[1]
        script = textwrap.dedent(source.split("        run: |\n", 1)[1].split("\n      #", 1)[0])
        # Exercise the actual shell step with independent fake make children.
        for failing in ("", "build-kandev", "build-web-e2e"):
            with self.subTest(failing=failing), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                make = root / "make"
                make.write_text("""#!/usr/bin/env bash
set -eu
if [[ "$1" == -C ]]; then shift 2; fi
target=$1
if [[ "$target" == build-kandev || "$target" == build-web-e2e ]]; then
  touch "$target.started"
  other=build-kandev
  if [[ "$target" == build-kandev ]]; then other=build-web-e2e; fi
  for ((i=0;i<200;i++)); do
    [[ -f "$other.started" ]] && break
    sleep 0.01
  done
  [[ -f "$other.started" ]] || exit 91
  touch "$target.joined"
  [[ "$FAIL_TARGET" != "$target" ]] || exit 7
else
  [[ -f build-kandev.joined && -f build-web-e2e.joined ]] || exit 92
  touch fixture.built
fi
""")
                make.chmod(0o755)
                result = subprocess.run(["bash", "-c", script], cwd=root, timeout=10,
                                        env={**os.environ, "PATH": str(root) + os.pathsep + os.environ["PATH"],
                                             "FAIL_TARGET": failing, "GITHUB_SHA": "test-source"},
                                        capture_output=True, text=True)
                self.assertEqual(result.returncode == 0, not failing, result.stderr)
                self.assertTrue((root / "build-kandev.joined").exists())
                self.assertTrue((root / "build-web-e2e.joined").exists())
                self.assertEqual((root / "fixture.built").exists(), not failing)

    def test_browser_build_and_parallel_compile_preserve_the_original_binary_union(self) -> None:
        workflow = E2E_WORKFLOW.read_text()
        helpers = job_block(workflow, "helper_compile", "build")
        build = job_block(workflow, "build", "e2e")
        self.assertIn("needs: changes", helpers)
        self.assertIn("runs-on: ubuntu-latest", helpers)
        self.assertIn("if: needs.changes.outputs.run == 'true'", helpers)
        self.assertNotIn("continue-on-error", helpers)
        self.assertNotIn("helper_compile", job_block(workflow, "e2e", "playwright_image"))
        host = ["build-kandev", "build-agentctl", "build-agentctl-linux", "build-mock-agent"]
        other = ["build-agentctl-linux-arm64", "build-agentctl-darwin-arm64",
                 "build-agentctl-darwin-amd64", "build-acpdbg", "build-codexdbg", "build-winjob"]
        self.assertIn("make -C apps/backend " + " ".join(host) + " &", build)
        self.assertIn("make -C apps/backend " + " ".join(other), helpers)
        self.assertIn("run: make -C apps/backend build-mock-agent-linux", build)
        self.assertNotIn("make build-backend", build)
        self.assertIn("make build-e2e-delivery-fixture", build)
        def outputs(targets):
            result = subprocess.run(["make", "-n", "-C", str(REPO_ROOT / "apps/backend"), *targets],
                                    capture_output=True, text=True, check=True)
            return set(re.findall(r"-o (bin/[^ ]+)", result.stdout))
        self.assertEqual(outputs(host + other + ["build-mock-agent-linux", "build-e2e-delivery-fixture"]),
                         outputs(["build", "build-mock-agent-linux", "build-e2e-delivery-fixture"]))
        report = workflow.split("  e2e-report:\n", 1)[1]
        self.assertIn("needs: [changes, helper_compile, build,", report)
        self.assertIn("HELPER_COMPILE_RESULT: ${{ needs.helper_compile.result }}", report)
        self.assertIn('"helper_compile:${HELPER_COMPILE_RESULT}"', report)

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
  "$FAKE_DOCKER_SLEEP" 1
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
                    "FAKE_DOCKER_SLEEP": shutil.which("sleep"),
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

        changes_job = job_block(workflow, "changes", "helper_compile")
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

    def test_normal_resource_observation_preserves_native_execution_and_artifacts(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        normal = job_block(workflow, "e2e", "playwright_image")
        self.assertIn("Capture E2E resource budget", normal)
        self.assertIn("Capture E2E resource outcome", normal)
        self.assertIn("TIMEFORMAT=", normal)
        self.assertIn('time bash e2e/scripts/run-planned-shard.sh "$GITHUB_WORKSPACE/e2e-manifests/normal/${E2E_MANIFEST_SHARD}.json"', normal)
        after = normal.partition("- name: Capture E2E resource outcome")[2].partition("- name: Upload blob report")[0]
        self.assertIn("if: always()", after)
        self.assertIn("e2e-resource-before.txt", normal)
        self.assertIn("e2e-resource-after.txt", after)
        self.assertNotIn("apps/web/e2e/blob-report/", after)
        self.assertIn("name: e2e-resources-${{ matrix.shard }}", after)
        self.assertNotIn("KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM", normal)
        self.assertIn('E2E_RUNNER_PROCESSES: "4"', normal)

    def test_resource_timing_propagates_native_failures_and_keeps_stderr(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        step = workflow.partition("      - name: Run planned E2E tests (shard")[2].partition("\n      - name:")[0]
        _, marker, raw_script = step.partition("        run: |\n")
        self.assertTrue(marker, "the guarded command must remain an unconditional timed run")
        script = textwrap.dedent(raw_script)
        command = 'bash e2e/scripts/run-planned-shard.sh "$GITHUB_WORKSPACE/e2e-manifests/normal/${E2E_MANIFEST_SHARD}.json"'
        self.assertEqual(script.count(command), 1)
        for status in (0, 37):
            with self.subTest(status=status):
                native = f"bash -c 'printf native-stderr >&2; exit {status}'"
                result = subprocess.run(
                    ["bash", "-eo", "pipefail", "-c", script.replace(command, native)],
                    text=True, capture_output=True, timeout=5,
                )
                self.assertEqual(result.returncode, status, result.stderr)
                self.assertIn("native-stderr", result.stderr)
                self.assertRegex(result.stderr, r"e2e_resources elapsed_seconds=\d+\.\d+")
                self.assertIn("cpu_user_seconds=", result.stderr)
                self.assertIn("cpu_system_seconds=", result.stderr)

    def test_only_normal_jobs_request_bounded_in_runner_processes(self) -> None:
        workflow = E2E_WORKFLOW.read_text(encoding="utf-8")
        normal_job = job_block(workflow, "e2e", "playwright_image")
        self.assertIn('E2E_RUNNER_PROCESSES: "4"', normal_job)
        self.assertEqual(workflow.count("E2E_RUNNER_PROCESSES:"), 1)
        wrapper = REPO_ROOT / "apps/web/e2e/scripts/run-planned-shard.sh"
        self.assertIn('export E2E_RUNNER_MAX_SHARDS="$(e2e_max_planned_processes)"', wrapper.read_text())

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

        changes = job_block(workflow, "changes", "helper_compile")
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
        self.assertIn("python3 .github/scripts/download-e2e-blobs_test.py", workflow)
        for trigger in ("push", "pull_request", "merge_group"):
            trigger_marker = f"  {trigger}:"
            _, separator, trigger_block_text = workflow.partition(trigger_marker)
            self.assertTrue(separator, f"Lint workflow has no {trigger} trigger")
            self.assertNotIn("    paths:", trigger_block_text.split("\n  ", 1)[0])

    def test_blob_transfer_uses_cli_before_unchanged_native_reconciliation(self) -> None:
        report = E2E_WORKFLOW.read_text().partition("  e2e-report:\n")[2]
        download = report.partition("      - name: Download blob reports\n")[2].partition("      - name:")[0]
        self.assertIn("python3 .github/scripts/download-e2e-blobs.py", download)
        self.assertIn('GH_TOKEN: ${{ github.token }}', download)
        self.assertIn('--run-id "$GITHUB_RUN_ID"', download)
        self.assertIn('--repo "$GITHUB_REPOSITORY"', download)
        self.assertIn('--output apps/web/e2e/blob-reports', download)
        self.assertNotIn("continue-on-error", download)
        self.assertIn('command -v gh', report)
        self.assertIn('--input e2e/blob-reports', report)
        self.assertIn('--catalog "$GITHUB_WORKSPACE/e2e-manifests/native-catalog.json"', report)

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
