---
status: building
created: 2026-08-22
owner: kandev
---

# Pull Request Walkthrough Generation

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
[ADR-2026-08-22-pr-walkthrough-r2-hosting](../../decisions/2026-08-22-pr-walkthrough-r2-hosting.md),
[ADR-2026-08-22-agent-owned-pr-walkthrough-rendering](../../decisions/2026-08-22-agent-owned-pr-walkthrough-rendering.md),
[ADR-2026-08-22-pr-walkthrough-description-link](../../decisions/2026-08-22-pr-walkthrough-description-link.md)

## What

- A project skill named `pr-walkthrough` is available to compatible agents.
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
  list.
- The configured workflow agent generates and renders the walkthrough for a
  non-draft same-repository pull request when it is opened, reopened, or marked
  ready for review. OpenCode is the initial runner, but the skill and artifact
  contract do not depend on it. A maintainer can explicitly retrigger
  generation by adding the `generate-pr-walkthrough` label.
- Walkthrough automation lives in `.github/workflows/pr-walkthrough.yml` and
  is enabled independently with the `PR_WALKTHROUGH_ENABLED` repository
  variable. It does not share the `OPENCODE_REVIEW_ENABLED` code-review gate.
- The initial runner uses
  `opencode-go/muse-spark-1.2-contributor#high`. OpenCode 1.17.7 reports native
  reasoning support for the model and resolves `#high` to its built-in high
  reasoning variant.
- The workflow preserves the generated JSON and HTML as CI artifacts and
  uploads only the HTML to the `kandev-pr-walkthroughs` R2 bucket.
- Each published object uses the key
  `pr/<pull-request-number>/<head-sha>.html` and is served at
  `https://walkthrough.kandev.ai/pr/<pull-request-number>/<head-sha>.html`.
- The initial workflow does not regenerate on `synchronize`. Future
  per-push generation can add that event without changing the object contract.
- After public validation succeeds, a separate minimum-permission job prepends
  a prominent marker-owned walkthrough callout to the pull request
  description. A rerun replaces only that callout and preserves the rest of
  the description.

## Generation contract

The walkthrough agent compares the exact pull request head SHA with the exact
base SHA. It creates the JSON, invokes the trusted renderer, and corrects its
data until both outputs pass renderer validation. The generated `pr` object
includes the pull request number, title, URL, repository slug, base branch,
head branch, and diff statistics when they are available. The managed runner
binds identity and links to trusted event metadata before rendering.

Each code change includes a real repository-relative file path, a concise
explanation, and at least one real code or rendered-Markdown block. Code
excerpts come from the pull request head or its diff. The agent does not invent
source code or present a review verdict.

The walkthrough is an explanation, not a code review. It does not approve,
request changes, post findings, or claim that the pull request is safe to
merge.

## Permissions

- The workflow may read pull request metadata and repository contents.
- The selected agent may read the checked-out pull request and repository
  context, but it may not run arbitrary shell commands, modify source files,
  invoke subagents, fetch external URLs, commit, push, or publish GitHub
  changes. A managed runner may expose a narrow trusted-renderer tool that
  writes only the fixed walkthrough JSON and HTML paths.
- The workflow uses the trusted base-commit copy of the walkthrough skill,
  renderer, and managed rendering adapter. Pull request changes cannot replace
  those instructions or executable generation components for the current run.
- The agent invokes the fixed renderer before it finishes. The workflow only
  verifies and packages the ignored walkthrough output directory.
- The R2 publishing job receives only the bucket-scoped S3-compatible R2
  credentials required to upload the rendered HTML. The generation job does
  not receive R2 credentials.
- The PR-link job receives `pull-requests: write`, but no model or R2
  credential. It checks out the immutable base commit and uses a trusted helper
  that validates the PR number, event head SHA, and exact walkthrough URL
  before constructing the GitHub API update.
- The public bucket contains only generated walkthrough HTML. It does not
  publish the JSON source artifact.

## Failure modes

- If the selected agent command fails, exits non-zero, or does not produce both
  required walkthrough files, generation fails and the workflow records the
  diagnostic output in its CI artifacts.
- If the JSON is missing required fields, contains invalid edges or risk data,
  includes a reserved renderer token, or otherwise violates the renderer
  contract, the renderer fails and no HTML is treated as generated.
- If the renderer cannot read its fixed shell or cannot write the output file,
  the workflow fails rather than exposing a partial page.
- If optional browser validation is unavailable on the runner, the workflow
  still validates the generated file structurally and reports live browser
  rendering as unverified. HTML generation itself remains a required check.
- If the R2 upload, object metadata validation, or public URL check fails, the
  workflow fails and does not report the walkthrough as published.
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
  current head SHA in R2.
- **GIVEN** a maintainer adds the `generate-pr-walkthrough` label, **WHEN** the
  label-triggered walkthrough job runs, **THEN** it regenerates the current PR
  head and updates the corresponding R2 object and job-summary URL.
- **GIVEN** a pull request receives a new head commit, **WHEN** the
  walkthrough workflow runs for the synchronize event, **THEN** no
  walkthrough generation occurs in the initial increment.
- **GIVEN** two pull requests use different numbers, **WHEN** both jobs run,
  **THEN** each output filename and R2 object key is distinct and neither run
  overwrites the other's result.
- **GIVEN** the agent submits malformed JSON or a review verdict instead of the
  walkthrough contract, **WHEN** the managed renderer rejects it, **THEN** the
  agent must correct the data; the job fails without publishing if no valid
  output is produced.
- **GIVEN** a generated code excerpt contains HTML characters or a Mermaid
  label contains unsafe markup, **WHEN** the page is rendered, **THEN** the
  source is escaped or sanitized and no executable PR-controlled markup is
  inserted by the renderer.
- **GIVEN** a draft pull request or an unauthorized fork event, **WHEN** the
  workflow is triggered, **THEN** no walkthrough agent job runs.
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
- Generating a new walkthrough on every push. The initial `synchronize` path
  remains disabled to control model usage.
- Retaining a walkthrough for an exact period measured from merge time. The
  initial lifecycle is measured from upload time.
- Adding a Kandev UI page for externally generated PR walkthroughs.
- Replacing Kandev's existing in-app `changes-walkthrough` behavior.
- Adding a merge-approval or automated code-review verdict to the page.
