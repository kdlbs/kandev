---
status: current
system: office
created: 2026-10-08
requirements:
  - REQ-OFFICE-CONFIG-SYNC-006
---

# Office Config Sync Surfaces System Design

## Purpose and boundaries

Office owns the source configuration and provider-specific editable fields in
[Config Sync Surfaces](../requirements/config-sync-surfaces.md). This design
defines the form's acknowledgment boundary within the existing
[Config Sync design](config-sync.md#frontend). Source validation, persistence,
status rendering, surface arbitration, and scheduling retain that design.

The form uses the independent
[Settings Manual Save policy](../../ui/requirements/settings-manual-save.md),
particularly AC-UI-SETTINGS-MANUAL-SAVE-001.4, and
[ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md).
Office maintains its draft and saved baseline; the shared coordinator owns
save ordering, feedback, Reset, and navigation protection. No new requirement,
coordinator policy, or global revision mechanism is introduced.

## Requirement mapping

| Office requirement | Design section |
| --- | --- |
| `REQ-OFFICE-CONFIG-SYNC-006` | [Components and responsibilities](#components-and-responsibilities), [Save acknowledgment](#save-acknowledgment), [Desktop and phone](#desktop-and-phone) |

The shared policy is a linked dependency, not an Office-owned requirement.
Provider field selection and clearing implement AC-OFFICE-CONFIG-SYNC-006.1
and AC-OFFICE-CONFIG-SYNC-006.2 within that policy.

## Components and responsibilities

- `apps/web/app/office/workspace/settings/components/office-config-sync-section.tsx`
  renders `OfficeConfigSyncSection`, the real provider tabs, target inputs,
  branch, directory, polling switch, and interval. Inputs remain editable
  during saving. Configured sources expose the form through the existing
  Edit configuration disclosure.
- `apps/web/hooks/domains/office/use-office-config-sync.ts` owns
  `OfficeConfigSyncFormState`, `useOfficeConfigSyncForm`, `configToForm`,
  `formRevision`, and `handleSave`. It constructs provider-specific payloads
  and maintains `config` as the acknowledged saved baseline.
- `useOfficeConfigSyncSaveContributor` registers `office-config-sync` at order
  20 with `SettingsSaveProvider`. Its revision describes the live form;
  dirtiness compares that form with `configToForm(config)`. Its discard
  callback reads the latest acknowledged config through normal rendering.
- `apps/web/lib/api/domains/office-config-sync-api.ts` sends the existing POST
  through `fetchJson`. API shape, backend authorization, and normalization
  remain unchanged.

## Snapshot contract

The submitted snapshot is the complete raw `OfficeConfigSyncFormState` used
to construct this request, captured before awaiting POST. It includes all eight
fields: `provider`, `repo_owner`, `repo_name`, `project_path`, `branch`, `path`,
`interval_seconds`, and `poll_enabled`. Equality uses the existing
`formRevision` value contract across this complete shape, including fields
omitted from the provider-specific payload. Comparison with a normalized
payload, a stale closure's current form, one field, or object identity is
insufficient.

Updates already create fresh form objects. Preserve that property. A changed
draft is retained whole, including unnormalized text and provider-cleared
fields; an older acknowledgment does not partially merge canonical values into
it. An edit reverted exactly to the submitted full snapshot is eligible for
normalization. This is snapshot equality, not a count of intermediate events.

## Save acknowledgment

1. Snapshot the complete raw form before awaiting the existing POST. Construct
   the payload from that snapshot, preserving trims for target/branch fields,
   verbatim directory, and provider-specific omission of the other identity.
2. On success, always publish the returned config with `setConfig(saved)`.
3. In the form owner, conditionally replace editable state with
   `configToForm(saved)` only if the current full snapshot equals the submitted
   full snapshot. Prefer a pure functional `setForm(current => ...)` update so
   comparison observes pending edits as well as the latest committed form.
   Expose only a local acknowledgment helper to `handleSave`; do not alter
   unconditional Reset used by initial load, Delete, or Sync Now.
4. Preserve existing success/error feedback and saving cleanup. A newer draft
   remains different from the acknowledged baseline and therefore stays dirty;
   the unchanged shared coordinator can observe its current revision and refuse
   leaving. A successful unchanged submission adopts canonical fields and can
   become clean even when normalization changes its raw revision.

This boundary needs only `handleSave` and immediately related form-state glue.
No state mutation or side effect belongs inside the functional form updater.
No ref maintained by a delayed effect, timer, global store, or new save engine
is needed.

## Failure and recovery

Rejected POSTs advance neither baseline nor editable form. The existing error
propagation to the coordinator and retry feedback remain intact. A later Save
uses the retained current form and the appropriate provider payload. Reset
after an acknowledged save returns the entire form to that acknowledged
baseline without POST, even when newer edits changed provider. Reset after a
failed save returns to the previous saved baseline.

An edit back to the old saved value during saving must survive the response:
after the baseline advances it can be dirty again. Do not use pre-acknowledgment
`isDirty` to decide whether replacement is allowed. A retained draft that
already equals the acknowledged baseline is naturally clean; preserve the
existing value-based dirty semantics.

## Desktop and phone

Both viewports share this hook, fields, and coordinator. Desktop retains the
existing two-column target and branch/directory groups; phones retain their
single-column inline form, native disclosure, and existing shared save surface.
There is no layout, touch, focus, scrolling, navigation, copy, or breakpoint
change. The existing phone composition is exercised by
`apps/web/e2e/tests/office/mobile-office-config-sync-provider.spec.ts`; it does
not cover the delayed acknowledgment regression. For this pure state correction,
faithful rendered component tests cover both viewport-independent semantics
under the mobile-parity state/data exception. No browser/build run is required.

## Verification boundary

Permanent coverage renders the actual `OfficeConfigSyncSection` with the actual
hook, `StateProvider`/`createAppStore`, `SettingsSaveProvider`, and coordinator.
Only external transports are mocked; internal hooks, API payload construction,
providers, coordinator, inputs, status card, and navigation guard remain real.
Use controlled pending transport promises and actual enabled inputs. Observe
both retained input values and real contributor dirty/leave results, then
prove later Save and Reset. Cover scalar fields, provider changes in both
directions, normalized unchanged success, and failure controls. Tests must be
independently authored, not imported or copied from disposable discovery proof.

Delivery details and exact commands belong to the
[draft-preservation plan](../../../plans/office-config-save-drafts/plan.md).

## Exclusions

- Shared coordinator/navigation engine or policy, backend, schemas, API,
  payload rules, layout, copy, or provider capability expansion.
- Sync Now, Delete, initial-load ownership, polling publication, workspace or
  unmount lifetimes, and global configuration versioning.
- Persistent client drafts, multi-user conflict resolution, and a global test
  framework. A proven need outside this boundary requires a scope checkpoint.
