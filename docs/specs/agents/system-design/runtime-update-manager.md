---
status: current
system: agents
requirements:
  - REQ-AGENTS-RUNTIME-MANAGER-001
  - REQ-AGENTS-RUNTIME-MANAGER-002
  - REQ-AGENTS-RUNTIME-MANAGER-003
created: 2026-11-10
owners:
  - Kandev
---

# Agent runtime update manager design

## Boundary and requirement mapping

Agents owns this vertical feature, including its desktop and phone presentations.
The existing controller retains release discovery, trusted update recipes, maintenance admission, validated activation, and policy persistence.
The manager changes presentation and coordinates individual requests. An optional review precondition protects bulk admission on the existing endpoint.
It adds no backend endpoint or database schema.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-RUNTIME-MANAGER-001 | Composition, runtime capabilities, policy drafts, responsive behavior |
| REQ-AGENTS-RUNTIME-MANAGER-002 | Target navigation, shared status, lifetime and recovery |
| REQ-AGENTS-RUNTIME-MANAGER-003 | Bulk review, submission and outcomes, lifetime and recovery |

This design replaces the settings presentation in [runtime notifications](runtime-update-notifications.md) after delivery.
It replaces the destination presentation in [runtime summaries](runtime-update-summary.md), without changing delivery or grouping.
The [plan](../../../plans/agent-runtime-update-manager/plan.md) records accepted ASCII views and sequential delivery.

## Existing boundaries

- `app/settings/agents/page.tsx` owns installed cards, composes `useAgentRuntimeUpdates` with shared runtime status, and mounts the page-level manager controller.
- `RuntimePolicyControllers` keeps one route contributor per runtime. The former bottom disclosure is retired; its row content now lives in manager details.
- `AgentRuntimeUpdateControl` owns a standalone trigger and detail surface for other callers.
- `AgentRuntimeUpdateSurface` selects desktop `Dialog` or phone `Drawer`; the manager reuses the same bounded composition for overview and details.
- `useRuntimeAutoUpdatePolicy` registers a route save contributor bound to agent and runtime identity.
- `SettingsTargetProvider` and `createSettingsTargetRegistry` own fragments, pending destinations, action registrations, and explicit discovery requests.
- `maintenanceCoordinator` serializes mutation per agent. Independent agents can run concurrently.
- `agent.update.started`, `agent.update.output`, and `agent.update.finished` publish into `updateJobs.byAgent`.
- `listAgentUpdateJobs` supplies one bounded retained-job recovery read per store; live job results still arrive through WebSockets.

## Composition

Introduce a page-owned `AgentRuntimeUpdateManager` controller above the responsive overlay branches.
Its view state is `closed`, `overview`, or `runtime(agentName)`.
The header action, installed-card control, overview Manage versions button, and fragment action use this owner.
Installed cards remain profile-focused and retain their existing card anchors.

Extract the reusable detail body and footer from `AgentRuntimeUpdateControl`.
Keep its standalone behavior available for other callers. The manager renders the detail content inside its existing overlay.
The overview and details never mount separate Dialog or Drawer roots at the same time.
Preserve existing operation labels, target selection, job output, compatibility notices, and terminal results.

Replace the bottom `AgentRuntimePolicies` disclosure with manager rows and persistent policy contributors.
The contributor controllers stay mounted outside conditional overlay content while the Agents route remains mounted.
Their views can mount in details without creating another contributor for the same runtime.
Keep package and owner information in a compact secondary line or details disclosure.
Retained failure and interruption outcomes remain visible from the affected runtime.

The overview groups available actions first, then up-to-date, unknown, external, and unavailable registrations.
Group names remain explicit. Unknown is never folded into Up to date.
No-update entry still allows version selection, repair, policy edits, and guidance.

## Runtime capabilities and counts

Derive rows from `AgentUpdateStatus`, supported `RuntimeUpdate` metadata, install jobs, and update jobs.
Reuse `presentAgentRuntimeIdentity` and the existing capability distinctions.
Do not infer ownership or update support from provider names, binary paths, or package labels.

