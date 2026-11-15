---
status: current
system: agents
created: 2026-11-07
updated: 2026-11-07
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004
owners:
  - Kandev
---

# Startup process diagnostics

## Purpose and boundaries

Agents own subprocess evidence and the explanation of a failed startup.
Tasks retain durable error storage, attempt admission, and atomic failure settlement.
This design extends the existing recovery contracts across agentctl, runtime,
orchestration, and the shared recovery card.

The [session recovery design](session-recovery-failures.md) owns recovery eligibility.
The [startup explanation design](session-startup-failure-explanations.md) owns
correlation and presentation. The [managed npm design](managed-npm-runtime-recovery.md)
owns setup retry. This extension preserves those boundaries.

The [delivery package](../../../plans/agent-startup-process-diagnostics/plan.md)
contains reproduction evidence and implementation work orders.

## Requirement mapping

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006 | .2, .13, .24-.26, .28-.29, .34, .36 | Evidence, safe projection, recovery presentation |
| REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007 | .1, .2, .5 | Classification, attempt isolation |
| REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-004 | .2-.4, .7-.8 | Retry compatibility, diagnostics |

## Evidence at the subprocess boundary

`process.Manager.readStderr` remains the only reader of the managed stderr pipe.
A new neutral package, `internal/common/startupdiag`, recognizes supported
toolchain signatures and returns fixed diagnostic markers. Both agentctl and
the backend can import this package without reversing tier dependencies.

The first supported signature is the reported mise refusal:
`mise ERROR Config files in <workspace>/mise.toml are not trusted.`
The recognizer requires the complete mise prefix and refusal suffix.
It examines the intervening path only in memory and never returns it.
ANSI handling precedes recognition. Oversized, partial, malformed, and unrelated
lines do not establish a toolchain reason.

Recognition returns `mise_config_untrusted` plus fixed English text.
It does not return the original line, an interpolated path, or executable arguments.
Other mise errors require verified fixtures before the recognizer accepts them.
This package does not claim support for arbitrary toolchain managers.

The safe projection runs before the launch-wrapper branch discards unsupported
stderr. Existing npm markers and protocol-specific stderr consumers keep their
current ordering and behavior. Unknown wrapper output remains excluded.
Known safe markers alone can enter the generic ring and exit event.
The existing read/drain lifecycle remains bounded and cannot block subprocess reaping.

## Additive evidence contract

Extend `agentctl/types.ManagedStartupEvidence` with these optional fields:

| Field | Accepted value |
| --- | --- |
| `diagnostic_reason` | `mise_config_untrusted` |
| `diagnostic_complete` | Boolean describing complete collection and retained diagnostic coverage |

Existing `process_generation`, `exit_disposition`, `exit_code`, and npm fields
retain their meaning. A complete non-npm diagnostic must never become empty npm
stderr. In particular, mise evidence must not make `NPMDiagnosticComplete` true
for the unexplained-exit retry path.

`waitForExitGeneration` records the immutable observation before publishing its
exit event. Ring eviction or reader failure clears trusted diagnostic attribution.
An observed exit status remains usable independently of diagnostic completeness.
Intentional stop and signal termination retain their distinct dispositions.

`handleWSInitialize` uses the existing bounded, cancellable evidence wait.
It sends evidence only for the requested current process generation.
`InitializeError` retains that evidence through the runtime client and typed
`SessionInitializationPhaseError`. The handshake error remains available as an
internal cause. The user-facing explanation comes from the structured evidence.
No additional wait, stderr API, or raw diagnostic response is necessary.

## Classification and retry compatibility

At the failed initialize boundary, compare evidence generation with the same
client generation captured for that initialization. Do this before a client
release or replacement can change the observation. Carry the validated result
into `BootstrapFailure`. Do not reconstruct it from later mutable runtime state.

Add the bootstrap cause `process_exited`. Its reason is either
`mise_config_untrusted` or `unclassified`. Only confirmed ordinary exits with a
nonnegative status qualify. Exit code zero remains a truthful exit before
readiness, rather than successful initialization. No synthetic status describes
a signal, intentional stop, unknown owner, or missing observation.

Cancellation, shutdown, deadlines, typed authentication failures, and existing
specific startup failures retain precedence. A closed pipe, EOF, or a quoted
mise phrase in an arbitrary RPC response does not prove a child exit.
Later session creation, load, model selection, and prompt failures cannot acquire
pre-initialize exit evidence. The original error remains available through wrapping.

The existing managed npm retry owner runs under its existing eligibility rules.
Known mise refusal is a configuration prerequisite and cannot trigger automatic
replacement, online npm retry, cache deletion, or provider fallback.
Empty stderr can still qualify for the established single retry.
For exhausted eligible retries, project the actual final child's evidence.
An earlier child's diagnostic cannot describe its replacement.

## Safe projection and persistence

Extend `BootstrapFailure` and `models.AgentErrorCause` with optional `exit_code`.
The cause code and reason use closed enums. The status is an integer in the
existing child-log nonnegative range, zero through 4294967295.
Reject non-integers, negatives, overflow, and status values on unrelated causes.
Omit malformed fields rather than copying them into detail text.

