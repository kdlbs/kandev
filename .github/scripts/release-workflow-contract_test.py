#!/usr/bin/env python3
"""Contract tests for the maintainer release workflow."""

import fnmatch
import re
import subprocess
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "release.yml"
DIAGNOSTICS_PATH = REPO_ROOT / ".github" / "scripts" / "collect-macos-desktop-diagnostics.sh"
PUBLISH_NPM_PATH = REPO_ROOT / "scripts" / "release" / "publish-npm.sh"
UPDATE_SCOOP_BUCKET_PATH = REPO_ROOT / "scripts" / "release" / "update-scoop-bucket.sh"
NPM_PACKAGES_PATH = REPO_ROOT / "scripts" / "release" / "npm-packages.sh"
PUBLIC_KEY_PATH = REPO_ROOT / ".github" / "release-signing-key.asc"
RELEASE_PROCESS_PATH = REPO_ROOT / "docs" / "public" / "release-process.md"
LINT_WORKFLOW_PATH = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
WORKFLOW = WORKFLOW_PATH.read_text()
DIAGNOSTICS = DIAGNOSTICS_PATH.read_text()
PUBLISH_NPM = PUBLISH_NPM_PATH.read_text()
UPDATE_SCOOP_BUCKET = UPDATE_SCOOP_BUCKET_PATH.read_text()
NPM_PACKAGES = NPM_PACKAGES_PATH.read_text()
RELEASE_PROCESS = RELEASE_PROCESS_PATH.read_text()
LINT_WORKFLOW = LINT_WORKFLOW_PATH.read_text()
NORMAL_RELEASE_IF = (
    "if: ${{ !inputs.dry_run && !inputs.desktop_validation_only "
    "&& inputs.backfill_tag == '' }}"
)


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


def job_condition(name: str) -> str:
    match = re.search(r"(?m)^    if: [^\n]*(?:\n {6,}[^\n]*)*", job_block(name))
    if match is None:
        raise AssertionError(f"job condition not found: {name}")
    return " ".join(match.group().split())