The header available count includes known newer versions with a supported individual mutation action.
It excludes external guidance, unsupported or unavailable entries, unknown checks, and active maintenance.
A harness-owned updater can contribute a known availability hint while remaining ineligible for bulk selection.
An explicit overview note distinguishes individual-only updates from the bulk count.

| Runtime shape | Overview and individual action | Bulk behavior |
| --- | --- | --- |
| Managed pinned npm candidate | Exact current and target, version selection and recovery | Eligible after an update preview |
| OpenCode managed current family | Existing family and source, explicit migration detail | Ordinary same-family update only |
| Native owner with managed fallback | Separate native observation and fallback details | Excluded |
| Harness `self_update` | Trusted target-free updater and advisory stable reference | Excluded |
| External/manual runtime | Verified guidance and explicit observed/unknown state | Excluded |
| Custom, unsupported, disabled, or unavailable | Explicit limitation and retained outcome | Excluded |

Missing or inconsistent metadata disables mutation for that row while keeping inspection and known guidance available.
Existing pinned, self-update, fallback, migration, and runtime-identity suites provide compatibility evidence for these branches.

## Shared status and policy drafts

Reuse `refreshRuntimeUpdateStatuses` and `useAgentRuntimeUpdateStatuses`.
Refresh remains one store-owned request with the existing queued forced refresh.
The UI action does not bypass backend release caching or promise an uncached upstream request.
Show loading, last checked information, and an inline refresh failure. Retain the last successful snapshot.
Disable new approval after a failed required refresh or preview until a successful review replaces it.

Policy controllers retain `useRuntimeAutoUpdatePolicy` and its authoritative-baseline behavior.
Expose its existing saved-baseline reset as a domain callback, without duplicating draft state.
The manager's dirty footer calls `useSettingsSaveCoordinator().saveAll` for explicit Save changes.
Reset invokes the runtime policy controllers' discard callbacks and performs no write.
Do not create a nested `SettingsSaveProvider`, change the route navigation guard, or persist switches immediately.
The existing page save surface remains the route owner outside the modal. Its controls are inactive behind the modal focus boundary.

The dirty footer is reachable in both overview and details. Closing preserves contributors and the page's unsaved marker.
Policy saves and immediate update admission are mutually exclusive through `runExclusive` and existing save state.
A dirty policy on an affected runtime disables its update action with a visible Save-or-Reset explanation.
Other eligible clean runtimes can still update.
After admitted manual updates disable saved consent, a clean controller adopts the refreshed saved value.
No update operation saves an unrelated draft.

## Target navigation

Preserve `/settings/agents#runtime-updates`, `#runtime-update-<agent>`, and `#installed-agent-<agent>`.
The first opens overview. The second opens focused runtime details, including individual-only and unavailable entries.
The third retains its existing card reveal behavior.

Extend the existing target registry with action registrations alongside element registrations.
An action registration resolves an existing pending request and supplies domain activation instead of DOM scrolling.
Add a provider hook for action registration. Keep normal element reveal unchanged for every existing caller.
Use that mechanism for manager targets, rather than a second hash listener or a notification-specific URL.
Do not register the same target as both a hidden row element and an action.

Register known runtime identities from the catalog or status snapshot.
A pending identity activates when delayed metadata registers its action.
An unknown fragment remains unresolved and leaves the Agents page usable. It cannot select another runtime.
Repeated explicit target requests reopen the manager. Dismissal alone does not replay the unchanged fragment and reopen it.
When a focused runtime disappears, show an unavailable state with Back to all runtimes.
The overlay owns focus after activation. It restores the actual opener on close, with the header action as fallback.

## Bulk review

Add a dependency-neutral row selector and review model under `lib/agents/`.
Use structural fields and operation enums. Do not compare translated copy or implement a second version comparator.

Status-level bulk candidates require all of these conditions:

- `owner: kandev`, `management: managed`, `mechanism: npm_candidate`, and `update_mode: pinned`.
- Available, enabled, supported runtime metadata, known current/effective version, and `check_state: update_available`.
- No active install or update, no managed fallback, and no affected dirty policy.

