---
status: active
system: ui
created: 2026-08-22
owners:
  - kandev
---
# Pull Request Walkthrough Generation Requirements

## Overview

Reviewers can inspect a diff in GitHub, but a large pull request still takes time to understand. Kandev should generate a visual explanation that gives a reviewer the change context, architecture, important code paths, risk, and review focus before they read the full diff.

## Requirements

### REQ-UI-PR-WALKTHROUGH-001: Pull Request Walkthrough Generation

**Intent:** Reviewers can inspect a diff in GitHub, but a large pull request still takes time to understand. Kandev should generate a visual explanation that gives a reviewer the change context, architecture, important code paths, risk, and review focus before they read the full diff.

#### Acceptance criteria

- **AC-UI-PR-WALKTHROUGH-001.1:** A project skill named `pr-walkthrough` is available to compatible agents. Its directory is a self-contained, copyable package with instructions, renderer assets, deterministic generation scripts, and focused tests.
- **AC-UI-PR-WALKTHROUGH-001.2:** The skill produces one JSON data file and one HTML file for a pull request: `docs/pr-walkthrough/pr-<number>.json` and `docs/pr-walkthrough/pr-<number>.html`.
- **AC-UI-PR-WALKTHROUGH-001.3:** The JSON describes the pull request, why it exists, an optional architecture diagram, key code changes, data or storage, risk, trade-offs, and review focus.
- **AC-UI-PR-WALKTHROUGH-001.4:** The renderer builds the HTML from the JSON and fixed renderer assets. It escapes code and prose, validates required fields, validates node edges, and rejects unreplaced template tokens.
- **AC-UI-PR-WALKTHROUGH-001.5:** The generated page contains a vertical reviewer story, a dark and light theme, architecture and data diagrams when supplied, highlighted code, diff tinting, GitHub file links, an interactive code canvas, and a linear fallback list. Canvas edges use separate endpoint and node-pair lanes, and their labels follow the routed edge without stacking. Horizontal code scrollbars stay subtle until a reviewer hovers over or focuses a code block. Patch and explicitly marked diff blocks use diff colors. Plain code blocks remain neutral because they provide context rather than a change. Its top bar uses the website brand mark and favicon, and its dark theme uses the documentation shell's dark-gray palette.
- **AC-UI-PR-WALKTHROUGH-001.6:** The configured workflow agent generates and renders the walkthrough for a non-draft same-repository pull request when it is opened, reopened, marked ready for review, or updated. OpenCode is the initial runner, but the skill and artifact contract do not depend on it. A maintainer can explicitly retrigger generation by adding the `generate-pr-walkthrough` label.
- **AC-UI-PR-WALKTHROUGH-001.7:** The workflow gives each runner the same fixed prompt, prepared context, draft JSON path, renderer command, and final output paths. A provider change does not change this contract.
- **AC-UI-PR-WALKTHROUGH-001.8:** Kandev CI uses `.pr-walkthrough/draft.json` as the provider-neutral draft path. Its renderer command invokes the script bundled under `.agents/skills/pr-walkthrough/scripts/`.

## Migrated source detail

## Why

Reviewers can inspect a diff in GitHub, but a large pull request still takes
time to understand. Kandev should generate a visual explanation that gives a
reviewer the change context, architecture, important code paths, risk, and
review focus before they read the full diff.

This increment generates the walkthrough HTML in CI and publishes the HTML to
the dedicated Cloudflare R2 walkthrough bucket. The hosted file remains
available after the pull request merges and expires through the bucket
lifecycle policy.

**Decisions:**
[ADR-2026-08-22-pr-walkthrough-r2-hosting](../../../decisions/2026-08-22-pr-walkthrough-r2-hosting.md),
[ADR-2026-08-22-pr-walkthrough-filesystem-runner](../../../decisions/2026-08-22-pr-walkthrough-filesystem-runner.md),
[ADR-2026-08-22-pr-walkthrough-description-link](../../../decisions/2026-08-22-pr-walkthrough-description-link.md),
[ADR-2026-08-23-pr-walkthrough-short-urls](../../../decisions/2026-08-23-pr-walkthrough-short-urls.md),
[ADR-2026-08-23-pr-walkthrough-workflow-provenance](../../../decisions/2026-08-23-pr-walkthrough-workflow-provenance.md),
[ADR-2026-08-24-unified-fork-approval-label](../../../decisions/2026-08-24-unified-fork-approval-label.md)

