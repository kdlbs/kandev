---
created: 2026-10-06
status: complete
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery-03.md
legacy_specs: []
---

# Implementation Plan: Provider Diagnostic Message Continuity

## Overview

Preserve one assistant message when ordinary prose contains a provider-error
signature, while retaining per-event recovery evidence. Platform owns this
repair because its active provider-recovery contract controls diagnostic
admission and runtime output delivery. Reuse the existing requirement,
especially acceptance criteria `.20`, `.21`, `.16` and `.23`; no new product
requirement or classifier policy is needed.

The lifecycle-to-orchestrator separation is implemented as one atomic backend
slice, and the real ACP, persistence and rendered chat path is proven on desktop
and phone. The existing design package defines the contract for this work.

## Evidence and assumption check

- The supplied investigation identifies task
  `69b1f974-3b49-4792-ad2f-8365afb4b355` and session
  `071361f2-1936-4bc7-a268-97f698bbe4f0`. Its raw trace and SQLite rows were
  supplied as findings, not independently inspected in this checkout.
- Historical baseline source confirmed that `convertMessageChunkWithProtocolID`
  marked assistant chunks using the shared classifier. The classifier accepted
  `i/o timeout`; `flushMessageBufferOnDiagnosticChange` then cleared
  `currentMessageID` both before and after publishing the prior segment.
- Historical temporary real-lifecycle test named
  `TestReproDiagnosticMarkerKeepsAssistantMessageIdentity` fed four chunks:
  `934 ` followed by an opening backtick and `dial tcp`, ` <ip>:6379:`,
  ` i/o timeout` followed by a closing backtick and ` err`, and
  `ors, meaning the TCP connect failed.`. Markers were false, false, true,
  false. It failed with **three message IDs instead of one**, with exact
  concatenated content preserved. It was removed after reproduction.
- Historical reproduction command for the removed temporary test:
  `(cd apps/backend && go test -tags fts5
  ./internal/agent/runtime/lifecycle -run
  '^TestReproDiagnosticMarkerKeepsAssistantMessageIdentity$' -count=1 -v)`.
- Historical baseline: `TestHandleMessageChunkEvent_LegacyBufferSplitsOnDiagnosticChange`
  asserted the accidental split. The implementation removed that contract and
  replaced it with the permanent identity regression below.
- Intended outcome: diagnostic candidates remain recovery evidence, and never
  become message boundaries. Repair only future output. Historical row repair
  is excluded because rows alone do not prove which splits were accidental.
- The active criterion `.20` is authoritative over the current buffer-aware
  implementation. Restore that boundary rather than narrowing a network rule
  or weakening recovery's matching-error and effect fences.

## Scope

### In scope

- Original, unbuffered assistant and reasoning observations for recovery and
  foreground progress, under the existing execution/prompt guards.
- Marker-independent transcript accumulation, coalescing and identity.
- Genuine-diagnostic, mixed-output, effect, stale-identity and terminal-ordering
  regressions, plus desktop/mobile live and reloaded transcript evidence.

### Out of scope

- Historical database edits, automatic row merging, markdown parser changes,
  new transcript metadata or schema, classifier signatures, retry budgets,
  flag rollout, new providers, and runtime restart of the user's instance.
- Frontend composition or copy changes. Existing task-chat surfaces render
  the corrected persisted record; no translation catalog changes are needed.
- Changes to the completed provider-error-policies and dynamic-routing packages.
  This repair references their shared contract without reopening their scope.

## Technical approach

### Original-event evidence

In `manager_events.go`, publish eligible `message_chunk` and `reasoning`
evidence copies through `EventPublisher.PublishAgentStreamEvent` before
buffering. Retain role, candidate, original text and immutable correlation
identity. Fill only missing prompt generation at intake. Reasoning copies
carry `ReasoningText` as `Text`. These backend-only copies have no message ID
and use the existing session agent-stream carrier and watcher.

In `event_handlers_streaming.go`, observe these original event types under the
existing guards, update foreground progress only for ordinary output, and
return without transcript writes or browser broadcasts. Tools retain their
existing effect observation. Keep lifecycle's terminal evidence snapshot and
the terminal-ordering reconciliation for asynchronous event-bus delivery.

### Transcript projection

