---
created: 2026-10-13
status: done
requirements:
  - REQ-AUTH-PUBLIC-SHARE-LINKS-001
system_design:
  - ../../specs/auth/system-design/public-share-links.md
legacy_specs: []
---

# Implementation Plan: Public Share Metadata Redaction

## Overview

Complete the existing pre-publication redaction pass at snapshot construction.
One sequential work order added independent regression evidence, applied the
existing redactor to exported descriptive metadata, and verified the preview
and mock gist artifact paths. Implementation followed ROOT's later explicit
release after review of the four-artifact design package and its command-only
correction, in the same primary session.

The Auth system owns this package because it owns public-share exposure and
the pre-upload redaction promise. Task storage and GitHub credential selection
retain their existing ownership. The requirement and design remain `draft`:
this narrow clarification does not promote the entire migrated share feature.

## Scope

### In scope

- AC-AUTH-PUBLIC-SHARE-LINKS-001.6 through AC-AUTH-PUBLIC-SHARE-LINKS-001.8.
- Task title and exported session agent type, model, and executor type.
- One applied-rule union for metadata and existing message redaction.
- Independent synthetic builder, service-preview, and in-memory gist evidence.
- Ordinary values, source nonmutation, schema, identifiers, timestamps, and
  existing message controls.
- A small public-doc clarification accompanying the implemented behavior.
- The proven test-fixture barrier correction required by the backend CI failure,
  without changing SSH runtime behavior.

### Out of scope

- New secret patterns, generic whole-snapshot JSON scrubbing, or exhaustive
  credential detection.
- Historical gist discovery, rewriting, scanning, automatic revocation, or
  changed publication/revoke behavior.
- Transport, provider auth, snapshot schema, UI, or runtime-flag redesign.
- Broad SQL, frontend, audit, or local full-backend suites.

## Confirmed root cause and evidence

At base `8bcf0cffe8ec575058ff5966de9bfcb6233102f3`, `BuildSnapshot` creates
one redactor but sends only messages through it. `taskMeta` copies the title
and workflow-step ID, while `sessionMeta` copies three strings from persisted
profile/executor maps without redaction. `Redactor.Applied` therefore also
omits rules triggered only by metadata.

ROOT qualified two synthetic task-title cases through the real builder and
in-memory `github.MockClient`: a known credential shape and a workspace path
reach `snapshot.json`, `share.html`, `README.md`, and the gist description.
Both metadata rule-log assertions fail. Ordinary metadata and existing body
redaction controls pass. Evidence is recorded in
`/tmp/kandev-root-share-metadata-discovery-20261013-myy038p7/qualified-proof.json`
and `repro.log`. The recorded native join exited 1, with package time 0.077s;
these are ROOT's discovery results, not this package's implementation results.
The protected candidate test was not read and must not be copied. Permanent
tests will be independently authored only after implementation release.

The adjacent session-string omission is confirmed by source inspection; it has
not been separately executed in this design turn. `WorkflowStepID` is an
identifier, not descriptive text. Version/build identity and timestamps also
remain outside this bounded correction.

## Technical approach

### Snapshot construction

In `apps/backend/internal/task/share/builder.go`, pass the build's existing
`*Redactor` to `taskMeta` and `sessionMeta`. Apply `Redactor.String` to `Title`,
`AgentType`, `Model`, and `ExecutorType` during projection into export values.
Leave `WorkflowStep`, timestamps, version/build fields, and `snapshotString`
empty/non-string behavior intact. Preserve `workspaceRootsFor` and
`mapMessages`. Collect `AppliedRules` after metadata and messages have used
the same redactor; do not create separate rule logs or redaction passes in
the renderers/backend.

### Preview and artifact consumers

`Service.PreviewSnapshot` and `Service.CreateShare` already use `BuildSnapshot`.
`GistBackend.Upload` already derives all three files and the description from
the snapshot. These call paths need regression evidence, not production
changes. Keep authorization and workspace credential selection intact.

| Boundary | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| Authorized service preview | Redacted metadata and body, no provider call | Real `PreviewSnapshot` with existing authorizer stub; retain existing denied-access tests. |
| GitHub Gist, workspace-scoped upload | Redacted JSON, HTML, README, and title-derived description | Real `NewGistBackend` with `github.MockClient`; no network. Existing provider errors remain unchanged. |
| Other share providers | No new provider support | Existing snapshot contract only; unknown backend behavior is unchanged. |