**Implementation plans:**
[Portable PR walkthrough runner fix](../../../plans/pr-walkthrough-portable-runner-fix/plan.md),
[PR walkthrough runner reliability fix](../../../plans/pr-walkthrough-runner-reliability-fix/plan.md)

## What

- A project skill named `pr-walkthrough` is available to compatible agents.
  Its directory is a self-contained, copyable package with instructions,
  renderer assets, deterministic generation scripts, and focused tests.
- The skill produces one JSON data file and one HTML file for a pull request:
  `docs/pr-walkthrough/pr-<number>.json` and
  `docs/pr-walkthrough/pr-<number>.html`.
- The JSON describes the pull request, why it exists, an optional architecture
  diagram, key code changes, data or storage, risk, trade-offs, and review
  focus.
- The renderer builds the HTML from the JSON and fixed renderer assets. It
  escapes code and prose, validates required fields, validates node edges, and
  rejects unreplaced template tokens.
- The generated page contains a vertical reviewer story, a dark and light
  theme, architecture and data diagrams when supplied, highlighted code, diff
  tinting, GitHub file links, an interactive code canvas, and a linear fallback
  list. Canvas edges use separate endpoint and node-pair lanes, and their
  labels follow the routed edge without stacking. Horizontal code scrollbars
  stay subtle until a reviewer hovers over or focuses a code block. Patch and
  explicitly marked diff blocks use diff colors. Plain code blocks remain
  neutral because they provide context rather than a change. Its top bar uses
  the website brand mark and favicon, and its dark theme uses the
  documentation shell's dark-gray palette.
- The configured workflow agent generates and renders the walkthrough for a
  non-draft same-repository pull request or an authorized contributor pull
  request when it is opened, reopened, marked ready for review, or updated.
  OpenCode is the initial runner, but the skill and artifact contract do not
  depend on it. A maintainer can explicitly retrigger same-repository
  generation by adding the `generate-pr-walkthrough` label.
- The workflow gives each runner the same fixed prompt, prepared context, draft
  JSON path, renderer command, and final output paths. A provider change does
  not change this contract.
- The workflow commit is the source for all trusted instructions, scripts,
  setup actions, context, and PR-description helpers. The event base SHA does
  not select executable workflow inputs.
- Kandev CI uses `.pr-walkthrough/draft.json` as the provider-neutral draft
  path. Its renderer command invokes the script bundled under
  `.agents/skills/pr-walkthrough/scripts/`.
- Walkthrough automation lives in `.github/workflows/pr-walkthrough.yml` and
  is enabled independently with the `PR_WALKTHROUGH_ENABLED` repository
  variable. It does not share the `OPENCODE_REVIEW_ENABLED` code-review gate.
- The initial runner uses `opencode-go/muse-spark-1.2-contributor` and its
  built-in `high` reasoning variant. The workflow passes these values with the
  OpenCode 1.17.7 `--model` and `--variant` options.
- The workflow preserves the generated JSON and HTML as CI artifacts and
  uploads only the HTML to the `kandev-pr-walkthroughs` R2 bucket.
- Each published object uses the key
  `pr/<pull-request-number>/<short-head-sha>.html`, where `short-head-sha` is
  the first 12 lowercase hexadecimal characters of the exact head SHA. It is
  served at
  `https://walkthrough.kandev.ai/pr/<pull-request-number>/<short-head-sha>.html`.
- The workflow regenerates on `synchronize` for same-repository and authorized
  contributor pull request updates. Each generated object remains keyed by
  pull request number and the 12-character prefix of the exact head SHA.
- After public validation succeeds, a separate minimum-permission job prepends
  a prominent marker-owned walkthrough callout to the pull request
  description. A rerun replaces only that callout and preserves the rest of
  the description.

## Generation contract

The skill-local context helper resolves the merge base between the exact pull
request head SHA and the trusted workflow SHA. It prepares a patch and bounded
text files from the immutable PR head. The walkthrough agent reads the trusted
workflow checkout, the prepared patch, and the bounded head files. It creates
a draft JSON file, invokes the trusted renderer, and corrects its data until
both outputs pass renderer validation. The generated `pr` object
includes the pull request number, title, URL, repository slug, base branch,
head branch, and diff statistics when they are available. The managed runner
binds identity and links to trusted event metadata before rendering.

The OpenCode adapter accepts an attempt only when the process exits zero and
both final files are non-empty. If the process exits zero without both files,
the adapter retries once. The retry starts with an empty draft and no final
files. Each attempt keeps separate status, standard output, standard error,
and draft diagnostics. A non-zero process exit fails without a retry.

