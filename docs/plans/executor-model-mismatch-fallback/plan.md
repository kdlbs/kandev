---
spec: docs/specs/agents/requirements/no-silent-model-fallback.md
decision: docs/decisions/2026-08-15-executor-authoritative-model-selection.md
created: 2026-08-15
status: implemented
---

# Implementation Plan: Executor Model Mismatch Fallback

## Overview

Replace strict launch failure with an executor-authoritative model decision.
If the executor omits the profile model, Kandev does not send a model-selection request.

The agent continues with its current or default model.
Kandev persists a warning in task chat.

This behavior does not depend on portable configuration copying.
Configuration copying remains an optional method to improve parity.

## Runtime decision

Refactor `applyStartModelPolicy` to return a typed decision.
The decision contains requested, fallback, effective, outcome, and warning values.

Apply this order after ACP initialization:

1. Apply the requested model only when the executor advertises it.
2. Apply an explicit fallback only when the executor advertises it.
3. Otherwise, send no model-selection request.
4. Continue with the agent current or default model.

An empty model catalog follows step 3.
A method-not-supported response also uses the agent default.

An advertised model apply error remains an explicit error.
If `auto_fallback` is active, the error remains best-effort and creates a warning.

Use the same decision helper for initial launch, context reset, and workspace rebind.
No later layer can repeat a handled model decision.

## Durable warning

Generalize the current fallback event into a provider-neutral model-selection warning.
Keep the old fallback event as a compatibility projection during migration.

The warning metadata contains these values:

- `kind = model_selection_warning`
- `reason`
- `requested_model`
- `effective_model`, when reported
- `fallback_model`, when applied
- `agent_id`
- `executor_type`
- `executor_profile_id`

The orchestrator persists one `status` message with `variant = warning`.
Use a stable idempotency key for one session-start decision.

The message is best-effort.
A message-persistence error cannot stop the task launch.

The frontend renders localized text from structured metadata.
If the effective model is unknown, it shows `provider default, model not reported`.

The message tells the user to inspect these items:

- Executor credentials.
- Copied agent configuration.
- The agent version in the executor.

## Frontend behavior

The host model catalog remains useful in profile editors.
It does not block task creation or Office setup.

A host-catalog difference shows an advisory warning.
Every affected profile remains selectable.

The chat status renderer shows the persisted warning on desktop and mobile.
The existing chat list remains the only scroll owner.

No new mobile overlay is necessary.
The warning uses the existing mobile chat composition and status-message component.

The live model selector can show the effective executor model.
The persisted chat message remains the durable audit record.

## Test strategy

### Backend

- Cover advertised requested models and advertised explicit fallbacks.
- Cover absent requested models, absent fallbacks, and empty catalogs.
- Make sure that absent models cause zero model-selection calls.
- Cover method-not-supported and advertised apply errors.
- Cover `auto_fallback` warning behavior.
- Cover initial launch, context reset, and workspace rebind.
- Cover event payloads, message metadata, idempotency, and persistence errors.
- Cover message hydration after service restart.

### Frontend

- Cover advisory profile warnings without disabled profile options.
- Cover localized status-message output for known and unknown effective models.
- Cover live model state after explicit fallback and provider-default decisions.
- Cover metadata with missing optional fields.

### E2E

- Create a profile model that the task agent does not advertise.
- Start the task and make sure that the task continues.
- Make sure that chat shows one warning with requested and effective values.
- Reload the task and make sure that the warning remains.
- Repeat the user outcome in the mobile project.
- Make sure that the task-create profile remains selectable.

## Desktop and mobile behavior

The desktop entry point is the task-create profile selector and task chat.
The mobile entry point is the same flow through the mobile task-create dialog.

The nearest shipped exemplar is `status-message.tsx` in the task chat.
The warning stays inside the message list on both viewport classes.

The settings or task page owns vertical scrolling.
The warning does not add a nested scroll area.

The profile option remains a standard selectable row.
Mobile E2E uses `tap()` and checks horizontal overflow.

## Public documentation

Update `docs/public/agents-and-profiles.md` with the host and executor authority boundary.
Update `docs/public/executors.md` with default-on-mismatch behavior and warning guidance.

The documentation must state that configuration copying cannot guarantee equal catalogs.
It must also state that Kandev does not rewrite the saved profile model.

## Implementation waves

Wave 1:

- [x] [Task 01: Define executor model decisions](task-01-executor-model-decision.md)

Wave 2:

- [x] [Task 02: Persist model-selection warnings](task-02-persist-model-warning.md)

Wave 3:

- [x] [Task 03: Remove host model launch gates](task-03-remove-host-model-gates.md)

Wave 4:

- [x] [Task 04: Prove model-mismatch recovery](task-04-model-mismatch-evidence.md)

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'TestApplyStartModelPolicy|TestInitializeAndPrompt.*Model|TestReapplySessionModel|TestWorkspaceRebind.*Model' ./internal/agent/runtime/lifecycle
cd apps/backend && go test -tags fts5 -run 'Test.*ModelSelectionWarning|Test.*ModelFallback' ./internal/orchestrator ./internal/gateway/websocket
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-options components/task/chat/messages/status-message lib/ws/handlers/session-models
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/settings/no-silent-model-fallback.spec.ts tests/session/model-mismatch-warning.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/settings/mobile-no-silent-model-fallback.spec.ts tests/session/mobile-model-mismatch-warning.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Risks

- A provider can report an incorrect current model after Kandev skips selection.
- Message creation happens during session initialization and needs a valid task-session identity.
- A replayed lifecycle event can create a duplicate warning without an idempotency key.
- Existing strict-mode tests and copy must change without weakening advertised-model error handling.
- The old fallback event has consumers that need a compatibility period.

## Results

- Implemented executor-authoritative model decisions for launch, reset, and
  workspace rebind, with default continuation and explicit advertised fallback
  handling.
- Persisted one provider-neutral warning per decision with durable metadata and
  reload hydration; host model probes remain advisory.
- Backend targeted tests: 7,299 passed across 45 packages. Desktop and mobile
  mismatch E2E scenarios passed after reload.
- Public agent-profile and executor documentation passed the public-doc audit.

## Out of scope

- Equal host and executor model catalogs.
- Automatic configuration copying.
- Mid-turn model switching.
- Office provider-route policy after a runtime error.
- Rewriting the saved profile model from executor state.