### Documentation and mobile assessment

After implementation, amend only the redaction paragraph in
`docs/public/sessions-and-review.md` (a how-to guide) to name task title/session
metadata and the frozen historical-artifact limit. Preserve its preview advice
and existing heuristic-redaction limitations. The feature-status row already
accurately describes heuristic redaction and needs no change. Searches of the
root README and screenshot catalog found no affected redaction promise.

Mobile parity was assessed: the backend emits the same redacted artifact data
for desktop and phones. There is no layout, touch, navigation, scroll, or
breakpoint change. No ASCII UI preview, new copy catalog, screenshot, or mobile
Playwright test is required for this bounded construction change.

## Tests

The independently authored `apps/backend/internal/task/share/metadata_redaction_test.go`
uses existing `stubReader`, `allowShareAccess`, and `github.MockClient` patterns.

| Acceptance criteria | Test and observable evidence |
| --- | --- |
| AC001.6, AC001.7, AC001.8 of REQ-AUTH-PUBLIC-SHARE-LINKS-001 | `TestShareMetadataRedaction_GistArtifacts`: real builder then mock upload; task-title secret and path cases, plus each exported session string with secret/path values. Assert absence in all three files and the description, expected sanitized values, and metadata-only rule recording. |
| AC001.6, AC001.7, AC001.8 | `TestShareMetadataRedaction_Preview`: real service preview returns sanitized metadata and the rule union without upload/persistence. |
| AC001.6, AC001.7, AC001.8 | `TestShareMetadataRedaction_Controls`: ordinary title/session/body values, empty or non-string metadata, identifiers, version and timestamps, multiple session roots, existing body secret/tool-argument redaction, sorted distinct rule union, and deep source nonmutation. |

Use harmless synthetic credential-shaped strings and synthetic roots under a
test-owned temporary directory. Cover the existing five secret shapes without
changing their catalog. Metadata-only cases have clean body content so a body
match cannot hide a missing metadata rule. Add a combined metadata/body case
for union/deduplication. Assert expected replacement text and preserved ordinary
content as well as absence, so empty artifacts cannot pass.

## E2E evidence

The user-visible outcome is verified through `BuildSnapshot` to the actual
gist-file/description builders and existing in-memory provider mock, plus the
real service preview. A browser test cannot strengthen this viewport-independent
construction boundary; no browser or live publication is planned. Existing
service/provider authorization tests run in the complete affected Go package.

## Work orders

- [x] [Task 01: Redact share metadata](task-01-redact-share-metadata.md)

## Verification results

Implementation and local work-order verification are complete. The single order
was closed before first publication and closed again after the required-CI
fixture correction, before its fixup publication. Commit hooks, remote CI/review,
and eventual merge outcomes are tracked externally; they are not claimed
complete here.

Implementation validation on 2026-10-13, sequential under the exclusive heavy
lease, with Node 24.18.0 and existing Go 1.26.0 from the mise installation:

- Frozen workspace install ran once from `apps/`: native handle 61007 joined
  exit 0; workspace pnpm 9.15.9, 937 packages reused, no lockfile change.
- Targeted regression command in the work order: RED native 68444 joined exit 1,
  package 0.098s, with causal metadata/artifact/rule-log failures. GREEN native
  37874 joined exit 0, package 1.103s, after the minimal builder correction.
- Complete `./internal/task/share` race/trimpath/fts5 package command: native
  59350 joined exit 0, package 1.773s. All Go commands used `GOTOOLCHAIN=local`,
  `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`, `-p 1`, and the bounded work-order timeout.
- Both affected Go files were staged before the canonical
  `bash scripts/lint-go-changed` invocation. Native 69649 joined exit 0:
  `0 issues.` The command checked the complete affected package.
