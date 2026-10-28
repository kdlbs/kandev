#!/usr/bin/env python3
"""Contract tests for persistence gates and bounded backend test summaries."""

import json
import os
import textwrap
import re
import shlex
import subprocess
import tempfile
from pathlib import Path
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "backend-tests.yml"
BACKEND_WORKFLOW = WORKFLOW
BASE_IMAGE_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "ci-base-image.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"

POSTGRES_18_DIGEST = "sha256:4ef4dbc939d61acea57712655ddb4b4ab27419c913f94cca0cd57cb3ea3c2280"


def step_block(workflow: str, name: str) -> str:
    marker = f"      - name: {name}\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"backend-tests.yml has no {name!r} step")
    return remainder.partition("\n      - name: ")[0]


def job_block(workflow: str, job: str, next_job: str) -> str:
    marker = f"  {job}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"backend-tests.yml has no {job!r} job")
    return remainder.partition(f"\n  {next_job}:\n")[0]


class BackendTestsWorkflowContractTest(unittest.TestCase):
    def setUp(self) -> None:
        self.workflow = WORKFLOW.read_text(encoding="utf-8")
        self.base_image_workflow = BASE_IMAGE_WORKFLOW.read_text(encoding="utf-8")

    def test_linux_go_caches_use_shared_families_and_main_only_publication(self) -> None:
        jobs = (
            ("static_checks", "test_shards", "static", "all"),
            ("test_shards", "repository_tests", "test-${{ matrix.shard }}", "build"),
            ("repository_tests", "test_ambient_env", "repository-${{ matrix.shard }}", "build"),
            ("test_ambient_env", "test", "ambient", "build"),
            ("postgres-boot", "sidebar-resources", "postgres-${{ matrix.cohort }}", "build"),
        )
        publisher = (
            "${{ github.event_name == 'push' && github.ref == 'refs/heads/main' "
            "&& '%s' || 'none' }}"
        )
        for name, next_job, family, mode in jobs:
            with self.subTest(job=name):
                block = job_block(self.workflow, name, next_job)
                self.assertIn("uses: ./.github/actions/cache-go", block)
                self.assertIn(f"family: {family}", block)
                self.assertIn(f"save-mode: {publisher % mode}", block)
                self.assertNotIn("uses: actions/cache@", block)

        static = job_block(self.workflow, "static_checks", "test_shards")
        self.assertIn("GOLANGCI_LINT_CACHE: /root/.cache/golangci-lint", static)
        detect = job_block(self.workflow, "changes", "static_checks")
        self.assertIn(".github/actions/cache-go/**", detect)
        self.assertIn(".github/scripts/go-cache-identity.*", detect)

    def test_windows_build_cache_refreshes_by_suite_without_cohort_duplication(self) -> None:
        discover = step_block(self.workflow, "Resolve Windows Go build cache")
        self.assertIn("go env GOCACHE", discover)
        self.assertIn("go env GOVERSION", discover)
        self.assertIn("go env GOMODCACHE", discover)
        self.assertIn("[DateTime]::UtcNow", discover)
        self.assertIn("yyyy-MM-dd", discover)
        setup = step_block(self.workflow, "Set up Go")
        self.assertIn("cache: false", setup)
        self.assertNotIn("cache-dependency-path", setup)
        cache = step_block(self.workflow, "Cache Windows compiled packages")
        self.assertIn("path: ${{ steps.windows_go_cache.outputs.path }}", cache)
        self.assertIn("${{ matrix.suite }}", cache)
        self.assertNotIn("${{ github.sha }}", cache)
        self.assertIn("${{ steps.windows_go_cache.outputs.day }}", cache)
        self.assertIn("${{ steps.windows_go_cache.outputs.version }}", cache)
        self.assertNotIn("matrix.cohort", cache)
        self.assertIn("restore-keys:", cache)
        self.assertIn("uses: actions/cache/restore@", cache)
        self.assertIn("continue-on-error: true", cache)
        modules = step_block(self.workflow, "Cache Windows Go modules")
        self.assertIn("path: ${{ steps.windows_go_cache.outputs.modulePath }}", modules)
        self.assertNotIn("outputs.path", modules)
        self.assertNotIn("github.sha", modules)
        self.assertNotIn("matrix.suite", modules)
        self.assertIn("continue-on-error: true", modules)
        build_save = step_block(self.workflow, "Save Windows compiled packages")
        self.assertIn("success()", build_save)
        self.assertIn("outputs.cache-hit != 'true'", build_save)
        self.assertIn("matrix.suite != 'process' || matrix.cohort == 1", build_save)
        self.assertIn("steps.windows_build_cache.outputs.cache-primary-key", build_save)
        self.assertIn("continue-on-error: true", build_save)
        module_save = step_block(self.workflow, "Save Windows Go modules")
        self.assertIn("success()", module_save)
        self.assertIn("matrix.suite == 'compile'", module_save)
        self.assertIn("outputs.cache-hit != 'true'", module_save)
        self.assertIn("steps.windows_module_cache.outputs.cache-primary-key", module_save)
        self.assertIn("continue-on-error: true", module_save)

    def test_postgres_16_uses_fixed_catalog_commands(self) -> None:
        self.assertNotIn("PostgresDSNFromEnv", self.workflow)
        self.assertNotIn("mapfile -t postgres_packages", self.workflow)
        persistence_gate = step_block(self.workflow, "Run fixed PostgreSQL 16 persistence gates")
        self.assertIn("./internal/coordinator", persistence_gate)
        self.assertIn("./internal/coordinator/ledger", persistence_gate)
        self.assertIn("./internal/persistence/storeconformance ./internal/backendapp", self.workflow)
        for test_name in (
            "TestStoreCatalogCompleteness",
            "TestStoreConformance",
            "TestPreviousStableUpgrade",
            "TestPostgresBootInitializesRepositories",
        ):
            self.assertIn(test_name, self.workflow)

    def test_postgres_16_scale_service_has_parallel_query_shared_memory(self) -> None:
        job = self.workflow.partition("  postgres-boot:\n")[2].partition("  sidebar-resources:\n")[0]
        self.assertIn("--shm-size=256m", job.partition("    defaults:\n")[0])

    def test_sql_guard_and_local_conformance_are_explicit(self) -> None:
        self.assertIn("go run ./cmd/sqlguard ./internal", self.workflow)
        self.assertIn("Run catalog and SQLite conformance gates", self.workflow)
        self.assertIn("TestUpgradeFixtureManifest", self.workflow)

    def test_postgres_scale_filter_preserves_other_sqlite_and_all_postgres_cases(self) -> None:
        gate = step_block(self.workflow, "Run fixed PostgreSQL 16 persistence gates")
        skip = "^TestDeliveryRetentionStorage(ProtectedGap)?$/SQLite$"
        self.assertIn("--mode postgres", gate)
        helper = (REPO_ROOT / ".github/scripts/repository-test-shard.py").read_text()
        self.assertIn('SCALE_SKIP = "' + skip + '"', helper)
        with tempfile.TemporaryDirectory(prefix="backend-workflow-contract-") as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module workflowcontract\n\ngo 1.20\n")
            source = 'package workflowcontract\nimport "testing"\n'
            for name in ("Storage", "StorageProtectedGap", "StorageAccounting", "StorageQueryPlan"):
                source += f'func TestDeliveryRetention{name}(t *testing.T) {{\n'
                source += 'for _, engine := range []string{"SQLite", "Postgres"} { t.Run(engine, func(t *testing.T) {}) }\n}\n'
            (root / "filter_test.go").write_text(source)
            result = subprocess.run(
                ["go", "test", "-json", "-count=1", "-skip", skip, "./"],
                cwd=root, capture_output=True, text=True, timeout=60,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            passed = {
                event["Test"] for line in result.stdout.splitlines()
                if (event := json.loads(line)).get("Action") == "pass" and "Test" in event
            }
        for name in ("Storage", "StorageProtectedGap"):
            self.assertNotIn(f"TestDeliveryRetention{name}/SQLite", passed)
            self.assertIn(f"TestDeliveryRetention{name}/Postgres", passed)
        for name in ("StorageAccounting", "StorageQueryPlan"):
            for engine in ("SQLite", "Postgres"):
                self.assertIn(f"TestDeliveryRetention{name}/{engine}", passed)

    def test_windows_warmup_compiles_exact_targeted_package_unions_without_tests(self) -> None:
        for suite in ("settings", "persistence"):
            warmup = step_block(self.workflow, "Compile Windows " + suite + " test packages")
            self.assertIn("if: matrix.suite == '" + suite + "'", warmup)
            compile_budget = 20 if suite == "persistence" else 10
            self.assertIn(f"timeout-minutes: {compile_budget}", warmup)
            self.assertIn("GOMAXPROCS: '2'", warmup)
            self.assertIn("GOMEMLIMIT: 2GiB", warmup)
            self.assertIn("go test -trimpath -tags fts5 -race -p=2 -run '^$' -count=1 -timeout=30s", warmup)
            selected = set()
            for step in self.workflow.split("      - name: ")[1:]:
                if not step.startswith("Test Windows ") or "if: matrix.suite == '" + suite + "'" not in step:
                    continue
                selected.update(re.findall(r"\./internal/[\w/.-]+", step))
            self.assertTrue(selected)
            self.assertEqual(set(re.findall(r"\./internal/[\w/.-]+", warmup)), selected)
            self.assertNotIn("continue-on-error", warmup)

    def test_four_ordinary_shards_cover_native_packages_once_except_repository(self) -> None:
        source = self.workflow.split("  test_shards:\n", 1)[1].split("  repository_tests:\n", 1)[0]
        self.assertEqual(re.findall(r"- shard: (\d+)\n            index: (\d+)", source),
                         [(str(n + 1), str(n)) for n in range(4)])
        step = step_block(self.workflow, "Run tests")
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        script = script.split("# Keep complete JSON", 1)[0]
        script += "\nprintf '%s\\n' \"${shard_packages[@]}\"\n"
        packages = ["github.com/kandev/kandev/internal/future/part" + str(n) for n in range(40)]
        heavy = ["github.com/kandev/kandev/internal/" + name for name in
                 ("backendapp", "agentctl/server/adapter/transport/acp", "coordinator")]
        packages.extend(heavy)
        repository = "github.com/kandev/kandev/internal/task/repository/sqlite"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            go = root / "go"
            go.write_text("#!/usr/bin/env bash\n[[ \"$*\" == 'list ./...' ]] || exit 9\nprintf '%s\\n' \"$NATIVE_PACKAGES\"\n")
            go.chmod(0o755)
            selected = []
            for index in range(4):
                result = subprocess.run(["bash", "-c", script], capture_output=True, text=True, check=True,
                                        env={**os.environ, "PATH": str(root) + os.pathsep + os.environ["PATH"],
                                             "NATIVE_PACKAGES": "\n".join(packages + [repository]),
                                             "SHARD_INDEX": str(index), "SHARD_NUMBER": str(index + 1)})
                assigned = [line for line in result.stdout.splitlines() if line.startswith("github.com/")]
                self.assertEqual(set(assigned) & set(heavy), set(heavy) if index == 2 else set())
                selected.extend(assigned)
            self.assertCountEqual(selected, packages)

    def test_repository_cohorts_are_required_and_excluded_only_from_ordinary_jobs(self) -> None:
        normal = self.workflow.split("  test_shards:\n", 1)[1].split("  test_ambient_env:\n", 1)[0]
        self.assertIn('if [[ "${package}" == github.com/kandev/kandev/internal/task/repository/sqlite ]]; then continue; fi', normal)
        self.assertIn("  repository_tests:\n", self.workflow)
        repository = self.workflow.split("  repository_tests:\n", 1)[1].split("  test_ambient_env:\n", 1)[0]
        self.assertIn("shard: [1, 2, 3]", repository)
        self.assertIn("--mode normal", repository)
        self.assertIn("if-no-files-found: error", repository)
        self.assertIn("fail-fast: false", repository)
        gate = self.workflow.split("  test:\n", 1)[1].split("  postgres-boot:\n", 1)[0]
        self.assertIn("- repository_tests", gate)
        self.assertIn('"repository_tests:${REPOSITORY_TESTS_RESULT}"', gate)
        postgres = self.workflow.split("  postgres-boot:\n", 1)[1].split("  sidebar-resources:\n", 1)[0]
        self.assertIn("cohort: [packages, 1, 2, 3]", postgres)
        self.assertIn("--mode postgres", postgres)
        self.assertIn('[[ "${package}" == ./internal/task/repository/sqlite ]] || ordinary_packages+=("${package}")', postgres)
        self.assertIn('"${ordinary_packages[@]}"', postgres)
        self.assertIn("if-no-files-found: error", postgres)

    def test_sidebar_memory_partitions_preserve_every_leaf_including_future_sort_keys(self) -> None:
        step = step_block(self.workflow, "Measure isolated native memory and pooled retention")
        self.assertIn("SIDEBAR_RESOURCE_MODE: ${{ matrix.mode }}", step)
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        with tempfile.TemporaryDirectory(prefix="sidebar-cohorts-") as directory:
            root = Path(directory)
            (root / "go.mod").write_text("module fixture\n\ngo 1.20\n")
            package = root / "internal/task/repository/sqlite"
            package.mkdir(parents=True)
            (package / "fixture_test.go").write_text("""package sqlite
import "testing"
func TestSidebarQueryPreparationMemory(t *testing.T) { t.Run("0",func(t *testing.T){}) }
func TestSidebarQueryMaximumInputMemory(t *testing.T) { t.Run("maximum",func(t *testing.T){}) }
func TestSidebarQueryPoolMemoryPlateau(t *testing.T) {
 for _, key := range []string{"lastActivityAt","runningFirstActivity","state","updatedAt","createdAt","title","custom","futureSort"} {
  t.Run(key+"/none/asc",func(t *testing.T){})
 }
 t.Run("alternating",func(t *testing.T){})
}
""")
            leaves = []
            for mode in ("memory-1", "memory-2", "memory-3"):
                result = subprocess.run(["bash", "-c", script], cwd=root, capture_output=True,
                                        text=True, timeout=60,
                                        env={**os.environ, "SIDEBAR_RESOURCE_MODE": mode})
                self.assertEqual(result.returncode, 0, result.stderr)
                leaves.extend(e["Test"] for line in result.stdout.splitlines()
                              if (e := json.loads(line)).get("Action") == "pass" and "/" in e.get("Test", ""))
            expected = ["TestSidebarQueryPreparationMemory/0", "TestSidebarQueryMaximumInputMemory/maximum",
                        "TestSidebarQueryPoolMemoryPlateau/alternating"]
            expected += ["TestSidebarQueryPoolMemoryPlateau/" + key + "/none/asc" for key in
                         ("lastActivityAt", "runningFirstActivity", "state", "updatedAt", "createdAt", "title", "custom", "futureSort")]
            self.assertCountEqual(leaves, expected)

    def test_postgres_18_is_a_required_pinned_gate(self) -> None:
        self.assertIn("postgres-18:", self.workflow)
        self.assertIn("postgres-18@" + POSTGRES_18_DIGEST, self.workflow)
        self.assertIn("POSTGRES_18_RESULT", self.workflow)
        self.assertIn('"postgres-18:${POSTGRES_18_RESULT}"', self.workflow)
        self.assertIn("TestPreviousStableUpgrade", self.workflow)

    def test_windows_job_has_headroom_for_hosted_runner_variance(self) -> None:
        _, marker, windows_job = self.workflow.partition("  test-windows:\n")
        self.assertTrue(marker)
        timeout = re.search(r"(?m)^\s*timeout-minutes:\s*(\d+)\s*$", windows_job)
        self.assertIsNotNone(timeout)
        self.assertGreaterEqual(int(timeout.group(1)), 90)
        self.assertIn("go test -race -timeout 25m ./cmd/windows-process-tests", windows_job)

    def test_windows_suites_run_independently_without_fail_fast(self) -> None:
        windows_job = self.workflow.partition("  test-windows:\n")[2]
        for value in (f"process, cohort: {n}, label: process {n}/4" for n in range(1, 5)):
            self.assertIn("{suite: " + value + "}", windows_job)
        for value in ("native", "compile", "persistence", "settings"):
            self.assertIn("{suite: " + value + ", label: " + value + "}", windows_job)
        self.assertIn("fail-fast: false", windows_job)
        self.assertIn("name: Backend (windows, ${{ matrix.label }})", windows_job)
        self.assertNotIn("continue-on-error", windows_job.partition("    steps:\n")[0])
        optional_cache_steps = {
            "Cache Windows Go modules", "Cache Windows compiled packages",
            "Save Windows Go modules", "Save Windows compiled packages",
        }
        for step in windows_job.split("\n      - name: ")[1:]:
            name = step.partition("\n")[0]
            if name not in optional_cache_steps:
                self.assertNotIn("continue-on-error", step, name)
        process = step_block(windows_job, "Test Windows process package")
        self.assertIn("if: matrix.suite == 'process'", process)
        self.assertIn("go run ./cmd/windows-process-tests -cohorts 4 -cohort $env:WINDOWS_PROCESS_COHORT", process)
        self.assertIn("WINDOWS_PROCESS_COHORT: ${{ matrix.cohort }}", process)
        helper = step_block(windows_job, "Test Windows process cohort runner")
        self.assertIn("if: matrix.suite == 'process'", helper)
        self.assertIn("go test -race -timeout 25m ./cmd/windows-process-tests", helper)
        native = step_block(windows_job, "Test windows-sensitive packages")
        self.assertNotIn("./internal/agentctl/server/process/", native)
        for package in (
            "./internal/agent/runtime/agentctl/launcher/...",
            "./internal/common/ptyexec/...",
            "./internal/backendapp/ownershiplock/...",
            "./internal/system/storage/workspaces/...",
        ):
            self.assertIn(package, native)

    def test_windows_native_checks_remain_required_in_native_suite(self) -> None:
        windows_job = self.workflow.partition("  test-windows:\n")[2]
        for name in (
            "Test Windows isolated instance safety",
            "Test Windows copy-file glob tokenization",
            "Test windows-sensitive packages",
            "Test Windows helper console behavior",
            "Test Windows Git executable memo",
            "Test Windows Hermes ACP detection",
            "Test Windows managed runtime cache repair",
            "Test Windows port collision handling",
            "Test Windows path semantics",
            "Test Windows SQLite database paths",
            "Test Cursor project slug path normalization",
            "Test Windows agent-survival flag availability",
        ):
            with self.subTest(step=name):
                self.assertIn("if: matrix.suite == 'native'", step_block(windows_job, name))
        gate = self.workflow.partition("  test:\n")[2].partition("  postgres-boot:\n")[0]
        self.assertIn("- test-windows", gate)
        self.assertIn("TEST_WINDOWS_RESULT: ${{ needs['test-windows'].result }}", gate)
        self.assertIn('"test-windows:${TEST_WINDOWS_RESULT}"', gate)

    def test_windows_cold_compilation_has_headroom_without_extending_test_deadlines(self) -> None:
        for name in ("Test Windows document attachment publication", "Test Windows profile enabled omission"):
            step = step_block(self.workflow, name)
            self.assertIn("GOMEMLIMIT = '2GiB'", step)
            self.assertIn("timeout-minutes: 6", step)
            self.assertIn("-timeout=4m", step)
            self.assertIn("GOMAXPROCS = '2'", step)

    def test_windows_expensive_steps_have_one_required_owner(self) -> None:
        windows = self.workflow.split("  test-windows:\n", 1)[1]
        groups = {
            "compile": ("Build", "Vet"),
            "persistence": ("Test Windows document attachment publication",
                            "Test Windows executor profile scripts",
                            "Test Windows workspace settings persistence",
                            "Test Windows repository checkout defaults"),
            "settings": ("Test Windows profile enabled omission",
                         "Test Windows workflow start selection"),
        }
        for group, steps in groups.items():
            for name in steps:
                with self.subTest(step=name):
                    self.assertEqual(windows.count(f"      - name: {name}\n"), 1)
                    block = step_block(windows, name)
                    self.assertIn(f"if: matrix.suite == '{group}'", block)
                    self.assertNotIn("continue-on-error", block)
        self.assertIn("runs-on: windows-latest", windows)

    def test_windows_git_executable_memo_runs_in_native_suite(self) -> None:
        step = step_block(self.workflow, "Test Windows Git executable memo")
        self.assertIn("if: matrix.suite == 'native'", step)
        self.assertNotIn("continue-on-error", step)
        command = shlex.split(step.partition("run: ")[2].splitlines()[0])
        self.assertEqual(command[:2], ["go", "test"])
        self.assertIn("./internal/common/subproc", command)
        self.assertIn("-race", command)
        self.assertEqual(command[command.index("-tags") + 1], "fts5")
        self.assertEqual(command[command.index("-timeout") + 1], "10m")
        selection = command[command.index("-run") + 1]
        source = (REPO_ROOT / "apps/backend/internal/common/subproc/git_command_test.go").read_text()
        tests = re.findall(r"^func (TestNewGitCommand\w+)\(", source, re.MULTILINE)
        self.assertIn("TestNewGitCommandResolvesAgainWhenPathExtChanges", tests)
        for name in tests:
            with self.subTest(test=name):
                self.assertRegex(name, selection)
        makefile = (REPO_ROOT / "apps/backend/Makefile").read_text()
        target = makefile.partition("\ntest-windows:\n")[2].partition("\n## ")[0]
        self.assertIn('./internal/common/subproc -run "^TestNewGitCommand"', target)
        self.assertIn('-tags fts5 -race -v -timeout 10m ./internal/common/subproc', target)

    def test_windows_hermes_detection_executes_bounded_regressions(self) -> None:
        windows_job = self.workflow.partition("  test-windows:\n")[2]
        step = step_block(windows_job, "Test Windows Hermes ACP detection")
        self.assertIn("runs-on: windows-latest", windows_job)
        self.assertIn("if: matrix.suite == 'native'", step)
        self.assertNotIn("continue-on-error", step)
        self.assertIn("timeout-minutes: 10", step)
        command = shlex.split(step.partition("run: ")[2].splitlines()[0])
        self.assertEqual(command[:2], ["go", "test"])
        for flag in ("-race", "-v", "-count=1", "-p=1"):
            self.assertIn(flag, command)
        self.assertEqual(command[command.index("-timeout") + 1], "5m")
        self.assertEqual(command[command.index("-tags") + 1], "fts5")
        self.assertEqual([v for v in command if v.startswith("./")], ["./internal/agent/agents"])
        selection = command[command.index("-run") + 1]
        tests = []
        for filename in ("hermes_acp_test.go", "detect_test.go"):
            source = (REPO_ROOT / "apps/backend/internal/agent/agents" / filename).read_text()
            tests.extend(re.findall(r"^func (Test(?:HermesACP|RunCommandCheck)\w+)\(", source, re.MULTILINE))
        self.assertIn("TestHermesACP_DetectionRequiresACPCheck", tests)
        self.assertIn("TestHermesACP_CallerCancellationDuringRetry", tests)
        self.assertIn("TestRunCommandCheck_SeparatesTimeoutFromFailure", tests)
        self.assertIn("TestRunCommandCheck_NonzeroExitBeforeDeadlineObservation", tests)
        self.assertIn("TestRunCommandCheck_ExpiredBoundBeforeStart", tests)
        self.assertIn("TestRunCommandCheck_SuccessBeforeDeadlineObservation", tests)
        for name in tests:
            with self.subTest(test=name):
                self.assertRegex(name, selection)
        self.assertIsNone(re.search(selection, "TestClaudeACP_Detection"))
        makefile = (REPO_ROOT / "apps/backend/Makefile").read_text()
        target = makefile.partition("\ntest-windows:\n")[2].partition("\n## ")[0]
        lines = [line for line in target.splitlines() if "./internal/agent/agents" in line]
        self.assertEqual(len(lines), 1)
        command = shlex.split(lines[0])
        self.assertEqual(command[command.index("-run") + 1].replace("$$", "$"), selection)
        for flag in ("-race", "-v", "-count=1", "-p=1"):
            self.assertIn(flag, command)
        self.assertEqual(command[command.index("-timeout") + 1], "5m")

    def test_windows_runtime_observation_shims_run_in_native_suite(self) -> None:
        step = step_block(self.workflow, "Test Windows runtime observation command shims")
        self.assertIn("if: matrix.suite == 'native'", step)
        self.assertIn("TestRuntimeObservationRunsWindowsCommandShimWithSpaces", step)

    def test_windows_loopback_listener_coverage_is_required(self) -> None:
        step = step_block(self.workflow, "Test Windows loopback listeners")
        makefile = (REPO_ROOT / "apps" / "backend" / "Makefile").read_text(encoding="utf-8")
        windows_target = makefile.partition("\ntest-windows:\n")[2].partition("\n## ")[0]
        self.assertTrue(windows_target)
        self.assertIn("if: matrix.suite == 'native'", step)
        self.assertIn("shell: bash", step)
        self.assertIn("set -euo pipefail", step)
        self.assertNotIn("continue-on-error", step)
        for package in (
            "./internal/common/portutil",
            "./internal/common/netprobe",
            "./internal/agentctl/server/config",
            "./cmd/plugin-fixture",
            "./internal/gateway/websocket",
            "./internal/launcher",
            "./internal/agentctl/server/api",
        ):
            with self.subTest(package=package):
                self.assertIn(package, step)
                self.assertIn(package, windows_target)
        for test_name in (
            "ConcurrentFailingStarts", "ResolveAndBind", "Tunnel", "TestCanBind",
            "TestWaitForHealthBypassesHTTPProxyForSpecificBind",
            "TestHandleVscodeStatus_AfterStart", "TestVscodeOpenFile_WaitsForRunning_E2E",
        ):
            with self.subTest(test=test_name):
                self.assertIn(test_name, step)
                self.assertIn(test_name, windows_target)

    def test_ordinary_linux_shards_keep_the_ten_minute_package_budget(self) -> None:
        test_step = step_block(self.workflow, "Run tests")
        self.assertIn("-timeout=10m", test_step)
        self.assertNotIn("-timeout=120m", test_step)

    def test_base_image_mirrors_postgres_18_at_the_same_digest(self) -> None:
        self.assertIn("POSTGRES_18_DIGEST: " + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_SOURCE: docker.io/library/postgres:18@" + POSTGRES_18_DIGEST, self.base_image_workflow)
        self.assertIn("POSTGRES_18_TARGET: ghcr.io/kdlbs/kandev-ci:postgres-18", self.base_image_workflow)
        self.assertIn("docker buildx imagetools create", self.base_image_workflow)
        self.assertIn('--tag "${POSTGRES_18_TARGET}"', self.base_image_workflow)

    def test_contract_is_registered_in_the_unfiltered_lint_workflow(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        self.assertIn("python3 .github/scripts/backend-tests-workflow-contract_test.py", lint_workflow)
        self.assertIn("python3 .github/scripts/bounded-step-summary_test.py", lint_workflow)

    def test_generated_report_is_redirected_then_published_with_a_bound(self) -> None:
        checkout = step_block(self.workflow, "Checkout Go test reporter")
        generate = step_block(self.workflow, "Generate test report")
        publish = step_block(self.workflow, "Publish bounded test report summary")

        temporary_summary = "$RUNNER_TEMP/backend-test-summary-${BACKEND_TEST_SHARD}.md"
        self.assertIn("uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10", checkout)
        self.assertIn("repository: robherley/go-test-action", checkout)
        self.assertIn("ref: 2f859e0c8769d755d3174eecb9af8f64660827f3", checkout)
        self.assertIn("path: .github/vendor/go-test-action", checkout)
        self.assertIn(f'report_summary="{temporary_summary}"', generate)
        self.assertIn("BACKEND_TEST_SHARD: ${{ matrix.shard }}", generate)
        self.assertNotIn("${{ runner.temp }}", generate)
        self.assertIn(': > "$report_summary"', generate)
        self.assertIn('GITHUB_STEP_SUMMARY="$report_summary"', generate)
        self.assertIn('INPUT_FROMJSONFILE="$GITHUB_WORKSPACE/apps/backend/test-results-${BACKEND_TEST_SHARD}.json"', generate)
        self.assertIn('INPUT_MODULEDIRECTORY="$GITHUB_WORKSPACE/apps/backend"', generate)
        self.assertIn('INPUT_OMIT="successful"', generate)
        self.assertIn('node ".github/vendor/go-test-action/dist/index.js"', generate)
        self.assertNotIn("uses: robherley/go-test-action", generate)
        self.assertIn("if: always()", publish)
        self.assertIn("python3 .github/scripts/bounded-step-summary.py", publish)
        self.assertIn(f'--input "{temporary_summary}"', publish)
        self.assertIn('--output "$GITHUB_STEP_SUMMARY"', publish)
        self.assertIn("--diagnostics-label", publish)
        self.assertIn("backend-test-results-${BACKEND_TEST_SHARD}", publish)
        self.assertIn("$GITHUB_RUN_ID#artifacts", publish)

    def test_full_json_diagnostics_remain_in_the_named_artifact(self) -> None:
        upload = step_block(self.workflow, "Upload test artifacts")
        self.assertIn("if: always()", upload)
        self.assertIn("name: backend-test-results-${{ matrix.shard }}", upload)
        self.assertIn("apps/backend/test-results-${{ matrix.shard }}.json", upload)

    def test_summary_writer_changes_run_the_backend_workflow(self) -> None:
        detect = step_block(self.workflow, "Detect relevant changes")
        self.assertIn(".github/scripts/bounded-step-summary.py", detect)


if __name__ == "__main__":
    unittest.main()