Prepare exact previews through `previewAgentUpdate(agent, displayedLatest)` with at most two active preview requests.
Capture agent, runtime ID, package, source, family, runtime revision when present, current selection, and exact target.
An eligible preview must classify the action as `update`, retain the current source/family, and match that reviewed target.
Never send migration, repair, rollback, use-default, fallback, or self-update requests through this path.
The presence of an optional migration does not exclude an ordinary update within the adopted family.

### Guarded individual admission

The current pinned POST accepts an exact target without an expected runtime identity.
Client revalidation alone cannot protect the interval between a preview and admission in another tab.
Extend `AgentUpdatePreviewDTO` and `AgentUpdateRequest` with optional `reviewed_runtime` evidence.
The frontend mirrors this field in `agent-update-api.ts` and echoes it only for bulk requests.

The evidence contains runtime ID, trusted package, current version, effective selection, and applicable family, source, and runtime revision.
Preview returns this evidence only for an ordinary managed candidate with known identity and an `update` operation.
The explicit `target_version` remains the reviewed target. Evidence contains no executable command or mutable package authority.
An older server without this evidence retains individual actions but cannot enable bulk approval.

The controller compares the evidence with its trusted current state under `runtimeAutoUpdateMu` before admission and policy changes.
Guarded requests reject active maintenance and stale or incomplete evidence with a structural conflict response.
Stale evidence returns HTTP 409 with `code: runtime_review_changed`. Invalid evidence or incompatible request forms return HTTP 400.
An existing maintenance conflict retains its current active-job fields and never counts as a fresh guarded admission.
They cannot reuse a different job, select a fallback, start native preparation, or classify the target as rollback or repair.
Rejection creates no job and does not disable automatic consent.

Capture the validated managed source and exact recipe in the admitted job.
Reuse `runtimeActivationGuard` before preparation and around the selection commit.
The guard rechecks runtime identity and selection under the policy mutex, without holding the job-store mutex during lock acquisition.
Guarded jobs always stage the captured managed candidate. A later source change cannot redirect preparation into a native installation.
A stale activation fails without changing the newer selection or publishing the rejected candidate's capabilities.
Admitted manual updates retain normal consent withdrawal, even if candidate preparation later fails.

Unguarded legacy requests, explicit migrations, self-update requests, and use-default requests keep their existing wire behavior.
Do not overload migration's `expected_runtime_revision` field. Reject `reviewed_runtime` combined with those incompatible request forms.
Handler, controller, and job tests cover competing selection, source changes, maintenance conflicts, and an unchanged valid target.

Display compatibility notices returned by the existing preview, including Copilot's advisory state and affected-profile count.
Preserve their advisory meaning. A failed required preview disables only its row and exposes Retry preview.
Candidate validation still occurs in the backend job. A preview alone does not prove that installation will succeed.

Select eligible reviewed rows initially. Keep user exclusions while navigating details or refreshing.
After the operator changes selection, newly discovered candidates stay unselected until explicitly added.
A changed target loses approval readiness and requires review before selection.
Use Update all only when every bulk-eligible reviewed row is selected. Otherwise use Update selected.

## Submission and outcomes

Use a store-owned coordinator under `lib/agents/runtime-update-batch.ts` with a narrow hook adapter.
Keep one active run per store. Its transient state lives in a small settings-slice field separate from `updateJobs.byAgent`.
The coordinator retains approved row identities and submitted job IDs. Renderers never own submission lifetime.
State modules must not import React components or route modules.

On approval, take an immutable selection snapshot and latch the run before asynchronous work.
Read current shared status and exact previews again before admission. Revalidation cannot silently change a displayed target.
Process admission requests sequentially without waiting for each job to finish.
The backend can execute independent admitted jobs concurrently through its existing maintenance boundary.
No jobs are created before approval. Closing an overview during preview cancels only its obsolete view publication.
Closing after approval does not cancel the coordinator.

Reuse the request and job-publication logic in `useAgentRuntimeUpdates` through dependency-neutral helpers.
Do not use a hook closure or component cleanup as the batch executor.
Each admitted request sends the exact `target_version` and its `reviewed_runtime` evidence through the existing pinned endpoint.
Busy responses can reference an existing job. Match runtime evidence, operation, target, and job identity before adopting it.
Observe a different existing job as separate maintenance, without counting it as success for the requested row.
An authorization or authenticated-identity change stops unsubmitted requests. Already admitted jobs retain server ownership.
Other per-row admission errors do not stop independent approved rows.

