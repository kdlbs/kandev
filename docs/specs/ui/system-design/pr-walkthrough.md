---
status: current
system: ui
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
---

# PR Walkthrough Publication and Description Integrity System Design

## Purpose and boundaries

The PR walkthrough contract owns the reviewer-facing relationship between a
published walkthrough object and the link in the pull request description.
The walkthrough requirement remains the source of truth for the public URL,
the marker-owned callout, and its repair behavior.

The CI automation system owns workflow authorization, event gates, and job
permissions. The preview command owns preview deployment. This design covers
the shared description-write protocol that those adjacent components must use
when they update the same GitHub pull request document.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PR-WALKTHROUGH-001` | [Public URL contract](#public-url-contract), [Description ownership and writes](#description-ownership-and-writes), [Reconciliation flow](#reconciliation-flow), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-PR-WALKTHROUGH-001.9` | [Public URL contract](#public-url-contract), [Publication flow](#publication-flow) |
| `AC-UI-PR-WALKTHROUGH-001.10` | [Description ownership and writes](#description-ownership-and-writes), [Failure and recovery](#failure-and-recovery) |
| `AC-UI-PR-WALKTHROUGH-001.11` | [Reconciliation flow](#reconciliation-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/pr-walkthrough.yml` | Generate the walkthrough, publish the HTML, and link the validated URL. |
| `.github/workflows/pr-walkthrough-reconcile.yml` | Reconcile an existing walkthrough callout after a pull request description edit without generating or executing pull request code. |
| `scripts/pr-walkthrough-pr-body` | Validate the pull request identity and canonical URL, merge the walkthrough marker block, and reject malformed ownership state. |
| `apps/backend/cmd/preview/github.go` | Update or remove the preview marker block using the same fresh-read, merge, compare, patch, and readback protocol. |
| GitHub pull request description | External mutable document containing contributor content plus independently marker-owned automation sections. |
| Cloudflare R2 and `walkthrough.kandev.ai` | Store and serve immutable head-keyed HTML objects for the published snapshots. |

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

- [Own a top-level PR walkthrough callout](../../../decisions/2026-08-22-pr-walkthrough-description-link.md)
- [Use 12-character SHA prefixes for PR walkthrough URLs](../../../decisions/2026-08-23-pr-walkthrough-short-urls.md)
- [Use the workflow SHA for trusted PR walkthrough inputs](../../../decisions/2026-08-23-pr-walkthrough-workflow-provenance.md)
- [Keep PR walkthrough description updates canonical and race-safe](../../../decisions/2026-09-05-pr-walkthrough-description-integrity.md)
- [Unified contributor pull request automation](../../ci/system-design/unified-contributor-pr-automation.md)
