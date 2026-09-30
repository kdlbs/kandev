---
status: draft
system: agents
created: 2026-09-29
requirements:
  - REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001
---

# Explicit Resume Settings Recovery

## Mapping and evidence

REQ-AGENTS-EXPLICIT-RESUME-SETTINGS-001 maps to admission, settings application, identity, and
presentation below. The implementation and evidence are recorded in the linked plan and work
orders.

For task `781ecde0-0e25-4c5f-8f45-f96cfdd90d53`, retained backend logs on
2026-09-29 at 09:44:00 +0100 show an Auggie resume failing because requested
permission mode `default` was unconfirmed; the stored provider token was retained.
The logs establish the failure boundary, not Auggie's exact ACP acknowledgment
semantics. No protocol relaxation depends on that unverified hypothesis.

## Admission and request ownership

Extend the existing `session.recover` request with optional
`settings_policy: provider_restored`; omission retains strict ordinary behavior.
Only the existing Resume recovery control for a failed Auggie session sends it.
`wsRecoverSession` validates the value and `Service.RecoverSession` verifies
session/task authorization, actual resolved provider identity, failed recovery
eligibility, and existing resume identity. Reject the option for other recovery
actions, active sessions, and unsupported providers. Legacy clients omitting the
field retain ordinary enforcement. Treat provider-neutral transport of the value
as infrastructure, not permission to enable other providers.

Carry a typed, zero-value-strict attempt policy through `LaunchSessionRequest`,
`executor.ResumeOptions`, launch assembly, and lifecycle initialization. Do not
use `StartModelPolicy.AutoFallback`, presence of a resume token, UI error text,
or generic `IntentResume` as authority. Validate under the existing per-session
resume/recovery admission boundary; retain attempt ownership through asynchronous
startup. An automatic caller never sets this policy. Existing failed-start
recovery guards, cancellation, and expected-state transitions remain effective.

## Settings application

For ordinary Auggie task start and resume, construct an exact effective model
policy for the attempt without modifying the profile. Preserve current winning
session/workflow override precedence. Require the executor-advertised model and
successful application; existing automatic/explicit/variation fallback cannot
rescue a failed selected model on these scoped paths. Other providers, Office,
and unrelated model-policy consumers retain their current policy.

For explicit recovery, omit both selections before applying startup layers:
profile model/mode, persisted runtime model/mode, workflow startup overrides,
and equivalent model/mode config options. Clear the attempt's model policy input
as well, so it cannot reapply the profile model. Audit creation/load metadata,
provider-specific command construction, and rebind paths for alternate carriers.
Use live option category/identity normalization rather than only literal `mode`
and `model` keys; with ambiguous option metadata omit the ambiguous optional
startup config entry and report the omission rather than risk reapplying a
selection. Preserve unrelated settings whose identity is known.

Keep saved profile and session selections intact. Do not persist the recovery
policy as a default. Do not replay omitted layers on the first subsequent prompt
in this initialized execution. A later explicit selector change uses normal
SetModel/SetMode validation; a later independent launch/resume uses strict policy.
Do not modify `awaitModeSettle`, legacy acknowledgment interpretation, or the
meaning of `ModeResult.Confirmed` to make recovery succeed.

## Identity, readiness, and failures

Resume/load uses the same provider identity and normal workspace. If the provider
cannot restore it, preserve the token and fail; no session/new fallback or token
clearing is authorized. Mark initialized only after normal initialization and
remaining required settings succeed. Never mark ready solely because overrides
were omitted. Existing locks and execution/attempt IDs fence cancellation,
duplicate clicks, and delayed terminal events from older executions.

Attempt IDs stored in selector snapshots and success-notice deduplication keys
must not be a process-local counter that restarts at the same value. Seed the
existing numeric identity range from a UUID and increment it within that
process. Keep this attempt identity separate from settings source ordering: a
process restart preserves the recovery attempt ID but starts a new host source
generation. Persist the source execution ID and source generation alongside
the selector snapshot. Within one source execution, reject lower source
generations; a higher generation replaces the effective snapshot. A report
from another execution is accepted only when the current executor ownership
for the session identifies that execution. Epoch-less legacy reports may merge
with legacy snapshots, but cannot overwrite a snapshot with a known source
epoch.