Normalization reconstructs fixed safe detail for `process_exited`.
It must not trust a caller-supplied `Detail` because the code is recognized.
Update cause equality, deduplication, and diagnostic byte accounting for status.
Preserve optional status zero distinctly from missing evidence.
The two-cause and existing detail-size bounds remain unchanged.

Carry the normalized cause through both asynchronous projections:
`handleAgentStartFailed` and executor `buildBootstrapLastAgentError`.
Use existing `LastAgentError` identity, phase, occurrence time, and stamp.
Keep `generic_launch_failure` as the outer category so action policy stays intact.
Update typed copies in DTOs, task summaries, session events, and historical messages.
Do not add a database table, migration, error store, or independent UI owner.

`waitForSessionReady` currently reconstructs an error from `session.ErrorMessage`.
At the owned recovery feedback boundary, recover the safe structured cause from
the matching `last_agent_error`, alongside its existing identity/stamp lookup.
Extend `SessionRecoveryFailure` only as needed to carry normalized causes.
The session, attempt, execution, and stamp must match before this enrichment.
Return these safe causes through the existing recovery error response details.
Never enrich a generic failure from an unrelated or historical session row.

## Distinct recovery attempts

A specific failed resume and a later failed fresh start remain different attempts.
The original cause and occurrence time stay in chronological history.
The latest unresolved failure owns current controls and its own explanation.
No unconditional first-error-wins rule replaces a newer failure with older evidence.

For projections of the same attempt, durable typed evidence takes precedence
over generic readiness, request, or transport wrappers. Existing request-local
identity and `SessionRecoveryFailure` correlation provide that association.
Workspace restoration retains its separate operation and cause.
Do not combine a later checkpoint failure with an earlier mise refusal.
The separate journal defect in issue #5129 remains outside this design.

## Recovery presentation

Extend the existing `AgentErrorCause` parser, bootstrap recovery model, mounted
`SessionRecoveryCard`, technical details, and historical adapters.
Known evidence selects localized title, summary, and prerequisite guidance.
The client does not classify raw error text or interpolate arbitrary diagnostic fields.

For mise refusal, the proposed title is `Agent startup blocked by mise`.
The summary is `mise did not trust this workspace's configuration.`
Guidance says to review the configuration on the execution host and, only if
trusted, run `mise trust` there before retry. The command is advice only.
Kandev never trusts a repository or executes a command from stderr.
No path from stderr appears in the summary, details, accessible labels, or clipboard.

For unknown ordinary exits, show `The agent exited before startup completed.`
Expanded details include the observed exit code, operation, phase, and existing
validated host references. Missing fields remain absent.
The existing safe display/copy primitive supplies the same bounded text to both destinations.

Keep the existing eligible Resume, fresh-start, and workspace controls.
Retry does not promise to remove a configuration prerequisite.
Pending state, confirmations, drafts, read-only workspace status, and successful
recovery retirement follow the current recovery owner.
Do not add a trust button, new modal, or extra active warning.

The nearest mobile exemplar is `SessionRecoveryCard` with `RecoveryActions`.
Desktop keeps a wrapping action row. Phone stacks the same actions at full width.
The cause stays above initially collapsed details on both surfaces.
Phone targets remain at least 44 pixels. The recovery region owns expanded
diagnostic scrolling, with existing dynamic viewport and safe-area behavior.
This content change does not introduce new breakpoint logic or saved layout state.
All new copy uses runtime translations in seven shipped languages and pseudo.

## Diagnostics

At process exit, emit a fixed message with bounded exit status, disposition,
generation, and recognized reason. Extend `launcher/child_log_context.go` to
validate those fields through its existing optional-field reconstruction.
Generation is a positive bounded integer. Reason and disposition are closed enums.
Duplicate or malformed fields follow existing omission rules.

Do not forward `recent_stderr`, raw `error`, paths, arbitrary maps, or
`startup_evidence` wholesale. The backend log retains a useful cause through
the fixed fields without weakening the launcher allowlist.
Log evidence does not authorize session transitions or retries.
No new metrics or identifier labels are necessary.

## Compatibility and verification

Older agentctl peers omit the new fields. Keep their existing behavior and safe
fallback. A newer peer's unknown reason degrades to the confirmed exit summary.
Older frontend clients ignore additive fields. No new action depends on them.

Real helper subprocess tests cover both event orders, wrapper behavior, status
zero, delayed drain, and replacement generations. Negative cases cover intentional
stop, signals, cancellation, and incomplete diagnostics.
Hostile-line tests prove that paths, URLs, tokens, and unmatched text cannot cross
the new projection or launcher fields.

Backend service tests cover persistence, readiness errors, distinct failure history,
same-attempt enrichment, stale attempts, and a healthy sibling alongside failure.
Browser tests trigger the real launch/resume pipeline with a private fake CLI.
They cover desktop and phone reload, safe details/copy, prerequisite guidance,
and preservation of an earlier cause after a distinct later failure.
Seeded cards supplement, but cannot replace, subprocess-to-browser evidence.

## Related decisions

- [Managed stderr diagnostics](../../../decisions/2026-08-02-agent-terminal-diagnostics-over-stderr.md)
- [Bounded managed startup retry](../../../decisions/2026-10-02-bounded-managed-npm-startup-retry.md)

This extension follows those accepted boundaries. It needs no new ADR.
