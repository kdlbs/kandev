---
status: current
system: ui
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
---

# PR Walkthrough Generation and Publication System Design

## Purpose and boundaries

The PR walkthrough contract owns generation of the portable output pair and
the relationship between a published object and its pull request callout.
The walkthrough requirement defines completion, the public URL, description
ownership, and repair behavior.

The CI automation system owns workflow authorization, event gates, and job
permissions. The preview command owns preview deployment. This design covers
generation and the shared description-write protocol used by adjacent writers.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PR-WALKTHROUGH-001` | [Public URL contract](#public-url-contract), [Description ownership and writes](#description-ownership-and-writes), [Reconciliation flow](#reconciliation-flow), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-PR-WALKTHROUGH-001.9` | [Public URL contract](#public-url-contract), [Publication flow](#publication-flow) |
| `AC-UI-PR-WALKTHROUGH-001.10` | [Description ownership and writes](#description-ownership-and-writes), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-PR-WALKTHROUGH-001.11` | [Reconciliation flow](#reconciliation-flow), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-PR-WALKTHROUGH-001.12` | [Rendering errors](#rendering-errors) |
| `AC-UI-PR-WALKTHROUGH-001.13` | [Generation completion](#generation-completion), [Output verification](#output-verification) |
| `AC-UI-PR-WALKTHROUGH-001.14` | [Runner supervision](#runner-supervision), [Generation diagnostics](#generation-diagnostics) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/pr-walkthrough.yml` | Generate the walkthrough, publish the HTML, and link the validated URL. |
| `.github/workflows/pr-walkthrough-reconcile.yml` | Reconcile an existing walkthrough callout after a pull request description edit without generating or executing pull request code. |
| `scripts/pr-walkthrough-pr-body` | Validate the pull request identity and canonical URL, merge the walkthrough marker block, and reject malformed ownership state. |
| `apps/backend/cmd/preview/github.go` | Update or remove the preview marker block using the same fresh-read, merge, compare, patch, and readback protocol. |
| GitHub pull request description | External mutable document containing contributor content plus independently marker-owned automation sections. |
| Cloudflare R2 and `walkthrough.kandev.ai` | Store and serve immutable head-keyed HTML objects for the published snapshots. |

## Generation completion

The [generation completion package](../../../plans/pr-walkthrough-generation-completion/plan.md) implements this extension. Its work orders record implementation and targeted verification results.

The portable renderer still owns the fixed draft and final output contract. The host adapter owns process supervision. Successful rendering, followed by independent verification, establishes completion without another model response.

`.agents/skills/pr-walkthrough/scripts/pr-walkthrough-render` removes the old completion receipt before each invocation. It validates the draft and binds trusted metadata. It then builds and replaces both fixed output files through the existing temporary directory.

Only after both replacements succeed, the renderer atomically writes `.pr-walkthrough/render-complete.json`. This versioned receipt contains the positive PR number, full event `HEAD_SHA`, fixed output paths, and SHA-256 hashes. `HEAD_SHA` must contain exactly 40 lowercase hexadecimal characters. It does not replace the branch name in `pr.head`.

The agent can edit only the draft. It cannot directly write the receipt or final outputs. The receipt records completion within this permission boundary. Its hashes detect stale or mismatched bytes but do not constitute a security signature.

## Rendering errors

`load_draft()` retains the JSON parser's message, line, and column. Its error identifies the fixed draft path. It does not print the draft or environment values.

Schema errors keep their existing field paths, such as `changes[1].title`. The skill directs managed agents to repair the reported location. After the first successful render, the agent finishes without rewriting the completed draft.

## Output verification

The new skill-local `scripts/pr-walkthrough-verify` entry point takes no arguments. The host invokes it after stopping and reaping the agent group. Its operations are read-only:

1. Read the fixed receipt and reject unsupported versions or incorrect event identity.
2. Require the fixed JSON and HTML paths. Reject symlinks, missing files, and empty files.
3. Compare file hashes with the receipt.
4. Parse the final JSON and compare its identity with `trusted_identity()` without silently rebinding it.
5. Apply `validate_managed_draft()` against the prepared manifest.
6. Compare the saved HTML with the in-memory result of `references/build.py:build()`.

Verification does not create missing outputs, rewrite final files, or infer completion from a model phrase. Any disagreement prevents publication. A subsequent incomplete render cannot pass using an earlier receipt.

## Runner supervision

The new `.github/scripts/pr-walkthrough-runner.py` is a GitHub adapter, outside the portable skill. Its executable command and parameters come only from the trusted workflow. It launches the agent in an owned process group and polls the fixed receipt every 250 milliseconds.

The first observed receipt initiates SIGTERM for that group. After five seconds, remaining owned processes receive SIGKILL. Cleanup has a ten-second bound and treats zombie-only process groups as stopped because their members cannot execute. The adapter reaps its direct child and invokes the verifier before returning success. `Popen` uses `start_new_session=True`, so the child PID is also the owned process-group ID. An expected supervisor-induced exit is distinct from an unexpected non-zero exit.

The adapter uses a 600-second monotonic deadline shared by both attempts. Only an incomplete zero-exit attempt can retry once. A retry removes the receipt, draft, and final outputs before starting. It consumes the remaining budget rather than receiving a fresh deadline.

Timeout without verified completion, invalid receipts, unexpected non-zero exits, and external cancellation fail without retry. Cancellation wins over observed completion. A cancelled job never becomes successful merely because files exist.

The outer generation job uses a 20-minute limit. Preparation steps use two minutes for checkout, four for history fetching, one for context, and one for installation. Independent verification has a 30-second limit. These budgets reserve at least one minute for artifact upload, apart from scheduler overhead. Infrastructure cancellation remains a failure even if diagnostic upload cannot finish.

## Git history preparation

Checkout remains fixed at `github.workflow_sha` with no persisted credentials. Use depth one to avoid `actions/checkout` fetching every branch and tag.

The new trusted `.github/scripts/pr-walkthrough-history.sh` fetches complete histories only for the trusted SHA and PR-head ref. It uses explicit private destination refs and `--no-tags`. For a shallow checkout, it removes shallow boundaries through targeted `--unshallow` fetching. For a complete checkout, it omits that option.

The helper validates the fetched head against the exact event `HEAD_SHA`. It requires a merge base with the trusted SHA before context preparation. `HEAD` remains the trusted SHA. Missing or advanced PR-head refs fail closed. This avoids the earlier shallow-head regression documented in the portable runner package.

## Generation diagnostics

Each attempt retains stdout, stderr, the draft, output files when present, and a structured outcome record. The record includes UTC start/end timestamps, elapsed time, raw exit status, stop reason, and verification result. Cleanup failure has its own stop reason and remains visible in the workflow summary.

Stop reasons distinguish render completion, incomplete zero exit, unexpected exit, cleanup failure, deadline, cancellation, and launch failure. Verification results distinguish cleanup failure from verifier failure and cancellation. Logs include timestamps and errors without environment dumps. Diagnostic capture runs in the adapter's cleanup path, including timeout and cancellation.

The workflow summary reports stage durations and the final generation result. It does not expose a completion marker as proof. Artifact upload retains the existing `always()` path. Publication still requires the generation job to succeed.

## Runner and hosting configuration

Generation remains independently gated by `PR_WALKTHROUGH_ENABLED`. CI owns same-repository and approved-contributor eligibility. The model remains `opencode/muse-spark-1.3-contributor-free` with the `high` variant. Its accepted training policy covers the patch and prepared head context. This package does not change that policy or the pinned setup action.

The artifact retains JSON and HTML. Only HTML is uploaded to the `kandev-pr-walkthroughs` R2 bucket. The canonical object URL remains `https://walkthrough.kandev.ai/pr/<number>/<head-sha[0:12]>.html`. Lifecycle retention remains 180 days from upload.

## Public URL contract

The workflow keeps the full lowercase 40-character head SHA for event identity,
object validation, and trusted workflow inputs. The public object key and
callout URL use only its first 12 lowercase hexadecimal characters:

```text
pr/<pull-request-number>/<head-sha[0:12]>.html
```

The publication job is the source of the URL consumed by the link job. The
link job does not reconstruct a URL from a different SHA length. The body
helper accepts only the exact custom-domain URL derived from the event head and
the 12-character public prefix.

Existing full-SHA objects are not renamed or rewritten. A stale full-SHA link
in an owned callout is corrected when a valid canonical object is available.

## Description ownership and writes

The walkthrough owns only the content between
`kandev-pr-walkthrough-start` and `kandev-pr-walkthrough-end`. The preview
automation owns only its corresponding preview markers. All content outside a
writer's markers is preserved.

Every Kandev-owned body mutation follows this bounded protocol:

1. Fetch the current pull request body.
2. Merge only the caller's marker-owned section.
3. Fetch the body again immediately before the write.
4. If the body differs from the first snapshot, discard the merged payload,
   repeat from the latest body, and do not patch the stale document.
5. Patch the complete body because the GitHub pull request update API replaces
   the body representation.
6. Fetch the body after the patch. Report success only when the writer's
   expected marker state is present and the other marker-owned content remains
   present.

The protocol is bounded to three attempts. A per-pull-request description
concurrency group serializes the walkthrough, reconciliation, and short
preview-description jobs that Kandev controls. Long-running preview lifecycle
jobs do not hold that lock. Compare-and-readback checks remain required because
users and external integrations can edit the document outside GitHub Actions.
An update that cannot converge fails without a best-effort broad rewrite.

## Publication flow

1. The generation job produces the JSON and HTML artifacts for the event head.
2. The publication job uploads only HTML under the 12-character object key.
3. It validates content type, non-zero length, public availability, and exact
   public bytes.
4. It exports the validated public URL to the link job.
5. The link job uses the description-write protocol to prepend or replace the
   walkthrough marker block.

The link job may link the validated published snapshot even if the pull request
head advances while generation or publication is running. A later
`synchronize` run produces and links the newer head snapshot.

## Reconciliation flow

The dedicated reconciliation workflow accepts the `edited` pull request event
for a non-generating path. The reconciliation job:

1. Uses the trusted workflow checkout and the existing authorization boundary.
2. Computes the current head's canonical 12-character public URL.
3. Checks that the canonical object is publicly available as HTML.
4. Reads the live pull request description.
5. If an existing walkthrough marker block contains a stale or legacy URL, it
   replaces only that block using the description-write protocol.
6. If no walkthrough marker exists, the object is unavailable, or the markers
   are malformed, it does not perform a destructive write and records the
   reason.

An already-canonical block is a no-op. A body PATCH emitted by Kandev may
itself produce an `edited` event; the canonical no-op path prevents a repair
loop.

## Failure and recovery

- A missing public object prevents reconciliation from changing the body. The
  next successful generation or publication remains responsible for creating
  the link.
- An unbalanced, duplicate, or non-leading walkthrough marker fails closed.
  Contributor content is not guessed or reconstructed.
- A stale body snapshot causes a fresh merge and bounded retry. The stale body
  is never sent to GitHub.
- A post-write readback that does not contain the expected owned result causes
  another fresh merge. Exhausting retries fails the job and exposes the
  condition in the job summary.
- A legacy full-SHA URL is treated as replaceable content only inside the
  walkthrough markers. Full-SHA links outside the owned block are preserved.
- Preview cleanup keeps its existing behavior. If no preview marker exists,
  removal remains a no-op and does not affect the walkthrough block.

## Persistence

R2 objects remain keyed by pull request number and the first 12 characters of
the exact head SHA. Reruns replace bytes at the same key for the same head;
different heads receive different keys. The pull request description remains
GitHub-owned mutable state and is not copied into Kandev persistence.

No migration removes existing full-SHA objects. The URL contract is enforced
for new publication and for repair of the marker-owned callout.

## Security

The generation and reconciliation jobs use the trusted workflow commit for
helpers and workflow assets. Reconciliation reads pull request metadata and a
public URL; it does not check out or execute pull request code and does not
receive model or R2 credentials.

The link and preview writers retain only the GitHub pull request permission
needed for their respective path. The preview fork path keeps its existing
explicit authorization because it executes the preview command against the
contributor head.

## Observability

The walkthrough workflow summary records whether linking was unchanged,
repaired, retried after a body race, or skipped because the canonical object
was unavailable. A failed post-write readback includes the final marker
validation error in the job log.

Contract tests cover event routing, trusted checkout, shared concurrency,
canonical URL validation, no-op reconciliation, fail-closed malformed markers,
and the absence of full-SHA URL construction in current writers. Unit or
integration tests cover body races and preservation of the walkthrough and
preview marker blocks.

## Related decisions

- [Complete generation after a verified render](../../../decisions/2026-09-30-pr-walkthrough-render-completion.md)
- [Own a top-level PR walkthrough callout](../../../decisions/2026-08-22-pr-walkthrough-description-link.md)
- [Use 12-character SHA prefixes for PR walkthrough URLs](../../../decisions/2026-08-23-pr-walkthrough-short-urls.md)
- [Use the workflow SHA for trusted PR walkthrough inputs](../../../decisions/2026-08-23-pr-walkthrough-workflow-provenance.md)
- [Keep PR walkthrough description updates canonical and race-safe](../../../decisions/2026-09-05-pr-walkthrough-description-integrity.md)
- [Unified contributor pull request automation](../../ci/system-design/unified-contributor-pr-automation.md)
