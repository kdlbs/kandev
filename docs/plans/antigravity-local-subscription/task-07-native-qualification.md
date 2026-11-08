---
id: "07-native-qualification"
title: "Qualify the native subscription path and document setup"
status: blocked
wave: 4
depends_on:
  - 01-local-runtime
  - 02-subscription-policy
  - 03-google-sign-in
  - 04-human-questions
  - 05-provider-failures
  - 06-native-usage
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
  - REQ-AGENTS-ANTIGRAVITY-ACP-012
  - REQ-AGENTS-ANTIGRAVITY-ACP-013
  - REQ-AGENTS-ANTIGRAVITY-ACP-014
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-009.4
  - AC-AGENTS-ANTIGRAVITY-ACP-010.4
  - AC-AGENTS-ANTIGRAVITY-ACP-010.5
  - AC-AGENTS-ANTIGRAVITY-ACP-011.2
  - AC-AGENTS-ANTIGRAVITY-ACP-011.3
  - AC-AGENTS-ANTIGRAVITY-ACP-012.1
  - AC-AGENTS-ANTIGRAVITY-ACP-012.2
  - AC-AGENTS-ANTIGRAVITY-ACP-013.2
  - AC-AGENTS-ANTIGRAVITY-ACP-013.3
  - AC-AGENTS-ANTIGRAVITY-ACP-014.2
  - AC-AGENTS-ANTIGRAVITY-ACP-014.4
  - AC-AGENTS-ANTIGRAVITY-ACP-015.2
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 07: Qualify the native subscription path and document setup

## Summary

Establish live vendor compatibility for the actual subscription path and publish accurate setup instructions.
This is the native integration gate that synthetic tests cannot provide, not a generic test or review pass.

## In scope

- Record the selected server version/digest, host OS/architecture, unmodified startup, and the user's completed native consent.
- Exercise the implemented Kandev settings flow with a personal Google account and real model discovery.
- Capture sanitized findings from acpdbg: native initialize, models, one small prompt, token fields, `/usage`, session load/resume, and process cleanup.
- Verify two simultaneous tasks share login but have independent processes. Stop one and continue the other.
- Exercise a native human question under auto-approve where the model/tool behavior supports it.
  A vague prompt cannot guarantee that frame.
- Qualify the native outcome schema without deliberately exhausting a subscription. Deterministic fixtures remain the error regression evidence.
- Check browser behavior and no-affinity startup on every platform claimed as supported; record untested/blocked platforms accurately.
- Update the public agent guide and README only after behavior and platform limits are known.
- Reconcile requirements/design/plan/work-order statuses with actual results and remaining limitations.

## Out of scope

New feature scope, binary patching, silent CPU pinning, deliberate quota exhaustion, global verification audits, and PR creation.
Support claims for untested platforms are also excluded.

## Acceptance

1. Each claimed platform has recorded unmodified startup and real local OAuth/model/prompt/resume evidence. A blocked platform remains explicitly blocked.
2. Live results state usage scope, native `/usage` behavior, shared-login concurrency, and observed compatibility limits without exposing secrets.
3. Public setup guidance matches shipped behavior and limitations; internal package results distinguish real evidence from synthetic fixtures.

## Verification

Run from the repository root after the implementation work orders pass.
The debugger commands below operate on the registered product-compatible agent after explicit Google sign-in through Kandev.
They make small real account requests. The user completes consent in their browser; the agent must not claim consent happened automatically.
Do not run the prompt while the runtime is crashing, auth is incomplete, or user consent is pending.

```bash
make -C apps/backend build-acpdbg
apps/backend/bin/acpdbg probe --timeout 60s --file acp-debug/antigravity-local/qualification-probe.jsonl antigravity-acp
apps/backend/bin/acpdbg prompt --prompt 'Reply only with ACP subscription check complete. Do not use tools.' --timeout 120s --file acp-debug/antigravity-local/qualification-prompt.jsonl antigravity-acp
apps/backend/bin/acpdbg prompt --prompt '/usage' --timeout 60s --file acp-debug/antigravity-local/qualification-usage.jsonl antigravity-acp
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Keep captures private under the ignored diagnostic directory. Do not use `--verbose` or publish raw consent/stderr data.
Before a committed fixture or report is created, replace identifiers and secrets and inspect every retained field.
For load/resume, take the native session ID from the successful capture and use acpdbg's existing `session-load --session-id` command.
For product concurrency/question checks, use two disposable tasks in the UI and record the visible result and sanitized process/session evidence.
Record exact completed commands, including the actual session-load command, in Results with identifiers redacted in public summaries.
If the registered-agent debugger encounters a native question, its Task 04 guard stops the capture without choosing an option.
Complete actual human-answer qualification in the product UI.

Public documentation validation uses the repository's current docs-maintainer checks plus the two commands above.
Repeat changed product tests only if qualification produces a new change or unresolved concern.
All other final task-defined checks are already owned by their implementation work orders.

## Files likely touched

- `docs/public/agents-and-profiles.md`, Antigravity setup and platform limitations.
- `README.md`, Antigravity agent table entry.
- `docs/specs/agents/requirements/antigravity-acp-agent.md`, status/evidence corrections only when justified.
- `docs/specs/agents/system-design/antigravity-acp-agent.md`, qualified artifact/schema/platform evidence.
- `docs/plans/antigravity-local-subscription/plan.md` and work-order Results/status fields.
- Private ignored `acp-debug/antigravity-local/qualification-*.jsonl`, never committed raw.

## Dependencies

Tasks 01-06. A working vendor build and the user's real Google consent are external native qualification prerequisites.
Missing prerequisites leave this work order pending/blocked with a specific result, not passed.

## Risks

- Current Linux 1.4.5 SIGILL is unresolved. Do not mask it with affinity in a shipping command.
- Native behavior can change across vendor releases. Record version-specific evidence and requalify any new pin.
- A model may not emit a human-question frame deterministically. Keep the fixture test as the exact protocol regression and label live observation limits.

## Parallelism

`sequential`

## Inputs

- Design Source evidence and limits, Qualification, and Security sections.
- acp-debug and docs-maintainer skills.
- Implementation work-order results and actual Google account entitlement.

## Results

Blocked. Tasks 01-06 are implemented, but the live gates are still missing:

The [sign-in launch repair](../antigravity-sign-in-launch/plan.md) is an additional prerequisite before retrying native sign-in qualification.
That repair is now implemented and its fixture regression passes, so the configure defect no longer blocks a retry.
The November 4 failure stopped at configuration, so it provides no new evidence about SIGILL, OAuth, or model readiness.
Its fixture regression cannot close this work order.

- No unmodified Linux startup on the investigated host. The vendor 1.4.5 SIGILL remains unresolved; CPU affinity is diagnostic only.
- No completed real Google consent, authenticated model discovery, prompt, `/usage`, or resume capture.
- No qualified native journal schema or live human-question observation.
- Other claimed platforms were not executed.

Do not mark this work order passed from synthetic fixtures. Public setup documentation updates remain gated on this evidence.

Review remediation is complete: managed-runtime admission, utility environment/redaction, scoped credential stripping, retained-agentctl negotiation, login fencing, harness pinning, web polling/stale-response handling, phone action composition, cancellation-aware installation, browser quoting, and registered-acpdbg auth order now have deterministic coverage. None of it substitutes for the live gates above.