- `python3 scripts/list-docs.py validate`: passed (380 decisions, 1580 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node scripts/validate-public-docs.mjs`: passed (48 published pages).
- Offline actual `validateCoverage` over the seven owned implementation paths:
  `covered`, `ok: true`, `errors: []`.
- `git diff --check`, `git diff --cached --check`, and plan-path status: passed;
  both new plan files were present. Final staging/diff checks are retained in
  the external publication receipt.

Logs and native receipts are retained under the task-owned temporary directory
`/tmp/kandev-child121-share-metadata-20261013-AVcjTX`. A preliminary `gofmt`
command used an incorrect relative path; its error did not prevent the RED Go
command from compiling and reaching the causal assertions. Formatting was
corrected from the repository root before GREEN. No share production code changed after the
passing GREEN/package checks. No real data or network gist publication was used.
The public how-to redaction paragraph was updated; no UI or mobile composition
changed.

### Required backend CI fixture correction

The initial published head's `Backend Tests (2/2)` job 115315316321, run
38402986234 attempt 1, failed only
`internal/ssh.TestProbeReachability_CoalescesConcurrentProbes` with
`dialCount = 2, want 1`. The full raw job log and shard-2 test artifact are
retained in the task evidence directory. An initial log fetch was rejected by
the CLI because of terminal escape sequences; the explicit raw-to-file fetch
succeeded. This was not a network or product failure.

The actual tested merge `4a7bc486eb93ccde9db21c415f334dcdc772eae4`, its base,
the published head, and current main `a02db609519e97c0d622bad7657c390feee0fc24`
have identical implicated fixture, handler, and poller blobs. The fixture
incremented its entry counter before joining `singleflight.Group.Do`; its test
therefore could release the first dial before the second caller registered.

Under ROOT's scoped lease, one deterministic test-only overlay paused the
second caller immediately after that marker, completed the first dial, then
released the second. RED native 85295 joined exit 1, package 0.067s, with the
same two-dial assertion. The minimal permanent fixture correction uses
`DoChan` to register synchronously before signaling entry and then waits for
the result. No runtime, handler, poller, expectation, sleep, retry, or dependency
changed. The witness source is temporary evidence, not a production test seam.

- Identical scheduling witness GREEN: native 87107 joined exit 0, 1.068s.
- Original coalescing test GREEN: native 72361 joined exit 0, 1.066s.
- Complete affected `./internal/ssh` race/trimpath/fts5 package once: native
  43906 joined exit 0, 5.558s.
- Staged canonical `bash scripts/lint-go-changed` for the changed SSH test file:
  native 39735 joined exit 0, `0 issues.`

Commands retain Go 1.26.0/local toolchain, `GOMAXPROCS=2`, `GOMEMLIMIT=512MiB`,
`-p 1`, and bounded timeouts. Passing share/frontend/full-backend suites were
not replayed. Hosted results remain pending; the original observer and its
deadline continue across the fixup head without a new timer or budget reset.

Design validation on 2026-10-13:

- `python3 scripts/list-docs.py specs --system auth --format paths`: the existing
  owning requirement and design are discoverable.
- `python3 scripts/list-docs.py validate`: passed (380 decisions, 1580 specs).
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs/auth docs/plans/public-share-metadata-redaction`:
  passed; status confirms both new plan files are present and untracked.
- The work order's offline `validateCoverage` invocation over prospective owned
  implementation paths: `covered`, `ok: true`, `errors: []`. This proves design
  reference coverage only, not completed implementation or a hosted PR gate.

At that design checkpoint, all four artifacts were unstaged/uncommitted, no
other files had changed, and the global heavy-command lease remained free.

ROOT's design review accepted the bounded package and requested removal of
unchanged validator unit-test commands from the implementation order. Those
two commands were removed; the completed 36-test design receipt above is
retained. No validator unit tests were rerun for this command-only correction.
Actual catalog, specification, public-doc, coverage, and diff gates remain in
the order. ROOT subsequently issued the separate implementation release.

The completed [share authorization package](../share-authorization-boundary/plan.md)
owns separate access-control evidence. Its scope explicitly excluded snapshot
redaction; it requires no status, test-matrix, or results change for this repair.

## Risks

- Redacting after rendering or collecting the rule log would leave an exposed
  consumer or an incomplete log; construction must remain the single boundary.
- Whole-object mutation or a generic scrub could alter identifiers, source maps,
  or timestamps; explicit export-field projection avoids that scope change.
- Existing heuristics do not detect arbitrary proprietary text or credentials.
  Existing published artifacts remain frozen and receive no automatic repair.
- Synthetic tests must traverse the builder rather than constructing an already
  sanitized snapshot, and must verify every file and the description.
