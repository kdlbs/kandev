# GitHub Actions security

Apply this guidance whenever editing `.github/**`.

- `issue_comment` and `pull_request_target` jobs execute the workflow from the
  trusted default/base branch. Syncing a PR does not change an existing
  comment-triggered run.
- Never check out or execute an untrusted PR head in a secret-bearing or
  comment-triggered job before a privileged agent or action. Keep the default
  branch checkout, use `persist-credentials: false`, and treat PR metadata,
  diffs, and files as untrusted data.
- Constrain capabilities in the tool policy, not prompt text alone. For PR-file
  reads, prefer a small GET-only helper bound to the event PR/head; validate
  normalized repository-relative paths, regular-file type, response path,
  size, encoding, and content. Do not grant generic `gh api`, arbitrary
  interpreters, or broad Bash merely to read PR files.
- PR label/metadata cleanup jobs that operate on pull requests must declare
  `pull-requests: write`, not `issues: write`; mirror the permission shape used
  by `preview-env.yml`.
- **Trusted-main workflow tests:** Some mutating or scheduled workflows
  intentionally check out `origin/main`. A `workflow_dispatch` started from a
  feature branch therefore tests the current default-branch workflow, not the
  PR's workflow files. Inspect the checkout ref and head SHA, separate PR CI
  from post-merge production-workflow smoke tests, and do not call a
  trusted-main failure a pre-merge implementation failure.
- Release workflow changes must trace prepare/summary outputs and conditions
  through every build and publication job: skip decisions must propagate
  without accidental publishing, while backfill must reuse and validate the
  existing tag and still run its required build/publication path. Verify each
  actual publication job and artifact, not only the aggregate workflow result.
- For workflow security changes, run the relevant raw workflow-contract tests,
  `python3 .github/scripts/lint-action-pinning_test.py`, `zizmor .github/workflows`,
  and `git diff --check`.