class ReleaseWorkflowContractTest(unittest.TestCase):
    def test_nightly_runs_on_schedule_or_manual_channel_and_delegates_metadata_resolution(
        self,
    ) -> None:
        self.assertIn('schedule:\n    - cron: "0 12 * * *"', WORKFLOW)
        self.assertRegex(
            WORKFLOW,
            r"(?ms)      channel:\n"
            r"        description: \"Release channel\"\n"
            r"        required: true\n"
            r"        type: choice\n"
            r"        default: stable\n"
            r"        options:\n"
            r"          - stable\n"
            r"          - nightly",
        )

        nightly = job_block("nightly-prepare")
        self.assertIn("github.event_name == 'schedule'", nightly)
        self.assertIn(
            "github.event_name == 'workflow_dispatch' && inputs.channel == 'nightly'",
            nightly,
        )
        self.assertIn("ref: ${{ github.sha }}", nightly)
        validation = step_block("Validate manual Nightly request")
        self.assertIn("refs/heads/main", validation)
        self.assertIn("DESKTOP_VALIDATION_ONLY", validation)
        self.assertIn("BACKFILL_TAG", validation)
        metadata = step_block("Resolve nightly metadata")
        self.assertIn("id: metadata", metadata)
        self.assertIn("bash scripts/release/nightly-release.sh prepare", metadata)
        self.assertIn('--scheduled-sha "${{ github.sha }}"', metadata)
        self.assertIn('--output "$GITHUB_OUTPUT"', metadata)
        self.assertNotIn("npm-view-version.sh", metadata)
        self.assertIn(
            "nightly_tags_at_start: ${{ steps.metadata.outputs.nightly_tags_at_start }}",
            nightly,
        )

    def test_manual_nightly_dry_run_stops_after_preflight(self) -> None:
        summary = step_block("Nightly dry-run summary")
        self.assertIn("github.event_name == 'workflow_dispatch' && inputs.dry_run", summary)
        self.assertIn("steps.metadata.outputs.should_publish", summary)
        self.assertIn("steps.metadata.outputs.version", summary)

        for name in ("build-web", "build-bundles", "publish-npm-nightly"):
            condition = job_condition(name)
            self.assertIn("inputs.channel == 'nightly'", condition)
            self.assertIn("!inputs.dry_run", condition)

        for name in ("build-web", "build-bundles"):
            block = job_block(name)
            self.assertIn(
                "needs.nightly-prepare.result == 'success' && "
                "needs.nightly-prepare.outputs.ref || needs.prepare.outputs.ref",
                block,
            )
            self.assertIn(
                "needs.nightly-prepare.result == 'success' && "
                "needs.nightly-prepare.outputs.tag || needs.prepare.outputs.tag",
                block,
            )

    def test_nightly_package_inventory_remains_shared(self) -> None:
        for package in (
            "kandev",
            "@kdlbs/runtime-linux-x64",
            "@kdlbs/runtime-linux-arm64",
            "@kdlbs/runtime-darwin-x64",
            "@kdlbs/runtime-darwin-arm64",
            "@kdlbs/runtime-win32-x64",
        ):
            self.assertIn(package, NPM_PACKAGES)

        self.assertIn('NIGHTLY_PACKAGES=("kandev" "${RUNTIME_PACKAGES[@]}")', NPM_PACKAGES)

    def test_only_shared_runtime_builds_run_for_a_scheduled_nightly(self) -> None:
        for name in ("build-web", "build-bundles"):
            block = job_block(name)
            self.assertIn("needs: [prepare, nightly-prepare", block)
            self.assertIn("github.event_name == 'workflow_dispatch'", block)
            self.assertIn("github.event_name == 'schedule'", block)
            self.assertIn("inputs.channel == 'nightly'", block)
            self.assertIn("needs.nightly-prepare.outputs.should_publish == 'true'", block)

        for name in (
            "prepare",
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
            "update-scoop-bucket",
        ):
            block = job_block(name)
            self.assertIn("github.event_name == 'workflow_dispatch'", block)
            self.assertIn("inputs.channel == 'stable'", block)
            self.assertNotIn("github.event_name == 'schedule'", block)

    def test_platform_archives_are_validated_before_tap_consumers_receive_them(
        self,
    ) -> None:
        package = step_block("Package bundle")
        validation = "bash scripts/release/package-bundle.sh --bundle-dir dist/kandev"
        archive = 'tar -czf "kandev-${{ matrix.platform }}.tar.gz" kandev'

        self.assertIn(validation, package)
        self.assertIn(archive, package)
        self.assertLess(package.index(validation), package.index(archive))

    def test_stable_jobs_continue_past_skipped_nightly_branch_only_after_successful_needs(
        self,
    ) -> None:
        direct_dependencies = {
            "build-desktop": ("prepare", "build-bundles"),
            "docker-amd64": ("prepare", "build-bundles"),
            "docker-arm64": ("prepare", "build-bundles", "docker-amd64"),
            "docker-manifest": ("prepare", "docker-amd64", "docker-arm64"),
            "docker-universal-amd64": ("prepare", "docker-manifest"),
            "docker-universal-arm64": (
                "prepare",
                "docker-manifest",
                "docker-universal-amd64",
            ),
            "docker-universal-manifest": (
                "prepare",
                "docker-universal-amd64",
                "docker-universal-arm64",
            ),
            "publish-release": (
                "prepare",
                "build-bundles",
                "build-desktop",
                "docker-universal-manifest",
            ),
            "publish-npm": ("prepare", "publish-release"),
            "update-homebrew-tap": ("prepare", "publish-release"),
            "update-scoop-bucket": ("prepare", "publish-release"),
        }

        for name, dependencies in direct_dependencies.items():
            with self.subTest(job=name):
                condition = job_condition(name)
                self.assertIn("!cancelled()", condition)
                for dependency in dependencies:
                    self.assertIn(
                        f"needs.{dependency}.result == 'success'",
                        condition,
                    )

    def test_nightly_publish_uses_exact_sha_local_assets_and_release_serialization(self) -> None:
        stable = job_block("publish-npm")
        nightly = job_block("publish-npm-nightly")

        workflow_preamble = WORKFLOW.split("\njobs:", 1)[0]
        self.assertIn("group: release-npm-publication", workflow_preamble)
        self.assertIn("cancel-in-progress: false", workflow_preamble)
        self.assertIn("queue: max", workflow_preamble)

        for block in (stable, nightly):
            self.assertNotIn("\n    concurrency:", block)
            self.assertIn("id-token: write", block)

        self.assertIn("needs: [nightly-prepare, build-bundles]", nightly)
        self.assertIn("needs.build-bundles.result == 'success'", nightly)
        self.assertIn("needs.build-web.result == 'success'", job_block("build-bundles"))
        self.assertIn("ref: ${{ needs.nightly-prepare.outputs.ref }}", nightly)
        self.assertIn("fetch-depth: 0", nightly)
        self.assertIn("pattern: bundle-*", nightly)
        self.assertIn("merge-multiple: true", nightly)
        self.assertIn('--version "${{ needs.nightly-prepare.outputs.version }}"', nightly)
        self.assertIn('--assets-dir dist/nightly-assets', nightly)
        publish = step_block("Publish npm nightly packages")
        self.assertIn("bash scripts/release/nightly-release.sh publish", publish)
        self.assertIn('--stable-at-start "$NIGHTLY_BASELINE"', publish)
        self.assertIn('--nightly-at-start "$NIGHTLY_AT_START"', publish)
        self.assertIn('--tags-at-start "$NIGHTLY_TAGS_AT_START"', publish)
        self.assertIn(
            "NIGHTLY_TAGS_AT_START: ${{ needs.nightly-prepare.outputs.nightly_tags_at_start }}",
            nightly,
        )
        self.assertNotIn("npm-view-version.sh", publish)
        self.assertNotIn("publish-npm.sh", publish)

        self.assertIn('--version "${{ needs.prepare.outputs.version }}"', stable)
        self.assertIn('--dist-tag latest', stable)
        self.assertIn('--release-tag "${{ needs.prepare.outputs.tag }}"', stable)

        self.assertIn('elif [[ "$DIST_TAG" == "latest" && -z "$RELEASE_TAG" ]]', PUBLISH_NPM)
        self.assertIn(
            'elif [[ "$DIST_TAG" == "nightly" && -z "$SOURCE_ASSETS_DIR" ]]',
            PUBLISH_NPM,
        )
        self.assertIn('bash "$ROOT_DIR/scripts/release/npm-view-version.sh"', PUBLISH_NPM)
        self.assertIn('CLI_PACKAGE_BACKUP="$WORK_DIR/cli-package.json"', PUBLISH_NPM)
        self.assertIn('cp "$CLI_PACKAGE_BACKUP" "$CLI_PACKAGE_JSON"', PUBLISH_NPM)

    def test_scoop_publication_uses_current_control_revision_and_deploy_key(self) -> None:
        scoop = job_block("update-scoop-bucket")
        self.assertIn("needs: [prepare, publish-release]", scoop)
        self.assertIn("ref: ${{ github.workflow_sha }}", scoop)
        self.assertIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", scoop)
        self.assertIn(
            "SCOOP_BUCKET_DEPLOY_KEY: ${{ secrets.SCOOP_BUCKET_DEPLOY_KEY }}",
            scoop,
        )
        self.assertIn("bash scripts/release/update-scoop-bucket.sh", scoop)
        self.assertIn('"${{ needs.prepare.outputs.version }}"', scoop)
        self.assertIn('"${{ needs.prepare.outputs.tag }}"', scoop)
        self.assertIn("!inputs.dry_run", scoop)
        self.assertIn("!inputs.desktop_validation_only", scoop)
        self.assertNotIn("inputs.backfill_tag == ''", scoop)

        preflight = step_block("Require Scoop bucket deploy key")
        self.assertIn('if [ -z "$SCOOP_BUCKET_DEPLOY_KEY" ]', preflight)
        self.assertIn("SCOOP_BUCKET_DEPLOY_KEY is required", preflight)

        self.assertIn("SCOOP_BUCKET_DEPLOY_KEY", UPDATE_SCOOP_BUCKET)
        self.assertIn("git push origin HEAD:main", UPDATE_SCOOP_BUCKET)
        self.assertIn('CHECKSUM_NAME="${ARCHIVE_NAME}.sha256"', UPDATE_SCOOP_BUCKET)

    def test_publish_npm_rejects_version_dist_tag_mismatches(self) -> None:
        cases = (
            (
                "1.2.3-nightly.shaabcdef123456",
                "latest",
                "--version must be stable X.Y.Z for --dist-tag latest",
            ),
            (
                "1.2.3",
                "nightly",
                "--version must be X.Y.Z-nightly.sha<12-hex> for --dist-tag nightly",
            ),
        )
        for version, dist_tag, expected in cases:
            with self.subTest(dist_tag=dist_tag):
                result = subprocess.run(
                    [
                        "bash",
                        str(PUBLISH_NPM_PATH),
                        "--version",
                        version,
                        "--dist-tag",
                        dist_tag,
                    ],
                    cwd=REPO_ROOT,
                    capture_output=True,
                    text=True,
                    check=False,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(expected, result.stderr)

    def test_normal_release_uses_release_environment_and_requires_main(self) -> None:
        prepare = job_block("prepare")
        self.assertIn(
            "environment: ${{ (!inputs.dry_run && !inputs.desktop_validation_only "
            "&& inputs.backfill_tag == '') && 'release' || 'release-validation' }}",
            prepare,
        )

        guard = step_block("Require main for normal release")
        self.assertIn(NORMAL_RELEASE_IF, guard)
        self.assertIn("CURRENT_REF: ${{ github.ref }}", guard)
        self.assertIn('if [ "$CURRENT_REF" != "refs/heads/main" ]', guard)

    def test_normal_release_uses_admin_token_only_for_exact_head_merge(self) -> None:
        create = step_block("Create release PR")
        merge = step_block("Merge release PR with administrator token")
        select = step_block("Select merged release commit")

        for step in (create, merge, select):
            self.assertIn(NORMAL_RELEASE_IF, step)

        self.assertIn("id: release_pr", create)
        self.assertIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", create)
        self.assertIn('MSG="release: publish $NEXT"', create)
        self.assertIn('HEAD_SHA="$(git rev-parse HEAD)"', create)
        self.assertIn('echo "url=$PR_URL" >> "$GITHUB_OUTPUT"', create)
        self.assertIn('echo "head=$HEAD_SHA" >> "$GITHUB_OUTPUT"', create)
        self.assertNotIn("RELEASE_PR_BYPASS_TOKEN", create)

        self.assertIn(
            "GH_TOKEN: ${{ secrets.RELEASE_PR_BYPASS_TOKEN }}",
            merge,
        )
        self.assertIn("PR_URL: ${{ steps.release_pr.outputs.url }}", merge)
        self.assertIn("EXPECTED_HEAD: ${{ steps.release_pr.outputs.head }}", merge)
        self.assertIn('if [ -z "$GH_TOKEN" ]', merge)
        self.assertIn("RELEASE_PR_BYPASS_TOKEN is required", merge)
        self.assertIn('gh pr merge "$PR_URL"', merge)
        self.assertIn("--admin", merge)
        self.assertIn("--squash", merge)
        self.assertIn('--match-head-commit "$EXPECTED_HEAD"', merge)
        self.assertNotIn("--delete-branch", merge)
        self.assertNotIn("--auto", merge)

        self.assertIn("GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", select)
        self.assertNotIn("RELEASE_PR_BYPASS_TOKEN", select)
        self.assertIn("--json state,mergedAt,mergeCommit,headRefOid", select)
        self.assertIn('if [ "$PR_STATE" != "MERGED" ]', select)
        self.assertIn('if [ "$PR_HEAD" != "$EXPECTED_HEAD" ]', select)
        self.assertIn('[[ "$MERGE_COMMIT" =~ ^[0-9a-f]{40}$ ]]', select)
        self.assertIn('git merge-base --is-ancestor "$MERGE_COMMIT" origin/main', select)
        self.assertIn('git checkout --detach "$MERGE_COMMIT"', select)

    def test_normal_release_preflights_and_revalidates_signing_key_around_merge(
        self,
    ) -> None:
        prepare = job_block("prepare")
        bump = step_block("Bump version + generate CHANGELOG (in working tree)")
        create_pr = step_block("Create release PR")
        merge = step_block("Merge release PR with administrator token")
        select = step_block("Select merged release commit")
        preflight_public_key = step_block("Validate committed release signing public key")
        preflight = step_block("Preflight release tag signing fingerprint")
        merged_public_key = step_block("Validate merged release signing public key")
        signing = step_block("Import release tag signing key")
        self.assertIn(NORMAL_RELEASE_IF, preflight_public_key)
        self.assertIn(
            "gpg --batch --with-colons --show-keys .github/release-signing-key.asc",
            preflight_public_key,
        )
        self.assertIn("SECRET_KEY_RECORDS", preflight_public_key)
        self.assertIn("PUBLIC_KEY_RECORDS", preflight_public_key)
        self.assertIn("PRIMARY_FINGERPRINT_COUNT", preflight_public_key)
        self.assertIn('if [ "$SECRET_KEY_RECORDS" -ne 0 ]', preflight_public_key)
        self.assertIn('if [ "$PUBLIC_KEY_RECORDS" -eq 0 ]', preflight_public_key)
        self.assertIn('if [ "$PUBLIC_KEY_RECORDS" -ne 1 ]', preflight_public_key)
        self.assertIn('[[ "$PUBLIC_FINGERPRINT" =~ ^[A-F0-9]{40}$ ]]', preflight_public_key)
        self.assertIn("PUBLIC_FINGERPRINT", preflight_public_key)
        self.assertIn(
            'echo "fingerprint=$PUBLIC_FINGERPRINT" >> "$GITHUB_OUTPUT"',
            preflight_public_key,
        )

        self.assertIn(NORMAL_RELEASE_IF, preflight)
        self.assertIn("EXPECTED_FINGERPRINT: ${{ vars.RELEASE_GPG_FINGERPRINT }}", preflight)
        self.assertIn(
            "COMMITTED_PUBLIC_FINGERPRINT: ${{ steps.committed_release_gpg.outputs.fingerprint }}",
            preflight,
        )
        self.assertIn('if [ -z "$EXPECTED_FINGERPRINT" ]', preflight)
        self.assertIn('if [ -z "$COMMITTED_PUBLIC_FINGERPRINT" ]', preflight)
        self.assertIn(
            'if [ "$COMMITTED_PUBLIC_FINGERPRINT" != "$EXPECTED_FINGERPRINT" ]',
            preflight,
        )

        self.assertIn(NORMAL_RELEASE_IF, merged_public_key)
        self.assertIn("id: merged_release_gpg", merged_public_key)
        self.assertIn(
            "gpg --batch --with-colons --show-keys .github/release-signing-key.asc",
            merged_public_key,
        )
        self.assertIn('echo "fingerprint=$PUBLIC_FINGERPRINT" >> "$GITHUB_OUTPUT"', merged_public_key)

        self.assertIn(NORMAL_RELEASE_IF, signing)
        self.assertIn("id: import_release_gpg", signing)
        self.assertIn(
            "uses: crazy-max/ghaction-import-gpg@2dc316deee8e90f13e1a351ab510b4d5bc0c82cd # v7.0.0",
            signing,
        )
        self.assertIn("gpg_private_key: ${{ secrets.RELEASE_GPG_PRIVATE_KEY }}", signing)
        self.assertIn("passphrase: ${{ secrets.RELEASE_GPG_PASSPHRASE }}", signing)
        self.assertIn("git_user_signingkey: true", signing)
        self.assertIn("git_tag_gpgsign: true", signing)

        validate = step_block("Validate release tag signing identity")
        self.assertIn(NORMAL_RELEASE_IF, validate)
        self.assertIn("EXPECTED_FINGERPRINT: ${{ vars.RELEASE_GPG_FINGERPRINT }}", validate)
        self.assertIn(
            "MERGED_PUBLIC_FINGERPRINT: ${{ steps.merged_release_gpg.outputs.fingerprint }}",
            validate,
        )
        self.assertIn(
            "IMPORTED_FINGERPRINT: ${{ steps.import_release_gpg.outputs.fingerprint }}",
            validate,
        )
        self.assertIn('if [ -z "$EXPECTED_FINGERPRINT" ]', validate)
        self.assertIn('if [ "$MERGED_PUBLIC_FINGERPRINT" != "$EXPECTED_FINGERPRINT" ]', validate)
        self.assertIn('if [ "$IMPORTED_FINGERPRINT" != "$MERGED_PUBLIC_FINGERPRINT" ]', validate)
        self.assertIn('if [ "$IMPORTED_FINGERPRINT" != "$EXPECTED_FINGERPRINT" ]', validate)
        self.assertIn("TAGGER_NAME: ${{ steps.import_release_gpg.outputs.name }}", validate)
        self.assertIn("TAGGER_EMAIL: ${{ steps.import_release_gpg.outputs.email }}", validate)
        self.assertIn('git config user.name "$TAGGER_NAME"', validate)
        self.assertIn('git config user.email "$TAGGER_EMAIL"', validate)
        self.assertIn('git config user.signingkey "$IMPORTED_FINGERPRINT"', validate)

        tag = step_block("Create and push signed release tag")
        self.assertIn(NORMAL_RELEASE_IF, tag)
        self.assertIn('git tag -s "$TAG" -m "release: $NEXT"', tag)
        self.assertIn('git tag -v "$TAG"', tag)
        self.assertLess(tag.index('git tag -s "$TAG"'), tag.index('git tag -v "$TAG"'))
        self.assertLess(tag.index('git tag -v "$TAG"'), tag.index('git push origin "$TAG"'))
        self.assertNotIn('git tag -a "$TAG"', tag)

        self.assertLess(prepare.index(preflight_public_key), prepare.index(preflight))
        self.assertLess(prepare.index(preflight), prepare.index(bump))
        self.assertLess(prepare.index(bump), prepare.index(create_pr))
        self.assertLess(prepare.index(create_pr), prepare.index(merge))
        self.assertLess(prepare.index(merge), prepare.index(select))
        self.assertLess(prepare.index(select), prepare.index(merged_public_key))
        self.assertLess(prepare.index(merged_public_key), prepare.index(signing))
        self.assertLess(prepare.index(merge), prepare.index(signing))
        self.assertLess(prepare.index(signing), prepare.index(validate))
        self.assertLess(prepare.index(validate), prepare.index(tag))

    def test_committed_release_key_is_one_public_primary_key_without_secret_material(self) -> None:
        result = subprocess.run(
            ["gpg", "--batch", "--with-colons", "--show-keys", str(PUBLIC_KEY_PATH)],
            check=True,
            capture_output=True,
            text=True,
        )
        records = [line.split(":") for line in result.stdout.splitlines() if line]
        primary_keys = [record for record in records if record[0] == "pub"]
        secret_keys = [record for record in records if record[0] in {"sec", "ssb"}]
        primary_fingerprints = [
            records[index + 1][9]
            for index, record in enumerate(records[:-1])
            if record[0] == "pub" and records[index + 1][0] == "fpr"
        ]

        self.assertEqual([], secret_keys)
        self.assertEqual(1, len(primary_keys))
        self.assertEqual(1, len(primary_fingerprints))
        self.assertRegex(primary_fingerprints[0], r"^[A-F0-9]{40}$")

    def test_release_documentation_declares_the_committed_public_key_fingerprint(self) -> None:
        result = subprocess.run(
            ["gpg", "--batch", "--with-colons", "--show-keys", str(PUBLIC_KEY_PATH)],
            check=True,
            capture_output=True,
            text=True,
        )
        records = [line.split(":") for line in result.stdout.splitlines() if line]
        primary_fingerprints = [
            records[index + 1][9]
            for index, record in enumerate(records[:-1])
            if record[0] == "pub" and records[index + 1][0] == "fpr"
        ]
        documented_fingerprint = re.search(
            r"The current key is committed at `\.github/release-signing-key\.asc`; "
            r"its full fingerprint is `([A-F0-9]{40})`\.",
            RELEASE_PROCESS,
        )

        self.assertIsNotNone(documented_fingerprint)
        self.assertEqual(primary_fingerprints[0], documented_fingerprint.group(1))

    def test_release_documentation_declares_the_pr_bypass_token_contract(self) -> None:
        for requirement in (
            "RELEASE_PR_BYPASS_TOKEN",
            "fine-grained personal access token",
            "Contents: Read and write",
            "organization administrator",
            "Only select repositories",
            "expiration date",
        ):
            self.assertIn(requirement, RELEASE_PROCESS)

    def test_release_contract_ci_runs_when_key_or_release_documentation_changes(self) -> None:
        """This contract must run on every change, not a listed subset.

        `lint-action-pinning.yml` used to name each subject of this file --
        the signing key, the release docs, every `scripts/release/` helper --
        in a hand-maintained `paths:` list, so that changing one brought the
        job up. That list is gone: the workflow now triggers on every push and
        every pull request, which covers every subject here and every one added
        later without anybody remembering to extend a list.

        The change was forced by the merge queue. A workflow skipped by a
        `paths:` filter reports no conclusion at all, and a required check that
        never reports blocks a pull request from entering the queue.
        """
        for trigger in ("push", "pull_request", "merge_group"):
            trigger_block = re.search(
                rf"  {trigger}:\n.*?(?=\n  [a-z_]+:|\nconcurrency:)",
                LINT_WORKFLOW,
                re.DOTALL,
            )
            self.assertIsNotNone(trigger_block)
            self.assertNotIn(
                "    paths:",
                trigger_block.group(0),
                f"lint-action-pinning.yml's {trigger} trigger must stay "
                "unfiltered. A `paths:` list there both un-guards every "
                "subject it omits and makes the check unreportable in a merge "
                "queue.",
            )

        setup_node = (
            "uses: actions/setup-node@48b55a011bda9f5d6aeb4c2d9c7362e8dae4041e # v6"
        )
        self.assertIn(setup_node, LINT_WORKFLOW)
        self.assertIn('node-version: "24"', LINT_WORKFLOW)
        self.assertIn(
            "run: bash scripts/release/runtime-bundle.test.sh",
            LINT_WORKFLOW,
        )
        self.assertLess(
            LINT_WORKFLOW.index(setup_node),
            LINT_WORKFLOW.index("- name: Test runtime bundle contract"),
        )
        self.assertLess(
            LINT_WORKFLOW.index(setup_node),
            LINT_WORKFLOW.index("- name: Test npm release helpers"),
        )
        self.assertIn(
            "node --test scripts/release/nightly-version.test.mjs "
            "scripts/release/nightly-release.test.mjs "
            "scripts/release/npm-view-version.test.mjs "
            "scripts/release/publish-npm.test.mjs "
            "scripts/release/update-scoop-bucket.test.mjs",
            LINT_WORKFLOW,
        )

    def test_tag_push_recovery_recreates_tag_at_logged_merge_commit(self) -> None:
        tag = step_block("Create and push signed release tag")
        self.assertIn('MERGE_COMMIT="$(git rev-parse HEAD)"', tag)
        self.assertIn('echo "Release merge commit: $MERGE_COMMIT"', tag)
        self.assertIn('git checkout --detach $MERGE_COMMIT', tag)
        self.assertIn("git tag -s $TAG -m 'release: $NEXT'", tag)
        self.assertIn('git tag -v $TAG', tag)
        self.assertIn("matches RELEASE_GPG_FINGERPRINT", tag)
        self.assertIn('git push origin $TAG', tag)
        self.assertNotIn('Recover manually: git push origin $TAG', tag)

    def test_npm_publish_preserves_oidc_provenance_for_all_packages(self) -> None:
        publish_job = job_block("publish-npm")
        self.assertIn("id-token: write", publish_job)
        self.assertEqual(PUBLISH_NPM.count("npm publish --access public --provenance"), 2)

        for package in (
            "@kdlbs/runtime-linux-x64",
            "@kdlbs/runtime-linux-arm64",
            "@kdlbs/runtime-darwin-x64",
            "@kdlbs/runtime-darwin-arm64",
            "@kdlbs/runtime-win32-x64",
        ):
            self.assertIn(f'"{package}"', NPM_PACKAGES)

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
            "Create release PR",
            "Merge release PR with administrator token",
            "Select merged release commit",
            "Import release tag signing key",
            "Validate release tag signing identity",
            "Create and push signed release tag",
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
            "update-scoop-bucket",
        ):
            block = job_block(name)
            self.assertRegex(
                job_condition(name),
                r"github\.event_name == 'workflow_dispatch'.*!inputs\.dry_run",
            )
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

    def test_macos_signed_build_selects_app_bundle_for_updater(self) -> None:
        build = step_block("Build Tauri desktop app")
        collect = step_block("Collect desktop artifacts")
        initialize = 'tauri_bundles="${{ matrix.tauri_bundles }}"'
        append = 'tauri_bundles="${tauri_bundles},app"'
        invoke = '--bundles "$tauri_bundles"'

        self.assertIn('[[ "${{ matrix.platform }}" == macos-*', build)
        self.assertIn('"${UPDATER_SIGNING_ENABLED:-false}" = "true"', build)
        self.assertIn(initialize, build)
        self.assertIn(append, build)
        self.assertNotIn('tauri_bundles="${tauri_bundles},updater"', build)
        self.assertIn(invoke, build)
        self.assertLess(build.index(initialize), build.index(append))
        self.assertLess(build.index(append), build.index(invoke))
        self.assertIn('"${UPDATER_SIGNING_ENABLED:-false}" = "true"', collect)
        self.assertIn('"$DESKTOP_ASSET_VERIFIER" --require-updaters', collect)

    def test_unsigned_macos_warning_includes_launch_recovery_command(self) -> None:
        warning = step_block("Add unsigned desktop warning to release notes")

        self.assertIn("macos_unsigned=false", warning)
        self.assertIn("macos_unsigned=true", warning)
        branch_if = 'if [ "$macos_unsigned" = "true" ]; then'
        command = "xattr -d com.apple.quarantine /Applications/Kandev.app"

        self.assertIn(branch_if, warning)
        self.assertIn("verify the checksum", warning)
        self.assertIn(command, warning)
        self.assertLess(
            warning.index("verify the checksum"),
            warning.index(command),
        )
        branch_start = warning.index(branch_if)
        branch_end = warning.index("\n            fi", branch_start)
        branch_body = warning[branch_start:branch_end]
        self.assertIn(command, branch_body)

    def test_release_asset_globs_are_disjoint_and_upload_sequentially(self) -> None:
        publish = step_block("Publish release")
        files = re.search(r"\n          files: \|\n((?:            \S+\n)+)", publish)
        self.assertIsNotNone(files)
        patterns = [line.strip() for line in files.group(1).splitlines()]
        assets = (
            "kandev-linux-x64.tar.gz",
            "kandev-linux-x64.tar.gz.sha256",
            "kandev-macos-arm64.tar.gz",
            "kandev-macos-arm64.tar.gz.sha256",
            "kandev-windows-x64.tar.gz",
            "kandev-windows-x64.tar.gz.sha256",
            "kandev-desktop-macos-arm64-Kandev.app.tar.gz",
            "kandev-desktop-macos-arm64-Kandev.app.tar.gz.sha256",
            "kandev-desktop-macos-arm64-Kandev.app.tar.gz.sig",
            "kandev-desktop-macos-arm64-Kandev.app.tar.gz.sig.sha256",
            "latest.json",
        )

        for asset in assets:
            path = f"dist/release-assets/{asset}"
            matches = [pattern for pattern in patterns if fnmatch.fnmatchcase(path, pattern)]
            self.assertEqual(len(matches), 1, f"release glob count for {asset}: {matches}")

        self.assertIn("preserve_order: true", publish)

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

    def test_ghcr_builds_retry_transient_failures_and_publish_digests(self) -> None:
        for job, name in (
            ("docker-amd64", "Build and push (amd64 staging tag)"),
            ("docker-arm64", "Build and push (arm64 staging tag)"),
            (
                "docker-universal-amd64",
                "Build and push (universal amd64 staging tag)",
            ),
            (
                "docker-universal-arm64",
                "Build and push (universal arm64 staging tag)",
            ),
        ):
            build = step_block(name)
            job_text = job_block(job)
            with self.subTest(step=name):
                runtime_setup = "crazy-max/ghaction-github-runtime@"
                self.assertIn(runtime_setup, job_text)
                self.assertLess(
                    job_text.index(runtime_setup),
                    job_text.index(f"- name: {name}"),
                )
                self.assertIn("bash scripts/release/retry-ghcr-command.sh", build)
                self.assertIn("docker buildx build", build)
                self.assertIn("--metadata-file", build)
                self.assertIn("containerimage.digest", build)
                self.assertIn('echo "digest=$digest" >> "$GITHUB_OUTPUT"', build)
                self.assertNotIn("docker/build-push-action@", build)

    def test_ghcr_manifest_mutations_retry_and_can_load_shared_helper(self) -> None:
        for job, step in (
            ("docker-manifest", "Combine arch images into multi-arch tags"),
            ("docker-universal-manifest", "Promote staging to final tags"),
        ):
            block = job_block(job)
            mutation = step_block(step)
            with self.subTest(job=job):
                self.assertIn("actions/checkout@", block)
                self.assertIn("ref: ${{ needs.prepare.outputs.ref }}", block)
                self.assertIn("bash scripts/release/retry-ghcr-command.sh", mutation)
                self.assertIn("docker buildx imagetools create", mutation)

    def test_ghcr_architecture_jobs_are_serialized(self) -> None:
        self.assertIn(
            "needs: [prepare, build-bundles, docker-amd64]",
            job_block("docker-arm64"),
        )
        self.assertIn(
            "needs: [prepare, docker-manifest, docker-universal-amd64]",
            job_block("docker-universal-arm64"),
        )

    def test_release_validation_runs_ghcr_retry_regression(self) -> None:
        self.assertIn("scripts/release/retry-ghcr-command.test.sh", LINT_WORKFLOW)
        makefile = (REPO_ROOT / "Makefile").read_text()
        self.assertIn("bash scripts/release/retry-ghcr-command.test.sh", makefile)


if __name__ == "__main__":
    unittest.main()
