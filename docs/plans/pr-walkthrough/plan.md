---
spec: docs/specs/pr-walkthrough/spec.md
created: 2026-08-22
status: superseded
superseded_by: ../pr-walkthrough-portable-runner-fix/plan.md
---

# Implementation Plan: Pull Request Walkthrough Generation

## Overview

Adapt the upstream `pr-walkthrough` skill into Kandev's `.agents/skills`
layout and retain its standard-library renderer and focused tests. Keep the
skill provider-neutral and make the selected agent own the complete
data-render-repair loop. A dedicated OpenCode walkthrough workflow supplies a
narrow trusted rendering tool, verifies the agent-built outputs, and hands
them to a separate R2 publication job. The publication job uploads only the
HTML to the preconfigured `kandev-pr-walkthroughs` bucket. A final trusted job
places the validated custom-domain URL in a marker-owned callout at the top of
the pull request description.

The workflow must not execute PR-owned code or use PR-owned renderer assets.
The agent reads the PR as untrusted source material and has no generic edit,
shell, network, subagent, commit, push, or GitHub-write capability. Its managed
rendering adapter may write only the fixed walkthrough outputs and run only the
trusted renderer.

---

## Repository harness

### `pr-walkthrough` skill and renderer

Add the provider-neutral skill under
`.agents/skills/pr-walkthrough/`. Adapt the upstream skill's default/base
branch instructions to Kandev and require the selected agent to own the full
data-render-repair loop. Keep the renderer and its fixed HTML shell in
`references/`, with the example data and standard-library unit tests. The
renderer remains independent of Kandev application code.

### Trusted rendering adapter

Add a small standard-library helper that accepts walkthrough JSON from a
managed agent tool, binds trusted pull-request identity, removes model-owned
URL overrides, writes the fixed JSON path, and invokes the trusted renderer.
The OpenCode-specific tool is a thin adapter around this helper. Other agent
runners follow the same skill and output contract through their native tools
or an equivalent narrow adapter.

---

## CI workflow

### Dedicated OpenCode generation job

Add `.github/workflows/pr-walkthrough.yml` as a dedicated workflow using the
`pull_request_target` event family. Gate it with the independent
`PR_WALKTHROUGH_ENABLED` variable, not `OPENCODE_REVIEW_ENABLED`, and keep it
decoupled from the code-review App token. The job runs for `opened`,
`reopened`, `ready_for_review`, and `synchronize`. A label-triggered rerun also
runs when the label is `generate-pr-walkthrough`.

Use the shared base-controlled setup action for OpenCode. Select
`opencode-go/muse-spark-1.2-contributor` with `--model`. Select its built-in
`high` reasoning variant with `--variant`. The pinned OpenCode 1.17.7 model
catalog declares this variant, so no custom provider override is necessary.

The job will:

1. Check out `github.event.pull_request.head.sha` with full history and no
   persisted credentials.
2. Compute the base-to-head context from the event SHAs.
3. Materialize the skill, renderer, shell, and managed rendering adapter from
   the trusted base SHA, never from the PR head.
4. Run OpenCode with an inline agent definition that permits reads and the
   narrow `render_pr_walkthrough` tool only.
5. Require OpenCode to iterate through renderer errors until it produces
   `docs/pr-walkthrough/pr-<number>.json` and `.html` itself.
6. Require non-empty JSON and HTML files and preserve OpenCode logs and the
   generated files as an uploaded CI artifact, including on failure.

The job keeps R2 credentials out of generation. The artifact is the only
handoff to the publication job. Add `docs/pr-walkthrough/` to `.gitignore`.

### Cloudflare R2 publication job

Add a dependent publication job that runs only after successful generation.
The job must not check out or execute PR-owned files. It downloads the HTML
artifact, uploads only the HTML through the R2 S3-compatible API, and validates
the public object.

The publication contract is:

- Bucket variable: `CLOUDFLARE_R2_BUCKET`, expected value
  `kandev-pr-walkthroughs`.
