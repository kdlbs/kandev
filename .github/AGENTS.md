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
- For workflow security changes, run the relevant raw workflow-contract tests,
  `python3 .github/scripts/lint-action-pinning_test.py`, `zizmor .github/workflows`,
  and `git diff --check`.
