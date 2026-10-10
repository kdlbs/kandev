---
created: 2026-10-10
status: implemented
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
legacy_specs: []
---

# Fix Quick Chat Resume

## Overview

The existing Resume control must recover an interrupted Quick Chat through native resume.
Scope includes workspace-only admission, explicit manual recovery, actionable read-only notices,
and acknowledgement of the retained delivery journal without replaying uncertain prompts.

## Evidence and scope

At main `851c3c69f`, an affected Quick Chat has an open `unknown_prompt_outcome` block
bound to a prompt submission with state `interrupted_unknown`.
An old stream disconnected during runtime replacement; later ordinary resume and workspace
restore requests both failed at `LaunchSession`'s prompt admission check. Quick Chat's manual
hook uses `session.launch`, while the existing recovery coordinator accepts explicit
`session.recover` action `resume`. Git warnings are expected for this repository-free workspace.

In scope: existing Resume routing, workspace inspection, native history preservation,
unknown-submission no-resend evidence, request fences, and desktop/phone browser verification.
Out of scope: new buttons/confirmation, new schema or APIs, blanket block deletion, automatic
history continuation, runtime survival redesign, production SQL repair, deployment or merge.

## Technical approach

1. Exempt workspace-only intent from prompt admission and recovery resolution; reject prompts.
2. Add a typed service request for the existing explicit Resume endpoint and use it in the
   shared manual hook. Automatic recovery keeps ordinary launch semantics.
3. Prove the existing native recovery coordinator retains tokens and never sends the old prompt.

| Runtime shape | Expected behavior | Evidence |
| --- | --- | --- |
| Native ACP with retained token | Existing Resume preserves native conversation | Repository-backed service test and Quick Chat browser flow |
| Live unresolved durable work | Existing refusal/reconciliation rules | Existing submission/retry suites |
| Native state missing | Existing explicit history-continuation failure | Existing continuity tests |
| Automatic open/focus | Remains ordinary admission; no recovery grant | Shared-hook tests |
| Workspace-only restore | No agent start or prompt; block remains open | Real-repository orchestrator test |

## Tests

- New `session_workspace_recovery_block_test.go`: block-preserving restoration, forbidden
  prompt and recovery-action resolution, zero agent calls (006.7, 006.10).
- New `use-session-resumption-manual-recovery.test.ts`: manual `session.recover` request,
  response hydration and stale response fencing (006.8).
- `session-recovery-service.test.ts`: resume response identity/workspace and timeout (006.8).
- New `session_native_delivery_recovery_test.go`: explicit recovery keeps native token and
  unknown submission, records resume action, sends no prompt, preserves block on failure (006.9).
- Existing queue dispatch and Send Now recovery/settlement tests cover accepted-work no-resend.

## E2E evidence

Repository-free Quick Chat: send an initial instruction, stop the runtime, seed an unknown
prompt block through isolated fixture SQL, reload, use the existing Resume, and send one
new instruction. Confirm native identity is retained and old prompt is not resent. Add the
scenario to shared desktop/mobile Quick Chat recovery helpers and thin spec files.
Work order 03 adds the read-only notice action and owns its shared desktop/mobile ASCII preview.

## Work orders

- [x] [01: Workspace-only access](task-01-workspace-access.md)
- [x] [02: Existing Resume recovery](task-02-manual-resume.md)
- [x] [03: Read-only workspace Resume](task-03-read-only-resume.md)

Implement sequentially in the primary session using TDD. No subagents are authorized.

## Verification results

Both work orders are implemented and verified. Backend race tests, 91 focused frontend
assertions, frontend typecheck, zero-warning ESLint, and desktop/mobile browser flows pass.
Documentation validators and 36 specification-linter tests pass. See each work order for
commands and evidence. The broader durable-delivery specification remains draft; this repair
implements only the added Resume and workspace-access acceptance criteria.

PR publication and remote check status belong to the external task handoff.

## Risks

- Do not turn automatic session open into explicit recovery authorization.
- Keep ordinary native-state-loss and unresolved live-stream rules unchanged.
- Failed recovery must preserve the block and token; late replies must preserve successor UI.

## Follow-up after PR #4401

The read-only notice omitted the Resume callback when no composer recovery owner
was present. A successful native resume also left the peer journal interrupted unknown,
causing subsequent prompts to fail with `unresolved_durable_work`. Work order 03 repairs
both boundaries without replaying uncertain prompts. Existing verification above refers
to work orders 01 and 02; follow-up results belong to work order 03.
