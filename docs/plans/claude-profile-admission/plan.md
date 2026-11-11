---
created: 2026-11-08
status: in_progress
requirements:
  - REQ-AGENTS-REMOTE-AUTH-ADMISSION-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
  - REQ-AGENTS-HOST-CLI-002
system_design:
  - ../../specs/agents/system-design/remote-auth-admission.md
  - ../../specs/tasks/system-design/change-workflow.md
  - ../../specs/executors/system-design/per-user-ownership.md
  - ../../specs/workspaces/system-design/repository-secrets.md
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
  - ../../specs/agents/system-design/profile-capability-discovery.md
  - ../../specs/agents/system-design/host-cli-model-discovery.md
legacy_specs: []
---

# Implementation plan: Claude profile admission

## Overview

This package plans the confirmed authentication fix for [#5180](https://github.com/kdlbs/kandev/issues/5180).
It also records a separate model-policy prerequisite for [#5183](https://github.com/kdlbs/kandev/issues/5183).
Auth implementation can proceed independently after a later explicit implementation request.
The model prerequisite authorizes investigation and a decision package only.

The authentication work orders (01 through 03) have started implementation.
Task 01 is complete, Task 02 is complete, and Task 03 has its catalog, SSH unit, settings unit, and public documentation portions complete; its managed SSH/settings browser suites remain pending.
Task 04 remains a decision prerequisite with no corrective production change.

The baseline is local HEAD and remote main `9aba7b2edd020f6ba0f92c2bfe318429723ea2ed`.
The [evidence record](evidence.md) contains live issue evidence, dependency heads, source traces, exact diagnostic commands, and their limits.

## Findings and confidence

| Issue | Confirmed finding | Confidence and disposition |
| --- | --- | --- |
| #5180 | Compatibility ignores authorized profile env and Claude's catalog omits API-key auth. Nonempty executor IDs are accepted without scoped reads. | High confidence from source and a disposable scoped-store probe. A corrective auth package is ready for file review. |
| #5183 | Strict `sonnet` is absent from the runtime catalog. Full `claude-sonnet-5-5` is present and succeeds in a policy probe. The reporter verified the full-ID workaround. | High confidence in the failure mechanism. No authoritative alias-equivalence evidence exists in the inspected Kandev contracts. The intended corrective model behavior remains unresolved. |

The model refusal conforms to active AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.1 and .10.
It is not evidence that strict runtime selection must accept an alias.
Task 04 must resolve the product outcome without blocking Tasks 01 through 03.

## Assumption check

The `/interview-me` check reused the user's settled decisions:

- API-key/OAuth separation, scoped secrets, actor attribution, and private compute checks are required.
- Exact-model identity and disabled fallback remain required.
- Destination defaults, explicit task mappings, and independent Review/QA policies must remain intact.
- Planning ends at this file-review handoff.

Source and disposable probes establish the auth defect and model refusal.
The unresolved choice is model alias qualification versus an actionable repair experience under the existing strict contract.
No question blocks the independent auth package. Task 04 makes that choice an explicit prerequisite rather than inventing a runtime patch.

## Scope

### In scope

- Add the missing Claude API-key catalog method.
- Validate usable credentials through existing user-visible, actor-scoped Global secret resolution.
- Check effective profile environment shape, conflicts, and all required bindings before accepting an authentication route.
- Keep preview read-only and repeat validation for guarded apply and later launch.
- Preserve defaults, task overrides, workflow guards, independent sessions, and sanitized errors.
- Cover the added executor-settings choice on desktop and phone.
- Plan public documentation with the behavior change.
- Investigate model evidence and define a separate reviewed model-policy/UX outcome.

### Out of scope

Owner-granted secret sharing, prepared-session actor redesign, cold-resume admission repair, and dynamic-provider routing are separate work.
No host login copying, provider login during preview, model/provider fallback, prefix-based alias inference, or automatic profile rewrites are allowed.
Issues #5191, #5186, #5203, #5169, and prompt-delivery repair #5178 are not absorbed into this package.

## Requirements and design traceability

| Outcome | Requirement and acceptance criteria | Design | Work orders |
| --- | --- | --- | --- |
| Credential proof and method separation | REQ-AGENTS-REMOTE-AUTH-ADMISSION-001, AC .1, .2, .4, .5, .7 | [Auth admission](../../specs/agents/system-design/remote-auth-admission.md) | 01, 02 |
| Actor/compute/secret boundaries | REQ-EXECUTORS-OWNERSHIP-004, AC .1; REQ-WORKSPACES-REPOSITORY-SECRETS-001, AC .2, .4, .10 | [Ownership](../../specs/executors/system-design/per-user-ownership.md), [secrets](../../specs/workspaces/system-design/repository-secrets.md) | 01, 02 |
| Atomic mappings and lifecycle | REQ-TASKS-CHANGE-WORKFLOW-002, AC .2 through .5 | [Workflow change](../../specs/tasks/system-design/change-workflow.md) | 02, 03 |
| Visible API-key method and SSH behavior | REQ-AGENTS-REMOTE-AUTH-ADMISSION-001, AC .1, .4, .6 | [Auth admission](../../specs/agents/system-design/remote-auth-admission.md#settings-surface) | 03 |
| Exact identity and settings preservation | REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001, AC .1, .3, .5, .8, .10; REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002, AC .1 | [Policy](../../specs/agents/system-design/no-silent-model-fallback-01.md), [recovery](../../specs/agents/system-design/no-silent-model-fallback-02.md) | 04 prerequisite |
| Host selection stays advisory | REQ-AGENTS-PROFILE-DISCOVERY-003, AC .3, .4; REQ-AGENTS-HOST-CLI-002, AC .3 | [Discovery](../../specs/agents/system-design/profile-capability-discovery.md), [host CLI](../../specs/agents/system-design/host-cli-model-discovery.md) | 04 prerequisite |

Only authentication admission needs a new requirement/design pair.
Existing workflow, secret, ownership, and model requirements remain the authoritative contracts.
The [proposed auth ADR](../../decisions/2026-11-08-profile-auth-admission-evidence.md) awaits review.
No alias-equivalence ADR is accepted or proposed as an implementation decision in this package.

## Technical approach

Task 01 adds an API-key method and a reusable readiness checker within `agent/remoteauth`.
It uses source-aware environment resolution and the existing Global user-visible secret store.
The new checker rejects missing identity/store, malformed configuration, conflicts, empty values, and invalid required bindings.
One successful alternative must not short-circuit those failures.

Task 02 wires that checker into `taskAgentExecutorCompatibilityValidator`.
Task service establishes the actor and authorizes the actual executor before credential reads.
This is read-only preparation, not `PrepareExecutorAdmission` with its admission writes.
Both workflow-change defaults and overrides receive the same checks.
Actual move and later launch repeat their existing guards and credential reads.

Task 03 proves the workflow and SSH child boundaries with disposable fixtures and updates public docs.
Its test adapter must retain Claude's real authentication catalog while using a credential-free ACP stub.
A methods-less mock agent would bypass the defect and is insufficient.
Existing real Claude-provider catalog tests and backend integration tests remain required even with that adapter.

### Compatibility matrix

| Provider / transport / evidence | Intended outcome | Required evidence | Unsupported or failed evidence |
| --- | --- | --- | --- |
| Claude ACP / SSH / usable profile API-key binding | Compatible without executor auth duplication. Key reaches the child under its own name. | Scoped-store admission + backend workflow integration + disposable SSH test | Deny missing, inaccessible, wrong-scope, empty, deleted, or revoked binding. No OAuth conversion. |
| Claude ACP / SSH / selected OAuth method | Retain OAuth method ID and provisioning. | Catalog and SSH lifecycle tests | Invalid selected binding denies. API key cannot impersonate OAuth. |
| Existing file-auth agent / remote / selected existing method | Preserve configured file route and existing provisioning checks. | Shared validator tests using a real file-auth catalog | Unknown method provides no evidence. No automatic host file selection. |
| Concrete agent / local or worktree | Retain existing credential-admission behavior. | Existing local compatibility test | No new host-login requirement. |
| Dynamic or methods-less agent | Retain existing runtime-validator and catalog behavior. | Existing dynamic and catalog controls | No new candidate routing or blanket auth bypass. |
| Other remote executors / same catalog | Shared validator tests cover configuration admission only. | Provider-neutral unit cases | No claim of real non-SSH provisioning from this package. |
| Exact Claude / runtime advertises only full ID | Bare alias fails before inference. Explicit full ID applies. | Task 04 policy and consumer controls | Current model, env text, family prefix, and host catalog do not prove equivalence. |

### Model decision boundary

The issue permits two directions, but their proof requirements differ.

1. **Qualified aliases:** Require explicit, authoritative alias-to-ID evidence tied to the actual runtime/profile/catalog context.
   A model name, family prefix, `ANTHROPIC_MODEL`, or `current_model_id` alone is insufficient.
   Accepting an unadvertised saved alias would amend the active strict contract and needs a reviewed ADR and updated requirements.
2. **Explicit full-ID repair:** Preserve strict runtime rejection and help the user select and save an exact advertised ID.
   A host probe can provide an advisory, but cannot reject an otherwise valid remote profile merely because its host list differs.
   Custom IDs must remain savable under AC-AGENTS-HOST-CLI-002.3.
   A useful UX proposal can combine a profile-context advisory with actionable runtime-preflight recovery, without automatic selection or prompt replay.

The older host-CLI design's alias discussion is corrected to defer to the active strictness contract.
That documentation correction is not an implemented fix for #5183.
Task 04 must establish which observable repair resolves the report before corrective model work orders are authorized.
The pinned bridge source was also inspected. Its preference heuristics and models response do not supply the missing strict alias proof.

## ASCII UI preview

UI-01: Settings > Executors > SSH executor profile > remote credentials > Claude.
Only the added catalog method changes rendered content. The same vertical composition serves desktop and phone.

Current catalog:

```text
Claude
  [ ] Provide secret
      CLAUDE_CODE_OAUTH_TOKEN
```

Proposed catalog:

```text
Claude
  [ ] Provide secret for CLAUDE_CODE_OAUTH_TOKEN
      [Stored secret selector]
  [ ] Provide secret for ANTHROPIC_API_KEY
      [Stored secret selector]
```

Labels are illustrative English renderings of existing translation keys.
Each selector appears when its method is selected. The two bindings remain independent.
The current generic renderer already adds env names when there are multiple env methods.
No API-key binding on this screen is required when the agent profile already supplies it.

The existing profile page owns scrolling. Phone rows and picker actions retain at least 44px targets.
Long variable names wrap inside the content region. Desktop controls retain their existing compact size.
No new drawer or navigation is introduced. AC-AGENTS-REMOTE-AUTH-ADMISSION-001.6 maps to Task 03's rendered checks.
No model UI preview is approved: Task 04 must produce desktop/phone previews with any proposed corrective UX package.

## Tests

All new names in this section are proposed regression names, not tests created during planning.

- `remoteauth/admission_test.go:TestRemoteAuthAdmission` covers the full binding and identity matrix for auth AC .1, .2, .5, .7.
  Use disposable real SQLite and synthetic users/secrets for visibility, scope, decrypt, deletion, and revocation cases.
- `remoteauth/catalog_test.go:TestBuildCatalogClaudeAPIKeyAndOAuth` covers stable, separate method identities and the absence of an API-key setup script.
- `backendapp/claude_workflow_admission_test.go:TestClaudeWorkflowAdmission` drives the real validator and service wiring through preview and guarded apply.
  Cover empty/default mappings, replacements, source CAS conflicts, and denied actor/executor/profile choices.
- `task/service/claude_workflow_admission_test.go:TestWorkflowCredentialAdmission` checks authorization-before-secret-read ordering and cancellation.
- `orchestrator/workflow_move_preview_test.go:TestPreviewWorkflowMoveClaudeIndependentSessions` pairs prediction with new Review/QA session identity checks.
- `lifecycle/executor_ssh_credentials_test.go:TestClaudeAPIKeySSHProvisioning` checks agent-profile and executor-method env delivery and no OAuth file writes.
- Existing start-model policy, cancellation, transport, startup dispatch, reset, and rebind consumer tests supply Task 04's controls.
  A future alias/UX change must prove preserved saved settings and consistent behavior on actual executor evidence across start/reset/resume.

## E2E tests

Task 03 adds `tests/ssh/claude-profile-admission.spec.ts` under the `containers` project.
Use the disposable sshd fixture and synthetic secrets. Exercise guarded preview/apply with defaults and an override.
Confirm independent Review and QA sessions, child key presence through a value-free sentinel, and absent OAuth credentials.
Remove/delete the binding between preview and apply. The second operation must fail without a successful workflow mutation.

`tests/settings/claude-remote-auth.spec.ts` and `tests/settings/mobile-claude-remote-auth.spec.ts` cover UI-01.
Desktop uses `chromium`. Phone uses `mobile-chrome` with its configured device.
Exercise both method choices, independent save/reload, localization, focus return, touch geometry, and document containment.
Existing `tests/task/change-workflow.spec.ts` and `mobile-change-workflow.spec.ts` guard unchanged workflow interactions.

These are fixture/provider-boundary tests. They cannot prove real Anthropic acceptance or a live deployment reproduction.
No live SSH account, user key, credential store, helper journal, or existing worktree is a permitted fixture.

## Dependency and overlap decisions

All 146 open PR titles/bodies and canonical issue timelines were inspected.
The 100 most recent merged PRs and relevant local path history were also inspected.
No explicit fix PR for #5180 or #5183 was found in that snapshot.
The [evidence record](evidence.md#github-overlap) lists the exact relevant heads.

- **#5184, draft/open:** Separate owner grants and prepared actor fix. No merge dependency and no copied sharing policy.
  Re-read its source, requirements, and actor resolver if it lands before implementation.
- **#5176 and #5179, draft/open:** Cold resume and retained-binding edits are separate.
  Recheck touched task authorization paths, but do not broaden this package to repair those issues.
- **#4995:** OpenCode credential provisioning changes shared catalog code. Reconcile edits if it lands, without adding an OpenCode bootstrap change here.
- **#4764:** Exact task-profile assignment is separate from exact model selection.
  Preserve its boundary if it lands. No dependency is needed.
- **#5152:** Dynamic routing work remains independent. Concrete Claude auth must not depend on it.
- **#5037 and #5055, merged:** Current profile-aware discovery and grouped catalog behavior are part of the baseline.
  Model planning must reuse them rather than rebuild discovery.

Resolve current main and relevant PR heads again before implementation and final integration.
Do not trust stale shared remote refs. Read remote SHAs directly.

## Work orders

- [x] [Task 01: Scoped authentication evidence and catalog](task-01-scoped-auth-evidence.md) (done, no dependency).
- [x] [Task 02: Guarded workflow integration](task-02-guarded-workflow-admission.md) (done, depends on 01).
- [ ] [Task 03: SSH, settings parity, and operator documentation](task-03-ssh-auth-surface.md) (in progress, depends on 02).
- [ ] [Task 04: Model identity evidence and decision prerequisite](task-04-model-identity-decision.md) (pending, independent).

Default implementation order is 01, 02, 03. Task 04 is a separate investigation path and does not gate auth delivery.
No wave authorizes delegation. Task 03's managed browser suites and the Task 04 decision remain open.

## Public documentation

Planning changes only internal artifacts.
Task 03 updates `docs/public/agents-and-profiles.md` and `docs/public/executors.md` with the auth behavior change.
Use a short configuration how-to and troubleshooting section within their existing structure.
Explain profile-bound Global secrets, explicit executor methods, caller scope, and the distinction between API keys and OAuth.
The existing model troubleshooting can document the manually verified full-ID workaround without claiming alias support.
No page, navigation entry, screenshot, or new sharing instruction is required.
Task 04 owns any later model-UX documentation changes.

## Verification results

Planning evidence is recorded in [evidence.md](evidence.md#diagnostic-results).
Implementation results for Tasks 01 and 02 and the completed Task 03 portions are recorded in their work orders.
Task 03's managed browser and real-SSH end-to-end suites have not run; Task 04 remains an evidence/decision prerequisite.

Document gates passed: catalog validation, all specification lint, 36 linter tests, relative file links, and whitespace checks.
The PR-documentation validator classified the original docs-only diff as exempt.
A separate prospective backend-trigger check validated all four work orders and their REQ/AC/design/plan references with no errors.
Implementation gates that passed: targeted Go race suites, the scoped SQLite admission matrix, the SSH provisioning regression, the settings unit suite, the task-service preview/apply integration fixture, changed-Go lint, and the full backend build. Review remediation also covers indexed Git composition, exact stored reference matching, and cancellation on literal/file routes and during resolution.

Issue assignment was verified after the fresh ownership check: #5180 is assigned to `carlosflorencio` and remains open.
#5183 remains open and unassigned because the corrective model outcome is unresolved.

## Risks and unresolved decisions

- #5183 has no proven alias policy. Its issue remains unclaimed until a corrective outcome has sufficient evidence.
- Scoped store reads cannot detect a provider-side credential revocation. Launch must retain provider authentication failures.
- Preview and apply have no credential lease. Deletion or ownership changes require fresh checks, including at deferred launch.
- Mixed env/auth methods can hide a bad binding if validation short-circuits. The all-bindings and at-least-one-route tests are mandatory.
- A mock alias that loses the Claude catalog would give false end-to-end evidence. Task 03 must retain that catalog explicitly.
- Remote-auth scripts currently have their own best-effort conditions. This package must preserve OAuth behavior and verify API-key isolation.
- The shared validator affects remote credential admission beyond SSH. Preserve existing routes and document intentional bogus-ID rejection.
- Public docs and UI acceptance remain part of Task 03. Passing unit probes cannot replace those results.