Remove `flushMessageBufferOnDiagnosticChange`, `messageBufferDiagnostic`, and
marker arguments/state throughout `manager_streaming.go`, `types.go` and
`stream_coalescer.go`. Stop observing recovery/progress from visible
`message_streaming` and `thinking_streaming` events: those projections can
combine differently marked original chunks. Preserve all existing tool,
completion, response-attempt-reset and explicit protocol-ID boundaries.

| Provider / transport | Identity shape | Intended result | Evidence / unsupported shape |
| --- | --- | --- | --- |
| Cursor ACP, ordinary message chunks | No protocol message ID | One assistant record across marker changes | Four-chunk lifecycle regression and inline ACP E2E; no Cursor-specific branch |
| Shared ACP assistant output | Explicit protocol message ID | Existing stable record mapping across marker changes | Protocol-ID lifecycle table; a changed ID retains its real boundary |
| Claude / gateway diagnostic | Current execution and prompt generation | Visible diagnostic plus existing matched-error recovery fence | Existing transport replay matrix and orchestrator correlation tests |
| Native and other normalized output | With or without a protocol ID | Ordinary output observation and established transcript boundaries | Lifecycle table includes absent marker; no new provider capability claim |
| Old/unmarked agentctl output | Candidate absent | Ordinary output, including error-like prose | Preserve `.20`'s fail-closed direction; do not reclassify downstream |
| Stale, cancelled or superseded execution | Wrong attempt or generation | Existing guards reject evidence side effects | Terminal-ordering and stale-identity tests |

### Documentation and compatibility

The requirement remains active and unchanged. Update its part-3 design with
the separation, and record execution results in this package. No new ADR is
needed: this restores the already documented evidence/transcript boundary.
Public commands, configuration, terminology and recovery policy do not change,
so no public documentation edit is required.

## ASCII UI preview

UI-01: Existing task chat at `/t/<task-id>`, after the assistant finishes.
Content is illustrative; the single message record and complete code span are
required by `.20`. Existing chat owns scrolling; existing header, composer,
mobile safe-area handling and actions keep their shipped behavior.

```text
Current defect (desktop and phone):
[Assistant] 934 `dial tcp <ip>:6379:
[Assistant]  i/o timeout` err
[Assistant] ors, meaning the TCP connect failed.

Corrected desktop:
[Assistant]
934 <code>dial tcp <ip>:6379: i/o timeout</code> errors,
meaning the TCP connect failed.
[existing message actions]

Corrected phone (existing dedicated Chat surface):
[Assistant]
934 <code>dial tcp <ip>:6379:
i/o timeout</code> errors,
meaning the TCP connect failed.
[existing message actions]
```

The phone still has one card and both complete inline code elements; wrapping
uses existing markdown styles. The nearest exemplar is
`mobile-markdown-separators.spec.ts`, which proves direct task navigation,
rendering, reload and document containment without introducing a new surface.
The drawing specifies structure, not exact line breaks or pixels.

## Tests

| Acceptance | Required test and evidence |
| --- | --- |
| `.20` | `provider_diagnostic_transition_test.go`: new `TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity`, with ID-less/protocol-ID, newline and repeated-marker cases; exact joined text, one create and stable append IDs |
| `.20`, `.21` | `provider_diagnostic_accumulation_test.go`: original-event markers/text/order survive exactly; transcript projection carries no candidate and does not generate duplicate evidence |
| `.16`, `.21`, `.23` | `event_handlers_streaming_provider_diagnostic_test.go`: raw marked diagnostic followed by matching terminal error, ordinary assistant/thought clearing, earlier output and tool effects; transcript-only projection does not clear a genuine diagnostic |
| `.16`, `.20`, `.23` | Existing `dynamic_evidence_diagnostic_correlation_test.go` and `replay_fixture_evidence_test.go`: containment, first diagnostic retained, mismatched and stale failures remain unsafe |
| `.16`, `.20`, `.21` | `dynamic_evidence_terminal_ordering_test.go`: delayed bus consumption and immutable terminal evidence remain safe; no assumption of synchronous publication |
| `.20` | `stream_coalescer_test.go` and existing lifecycle streaming tests: preserve order and prompt/attempt/message correlation keys after removing diagnostic key |
| `.17` to `.19` | Existing ACP `replay_fixture_test.go` and `replay_fixture_queued_test.go`: original classifier and prompt drain remain unchanged |