Each code change includes a real repository-relative file path, a concise
explanation, and at least one real code or rendered-Markdown block. Code
excerpts come from the pull request head or its diff. The agent does not invent
source code or present a review verdict.

The walkthrough is an explanation, not a code review. It does not approve,
request changes, post findings, or claim that the pull request is safe to
merge.

All reusable generation logic lives in
`.agents/skills/pr-walkthrough/`. The skill-local scripts resolve their
renderer assets relative to that directory. A copied skill does not depend on
`scripts/pr-walkthrough-context`, `scripts/pr-walkthrough-render`, or tests at
the repository root. GitHub workflow, setup, publication, and PR-description
adapters remain outside the skill because they are platform integration code.

## Permissions

- The workflow may read pull request metadata and repository contents.
- The selected agent reads the trusted workflow checkout and prepared context. The
  context contains a patch, a manifest, and bounded regular UTF-8 files from
  the immutable PR head Git object.
- The agent can edit only one fixed draft JSON file. It can run only the fixed
  workflow-controlled renderer command. It cannot run arbitrary shell commands,
  read outside the trusted worktree, change source files, invoke subagents,
  fetch external URLs, commit, push, or publish GitHub changes.
- The workflow checks out only `github.workflow_sha` in the secret-bearing
  worktree. It fetches enough history for the event head SHA to resolve the
  merge base without checking out that SHA. It uses the same workflow-commit
  copy of the skill bundle, runner setup action, and PR-description helper.
  Pull request changes cannot replace these executable components.
- Contributor generation requires the durable `safe-to-review` approval label
  or the existing trusted-contributor allowlist path. The old `safe-to-test`
  label is not an authorization source.
- The agent invokes the fixed renderer before it finishes. The workflow only
  verifies and packages the ignored walkthrough output directory.
- The R2 publishing job receives only the bucket-scoped S3-compatible R2
  credentials required to upload the rendered HTML. The generation job does
  not receive R2 credentials.
- The PR-link job receives `pull-requests: write`, but no model or R2
  credential. It checks out the immutable workflow commit and uses a trusted
  helper. The helper validates the PR number, event head SHA, and exact
  walkthrough URL before it constructs the GitHub API update.
- The public bucket contains only generated walkthrough HTML. It does not
  publish the JSON source artifact.

## Failure modes

- If the selected agent command exits non-zero, generation fails and the
  workflow records the diagnostic output in its CI artifacts.
- If OpenCode exits zero without both required files, the adapter removes
  partial output and retries once from an empty draft. A second incomplete
  attempt fails and preserves diagnostics from both attempts.
- If the workflow cannot resolve a merge base between the workflow SHA and the
  event head SHA, context preparation fails before the agent starts.
- If a changed PR file is unsafe, binary, oversized, or outside the total
  context budget, the manifest records the omission. The patch remains
  available to the agent.
- If the JSON is missing required fields, contains invalid edges or risk data,
  includes a reserved renderer token, or otherwise violates the renderer
  contract, the renderer fails and no HTML is treated as generated.
- If the renderer cannot read its fixed shell or cannot write the output file,
  the workflow fails rather than exposing a partial page.
- If optional browser validation is unavailable on the runner, the workflow
  still validates the generated file structurally and reports live browser
  rendering as unverified. HTML generation itself remains a required check.
- If the R2 upload, object metadata validation, or bounded-retry public URL
  check fails, the workflow fails and does not report the walkthrough as
  published.
- If the PR body contains malformed, duplicate, or non-leading walkthrough
  markers, the link job fails closed and does not rewrite contributor content.

## Persistence guarantees

Generated JSON and HTML are ignored working-tree artifacts. They do not merge
into `main` or become Kandev application state. The HTML is also published to
R2 independently of GitHub artifact retention, so it remains available after
the pull request merges. The initial lifecycle deletes objects under `pr/`
after 180 days from upload; this is intentionally measured from generation,
not from merge time.

## Scenarios

- **GIVEN** a non-draft same-repository pull request is opened, reopened, or
  marked ready for review, **WHEN** the walkthrough job runs, **THEN** it
  creates non-empty JSON and HTML artifacts and publishes the HTML under the
  12-character prefix of the current head SHA in R2.
- **GIVEN** a fetched PR head, **WHEN** the workflow computes the triple-dot
  diff, **THEN** the merge base exists and context preparation succeeds.
- **GIVEN** the event base SHA is older than the workflow SHA, **WHEN** the
  pipeline runs, **THEN** every trusted executable input comes from the
  workflow SHA and the short public URL passes PR-description validation.
