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
- **Workflow and checkout identity:** A `workflow_dispatch` uses the workflow
  definition at the selected dispatch ref. An explicit `checkout` of `main`
  selects the files that subsequent steps consume. It does not replace the
  running workflow definition. Record `github.workflow_ref`,
  `github.workflow_sha`, and the checkout SHA separately. Attribute failures
  to the definition or consumed files before deciding which branch needs a fix.
- Release workflow changes must trace prepare/summary outputs and conditions
  through every build and publication job: skip decisions must propagate
  without accidental publishing, while backfill must reuse and validate the
  existing tag and still run its required build/publication path. Verify each
  actual publication job and artifact, not only the aggregate workflow result.
- When validating a public artifact through redirects, inspect the final HTTP
  status, content type, effective URL, and non-empty body. Intermediate
  redirect headers do not prove that the delivered artifact is valid.
- For workflow security changes, run the relevant raw workflow-contract tests,
  `python3 .github/scripts/lint-action-pinning_test.py`, `zizmor .github/workflows`,
  and `git diff --check`.
- The `pr-docs.yml` workflow is a base-controlled, metadata-only check. It reads
  pull-request files through the bounded `.github/scripts/pr-docs.cjs` adapter;
  it must never check out or execute a pull-request head.
- Its `PR documentation coverage` status is revision-specific. The exact
  `no-docs-allow` label is the only policy override, and merge-group evaluation
  must resolve and validate every member independently against the group's
  entry boundaries.
