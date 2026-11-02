---
status: draft
system: agents
created: 2026-10-30
requirements:
  - REQ-AGENTS-SANDBOX-009
  - REQ-AGENTS-SANDBOX-010
---

# Sandbox evidence and reports

## Ownership and mapping

Agents owns attempt evidence and operator interpretation across storage, API, and UI. This extends [sandbox policies](sandbox-policies.md) without changing task lifecycle. REQ-AGENTS-SANDBOX-009 maps to facts, projection, and UI. REQ-AGENTS-SANDBOX-010 maps to report ingestion, verification, and learning. [Provider major 2](../../plugins/system-design/sandbox-provider-v2.md) owns transport shapes. The [decision](../../../decisions/2026-10-30-sandbox-evidence-and-operation-authority.md) records the claim and trust boundaries.

## Baseline and evidence

The inspected plugin returns registry identity in `server/status.go` and filters reports to filesystem `capability_decision` entries in `server/reports.go`. Nono 0.80.0 and 0.81.0 also define network, command-policy, and conditional sandbox-runtime audit events. The latter is emitted for command mediation, not every sandbox launch. Missing that event is not proof that filesystem confinement is absent.

Nono session records and `inspect --json` still lack the effective-policy digest required by the existing active-confinement check. Signing occurs at session finalization. No part of this extension turns these sources into continuous current-policy attestation.

## Independent facts

Extend the common `Observation` and authenticated execution projection with versioned facts, rather than replacing `StatusActive` semantics. Facts inherit the enclosing observation's attempt ID and generation. Each fact retains its own observation time, optional expiry, and evidence digest where relevant.

- `process`: running, stopped, or unknown. Only core's current ownership verification can assert running. Use captured supervisor birth identity and owned containment, not a registry PID alone.
- `launch_checks`: passed, failed, or unavailable. Record the named checks that actually completed, runtime/package/policy digests, and completion time. Configuration compilation is not a kernel check. Old snapshots have unavailable checks; never backfill from current settings.
- `runtime_observations`: typed platform, detected ABI, execute-enforcement claim, and command-mediation state. Preserve provider source and collection time. Detected kernel capability differs from a runtime assertion that enforcement was installed. A one-time runtime event remains historical.
- `audit`: independent coverage, integrity, finalization, and signer-trust results. No audit result controls current confinement status.

`internal/agent/sandboxruntime/evidence.go` retains `boundConfinementEvidence`. The existing observer supplies bounded refresh. The retained ownership handle checks process birth, ancestry, and containment independently of provider availability. `VerifyLive` retains its separate policy/provider authority checks for approvals and active confinement. Do not introduce a browser polling loop or a provider-specific kernel probe in core.

## Persistence and ordering

Add an optional versioned facts payload to the existing execution observation model and attempt-bound persistence. The existing SQLite and PostgreSQL JSON payload columns store the optional facts without new DDL. Test round trips, backend-owner replacement, and legacy decoding against both databases. Legacy rows decode to explicit unavailable facts. Never alter immutable launch snapshots or revision hashes.

Reuse session/attempt/generation/projection ordering in session reads, WebSocket handlers, and the session store. Backend-owner replacement and browser disconnect expire live facts. Historical checks and verified artifacts remain readable with their original timestamps. A new attempt starts with new live observations and cannot reuse a sibling's receipt.

Preserve prior valid facts when a refresh fails and mark them stale. Terminal state cannot be reversed by late running evidence. Provider health remains independent; core-owned Stop works without the plugin.

## Typed report ingestion

Extend `sandboxruntime/report.go`, private-source reads, and `sandbox/report_validation.go` through the major 2 contract. Admit bounded typed events, including filesystem decisions, TCP decisions, command mediation, and HTTP endpoint policy events. Each record has a source ID, immutable byte digest, sequence/offset, attempt binding, event time, decision, and a typed target.

Keep three axes separate: coverage (`supported`, `partial`, `unsupported`, `unavailable` per class), integrity (`unchecked`, `valid`, `invalid`), and finalization (`live`, `final`, `interrupted`). An intact hash chain does not prove complete collection. An empty valid report differs from a missing report.