- Endpoint variable: `CLOUDFLARE_R2_ENDPOINT`, normally
  `https://<account-id>.r2.cloudflarestorage.com` for the default bucket
  jurisdiction.
- Base URL variable: `CLOUDFLARE_R2_BASE_URL`, expected value
  `https://walkthrough.kandev.ai`.
- Access-key secret: `CLOUDFLARE_R2_ACCESS_KEY_ID`.
- Secret-key secret: `CLOUDFLARE_R2_SECRET_ACCESS_KEY`.
- Existing account variable: `CLOUDFLARE_ACCOUNT_ID` remains available for
  endpoint construction and is not a secret.
- Object key: `pr/<pull-request-number>/<short-head-sha>.html`, where
  `short-head-sha` is the first 12 lowercase hexadecimal characters of the
  exact head SHA.
- Object metadata: `Content-Type: text/html; charset=utf-8` and a short cache
  lifetime that does not outlive lifecycle deletion or same-head reruns.

Use an S3-compatible upload command such as `aws s3 cp` with the endpoint,
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_DEFAULT_REGION=auto`
environment variables. Do not pass the bucket-scoped R2 Object Read & Write
credentials to `cloudflare/wrangler-action`; those credentials are for the R2
S3-compatible API. The existing Pages `CLOUDFLARE_API_TOKEN` remains separate.

After upload, run an object metadata check and a bounded-retry public HTTPS GET
against the reported URL. Export the validated URL for the dependent link job
and write it to the workflow job summary. Do not upload the JSON artifact to
the public bucket. The publication job itself has no GitHub write permission.

### Pull request description link job

Add a final job after successful publication. Give only this job
`pull-requests: write`; do not give it model or R2 credentials. Check out the
exact base SHA without persisted credentials and run a trusted
standard-library helper that validates the GitHub PR number/head SHA and exact
hosted URL. The helper allows the live PR head to advance while publication
finishes because the SHA-keyed walkthrough remains the intended initial
snapshot.

The helper prepends a GitHub alert between `kandev-pr-walkthrough-start` and
`kandev-pr-walkthrough-end` markers. On a rerun it replaces only that leading
block. It fails on unbalanced, duplicate, or non-leading markers instead of
rewriting content it does not own.

The Cloudflare setup is an operator prerequisite rather than repository
automation: the bucket is `kandev-pr-walkthroughs`, the custom domain is
`walkthrough.kandev.ai`, and the bucket has an Object Lifecycle rule for the
`pr/` prefix. The initial plan assumes a 180-day expiration from upload so
walkthroughs normally survive the time before merge. Exact post-merge
retention would require a later merge-promotion job.

### Workflow contract coverage

Add `.github/scripts/pr-walkthrough-workflow-contract_test.py` and wire it into
`.github/workflows/lint-action-pinning.yml`. The contract test will assert the
event gates, exact base checkout, immutable head-object fetch, constrained
PR-head file reads, narrow OpenCode permissions, trusted metadata binding,
generation output paths,
artifact handoff, R2 publication command, required secret/variable names,
public URL validation, isolated PR-body write permissions, trusted helper use,
and the independently gated dedicated workflow boundary.

---

## Tests

- **What:** Renderer accepts valid walkthrough data, escapes source, converts
  patches, validates edges and risk data, fills the HTML shell, and rejects
  malformed fields.
  **File:** `.agents/skills/pr-walkthrough/references/test_build.py`
  **How:** Run the upstream-adapted Python standard-library unit suite.
- **What:** The managed renderer binds trusted PR metadata, removes model URL
  overrides, produces both fixed outputs, and leaves no partial files after a
  renderer failure.
  **File:** `scripts/pr-walkthrough-render.test.py`
  **How:** Run the Python standard-library unit suite.
- **What:** The workflow keeps PR content read-only and uses trusted base
  assets while producing ignored JSON/HTML artifacts and publishing only the
  rendered HTML to R2.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py`
  **How:** Run the static workflow contract test.
