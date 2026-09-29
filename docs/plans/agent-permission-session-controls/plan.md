---
created: 2026-09-27
status: in_progress
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Replace permission settings overlays with session controls

## Outcome

Apply permission modes through ACP before the first prompt. Preserve Claude user
settings and authentication paths. Remove the automatic mode overlay from PR #3886.
The protocol and lifecycle changes are implemented. Real-provider acceptance
remains incomplete because no separate Claude test credential is available.

## Sources

- [Requirements](../../specs/agents/requirements/permission-control-integrity.md)
- [System design and pinned upstream evidence](../../specs/agents/system-design/agent-permission-control-integrity.md#initial-mode-delivery)
- [Decision](../../decisions/2026-09-27-session-permissions-without-settings-mutation.md)

## Delivery status

The implementation was integrated onto the latest contributor PR in a separate
checkout. This preserves the original shared checkout and subsequent contributor
commits. The earlier CI results covered the overlay implementation; they do not
verify this replacement. Exact commit and CI evidence is recorded in the Kandev
task plan after delivery.

The bridge pin remains Claude ACP `0.81.2`. Installed runtime overrides can select
other versions.

## Scope and order

| Order | Work order | Status | Depends on |
| --- | --- | --- | --- |
| 1 | [Replace mode delivery](task-01-protocol-mode-delivery.md) | complete | None |
| 2 | [Prove settings isolation](task-02-session-mode-evidence.md) | in_progress | 01 |

Keep the existing auto-approval audit, CLI-destination validation, and task-profile
validation. Do not add a shared-settings fallback option without demonstrated need.
The disabled-by-default consent contract constrains future work.
No new UI control or layout is proposed. Existing mode and error surfaces remain.

## Risks and verification

- Claude can enforce policy restrictions even after a mode request succeeds.
  Test the actual tool outcome separately from reported mode.
- Older or custom bridges can expose only legacy modes. Test capability fallback
  and report unsupported modes without a settings write.
- A requested mode can be restrictive. Hold the first prompt on an unknown or
  different effective mode rather than start with a possibly broader default.
- Historic session overlays can remain in old executor homes. Do not delete user
  files during an update. Apply ACP mode explicitly on resume and test that path.
- Keep regression coverage for mode config-option results and legacy
  `config_option_update` notifications, including responses without a
  `current_mode_update`.

Task 01 owns focused regression tests. Task 02 owns the mock workflow check and
an isolated real-Claude check. Record missing provider access as an evidence
gap. Do not claim that mock behavior proves Claude permission enforcement.

## Results

Task 01 is implemented and its ACP adapter, lifecycle, and agent declaration
tests pass on the locally reconciled tree. Regressions cover mode/model/effort
snapshot ordering in both request orders, preservation of mode-timeout
uncertainty, mode-only config catalogs on new/load/reset, unsolicited mode
updates, and stale-session rejection. The mode event uses the existing
orchestrator persistence path.

The follow-up cancellation review finding is also fixed. A context-aware shared
config gate now covers mode, model, other config changes, and new/load/reset
session transitions. Regression coverage cancels each waiting operation while
a model RPC still owns the gate, then verifies the waiter returns without a
provider request, cache mutation, or retained mode/transition ownership. The
complete ACP package and focused race checks pass with the new coverage.

Focused ACP, lifecycle, and agent tests pass on the integrated tree. The
cancellation and config-ordering race tests also pass. Documentation catalog,
specification lint, and public-document validation pass.

The gated real-Claude harness compiled and skipped because
`KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY` is unavailable. It tests an actual
Git commit and explicit allow-once approval. The original refusal remains
unverified, and Task 02 remains in progress. Do not report provider acceptance
as complete from mock tests or CI alone.