- **GIVEN** a changed file contains bounded UTF-8 text, **WHEN** context is
  prepared, **THEN** the agent reads the exact head bytes.
- **GIVEN** a changed path is unsafe or unsupported, **WHEN** context is
  prepared, **THEN** the manifest records the omission and no file appears.
- **GIVEN** the agent receives the prompt, **WHEN** it generates the page,
  **THEN** it changes only the draft and invokes only the fixed renderer.
- **GIVEN** OpenCode exits zero without both final files, **WHEN** the first
  attempt ends, **THEN** the adapter retries once with clean output state and
  keeps separate diagnostics for each attempt.
- **GIVEN** the clean retry also exits zero without both final files, **WHEN**
  the second attempt ends, **THEN** generation fails and publication does not
  start.
- **GIVEN** a future agent replaces OpenCode, **WHEN** its adapter starts the
  walkthrough, **THEN** it receives the same prompt, context, draft path,
  renderer command, and output contract.
- **GIVEN** a user copies `.agents/skills/pr-walkthrough/` to another compatible
  project, **WHEN** the user runs the skill, **THEN** its generation scripts,
  renderer assets, and focused tests do not require repository-root helper
  files.
- **GIVEN** a maintainer adds the `generate-pr-walkthrough` label, **WHEN** the
  label-triggered walkthrough job runs, **THEN** it regenerates the current PR
  head and updates the corresponding R2 object and job-summary URL.
- **GIVEN** a pull request receives a new head commit, **WHEN** the
  walkthrough workflow runs for the synchronize event, **THEN** it generates
  a walkthrough for the new head when the pull request is same-repository or
  has contributor approval.
- **GIVEN** two pull requests use different numbers, **WHEN** both jobs run,
  **THEN** each output filename and R2 object key is distinct and neither run
  overwrites the other's result.
- **GIVEN** two walkthrough triggers target the same pull request, **WHEN** the
  newer workflow starts, **THEN** per-PR workflow concurrency cancels the older
  pipeline so publication and PR linking cannot race across runs.
- **GIVEN** the agent submits malformed JSON or a review verdict instead of the
  walkthrough contract, **WHEN** the managed renderer rejects it, **THEN** the
  agent must correct the data; the job fails without publishing if no valid
  output is produced.
- **GIVEN** a generated code excerpt contains HTML characters or a Mermaid
  label contains unsafe markup, **WHEN** the page is rendered, **THEN** the
  source is escaped or sanitized and no executable PR-controlled markup is
  inserted by the renderer.
- **GIVEN** two canvas edges share an endpoint or node pair, **WHEN** the
  canvas draws them, **THEN** their paths and labels use separate lanes and do
  not occupy the same route.
- **GIVEN** a code block overflows horizontally, **WHEN** it is not hovered or
  focused, **THEN** its scrollbar thumb is transparent; **WHEN** it is hovered
  or focused, **THEN** a subtle thumb appears.
- **GIVEN** a code block uses a patch or explicit diff marker, **WHEN** the
  page renders it, **THEN** added and removed lines receive diff colors, and a
  plain code block remains neutral context.
- **GIVEN** a draft pull request or an unauthorized fork event, **WHEN** the
  workflow is triggered, **THEN** no walkthrough agent job runs.
- **GIVEN** an approved contributor pull request, **WHEN** the workflow is
  triggered, **THEN** it prepares the exact head data without checking out or
  executing contributor code in the secret-bearing generation worktree.
- **GIVEN** a successfully generated HTML file, **WHEN** the publishing job
  uploads it, **THEN** a public GET to the reported URL returns the same HTML
  with an HTML content type.
- **GIVEN** a validated public walkthrough URL, **WHEN** the link job runs,
  **THEN** the first block in the PR description is a prominent walkthrough
  callout and all existing contributor content remains below it.
- **GIVEN** the walkthrough label regenerates the same or a newer PR head,
  **WHEN** the link job runs again, **THEN** it replaces the owned callout
  without duplicating markers or changing the remaining description.

## Out of scope

- Publishing walkthrough screenshots or using the screenshot media branch.
- Generating walkthroughs for unauthorized contributor pull requests.
- Retaining a walkthrough for an exact period measured from merge time. The
  initial lifecycle is measured from upload time.
- Adding a Kandev UI page for externally generated PR walkthroughs.
- Replacing Kandev's existing in-app `changes-walkthrough` behavior.
- Adding a merge-approval or automated code-review verdict to the page.
