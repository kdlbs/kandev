#!/usr/bin/env python3
"""Contract tests for the maintainer release workflow."""

import re
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "release.yml"
DIAGNOSTICS_PATH = REPO_ROOT / ".github" / "scripts" / "collect-macos-desktop-diagnostics.sh"
WORKFLOW = WORKFLOW_PATH.read_text()
DIAGNOSTICS = DIAGNOSTICS_PATH.read_text()


def step_block(name: str) -> str:
    marker = f"      - name: {name}"
    start = WORKFLOW.find(marker)
    if start == -1:
        raise AssertionError(f"step not found: {name}")
    next_step = re.search(r"\n      - (?:name|uses): ", WORKFLOW[start + 1 :])
    end = len(WORKFLOW) if next_step is None else start + 1 + next_step.start()
    return WORKFLOW[start:end]


def job_block(name: str) -> str:
    marker = f"  {name}:"
    start = WORKFLOW.find(marker)
    if start == -1:
        raise AssertionError(f"job not found: {name}")
    next_job = re.search(r"\n  [a-zA-Z0-9_-]+:\n", WORKFLOW[start + 1 :])
    end = len(WORKFLOW) if next_job is None else start + 1 + next_job.start()
    return WORKFLOW[start:end]


class ReleaseWorkflowContractTest(unittest.TestCase):
    def test_backfill_tag_input_uses_existing_tag_without_recreating_it(self) -> None:
        self.assertIn("backfill_tag:", WORKFLOW)
        self.assertIn("BACKFILL_TAG: ${{ inputs.backfill_tag }}", WORKFLOW)
        self.assertIn("Backfill existing release tag:", step_block("Compute next version"))
        self.assertIn("backfill_tag is set; the 'bump' input", WORKFLOW)
        self.assertIn("backfill_tag cannot be used: no existing release tags found.", WORKFLOW)
        self.assertIn("must be the latest release tag", WORKFLOW)
        self.assertIn("(expected ${next})", WORKFLOW)
        self.assertIn('BACKFILL_REF="refs/tags/$BACKFILL_TAG"', WORKFLOW)
        self.assertIn('echo "ref=$BACKFILL_REF" >> "$GITHUB_OUTPUT"', WORKFLOW)
        self.assertIn('echo "ref=refs/tags/$TAG" >> "$GITHUB_OUTPUT"', WORKFLOW)
        self.assertNotIn("ref: ${{ needs.prepare.outputs.tag }}", WORKFLOW)

        for path in (
            "apps/cli/package.json",
            "apps/desktop/package.json",
            "apps/desktop/src-tauri/tauri.conf.json",
            "apps/desktop/src-tauri/Cargo.toml",
            "apps/desktop/src-tauri/Cargo.lock",
        ):
            self.assertIn(path, WORKFLOW)

        for name in (
            "Bump version + generate CHANGELOG (in working tree)",
            "Create release PR + squash-merge + tag",
        ):
            self.assertIn("inputs.backfill_tag == ''", step_block(name))

    def test_backfill_tag_still_runs_build_and_publish_jobs(self) -> None:
        for name in (
            "build-web",
            "build-bundles",
            "build-desktop",
            "docker-amd64",
            "docker-arm64",
            "docker-manifest",
            "docker-universal-amd64",
            "docker-universal-arm64",
            "docker-universal-manifest",
            "publish-release",
            "publish-npm",
            "update-homebrew-tap",
        ):
            block = job_block(name)
            self.assertIn("if: ${{ !inputs.dry_run", block)
            self.assertNotIn("inputs.backfill_tag == ''", block)

    def test_updater_signing_validation_uses_workflow_control_revision(self) -> None:
        build_desktop = job_block("build-desktop")
        self.assertIn("ref: ${{ needs.prepare.outputs.ref }}", build_desktop)

        detect = step_block("Detect Tauri updater signing input")
        self.assertIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", detect)
        self.assertIn("gh api", detect)
        self.assertIn("$RUNNER_TEMP/updater-signing-ready.sh", detect)
        self.assertIn(
            "scripts/release/updater-signing-ready.sh?ref=${{ github.workflow_sha }}",
            detect,
        )
        self.assertIn('if [ -z "$TAURI_SIGNING_PRIVATE_KEY" ]', detect)
        self.assertLess(
            detect.index('if [ -z "$TAURI_SIGNING_PRIVATE_KEY" ]'),
            detect.index("gh api"),
        )
        self.assertIn('bash "$helper"', detect)
        self.assertNotIn("bash scripts/release/updater-signing-ready.sh", detect)

    def test_desktop_asset_validation_uses_workflow_control_revision(self) -> None:
        for job in ("build-desktop", "publish-release"):
            block = job_block(job)
            self.assertIn("ref: ${{ needs.prepare.outputs.ref }}", block)
            self.assertIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", block)
            self.assertIn(
                "scripts/release/verify-desktop-assets.sh?ref=${{ github.workflow_sha }}",
                block,
            )
            self.assertIn("DESKTOP_ASSET_VERIFIER=$helper", block)
            self.assertIn('"$DESKTOP_ASSET_VERIFIER"', block)
            self.assertNotIn('scripts/release/verify-desktop-assets.sh "', block)

    def test_linux_appimage_build_installs_xdg_open(self) -> None:
        install = step_block("Install Linux desktop dependencies")
        self.assertIn("startsWith(matrix.platform, 'linux-')", install)
        self.assertIn("xdg-utils", install)
        self.assertIn("command -v xdg-open", install)

    def test_updater_manifest_date_dereferences_annotated_tag(self) -> None:
        generate = step_block("Generate and verify updater manifest")
        self.assertIn("git show -s --format=%cI", generate)
        self.assertRegex(
            generate,
            r"needs\.prepare\.outputs\.tag \}\}\^\{commit\}",
        )

    def test_macos_dmg_build_has_retry_timeout_and_diagnostics(self) -> None:
        build = step_block("Build Tauri desktop app")
        self.assertIn("timeout-minutes: 70", build)
        self.assertIn("TAURI_BUNDLE_ATTEMPTS:", build)
        self.assertIn("run_with_timeout", build)
        self.assertIn("collect_macos_desktop_diagnostics", build)
        self.assertIn("DESKTOP_DIAGNOSTICS_SCRIPT", build)
        self.assertIn("terminate_process_tree_with_signal", build)
        self.assertIn("else\n              status=$?", build)
        self.assertIn("for attempt in", build)

        helper = step_block("Prepare macOS desktop diagnostics helper")
        self.assertIn("continue-on-error: true", helper)
        self.assertIn("github.workflow_sha", helper)
        self.assertIn("DESKTOP_DIAGNOSTICS_SCRIPT=$helper", helper)

        collect = step_block("Collect macOS desktop diagnostics")
        self.assertIn("if: failure() && startsWith(matrix.platform, 'macos-')", collect)
        self.assertIn("DESKTOP_DIAGNOSTICS_SCRIPT", collect)

        upload = step_block("Upload macOS desktop diagnostics")
        self.assertIn("if: failure() && startsWith(matrix.platform, 'macos-')", upload)
        self.assertIn("dist/desktop-diagnostics/**", upload)
        self.assertIn("/release/bundle/dmg/**", upload)

        self.assertIn("hdiutil info", DIAGNOSTICS)
        self.assertIn("df -h", DIAGNOSTICS)
        self.assertIn("bundle_dmg.sh", DIAGNOSTICS)
        self.assertIn('cp "$bundle_root/dmg/bundle_dmg.sh"', DIAGNOSTICS)
        self.assertIn("|| true", DIAGNOSTICS)


if __name__ == "__main__":
    unittest.main()