- **What:** The publication job uses the intended bucket, endpoint, object key,
  content type, lifecycle-compatible cache policy, and public URL check without
  exposing R2 credentials to generation.
  **File:** `.github/scripts/pr-walkthrough-workflow-contract_test.py`
  **How:** Run the static workflow contract test and a configured CI dry run.
- **What:** The PR body helper prepends one prominent callout, updates it
  idempotently, validates event identity, preserves contributor content, and
  rejects malformed ownership markers.
  **File:** `scripts/pr-walkthrough-pr-body.test.py`
  **How:** Run the Python standard-library unit suite.
- **What:** New skill and reference files satisfy Kandev harness limits.
  **File:** `.agents/skills/pr-walkthrough/SKILL.md` and `references/*`
  **How:** Run the harness linter on the new skill directory.
- **What:** All workflow action refs remain immutable and the workflow edit has
  no whitespace errors.
  **File:** `.github/workflows/pr-walkthrough.yml` and
  `.github/workflows/opencode-code-review.yml`
  **How:** Run the action-pinning tests/linter and `git diff --check`.

---

## E2E Tests

There is no Kandev UI flow in this increment. The generated HTML is validated
structurally by the renderer and workflow checks, and the publication job
performs a live public GET against the R2 custom domain. Full browser
interaction testing remains out of scope.

---

## Verification Results

Completed 2026-08-22:

- `python3 scripts/pr-walkthrough-render.test.py`: 2 tests passed.
- `cd .agents/skills/pr-walkthrough/references && python3 -m unittest test_build`: 56 tests passed.
- `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py`: 18 tests passed.
- `python3 scripts/pr-walkthrough-pr-body.test.py`: 8 tests passed.
- `python3 scripts/lint-harness-files.test.py`: 19 tests passed.
- `python3 .github/scripts/lint-harness-files.py .agents/skills/pr-walkthrough`: passed.
- `python3 .github/scripts/lint-action-pinning_test.py`: 9 tests passed.
- `python3 .github/scripts/lint-action-pinning.py`: 19 workflows passed.
- `actionlint` passed for the dedicated walkthrough, OpenCode review, and
  action-pinning workflows.
- Pinned OpenCode 1.17.7 resolved the default-deny walkthrough agent with only
  read, glob, grep, and the trusted renderer enabled. Its refreshed model
  catalog reports Muse Spark 1.2 Contributor reasoning support and native
  `high` and `xhigh` variants.
- The OpenCode custom tool bundled successfully with Bun after marking the
  runtime-provided `@opencode-ai/plugin` import external.
- `git diff --check`: passed.
- The example JSON rendered a 1,161-line HTML file with no unreplaced tokens.
- `zizmor` was run through `mise x zizmor@1.25.2`; the repository-wide audit
  remains non-zero because of existing findings in unrelated workflows, and
  both targeted base-controlled workflows retain the expected
  `pull_request_target` dangerous-trigger finding.
- Chromium is not installed, so the optional headless DOM check was not run.
- Operator provisioning is complete. A live R2 upload and public GET remain
  pending the first eligible PR workflow run.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-walkthrough-skill-renderer](task-01-walkthrough-skill-renderer.md)
- [x] [task-02-agent-rendering-contract](task-02-agent-rendering-contract.md) (superseded by the portable filesystem contract)

Wave 2:

- [x] [task-03-workflow-artifact-generation](task-03-workflow-artifact-generation.md)

Wave 3:

- [x] [task-04-r2-html-publication](task-04-r2-html-publication.md)

Wave 4:

- [x] [task-05-dedicated-workflow-pr-link](task-05-dedicated-workflow-pr-link.md)

Execute sequentially in the primary conversation unless the user explicitly
authorizes parallel implementation.

## Open Questions

- Whether to enable `synchronize` generation later remains a product/cost
  decision. The SHA-keyed R2 object contract already supports it.
- Whether retention must be exactly 90 days after merge remains deferred. The
  initial lifecycle is 180 days after upload.
