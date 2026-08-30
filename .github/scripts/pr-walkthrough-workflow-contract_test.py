#!/usr/bin/env python3
"""Contract tests for PR walkthrough generation, publication, and linking."""

from pathlib import Path
import re
import unittest


REPO_ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = REPO_ROOT / ".github" / "workflows" / "pr-walkthrough.yml"
REVIEW_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "opencode-code-review.yml"
LINT_WORKFLOW = REPO_ROOT / ".github" / "workflows" / "lint-action-pinning.yml"
SETUP_OPENCODE_ACTION = REPO_ROOT / ".github" / "actions" / "setup-opencode" / "action.yml"
SKILL_DIR = REPO_ROOT / ".agents" / "skills" / "pr-walkthrough"
SKILL_CONTEXT_HELPER = SKILL_DIR / "scripts" / "pr-walkthrough-context"
SKILL_RENDER_HELPER = SKILL_DIR / "scripts" / "pr-walkthrough-render"
PR_BODY_HELPER = REPO_ROOT / "scripts" / "pr-walkthrough-pr-body"
ROOT_CONTEXT_HELPER = REPO_ROOT / "scripts" / "pr-walkthrough-context"
ROOT_RENDER_HELPER = REPO_ROOT / "scripts" / "pr-walkthrough-render"
ROOT_READ_HELPER = REPO_ROOT / "scripts" / "pr-walkthrough-read-file"


def workflow_job(workflow: str, name: str) -> str:
    marker = f"  {name}:\n"
    _, separator, remainder = workflow.partition(marker)
    if not separator:
        raise AssertionError(f"workflow job is missing: {name}")
    next_job = re.search(r"^  [A-Za-z0-9_-]+:\n", remainder, re.MULTILINE)
    return remainder[: next_job.start()] if next_job else remainder


class PRWalkthroughWorkflowContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        if not WORKFLOW.is_file():
            raise AssertionError("PR walkthrough must have a dedicated workflow file")
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")
        cls.review_workflow = REVIEW_WORKFLOW.read_text(encoding="utf-8")
        cls.skill = (SKILL_DIR / "SKILL.md").read_text(encoding="utf-8")
        cls.generation = workflow_job(cls.workflow, "pr-walkthrough-generate")
        cls.publication = workflow_job(cls.workflow, "pr-walkthrough-publish")
        cls.link = workflow_job(cls.workflow, "pr-walkthrough-link")
        cls.same_repo_review = workflow_job(cls.review_workflow, "opencode-review-same-repo")
        cls.fork_review = workflow_job(cls.review_workflow, "opencode-review-fork")

    def test_walkthrough_is_separate_and_independently_enabled(self) -> None:
        self.assertNotIn("pr-walkthrough-generate:", self.review_workflow)
        self.assertNotIn("pr-walkthrough-publish:", self.review_workflow)
        self.assertNotIn("PR_WALKTHROUGH_ENABLED", self.review_workflow)
        self.assertIn("vars.PR_WALKTHROUGH_ENABLED == 'true'", self.generation)
        self.assertIn("vars.PR_WALKTHROUGH_ENABLED == 'true'", self.publication)
        self.assertNotIn("OPENCODE_REVIEW_ENABLED", self.workflow)

    def test_generation_is_limited_to_authorized_repository_events(self) -> None:
        self.assertIn("pull_request_target:", self.workflow)
        self.assertIn(
            "types: [opened, ready_for_review, reopened, synchronize, labeled]",
            self.workflow,
        )
        for condition in (
            "github.event_name == 'pull_request_target'",
            "github.event.pull_request.draft == false",
            "github.event.pull_request.head.repo.full_name == github.repository",
            "github.event.pull_request.head.repo.full_name != github.repository",
            "github.event.action == 'opened'",
            "github.event.action == 'reopened'",
            "github.event.action == 'ready_for_review'",
            "github.event.action == 'synchronize'",
            "github.event.action == 'labeled' && github.event.label.name == 'generate-pr-walkthrough'",
            "github.event.label.name == 'safe-to-review'",
            "github.event.action != 'labeled'",
            "contains(github.event.pull_request.labels.*.name, 'safe-to-review')",
            "vars.CLAUDE_REVIEW_ALLOWLIST != ''",
            "contains(fromJSON(vars.CLAUDE_REVIEW_ALLOWLIST), github.event.pull_request.user.login)",
        ):
            self.assertIn(condition, self.generation)
        self.assertNotIn("safe-to-test", self.generation)

    def test_skill_explains_trusted_context_without_provider_names(self) -> None:
        self.assertNotIn("Kandev", self.skill)
        self.assertIn("managed runner", self.skill)
        self.assertIn("arbitrary Git or shell commands", self.skill)

    def test_generation_uses_requested_model_native_high_reasoning_variant(self) -> None:
        model = "opencode-go/muse-spark-1.2-contributor"
        variant = "high"
        self.assertIn(f"PR_WALKTHROUGH_MODEL: {model}", self.workflow)
        self.assertIn(f"PR_WALKTHROUGH_VARIANT: {variant}", self.workflow)
        self.assertIn(f'model: "{model}"', self.generation)
        self.assertIn('--model "$PR_WALKTHROUGH_MODEL"', self.generation)
        self.assertIn('--variant "$PR_WALKTHROUGH_VARIANT"', self.generation)
        self.assertNotIn("#high", self.workflow)
        self.assertNotIn("reasoningEffort", self.generation)

    def test_generation_keeps_untrusted_head_out_of_secret_bearing_workspace(self) -> None:
        self.assertIn("ref: ${{ github.workflow_sha }}", self.generation)
        self.assertNotIn("ref: ${{ github.event.pull_request.base.sha }}", self.generation)
        self.assertNotIn("ref: ${{ github.event.pull_request.head.sha }}", self.generation)
        self.assertIn("fetch-depth: 0", self.generation)
        self.assertIn("persist-credentials: false", self.generation)
        self.assertIn('test "$(git rev-parse HEAD)" = "$TRUSTED_SHA"', self.generation)
        self.assertIn(
            'git fetch --no-tags --filter=blob:none '
            '--negotiation-tip="$TRUSTED_SHA" origin "refs/pull/${PR_NUMBER}/head"',
            self.generation,
        )
        self.assertIn(
            'PR_NUMBER: ${{ github.event.pull_request.number }}',
            self.generation,
        )
        self.assertNotIn("--depth=1", self.generation)
        self.assertIn('test "$(git rev-parse FETCH_HEAD)" = "$HEAD_SHA"', self.generation)
        self.assertIn('git merge-base "$TRUSTED_SHA" "$HEAD_SHA"', self.generation)

    def test_generation_and_link_use_one_trusted_workflow_sha(self) -> None:
        for job in (self.generation, self.link):
            self.assertIn("ref: ${{ github.workflow_sha }}", job)
            self.assertIn("TRUSTED_SHA: ${{ github.workflow_sha }}", job)
            self.assertIn('test "$(git rev-parse HEAD)" = "$TRUSTED_SHA"', job)
            self.assertNotIn("github.event.pull_request.base.sha", job)

        for value in (
            'git fetch --no-tags --filter=blob:none --negotiation-tip="$TRUSTED_SHA" origin "refs/pull/${PR_NUMBER}/head"',
            'git merge-base "$TRUSTED_SHA" "$HEAD_SHA"',
            'git archive "$TRUSTED_SHA" .agents/skills/pr-walkthrough | tar -x',
            '--base-sha "$TRUSTED_SHA"',
            'RANGE="$TRUSTED_SHA...$HEAD_SHA"',
            'git show "$TRUSTED_SHA:AGENTS.md"',
            'git show "$TRUSTED_SHA:.agents/skills/pr-walkthrough/SKILL.md"',
        ):
            self.assertIn(value, self.generation)

        for value in (
            'test -f scripts/pr-walkthrough-pr-body',
            'python3 scripts/pr-walkthrough-pr-body',
        ):
            self.assertIn(value, self.link)

        self.assertIn("trusted workflow checkout", self.generation)
        self.assertIn("trusted workflow commit", self.generation)
        self.assertNotIn("trusted base checkout", self.generation)
        self.assertNotIn("trusted base commit", self.generation)

    def test_generation_retries_only_incomplete_zero_exit_once(self) -> None:
        for value in (
            "for attempt in 1 2; do",
            'ATTEMPT_DIR=".pr-walkthrough/attempt-${attempt}"',
            'rm -f \\\n              "docs/pr-walkthrough/pr-${PR_NUMBER}.json" \\\n              "docs/pr-walkthrough/pr-${PR_NUMBER}.html"',
            "printf '{}\\n' > .pr-walkthrough/draft.json",
            '> "$ATTEMPT_DIR/stdout"',
            '2> "$ATTEMPT_DIR/stderr"',
            'printf \'%s\\n\' "$opencode_status" > "$ATTEMPT_DIR/status"',
            'cp .pr-walkthrough/draft.json "$ATTEMPT_DIR/draft.json"',
            'if [ "$opencode_status" -ne 0 ]; then',
            'if [ -s "docs/pr-walkthrough/pr-${PR_NUMBER}.json" ] && [ -s "docs/pr-walkthrough/pr-${PR_NUMBER}.html" ]; then',
            'if [ "$attempt" -eq 2 ]; then',
            'exit "$opencode_status"',
            'exit 1',
        ):
            self.assertIn(value, self.generation)
        self.assertNotIn("> .pr-walkthrough/opencode.stdout", self.generation)
        self.assertNotIn("2> .pr-walkthrough/opencode.stderr", self.generation)

    def test_generation_uses_trusted_base_skill_renderer_and_helper(self) -> None:
        self.assertIn(
            'git archive "$TRUSTED_SHA" .agents/skills/pr-walkthrough | tar -x',
            self.generation,
        )
        for path in (
            'test -f "$SKILL_DIR/SKILL.md"',
            'test -f "$SKILL_DIR/references/build.py"',
            'test -f "$SKILL_DIR/references/shell.html"',
            'test -f "$SKILL_DIR/scripts/pr-walkthrough-context"',
            'test -f "$SKILL_DIR/scripts/pr-walkthrough-render"',
        ):
            self.assertIn(path, self.generation)
        self.assertNotIn('git show "$BASE_SHA:scripts/pr-walkthrough-render"', self.generation)
        self.assertNotIn('git show "$BASE_SHA:scripts/pr-walkthrough-read-file"', self.generation)
        self.assertIn("git diff --find-renames --find-copies", self.generation)
        self.assertIn("git diff --name-only", self.generation)

    def test_opencode_walkthrough_agent_is_read_only(self) -> None:
        self.assertIn('permission:\n            "*": deny', self.generation)
        for allowed in ("read: allow", "glob: allow", "grep: allow"):
            self.assertIn(allowed, self.generation)
        self.assertIn("external_directory: deny", self.generation)
        for denied in ("patch: deny", "task: deny", "todo: deny", "fetch: deny", "web: deny"):
            self.assertIn(denied, self.generation)
        self.assertIn('".pr-walkthrough/draft.json": allow', self.generation)
        self.assertIn(
            '"python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render": allow',
            self.generation,
        )
        self.assertIn("--agent github-pr-walkthrough", self.generation)
        self.assertIn("--file .pr-walkthrough/guidelines.md", self.generation)
        self.assertIn(
            "Never put shell template sentinels such as PR_TITLE or PR_URL in a code, patch, or rendered-Markdown block.",
            self.generation,
        )
        self.assertIn("--file .pr-walkthrough/walkthrough.patch", self.generation)
        self.assertIn(".pr-walkthrough/head-context/manifest.json", self.generation)
        self.assertNotIn("render_pr_walkthrough", self.generation)
        self.assertNotIn("read_pr_file", self.generation)
        self.assertNotIn('mkdir -p .opencode/agents .opencode/tools', self.generation)
        self.assertNotIn("CLOUDFLARE_R2", self.generation)
        self.assertNotIn("pull-requests: write", self.generation)

    def test_skill_bundle_is_the_only_walkthrough_generation_package(self) -> None:
        self.assertTrue(SKILL_CONTEXT_HELPER.is_file())
        self.assertTrue(SKILL_RENDER_HELPER.is_file())
        self.assertFalse(ROOT_CONTEXT_HELPER.exists())
        self.assertFalse(ROOT_RENDER_HELPER.exists())
        self.assertFalse(ROOT_READ_HELPER.exists())
        self.assertFalse((REPO_ROOT / "scripts" / "pr-walkthrough-read-file.test.py").exists())

    def test_opencode_typescript_tools_are_removed(self) -> None:
        for path in (
            REPO_ROOT / ".github" / "scripts" / "opencode-pr-walkthrough-tool.ts",
            REPO_ROOT / ".github" / "scripts" / "opencode-pr-file-tool.ts",
        ):
            self.assertFalse(path.exists())
        self.assertNotIn("opencode-pr-walkthrough-tool.ts", self.generation)
        self.assertNotIn("opencode-pr-file-tool.ts", self.generation)
        for value in (
            "PR_NUMBER: ${{ github.event.pull_request.number }}",
            "PR_TITLE: ${{ github.event.pull_request.title }}",
            "PR_URL: ${{ github.event.pull_request.html_url }}",
            "PR_REPO: ${{ github.repository }}",
            "PR_BASE: ${{ github.event.pull_request.base.ref }}",
            "PR_HEAD: ${{ github.event.pull_request.head.ref }}",
        ):
            self.assertIn(value, self.generation)

    def test_generation_uploads_agent_built_outputs_and_diagnostics(self) -> None:
        self.assertIn("Verify agent-built walkthrough", self.generation)
        self.assertIn('"docs/pr-walkthrough/pr-${PR_NUMBER}.json"', self.generation)
        self.assertIn('"docs/pr-walkthrough/pr-${PR_NUMBER}.html"', self.generation)
        self.assertIn(
            "uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
            self.generation,
        )
        self.assertIn("- name: Stage walkthrough artifact\n        if: always()", self.generation)
        self.assertIn("path: pr-walkthrough-artifact/", self.generation)
        self.assertIn("if-no-files-found: error", self.generation)

    def test_walkthrough_runs_are_serialized_for_the_entire_pr_pipeline(self) -> None:
        workflow_header = self.workflow.split("jobs:", 1)[0]
        self.assertIn("concurrency:", workflow_header)
        self.assertIn("group: pr-walkthrough-${{ github.event.pull_request.number }}", workflow_header)
        self.assertIn("cancel-in-progress: true", workflow_header)
        self.assertNotIn("concurrency:", self.generation)
        self.assertIn("concurrency:", self.same_repo_review)
        self.assertIn("concurrency:", self.fork_review)

    def test_walkthrough_label_does_not_run_the_normal_code_review(self) -> None:
        self.assertIn("github.event.action != 'labeled'", self.same_repo_review)
        self.assertIn(
            "github.event.label.name != 'generate-pr-walkthrough'",
            self.same_repo_review,
        )

    def test_all_opencode_jobs_use_the_trusted_reusable_setup_action(self) -> None:
        workflows = self.workflow + self.review_workflow
        self.assertEqual(workflows.count("uses: ./.trusted-actions/setup-opencode"), 3)
        self.assertEqual(
            workflows.count(
                'git show "$BASE_SHA:.github/actions/setup-opencode/action.yml"'
            ),
            2,
        )
        self.assertEqual(
            workflows.count(
                'git show "$TRUSTED_SHA:.github/actions/setup-opencode/action.yml"'
            ),
            1,
        )
        self.assertNotIn("curl -fsSL", workflows)
        self.assertTrue(SETUP_OPENCODE_ACTION.is_file())
        action = SETUP_OPENCODE_ACTION.read_text(encoding="utf-8")
        self.assertIn(
            "60fe5a92dc9af64ec079348fedde17e12da6a867efe7e8353be8038480607924",
            action,
        )

    def test_fork_job_fetches_base_before_materializing_setup_action(self) -> None:
        fetch = self.fork_review.index('git fetch --no-tags --prune origin "$BASE_SHA"')
        materialize = self.fork_review.index(
            'git show "$BASE_SHA:.github/actions/setup-opencode/action.yml"'
        )
        self.assertLess(fetch, materialize)

    def test_publication_is_artifact_only_and_exports_validated_url(self) -> None:
        self.assertIn("needs: pr-walkthrough-generate", self.publication)
        self.assertIn(
            "uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c",
            self.publication,
        )
        self.assertNotIn("actions/checkout", self.publication)
        self.assertNotIn("opencode run", self.publication.lower())
        self.assertNotIn("CLOUDFLARE_API_TOKEN", self.publication)
        self.assertIn("permissions: {}", self.publication)
        self.assertIn("url: ${{ steps.publish.outputs.url }}", self.publication)
        self.assertIn("printf 'url=%s\\n' \"$PUBLIC_URL\"", self.publication)

    def test_publication_uploads_only_html_with_expected_metadata(self) -> None:
        for value in (
            'SHORT_HEAD_SHA="${HEAD_SHA:0:12}"',
            'OBJECT_KEY="pr/${PR_NUMBER}/${SHORT_HEAD_SHA}.html"',
            "aws s3 cp",
            '--content-type "text/html; charset=utf-8"',
            '--cache-control "public, max-age=300, no-transform"',
            "aws s3api head-object",
            'test "$content_type" = "text/html; charset=utf-8"',
            'test "$content_length" -gt 0',
            "CLOUDFLARE_R2_ACCESS_KEY_ID",
            "CLOUDFLARE_R2_SECRET_ACCESS_KEY",
        ):
            self.assertIn(value, self.publication)

    def test_r2_secrets_are_scoped_to_the_upload_step(self) -> None:
        job_header, _, steps = self.publication.partition("    steps:\n")
        self.assertNotIn("CLOUDFLARE_R2_ACCESS_KEY_ID", job_header)
        self.assertNotIn("CLOUDFLARE_R2_SECRET_ACCESS_KEY", job_header)
        upload_step = steps.split("      - name:", 2)[2]
        self.assertIn("CLOUDFLARE_R2_ACCESS_KEY_ID", upload_step)
        self.assertIn("CLOUDFLARE_R2_SECRET_ACCESS_KEY", upload_step)

    def test_publication_validates_public_url_before_linking(self) -> None:
        self.assertIn("curl --fail", self.publication)
        self.assertIn("--retry 5", self.publication)
        self.assertIn("--retry-all-errors", self.publication)
        self.assertIn("--retry-delay 5", self.publication)
        self.assertIn("--retry-max-time 120", self.publication)
        self.assertIn('cmp -- "$HTML_PATH" "$FETCHED_PATH"', self.publication)
        self.assertIn("GITHUB_STEP_SUMMARY", self.publication)
        self.assertIn("https://walkthrough.kandev.ai", self.publication)
        self.assertNotIn("pull-requests: write", self.publication)
        self.assertNotIn("gh api", self.publication)

    def test_link_job_uses_trusted_base_helper_and_minimal_write_permission(self) -> None:
        self.assertIn("needs: pr-walkthrough-publish", self.link)
        self.assertIn("contents: read", self.link)
        self.assertIn("pull-requests: write", self.link)
        self.assertIn("ref: ${{ github.workflow_sha }}", self.link)
        self.assertNotIn("github.event.pull_request.base.sha", self.link)
        self.assertIn("persist-credentials: false", self.link)
        self.assertIn('test "$(git rev-parse HEAD)" = "$TRUSTED_SHA"', self.link)
        self.assertIn("scripts/pr-walkthrough-pr-body", self.link)
        self.assertIn("--github-response", self.link)
        self.assertIn("--input", self.link)
        patch_call = self.link.split("gh api --method PATCH", 1)[1].split("echo", 1)[0]
        self.assertIn("--silent", patch_call)
        self.assertIn(
            "PUBLIC_URL: ${{ needs.pr-walkthrough-publish.outputs.url }}",
            self.link,
        )
        self.assertIn('test -n "$PUBLIC_URL"', self.link)
        self.assertNotIn(
            'PUBLIC_URL="https://walkthrough.kandev.ai/pr/${PR_NUMBER}/${HEAD_SHA}.html"',
            self.link,
        )
        self.assertNotIn("CLOUDFLARE_R2", self.link)
        self.assertNotIn("OPENCODE_API_KEY", self.link)
        self.assertTrue(PR_BODY_HELPER.is_file())

    def test_link_job_retries_transient_github_api_responses(self) -> None:
        for value in (
            "pr_response_ok=false",
            "for attempt in 1 2 3; do",
            "python3 -c 'import json; json.load(open(\"pr-response.json\", encoding=\"utf-8\"))'",
            'test "$pr_response_ok" = true',
            "patch_ok=false",
            'test "$patch_ok" = true',
        ):
            self.assertIn(value, self.link)

    def test_lint_workflow_runs_contract_and_body_helper_tests(self) -> None:
        lint_workflow = LINT_WORKFLOW.read_text(encoding="utf-8")
        for command in (
            "python3 .github/scripts/pr-walkthrough-workflow-contract_test.py",
            "python3 scripts/pr-walkthrough-pr-body.test.py",
            "python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py",
            "python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py",
        ):
            self.assertIn(command, lint_workflow)


if __name__ == "__main__":
    unittest.main()
