---
created: 2026-11-17
status: completed
requirements:
  - REQ-AGENTS-SESSION-SETTING-SEMANTICS-001
system_design:
  - ../../specs/agents/system-design/session-setting-semantics.md
legacy_specs: []
---

# Implementation plan: Pi thinking settings

## Overview

Separate Pi thinking intent from permission policy. Apply the model before
validating thinking choices and make clearing a profile override explicit.
Deliver runtime semantics first, then profile and recovery controls.
The existing permission-confirmation checks remain strict.

## Evidence and requirement conformance

The user reported Kandev 0.98.0 with Pi 1.5.0, pi-acp 0.0.34, and
pi-cursor-agent 0.4.4. Isolated testing of the exact Pi stack on checkout
8b46ceba098d20ddb11de01d6d9d9d947edc71e5 reproduced the same startup failure.
The startup gate and mode-state helpers inspected match v0.98.0; the full
v0.98.0 release binary was not launched.

Argon High and Carbon High are marked reasoning=false by the extension's
catch-all metadata. Pi advertises only off. A saved medium/high is either
rejected against a cached catalog or acknowledged with effective off.
Kandev's generic mode startup gate then reports a permission failure and holds
the prompt. Model changes do not refresh the equivalent legacy thinking list.
The generic config and legacy mode layers can also carry competing values.

The running backend reproduced requested medium/effective off with
prompt_not_sent=true. Clearing only profile mode let fresh Argon and Carbon
tasks receive their prompts and reach WAITING_FOR_INPUT, retaining exact-model
requirements. Their replies were empty. Direct Pi RPC also produced empty
assistant content, excluding Kandev and pi-acp as necessary causes of that
separate symptom. Its root cause is not established.

Existing AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001.4 and -003.2 already require
post-model authoritative options. The missing setting classification,
compatibility, and default recovery contract is defined in
[session setting semantics](../../specs/agents/requirements/session-setting-semantics.md).
This is a semantic extension, not permission-check removal.
Temporary evidence remains at `/tmp/kandev-pi-cursor-repro/evidence/`; tests in
this package must encode sanitized fixtures and must not depend on that path.

## Scope

### In scope

- Qualified Pi legacy thinking aliases and canonical ACP thinking options.
- Selected-model capability reconciliation, strict explicit thinking validation,
  bounded failure evidence, and existing record compatibility.
- Explicit profile default, unsupported-choice preservation, and desktop/phone recovery.

### Out of scope

- Upstream Cursor extension changes or empty-response diagnosis.
- Silent fallback, global settings writes, mass migration, or relaxed permissions.
- New runtime reset policy, generic provider framework, or delegated implementation.

## Technical approach

Follow the [design](../../specs/agents/system-design/session-setting-semantics.md).
Reuse `acpcompat`, adapter configuration snapshots, profile config maps,
startup failure projections, model-aware resolvers, and current selector shells.
No new database table, SQL column, runtime flag, or dependency is required.
Only explicit save canonicalizes the edited profile. Read compatibility covers
old profile/session/workflow records without rewriting history.

| Provider/shape | Meaning and transport | Verification | Unsupported shape |
| --- | --- | --- | --- |
| Qualified Pi + thought_level option | Canonical config RPC; legacy modes are aliases | Transport, lifecycle, profile tests | Ambiguous alias fails visibly |
| Qualified legacy Pi, modes only | Thinking via legacy set_mode | Synthetic legacy bridge | Unadvertised/refused choices hold prompt |
| Real permission provider | Existing strict permission mode path | Existing mode refusal/confirmation suites | Existing safe failure |
| Other/unknown legacy mode | Existing session-mode behavior | Negative semantic classification test | Never infer thinking from labels |
| Config-only thinking provider | Preserve explicit ACP category | Shared normalization test | No invented legacy alias |

Modern Pi's complete post-model thinking options replace its legacy projection.
Unknown snapshots stay unknown. Stale updates are rejected by existing session
and config generations. Source precedence is resolved before applying one
thinking value, so later mode application cannot undo the chosen config value.

## ASCII UI preview

UI-01: Profile editor, unsupported saved thinking level (proposed).

```text
Desktop: Settings > Agents > Pi > Planning
Model             [ Argon High                         v ]
Thinking level    [ medium (unavailable)                v ]
This model supports only off. Saved choice: medium.
Choices: Use provider default | off
                                  [Discard] [Save]

Phone: the same profile page opens one choice sheet
[Back]                 Thinking level             [Close]
( ) Use provider default
( ) off
Saved: medium (unavailable)
Select a choice to update the draft.
```

The phone sheet owns one vertical scroller. Header and choice rows remain reachable
above safe areas; profile Save remains the persistence action. Targets are at
least 44px on phone/coarse pointers, while desktop uses 28px shared controls.
Changing viewport retains the draft. Selection updates the draft; the parent Save persists it. This reuses `model-config-sheet.tsx` and
the `MobilePickerSheet` interaction pattern, not a new nested overlay.