Nono's provider parses `network`, `command_policy`, and `sandbox_runtime` variants and retains closed reason categories. Core may contribute its own egress-denial records with a distinct source. Avoid duplicate logical events by source plus sequence, never by display text alone. Correlate only explicit request IDs; do not guess associations from matching timestamps.

Retain existing 1 MiB provider message and 1000-record/256 KiB response limits. Add core-owned opaque chunk handles for larger final artifacts: core opens declared private files, freezes their identity and digest, and streams bounded contiguous chunks through `ReadSandboxReport`. The provider keeps a bounded in-memory verifier per operation. Restart discards verifier state and starts from byte zero. Cursor reuse across artifacts, truncation, replacement, or out-of-order chunks fails. A partial final line remains pending during live reads and invalid in a finalized artifact.

Raw audit input remains private. Public projections remove credentials, control URLs, unsafe argv, and redacted target fragments. Unknown variants produce an explicit unsupported coverage result, not a successful empty report. Reads cannot follow symlinks or arbitrary provider paths.

## Completed audit verification

Verification is an explicit operator action on a finalized record, with bounded asynchronous work and a persisted result. Extend the existing report service and session authorization rather than exposing an unauthenticated Nono endpoint.

Verify the recorded event chain and final root against immutable metadata. If the captured local ledger is supplied, report ledger-inclusion verification separately. If a signature bundle exists, verify its session binding and root against an operator-configured public-key fingerprint. An embedded self-selected public key can verify mathematical consistency but cannot yield trusted-signer status.

Core stores trust configuration with operator, version, and provenance. It sends only the expected public key and bounded declared artifact bytes to the provider verifier. Private keys never enter the plugin RPC or public DTO. Missing signatures remain unsigned, not invalid. Missing final metadata or interruption remains incomplete. A changed trust revision produces a separate verification record; revocation removes a current trusted-signer claim while preserving historical verification.

This phase consumes existing signed bundles. It does not enable Nono platform enrollment, remote audit delivery, or automatically provision signing keys. Those are separate deployment contracts. The implementation must validate Nono's exact DSSE encoding and signing algorithm using upstream fixtures and an independent mutation test, rather than implementing a generic arbitrary-algorithm verifier.

## Learning

Only exact, supported operation identities can produce candidates. A redacted path, missing port, ambiguous command identity, incomplete arguments, or unknown decision disables learning for that row with a reason. Human review can inspect partial evidence but cannot convert missing information into a wildcard.

Extend `learning_diff.go` and revision validation using the typed operation design. All candidate grants stay inside the captured/current authorized ceiling and task guard. Show source records and explicit added access. Publication remains an immutable revision write. Selecting that revision is a separate action.

## Desktop and phone

Reuse `SessionExecutionDetails`, `SessionExecutionBody`, `SandboxDenialReport`, and existing report hooks. Display Process, Launch checks, Runtime observations, and Audit as labelled groups before policy history. Each fact shows its own timestamp and limitation. Keep the Dockview indicator neutral without accepted active evidence. Do not add a task-wide protection claim.

Desktop uses the existing focused dialog. Phone retains its full-height `h-dvh` surface with fixed Back/title and a single internal scroller. Report details and verification use the next view in that surface, not stacked dialogs. The curated mobile task-switcher supplies header and safe-area conventions. Share filtering, authority, stale guards, and action state between presentations.

Use 28px desktop controls and at least 44px phone/coarse-pointer targets. Keep visible text alongside state icons, preserve focus return, and use the current icon library and tokens. No pulsing success shield or color-only verification state. Localize all new text in seven locales; use the existing Traditional Chinese generator. Loading, expired, unsigned, invalid, unavailable, and empty states have distinct copy. Test 393, 700, 767, and 768px layouts and narrow fine-pointer interaction.

## Acceptance and delivery

The [expansion plan](../../../plans/nono-sandbox-expansion/plan.md) defines tests and work orders. Pair malformed and stale fixtures with actual Nono audit samples. Never promote historical 0.80.0 managed acceptance to evidence for a new runtime or package.