## E2E tests

Add desktop `tests/chat/provider-diagnostic-continuity.spec.ts` and phone
`tests/chat/mobile-provider-diagnostic-continuity.spec.ts`, with a shared
`provider-diagnostic-continuity-helpers.ts`. Use existing multiline
`e2e:message(...)` commands to emit exact separate ACP chunks; do not seed an
already combined assistant row. Create a disposable task, open its route
directly, and check the completed message and API storage before and after
reload. Assert one relevant assistant row/card, both full inline code elements,
intact `errors`, and absence of a retry notice on this successful turn. Phone
also checks document containment. This proves `.20` and the visible part of
`.21`; backend tests prove actual recovery admission under `.16` and `.23`.

## Work orders

- [x] [Task 01: Separate output evidence from transcript buffering](task-01-separate-evidence-and-transcript.md)
- [x] [Task 02: Prove desktop and phone continuity through ACP](task-02-prove-chat-continuity.md)

Both run sequentially. Task 02 depends on Task 01. Each work order owns its
exact commands; no extra broad audit is scheduled.

## Verification results

The permanent test `TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity`
first failed before production changes with three IDs for one assistant
message and exact joined text. It now passes with one stable identity, while
protocol IDs, newline flushes, and real tool boundaries retain their own
identity behavior.

The named regression passed on the host with task-owned `GOCACHE`. The exact
three-package race suite passed in a task-owned writable Linux container using
the Go 1.26 CI builder image, a byte-verified no-local checkout, a private Git
config, and `TMPDIR=/tmp`:

- `(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestHandleMessageChunkEvent_DiagnosticMarkerKeepsMessageIdentity$' -count=1 -v)` with `GOCACHE=/private/tmp/kandev-provider-diagnostic-go-cache`: passed.
- `(cd apps/backend && go test -race -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/agentctl/server/adapter/transport/acp -count=1)`: passed; lifecycle 219.463s, orchestrator 105.202s, ACP transport 21.881s.

Initial full race attempts found lifecycle fixtures that still expected
transcript-only events. They were updated to assert original evidence alongside
transcript projections and to check the reset's retracted message identities.
The remaining runner attempts exposed environment constraints: the macOS
sandbox blocked writes to the shared Git config, represented its temporary
root through a noncanonical `/var` path, and restricted process inspection and
`/dev/fd/3` in retained-worktree tests. The first Linux attempt used a read-only
worktree mount with an external host `.git` pointer, so Git fixture tests could
not access or update checkout metadata; its nested helper build also exceeded
its two-minute timeout while compiling cold. The final Linux run used a
writable, self-contained clone and warmed the nested helper build. It ran the
unfiltered required package command to pass; no tests were excluded or
suppressed by the command.

Desktop and phone E2E passed sequentially after Prettier and focused ESLint:

- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/provider-diagnostic-continuity.spec.ts)`: passed, 26.2s.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-provider-diagnostic-continuity.spec.ts)`: passed, 29.9s.

Design-package validation on 2026-10-06:

- `python3 scripts/list-docs.py validate`: passed, 355 decisions and 1394
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Historical pre-implementation `.github/scripts/pr-docs.cjs`
  `validateCoverage` preflight: `covered`, both work orders accepted with no
  errors. It included the planned `manager_events.go` path as a synthetic
  trigger to validate REQ/AC/design/plan linkage before production changes.
- Final checks after implementation and status edits: `python3
  scripts/list-docs.py validate` passed (355 decisions and 1394 specifications);
  `python3 scripts/lint-spec-files.py --all` passed; `gofmt -l` reported no
  changed Go files; `git diff --check` passed; focused Prettier and ESLint
  checks passed for all three new E2E files.

## Risks

- Observing both raw evidence and transcript projections would count output
  twice and clear a real diagnostic. Switch producers and consumers atomically.
- The additional uncoalesced evidence traffic stays backend-only. Keep it free
  of persistence, generic text logs and browser broadcasts; retain streaming
  teardown and correlation-key tests.
- Async terminal delivery can precede stream consumption. Preserve immutable
  terminal snapshots and prove this ordering, rather than relying on the
  synchronous tracking test bus.
- Historical broken records remain broken. No data repair is authorized by this
  package; future response correctness is independently verifiable.