UI-02: Startup recovery card, profile-owned choice (proposed, same hierarchy on phone).

```text
Thinking level is unavailable
Planning requests medium. Argon High supports only off.
No prompt was sent.
[Edit profile]
Choose Use provider default and save before starting a fresh session.
```

A session/workflow source names that source and its existing edit/reset route.
Existing retry and fresh-session controls retain their ownership and guards.
The card must not imply Edit profile clears a stronger override or sends a prompt.
Unavailable copy, source visibility, default selection, control order, and
single-sheet phone behavior are structural requirements. Spacing is illustrative.
Map both views to AC-AGENTS-SESSION-SETTING-SEMANTICS-001.4, .6, and .9.

## Tests

- .1/.8: `acpcompat/session_setting_semantics_test.go` (new),
  `TestSessionSettingSemantics`; distinguish Pi from real and unknown permissions.
- .2/.3: `adapter/transport/acp/pi_thinking_settings_test.go` (new),
  `TestPiThinkingSettings`; model-switch off-only catalogs, stale updates,
  canonical/legacy routes, refusal and confirmation.
- .3/.5/.7: `runtime/lifecycle/pi_thinking_settings_test.go` (new),
  `TestPiThinkingStartup`; assert zero prompt dispatch on failure and exactly one
  dispatch after valid/default startup, source precedence, resume, and reset.
- .4/.5/.7: profile save/patch/duplicate tests and
  `settings/profileconfig/pi_thinking_test.go` (new), `TestPiThinkingIntent`;
  test omitted fields, explicit clear, off, legacy aliases, and unrelated options.
- .1/.6: bounded cause normalization and frontend recovery-model tests.
- .4/.6/.9: profile form, model capability hook, and selector tests including
  singleton off, unavailable value, stale probes, source routing, and Save/reload.

## E2E tests

Add `apps/web/e2e/tests/settings/pi-thinking-settings.spec.ts` (chromium) and
`mobile-pi-thinking-settings.spec.ts` (mobile-chrome) covering .1-.6 and .9.
Use mock-agent fixtures that expose Pi-shaped modes plus config options,
change choices after model selection, and clamp unsupported thinking to off.
Test fresh startup after explicit profile default, retaining exact model,
profile reload/duplication, unsupported overrides, and a real-permission control.
Phone coverage asserts one sheet, focus return, at least 44px targets,
viewport containment, and retained draft across desktop/phone transitions.
No paid provider calls or real credentials in the acceptance gate.

## Work orders

- [x] [Task 01: Runtime semantics and compatibility](task-01-runtime-semantics.md)
- [x] [Task 02: Selection and recovery](task-02-selection-recovery.md)

## Verification results

Design validation on 2026-11-17:

- `python3 scripts/list-docs.py validate`: passed (453 decisions, 2020 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Repository `validateCoverage` from `.github/scripts/pr-docs.cjs`: passed for
  both work orders with the proposed runtime change supplied as a dry-run input.
  This validates references; no production change was claimed or published.
- `git status --short -- docs/plans/pi-thinking-settings`: all three planned files present and untracked.

Implementation completed on 2026-11-19. The two work orders contain the final
change details, exact verification commands, and limitations.

- Backend lifecycle/profile/cause regression suites and the full ACP adapter
  suite passed with race detection. Changed-Go lint passed after a test-only
  string-constant cleanup.
- 230 focused frontend tests passed across 12 files. Typecheck, lint, localization,
  and the localization ratchet passed.
- Managed desktop E2E passed through blocked startup, recovery navigation,
  profile default Save/reload, and a fresh task receiving its prompt with the
  exact model retained.
- Managed phone E2E passed recovery, default selection, focus return, 44px targets,
  Save/reload, and viewport containment. The existing permission-mode phone
  regression also passed.
- Public docs, specification validators, and diff whitespace checks passed.

The implementation reuses existing agent identity and configuration categories
instead of introducing duplicate semantic transport fields. No mass migration,
permission bypass, global Pi change, or upstream extension change was made.
Earlier authenticated Pi/Cursor reproduction remains distinct from these
post-fix automated checks. The separate empty upstream response is not repaired
by this package. Changes are uncommitted.

## Risks

- Clearing a canonical value while leaving a legacy alias would resurrect it.
- Model-change responses can race with old notifications; fencing is mandatory.
- Generic option reconciliation can silently delete an invalid saved draft.
- Profile default does not clear stronger workflow or session intent.
- A successful startup may still have an empty upstream reply.

## Related delivery records

The completed permission-control and startup-failure packages remain historical
records. This package supersedes only the assumption that Pi legacy modes are
permission controls. It does not reopen their unrelated tasks or mark their
existing verification as evidence for this repair.
The catalog repair package remains independent; reuse its canonical options,
without depending on Devin composition work or changing its acceptance matrix.
