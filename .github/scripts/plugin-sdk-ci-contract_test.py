"""SDK-only changes must exercise the SDK and every host consumer in CI."""

from importlib import import_module
from pathlib import Path
import re
import unittest

matcher = import_module("changed-paths")
ROOT = Path(__file__).resolve().parents[2]


class SDKSelectionTests(unittest.TestCase):
    # @covers AC-PLUGINS-SDK-DISTRIBUTION-004.1
    def test_sdk_only_changes_select_host_and_e2e(self):
        for name in ("backend-tests.yml", "e2e-tests.yml"):
            text = (ROOT / ".github/workflows" / name).read_text()
            block = re.search(r"PATTERNS: \|\n((?:            .*\n)+)", text)
            self.assertIsNotNone(block)
            paths = matcher.Filter(block[1].splitlines())
            for path in ("pluginsdk/serve.go", "pluginsdk/go.mod", "pluginsdk/go.sum",
                         "pluginsdk/proto/kandev/plugin/v1/plugin.proto", "pluginsdk/Makefile",
                         "scripts/pluginsdk/consumer-smoke.mjs",
                         ".github/scripts/plugin-sdk-ci-contract_test.py"):
                with self.subTest(workflow=name, path=path):
                    self.assertTrue(paths.includes(path), path)
            self.assertFalse(paths.includes("pluginsdk/README.md"))

    def test_release_sdk_changes_select_the_required_artifact_job(self):
        text = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        block = re.search(r"PATTERNS: \|\n((?:            .*\n)+)", text)
        paths = matcher.Filter(block[1].splitlines())
        for path in ("scripts/release/plugin-sdk.mjs", "scripts/release/sdk-assets.mjs", "scripts/release/publish-plugin-sdk.sh", "scripts/release/plugin-sdk-contract-inputs.json"):
            self.assertTrue(paths.includes(path), path)

    def test_sdk_has_explicit_race_vet_and_contract_checks(self):
        text = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        self.assertIn("working-directory: pluginsdk", text)
        self.assertIn("go test -trimpath -race ./...", text)
        self.assertIn("go vet ./...", text)
        self.assertIn("python3 .github/scripts/plugin-sdk-ci-contract_test.py", text)

    def test_runtime_shards_fetch_the_legacy_sdk_release_source(self):
        text = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        job = text.split("  test_shards:\n", 1)[1].split("\n  test_repository", 1)[0]
        checkout = job.split("      - name: Checkout code\n", 1)[1].split("\n      - name:", 1)[0]
        self.assertIn("fetch-depth: 0", checkout)

    def test_host_fixture_build_and_artifact_checks_remain_selected(self):
        backend = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        e2e = (ROOT / ".github/workflows/e2e-tests.yml").read_text()
        self.assertIn("scripts/pluginsdk/check-proto-generation.mjs", backend)
        self.assertIn("scripts/pluginsdk/consumer-smoke.test.mjs", backend)
        self.assertIn("consumer-smoke.mjs --mode local --backend-only", backend)
        self.assertNotIn("--skip-compatibility-check", backend)
        self.assertIn("make -C apps/backend e2e-plugin-ui e2e-plugin-package", e2e)
        self.assertIn("make -C apps/backend build-kandev build-agentctl", e2e)

    def test_docker_builder_is_part_of_required_backend_gate(self):
        text = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        self.assertIn("docker build --target go-builder -f e2e.Dockerfile", text)
        gate = text.split("  test:\n", 1)[1]
        self.assertIn("      - sdk_builder", gate)
        self.assertIn("SDK_BUILDER_RESULT: ${{ needs.sdk_builder.result }}", gate)
        self.assertIn('"sdk_builder:${SDK_BUILDER_RESULT}"', gate)

    def test_packed_sdk_has_a_required_independent_consumer_job(self):
        text = (ROOT / ".github/workflows/backend-tests.yml").read_text()
        self.assertIn("  sdk_package:\n", text)
        gate = text.split("  test:\n", 1)[1]
        self.assertIn("      - sdk_package", gate)
        self.assertIn('"sdk_package:${SDK_PACKAGE_RESULT}"', gate)
        self.assertIn("node --test scripts/pluginsdk/sync-record.test.mjs scripts/pluginsdk/pack-candidate.test.mjs", text)
        self.assertIn("pack-config.json", text)
        job = text.split("  sdk_package:\n", 1)[1].split("  static_checks:\n", 1)[0]
        self.assertIn("consumer-smoke.mjs --mode local\n", job)

    def test_docker_build_preserves_the_backend_replacement_layout(self):
        stage = (ROOT / "e2e.Dockerfile").read_text().split("FROM node:", 1)[0]
        self.assertIn("WORKDIR /build/apps/backend", stage)
        self.assertLess(stage.index("COPY pluginsdk/ /build/pluginsdk/"), stage.index("RUN go mod download"))

    def test_sdk_changes_cannot_take_the_e2e_test_only_shortcut(self):
        classifier = import_module("ci-test-only-changes")
        for path in ("pluginsdk/serve.go", "pluginsdk/go.mod", "pluginsdk/go.sum",
                     "pluginsdk/proto/kandev/plugin/v1/plugin.proto", "scripts/pluginsdk/consumer-smoke.mjs"):
            diff = (":100644 100644 " + "a" * 40 + " " + "b" * 40 + " M\0" + path + "\0").encode()
            self.assertFalse(classifier.classify(diff, "pull_request")[0], path)

    def test_caches_include_both_module_identities(self):
        for name in ("backend-tests.yml", "e2e-tests.yml"):
            text = (ROOT / ".github/workflows" / name).read_text()
            for expression in re.findall(r"hashFiles\([^\n]*apps/backend/go.mod[^\n]*\)", text):
                with self.subTest(workflow=name, expression=expression):
                    self.assertIn("pluginsdk/go.mod", expression)
                    self.assertIn("pluginsdk/go.sum", expression)


if __name__ == "__main__":
    unittest.main()
