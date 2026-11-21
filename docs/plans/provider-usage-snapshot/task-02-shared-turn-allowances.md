---
id: "02-shared-turn-allowances"
title: "Admit configured shared Kandev-turn allowances before prompt dispatch"
status: done
wave: 2
depends_on: ["01-native-snapshot"]
plan: plan.md
requirements:
  - REQ-COSTS-PROVIDER-TURN-ALLOWANCE-002
acceptance_criteria:
  - AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.1
  - AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.2
  - AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.3
  - AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.4
  - AC-COSTS-PROVIDER-TURN-ALLOWANCE-002.5
system_design:
  - ../../specs/costs/system-design/provider-usage-snapshot.md
legacy_specs: []
---

# Shared native turn allowances

Implement the linked allowance increment on the native snapshot base. Reuse
internal host binding, native feature gating, task repository transaction entry
and lifecycle delivery identity. Expose backend GET/PATCH configuration with the
existing organization configuration permission. Preserve owner/learned JSON,
history and the off path. Keep grant/capacity/grade work outside this atom.

Sequence: policy/claim persistence; narrow settings/domain binding; common prompt
admission and completion/stop/refund; disposable SQLite and transport evidence;
schema/SQL guard and changed-package validation. No permanent tests or live
provider/model calls. Integration alone owns Git references and operating changes.

The source input is immutable `1cabfe1ddc9fa4adb37aa607cbb4a7630d7dd418` for
owner/learned separation and optional limits. Its post-response stream counts
are not claims. The existing strict snapshot base is the binding/cache input.

## Original remaining work

Provider HTTP request counts, token/USD estimates and exact monetary admission,
model/group scopes, other/remote account adapters, natural-error learning and
the reset/learned-value editor remain original T1b scope. Turn allowance APIs
must identify their unit and unknown provider spend explicitly. This atom does
not rename a turn as one provider request or complete those remaining requirements.
Utility/review inference uses the separate agentctl `InferencePrompt` child/HTTP
path and is not enforced by this normal-task/Office prompt boundary. The C2
child-owner integration must attach that consumer to the same provider claim
seam after releasing the grade mutex; this atom does not claim utility/review
coverage or expand into that separate writer's scope.

## Implemented boundary

`GET/PATCH /api/v1/agent-profiles/:id/turn-allowance` configures optional
`concurrent_turns`, `turns_per_window`, `window_seconds`, `reset_anchor` and
`block_until` for the profile's verified shared local credential. PATCH reuses
native organization configuration permission and settings interlock. Responses
identify `unit: kandev_turn`, observation binding scope and
`provider_spend_state: unknown`; they expose no credential/cache key or path.
Stored owner patches retain omitted/unknown fields and separate learned JSON.

The task repository owns `provider_turn_allowances` and `provider_turn_claims`.
SQLite immediate writer admission and PostgreSQL account-row locking serialize
policy and count/claim. The shared runtime boundary admits before agentctl I/O.
Submission/execution-generation reuse is idempotent. Only a fresh acquisition
owns a confirmed-unsent refund: an unsent steer or a retry after an ambiguous
delivery cannot refund its predecessor. A validated completion/confirmed stop
ends concurrency while retaining period charge. Ambiguous delivery and unknown
terminal evidence retain the claim; neither timeout nor backend detach invents
a refund. Explicit zero disables that owner dimension; expiry ends a timestamp
block without interpreting unknown usage as zero.

Manager snapshots include active incumbents from before the flag/policy was
enabled, unioned with durable claims instead of counted twice. Native active
incumbents are not invented as new charged period turns. An unmetered generation
marker also covers policy creation while an earlier turn waits for its first
acknowledgment. Current binding/generation uncertainty closes configured
concurrency. The policy/admission mutex orders this marker with settings changes;
it is not the grade mutex, and no allowance SQL transaction calls grade code.
Unrelated secret environment references do not make credential binding unknown.

No provider usage request or model execution was made during implementation.
The existing Office cache, strict provider observations, model order and history
were retained. Native configuration endpoints close before work when off and
dispatch enforcement is only wired at enabled startup. Stored data remains.

## Settlement while admission is disabled

The task repository supplies the settlement-only dependency independently of
new admission. Composition installs it before native runtime recovery. Disabling
settings and new claims does not discard existing claims or block their verified
completion/confirmed-stop settlement. Only the matching execution and generation
settle; ambiguous, late or foreign evidence stays conservative. Concurrency is
returned without refunding a completed period turn. A survivable detach does not
settle a still-running execution. Settlement failure after successful stop is
logged without preventing the existing runtime cleanup.

## Verification and source

The source lineage is c7890462d followed by the disabled-settlement correction
d5525cef9. Use existing provider/flag tests and disposable real SQLite/native
mock-agentctl checks for shared accounts, idempotence, unknown bindings, unsent
refunds, completion and disabled settlement. No permanent test or operating
activation is introduced. The later inference-child population hook belongs to
the separately preserved inference integration and is not imported here.


## Local delivery failure settlement follow-up

A locally synthesized initial-prompt error no longer proves remote completion.
Existing error/UI handling stays intact while provider concurrency is retained.
The trusted remote completion path also accepts matching settlement after that
local finalization without publishing another terminal UI event. Confirmed stop
and the existing confirmed-unsent refund retain their own boundaries.

Disposable actual WebSocket receipt-without-acknowledgment plus real SQLite
concurrent-turn policy reproduced early claim settlement and a second task
starting at a limit of one. The candidate passed both remote-completion and
confirmed-stop variants under the race detector. The stale provider-read issue
is covered by the native snapshot work order; no live provider or operating
configuration is changed.


## Startup and stale-cleanup follow-up

Two later review findings concern the original enabled startup ordering and a
missing verified-stop settlement call. The usage/admission block now precedes
all startup consumers, and cleanup settles only after actual backend stop or
retired-owner termination proof. Caller cancellation cannot cancel the bounded
settlement. This does not infer death from a restart or a missing execution,
settle graceful detach, expire claims, or gate runtime cleanup on SQL success.
Source-order inspection confirmed the original assignment followed startup
consumers and the candidate precedes all five relevant startup sites. After a
fixture-only correction to the existing StopAgentWithReason API, the actual
SQLite baseline reproduced retained claims after confirmed stop and cancellation
after stop. The candidate passed the selected race checks (lifecycle 2.453s),
including unconfirmed/unknown runtime and survivable-detach controls plus eight
existing cleanup/stop cases. Full-PR documentation coverage passed against the
actual PR merge base. Normal hooks must pass before publication; no live provider,
new standing tests, expiry policy or operating mutation is introduced.