Project job phases from `updateJobs.byAgent` only when the captured job ID matches.
Keep a run's terminal result when a newer unrelated job replaces that agent's current entry.
Use `last_outcome` for retained interruption or recovery information after job retention ends.
Do not infer success from a changed latest version, an accepted POST, or the absence of an active job.
Show capability-refresh errors as activation success with a refresh limitation, matching the existing detail behavior.

Retry failed returns failed rows to review. It does not immediately replay a POST.
Unconfirmed submissions require reconciliation and an explicit individual review before another mutation.
Successful rows are never included in Retry failed.
Show one translated aggregate live status plus per-row phase and cause. Keep logs in existing per-runtime details.

## Lifetime and recovery

The approved coordinator survives manager dismissal and SPA route changes through its owning store.
It stops remaining admissions when store authority changes or permission is lost.
It never replays an unconfirmed row. Other approved rows continue unless authority is uncertain for the whole run.
No browser storage records an executable queue. Reload never restarts unconfirmed POST requests.

On manager entry and reconnect, read retained jobs through `listAgentUpdateJobs` and use existing merge-safe upsert actions.
Do not replace a newer live job with an older list snapshot.
Use a single bounded recovery read per store. An unmounted caller cannot cancel another observer's read.
During a visible outage or uncertain submission, offer Refresh status without automatic mutation retries.
Clean up coordinator subscriptions when its run settles. Late responses from an old authenticated identity cannot publish.

Reload can recover retained individual jobs and outcomes, but cannot reconstruct which unsubmitted requests a lost browser approved.
The UI states this limit and never claims a durable batch or atomic result.
Backend restart retains its existing interrupted-outcome contract and job-output retention limits.

## Responsive behavior

The nearest shipped exemplar is `AgentRuntimeUpdateSurface`: bounded desktop Dialog, tall phone Drawer, fixed header/footer, and internal scrolling.
The curated `mobile-picker-sheet.tsx` contributes list-to-detail Back navigation within one sheet.
The task-specific picker component itself is not imported into Settings.

The page uses its existing scroller and a visible full-width phone runtime action.
The manager uses `useResponsiveBreakpoint`, `Dialog`, and `Drawer` from `@kandev/ui`.
Phone composition is a tall inset bottom drawer because runtime maintenance is a temporary, occasional task with several detail steps.
Use a bounded `dvh` height and one `min-h-0 flex-1 overflow-y-auto` body.
Keep the header and primary action outside that body, with footer safe-area padding.
The policy save row shares this fixed footer and appears only while dirty.
No nested vertical log scroller or second update overlay is required.

Keep controls at 28px on fine-pointer desktop and at least 44px on phones or coarse pointers.
Use 48px nominal size for the phone primary action, then measure its actual hit target.
Cover canonical phone, 767px fine-pointer, 768px desktop boundary, and coarse-pointer tablet behavior.
Preserve state above responsive branches and mount only the active presentation.
Return from detail restores list position. Escape and dismissal retain drafts and admitted updates.
Use translated accessible names, focus containment, focus return, and explicit live status regions.
Locale additions include English, Portuguese, Simplified Chinese, both generated Traditional Chinese variants, Japanese, Korean, and pseudo.

## Decisions and verification

The existing ownership decisions remain authoritative:

- [Validated managed selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
- [Harness-owned updates](../../../decisions/2026-09-26-harness-owned-runtime-updates.md)
- [OpenCode adoption](../../../decisions/2026-09-27-opencode-runtime-adoption.md)
- [Automatic update authority](../../../decisions/2026-10-01-agent-runtime-update-authority.md)

This local presentation and request-coordination change needs no new ADR. The design preserves the rationale without adding an activation mechanism.
Existing job events, output, and retained outcomes remain the operational evidence. No new metrics or identifier labels are required.
The plan maps criteria to focused unit and desktop/phone browser scenarios, including cancellation, stale responses, partial failure, and recovery.