When lifecycle reconstructs an adopted execution, it reserves and persists the
next source generation in task-session metadata while holding recovery
ownership, before tracking, publishing, or reconnecting the execution. If the
durable reservation cannot be written, recovery stops instead of reconnecting
with an epoch that could be rejected as stale. A matching execution keeps its
effective selector values while resetting adapter-local sequence counters; a
different current execution starts with an unknown effective projection and
does not change saved launch selections or the native conversation token.
Reconstruction restores provider-report provenance only. Startup authority
remains strict, and each independent start synchronizes its report projection
from the accepted startup policy before callbacks are wired.
An adopted execution with no task session has no selector metadata to fence, so
it keeps the legacy zero source generation and strict report projection.

The ACP adapter's settings-generation counter orders reports only within one
host source execution and generation. Keep it monotonic across session load
and reset transitions handled by that adapter. A process restart can reset the
adapter counter because the host advances the source generation before
replacement callbacks are wired. A different attempt replaces the effective
selector snapshot, including when a legacy event has an empty attempt ID.

## Presentation, persistence, and observability

The shared recovery request helper and recovery view model expose the policy
only for eligible failed Auggie sessions. All recovery surfaces (bootstrap card,
stopped banner, and run error entry, including task preview/Quick Chat consumers)
use that shared decision. The server independently enforces eligibility.

Before Resume, show localized helper copy explaining that this retry keeps the
conversation and skips mode/model overrides. On success, persist a structured
session status notice using the existing session-message path and an attempt-ID
deduplication key. Failed writes remain retryable, and stale attempts cannot
publish success for a replacement execution. Record the policy and skipped
selection sources as bounded structured metadata; never include credentials or
raw ACP frames. Provider reports populate effective selectors; do not overwrite
the saved requested values or present them as effective when unknown.

Keep effective mode/model state in the existing persisted ACP selector snapshot.
The profile snapshot, session mode, runtime configuration and overrides, original
configuration, and configuration baseline remain inputs to ordinary launches.
A new recovery attempt clears the previous effective projection even when the
provider reports no settings. Partial reports within that attempt preserve the
other independently reported selector.

Stamp recovery provenance at host-owned event producers: provider load reports,
unsolicited reports from the restored execution, startup configuration RPC
outcomes, and lifecycle-generated startup snapshots. Later explicit selector
RPC outcomes retain ordinary persistence. Do not infer event provenance from
current readiness or an admission registry that may already have released its
attempt. Serialize mode and model snapshot read/merge/write and publication per
session so concurrent reports preserve both independent selector values. Host
source execution/generation and adapter-local settings generation reject old
reports before persistence and broadcast. Existing session and
execution/attempt ownership guards reject reports from superseded sessions and
executions.
Live and replay selector projections explicitly identify strict or restored
state. The frontend must not infer that distinction from `STARTING` or other
session-state events, which can arrive independently of selector updates.

Reuse `SessionRecoveryFeedback`, `SessionBootstrapRecoveryCard`, and
`RecoveryActions`. The phone surface stays inline in the task chat's existing
scroll owner because this is a short recovery action, not a navigation or
configuration workflow. Actions stack on phones and wrap in a desktop row;
pending state disables all equivalent controls. Preserve current dynamic viewport
and safe-area behavior. Phone/coarse-pointer hit targets are at least 44px;
ordinary fine-pointer desktop actions retain 28px sizing. Model/mode changes
continue through the existing `MobilePickerSheet` selectors. Localize copy in
en, pseudo, pt-pt, zh-cn, zh-hk, zh-tw, and ja; generate the Traditional Chinese
pair with the repository script.

## Compatibility and tests

| Path | Policy | Evidence |
| --- | --- | --- |
| Auggie start/automatic or ordinary resume | Enforce selected settings | Executor/lifecycle integration tests |
| Failed Auggie + explicit recovery Resume | Omit both settings, restore identity | Recovery-to-adapter request assertions plus desktop/mobile E2E |
| Auggie explicit selector after recovery | Normal advertised selection and confirmation | Lifecycle and rendered selector tests |
| Other providers, Office, fresh start, branch/runtime recovery | No recovery exception | Negative admission tests and existing policy regressions |
| Legacy client without policy | Ordinary enforcement | Handler compatibility test |

Use mock-provider evidence for request routing, not claims about real Auggie
wire behavior. A disposable real-Auggie smoke check may supplement tests when
credentials are available; never retry the user's live task as a test.

## Related contracts

- [Decision](../../../decisions/2026-09-29-explicit-resume-settings.md)
- [Permission integrity](agent-permission-control-integrity.md)
- [Resume identity](agent-resume-runtime-recovery.md)
- [Model policy](no-silent-model-fallback-01.md)
