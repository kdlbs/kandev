---
status: draft
system: integrations
created: 2026-10-09
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-001
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-003
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-006
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-007
owners:
  - kandev
---

# Task change request registration system design

## Ownership and baseline

Integrations owns registration, canonical contribution identity, and provider state. The task service owns admission transactions and session guards.
The frontend presents committed state. It never creates an association before message acceptance.

Reuse [task change management](task-change-link-mcp.md), [reference composition](../../ui/system-design/entity-reference-composer.md), and [queue admission](../../tasks/system-design/queue-admission.md).
Preserve [initial brief](../../tasks/system-design/initial-task-brief.md) and [queue editing](../../ui/system-design/message-queue-edit.md) contracts.
[Remote contributions](../../tasks/system-design/remote-contribution-tasks.md) and the [comparison decision](../../../decisions/2026-08-19-repository-qualified-comparison-targets.md) retain explicit review/checkout behavior.
This proposal adds an observation-only exception for registration. It does not reinterpret existing manual comparison selection.

Current code and pinned t3code evidence are in [investigation](../../../plans/pr-mention-linking/investigation.md).
All new fields, tables, and endpoints below are proposed. Existing symbol names refer to that inspected revision.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-001 | Reference intent; Admission; Durable processing |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-002 | Identity and provider support; Security |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-003 | Agent tool extension; Shared guidance |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-004 | Persistence; Unlink ordering; Events and recovery |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-005 | Observation and downstream effects; Admission |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-006 | UI and accessibility; Events and recovery |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-007 | [Plugin registration design](task-change-registration-plugins.md) |

## Components

Paths are relative to the repository root.

- `apps/backend/internal/backendapp/task_change_link_coordinator.go` remains the provider-neutral composition seam. Add a shared URL resolver and typed registration options here.
- `internal/github` and `internal/gitlab`, below `apps/backend`, retain provider reads, association rows, status sync, and exact unlink cleanup.
- `apps/backend/internal/mentions` validates provider destinations. `internal/entityrefs` normalizes immutable reference data, not task identity.
- Message and queue repositories persist registration intent within their existing acceptance transactions. A narrow injected registration service consumes that intent.
- `apps/backend/internal/sysprompt` supplies shared guidance according to effective tool availability.
- Existing reference chips, message metadata, queue metadata, provider WS handlers, and provider store slices display the results.

Do not import `backendapp` into task repositories. Define small transaction/data interfaces at the persistence boundary and inject the coordinator from composition.

## Reference intent

Search and linking are separate capabilities. Active plugin `reference_sources` already join the registry through `MentionSourceBridge`.
The [plugin registration design](task-change-registration-plugins.md) adds backend association capability and maps it to those sources.
Keep `EntityReference` version 1 and its portable Markdown fallback. Add optional `association_intent: link | context_only` to Go and TypeScript DTOs.
Add `selection_id`, a client-generated UUID required for a new `link` choice. It identifies an action, never authority.
Only a new supported picker selection writes `link`. Old clients, old drafts, recalled historical references, and absent intent default to `context_only`.
Recalling a submitted message must not generate a new link intent automatically. A fresh picker selection or explicit toggle creates new intent.
Fresh selection, remove-and-reselect, and context-only-to-link each generate a new `selection_id`.
Draft serialization, network retries, queue folds, unchanged queue edits, and viewport changes preserve that ID.
The queue editor retains committed IDs. Save compares both `ref` and `selection_id` with the committed row.
A changed ID requests registration even when the final reference and mode match the original values. Cancel commits no intent.
Bind each ID to its task, reference, and first accepted origin. Reuse returns that intent, including suppression after unlink.
Reject reuse for a different reference. Include the ID in request fingerprints and stored metadata through dispatch.
Deduplicate one reference identity in one draft. The draft has one visible choice per identity; toggling any repeated occurrence updates that shared choice.
Reject duplicate references with conflicting modes or selection IDs, rather than choosing the first value.

Search descriptors can advertise association capability. Treat this as UI guidance; revalidate on submission.
Do not accept client-supplied canonical repository IDs, provenance, success state, or actor IDs as proof.
Safe reference authorization remains distinct from the later credential-dependent association read.
Unsafe or unauthorized references reject submission and preserve the draft under the existing contract.
A valid reference with unavailable association credentials can still be accepted as context and report association failure.
The server projects `change_request_registrations` beside references in messages and queue rows. Each result contains:

- Stable `registration_id`, reference `ref`, and source admission identity/revision.
- State: `pending | linked | failed | suppressed | context_only | uncertain`. Uncertain means a mutation can have committed without confirmation.
- Resolved provider/host/repository/number and association ID when available.
- Structured error code, retry capability, and current association revision.
- Origin kind, nullable message/session pointers, and a bounded origin timestamp.

The reference snapshot remains immutable. Current association state is a separate projection, so unlink cannot leave a misleading live “Linked” label.

## Identity and provider support

Resolve only after task/workspace authorization. Match URL identity to the persisted task repository set, not all repositories visible to the user.
Require a unique `repositories.ID`, workspace match, verified provider origin, repository path, and positive number.
Provider-returned identity must agree with the requested identity before persistence.
For built-ins, verify the immutable target repository ID: GitHub base repository database ID or GitLab project ID, normalized as decimal text.
Fetch missing target identity through the authenticated provider client after acceptance. Missing identity never falls back to a path-based association key.
Never parse a number from a title or treat a provider node ID as a PR number.

| Provider | Association capability in this package | Resolution and fallback |
| --- | --- | --- |
| GitHub | Selected mentions and agent URL registration | Current supported GitHub origin; canonical URL owner/repo must match an attached repository. Preserve current host restrictions; no new Enterprise support claim. |
| GitLab | Selected mentions and agent URL registration | Match configured workspace host, full subgroup/project path, and IID; use existing host-aware parser and repository validator. |
| Azure DevOps | Context only | `AssociateTaskPR` and REST creation exist, but neutral list/manage and exact unlink parity do not. Do not advertise support from search capability alone. |
| Compatible registered plugins, including Bitbucket/Forgejo adapters | Selected mentions and agent URL registration | Active owner, versioned backend actions, exact scope/repository identity; reuse plugin-owned association state and review projections. |
| Search-only, legacy, disabled or incompatible plugins | Context only/unavailable | Preserve authorized context and explain missing capability; repository/search registration alone grants no association support. |

For a fork PR, the repository owning the PR number is the target/base repository from its URL.
An attached head fork does not authorize assigning the upstream number to that fork's repository ID.
If only the head fork is attached, report `repository_not_attached`. If the exact target repository is also attached, link that identity.
A foreign repository visible in workspace search remains valid context but cannot bypass the association contract.
Two matching persisted repository records produce `ambiguous_repository`; no first-match fallback is allowed.

Use existing provider parsers and origin checks. Reject userinfo, unsafe schemes, unexpected ports, ambiguous encoded separators, and host-conflicting redirects.
Normalize known URL suffixes through provider rules; never follow arbitrary redirects to infer identity.
No network request reaches an origin solely because a URL names it.

## Agent tool extension

Extend `manage_task_change_request_kandev`; do not add another MCP resolver or link tool.
Keep `operation` mandatory. The provider field accepts built-ins and validated active registration-provider IDs; it is not a hardcoded GitHub/GitLab enum.
The registry, not a free-form string, determines support. See the plugin design for string change identifiers and connection scope. Add optional `url`, `association_mode: observation`, `restore_detached: boolean`, and `operation_id`.
Keep optional `task_id` as a legacy target selector. When absent, use the trusted calling task.
Always derive caller task/session, user, and workspace from authenticated context. Apply existing reachable-target checks to an explicit target.
Reject supplied caller/session/workspace authority fields at the public tool boundary.

Rules:

1. Without URL, retain the complete provider/repository/number tuple. Never accept number alone.
2. With URL, derive contribution identity from the URL. Supplied tuple fields are assertions and must agree; reject any conflict before mutation.
3. URL operations default to observation mode. Legacy tuple calls without the new mode preserve established explicit management behavior.
4. New tuple-based agent guidance must pass `association_mode: observation`.
5. `replace` retains complete `old_*` identity, same-provider restrictions, new-link-first order, and compensation guarantees. URL identifies only the new target.
6. `unlink` can resolve URL against stored active/detached identity even after repository detachment. It requires no provider fetch. Ambiguous identity fails closed.
7. Observation registration defaults `restore_detached` to false. Only an explicit relink sets true. Legacy explicit tuple linking keeps its established restoration semantics.
8. Mode and restoration fields are valid only for link/replace. Reject unknown fields, nulls, fractional numbers, and conflicting shapes in both server and backend handlers.

Preserve the portable JSON Schema subset: a closed object root with handler-level cross-field rules, without root combinators.
Preserve `links`, `state_known`, and failure/rollback fields. Add `registration_status`, resolved identity, association revision, and structured error code.
`get_task_change_requests_kandev` remains current-task-bound and adds provenance plus observation policy to existing items.
An explicit-target caller verifies with the management response; the current-task read tool does not silently change its target.
Provider-read failure retains incomplete-result semantics. A successful write with failed readback reports uncertain state, never a false failure-free empty set.

## Shared guidance

Inject one shared block when the effective MCP catalog supports registration. Preserve mode restrictions and existing system markers.
Use `FormatKandevContextWithOptions` and the shared prompt assembly path, including launch/resume and later-turn context.
Test supported harness paths and restricted/config/passthrough modes. Do not inject unavailable-tool instructions into those modes.

Proposed guidance:

> Register every PR/MR you create or actively review, fix, or update for this task. Use manage_task_change_request_kandev with operation link and the full URL when work starts. Register every relevant stack layer. Use observation mode if you supply explicit identity fields. CLI/API creation, including gh pr create, does not register a task link; register its returned URL. Background-only references need no link. Respect context-only selections and explicit unlinks. Do not restore a detached link unless the user intentionally requests relinking. Before concluding PR work, call get_task_change_requests_kandev and register missing relevant contributions. Report any failed or suppressed association accurately. Registration does not authorize monitoring, automatic fixes, merging, or task advancement.

Expose context-only intent in the sanitized reference block. It applies to that background reference, not an irrevocable ban on later explicitly requested work.
Explicit unlink is stronger: ordinary missing-link checks report suppression and do not relink.
Do not append t3code's watch/babysit instructions. Existing automation tools remain the sole authority for automation.

## Admission

The common semantic boundary is durable acceptance, not agent dispatch. Queue acceptance counts as submission.

| Path | Acceptance boundary | Before commit | After commit |
| --- | --- | --- | --- |
| First ordinary task-chat message | `CreateMessageIdempotent`/initial-brief transaction, including queued first-message branch | Validate guards; persist selected intents with description snapshot and prompt index | Process committed intents |
| Later direct message or steer | Existing durable user-message transaction | Validate guards; persist selected intents | Process registration independent of dispatch success |
| Queue insertion or automatic fold | Queue row + admission receipt transaction | Check incarnation, capacity, attachments and barriers; persist contributing intents | Process all committed intents, including folded origins |
| Queue-edit Save | Lease/revision/operation-ID guarded save transaction | Persist only new selection intents; retain unchanged intent IDs | Process committed new intents; Cancel has no effects |
| Queue dispatch/manual merge | No new registration admission | Retain existing leases and reservations | Carry source intent IDs; never scan text to create new intents |
| Plain initial task description | No structured picker admission today | Preserve existing editor scope | Agent registers relevant work when it starts |

Include intent fields in direct and queue request fingerprints. Exact receipt replay returns the same intent IDs before mutable provider checks.
A changed payload under the same admission identity fails. Existing title ownership and first-brief selection stay unchanged.
Queue folds preserve contributing IDs independently from display-reference deduplication. Context-only selection never cancels another already accepted link intent.
A saved removal of a reference prevents new intent only. It does not reverse an earlier accepted intent or association.
After acceptance, cancellation stops execution under existing rules but does not revoke the submitted association request.
Task deletion cancels pending registration and removes its records. Session/message deletion preserves established links and tombstones, with nullable origin pointers.

## Persistence and durable processing

Keep `github_task_prs` and `gitlab_task_mrs` as the built-in association source of truth.
Plugins retain their existing association storage; do not copy plugin links into either table.
The atomic SQL commit described below applies to built-ins. Plugin writes use the receipt/reconciliation protocol in the [plugin design](task-change-registration-plugins.md). Add no neutral association graph or mirror table.
Extend both provider rows with canonical host, connection scope, immutable provider repository ID, path snapshot, comparison policy, revision, and first-origin fields.
The comparison policy is `legacy | preserve`.
GitHub retains `source` and `detached_at`; add source values `selected_mention` and `agent_registration`.
GitLab adds equivalent source/detachment fields. Its public unlink becomes soft detach plus existing exact watch/automation cleanup.
Active-list and sync queries exclude detached rows. Internal exact lookup includes them.

Canonical uniqueness is `(task_id, provider, host, connection_scope, provider_repository_id, change_request_id)`.
Built-in `connection_scope` is `builtin:<workspace_id>`, the existing workspace integration namespace. Credentials and credential rotation do not change this scope.
Plugin scopes follow the plugin design. Built-in change IDs are positive PR numbers or MR IIDs, normalized as decimal text.
Keep `repository_id` as the authorized Kandev binding. Names, URLs, and path snapshots are locators and display data, never uniqueness keys.
Preserve one row across a verified rename with the same provider ID. A reused path with a different ID cannot inherit links or tombstones.
Reject a fetched ID that conflicts with the attached repository's verified identity. Repository retargeting requires the existing explicit repository flow.
Migrations perform no remote reads. Backfill only verified immutable IDs already available locally. Leave other rows unresolved and removable by association ID.
An unresolved row blocks automatic writes for its existing task/repository/number binding until authorized repair proves identity.
Do not infer an old ID from today's occupant of its path. Detect uniqueness collisions and fail with an actionable diagnostic.

Add one bounded-purpose `task_change_registration_intents` table in the shared SQL database.
Plugin intents additionally retain the owning installation identity, contract version, immutable connection identity, and provider operation receipt; no provider secrets are stored.
It is an acceptance receipt and recovery record, not a second association store.
Fields: intent ID; task/workspace; source kind; nullable session/message; queue admission/edit ID; source revision; selection ID; canonical reference snapshot; nullable resolved identity/revision; state; bounded error code; attempt timestamps.
Unique origin key: task + source kind + admission/edit ID + source revision + reference ref.
Also enforce `(task_id, selection_id)` uniqueness for selected references. Unchanged edits retain their original intent and origin.
Use existing `client_message_id`, `client_queue_id`, and queue-edit operation IDs. Legacy callers receive a server intent ID only after commit; they gain no transport replay guarantee.
Agent calls use a trusted request/invocation identity where available; exact association upsert remains safe when callers retry with a new request ID.
Persist the trusted invocation identity and fingerprint before mutation for explicit relink calls. A replay must resolve its original revision.
Explicit observation-mode restoration requires `operation_id`, bound server-side to caller, target, identity, fingerprint, and expected revision.
It is an idempotency key, never authority. Exact retries retain the key; changed-payload reuse fails.
Other operations can omit it and keep current compatibility. Browser relink uses the same contract through its retry endpoint.

Write intent rows in the same transaction as accepted message/queue work. Provider HTTP calls never run inside that transaction.
After commit, request bounded processing through the existing service lifecycle. Also recover pending intents on startup and relevant authorized read/retry.
Do not introduce a polling daemon, agent run, or independent lifecycle controller.
Process a bounded batch with per-provider timeouts. Transient failures settle as retryable; explicit Retry schedules only that intent.
A crashed `pending` attempt is safe to recover. Preserve the admitting principal ID without credentials; reauthorize access and use current credentials on every retry.

After a provider read, commit the exact association, origin, revision, and intent result in one shared SQL transaction.
Expose transaction-aware provider writes through a narrow internal seam. SQLite and PostgreSQL use existing transaction/rebind conventions.
Lock/revalidate task existence and exact association revision before this commit. A failure commits neither success state nor association.
Publish only after commit. A failed publication cannot reverse a durable success; replay/read recovery supplies state.
Keep provider lifecycle snapshots and existing automation options on already-active rows. Do not overwrite provenance during normal status sync.
First-origin fields explain the link; subsequent mention results point to the same association without changing its first origin.
Retain intent receipts while the task exists, even when message history is deleted. Purge on task deletion; omit prompt bodies and credentials.

## Unlink ordering

Use monotonically increasing association revisions, not timestamps, for concurrency.
Soft unlink increments the exact row revision and retains its canonical identity. Unlink of an absent known identity creates a minimal detached tombstone.
This closes the race where unlink commits while the first provider read remains in flight.
Both operations use the same unique identity and transactional lock/upsert rule.

At admission, capture the current revision when identity resolves.
For built-ins with unresolved immutable identity, retain a null expected revision and prohibit restoration during recovery.
An unresolved-origin retry can attempt first registration after identity repair, but cannot restore any existing tombstone; restoration requires Link again. If identity cannot resolve locally, record failure for explicit recovery; do not queue a blind write.
For pending automatic work, commit only if the expected revision still matches. An active same-identity concurrent winner is an idempotent success.
A newer detached revision yields `suppressed`; it never becomes an automatic retry.
Automatic detection and status sync cannot clear detached state or change `comparison_policy`.

A new selected mention after unlink carries a new admission and explicitly permits restoration at the observed detached revision.
The composer states “Link again on send” when known. Built-in ordering uses the admission transaction, never draft creation time.
Plugin revisions use the conservative resolution fence in the plugin design. An unavailable or stale fence requires a fresh Link again action.
A selection accepted before unlink loses, even if its provider read finishes later.
Ordinary Retry retains the old expected revision. Link again creates a new intent after fresh authorization and explicit user action.
URL agent registration defaults to suppression. An authorized deliberate agent relink uses `restore_detached: true` and a fresh request.
Replays of that request still retain its original expected revision and cannot undo a subsequent unlink.

## Observation and downstream effects

Current GitHub and GitLab URL association calls reconcile comparison targets.
GitHub's sync also calls `reconcileComparisonTargetFromSync`; merely skipping the first call is insufficient.
The task reconciler can clear manual overrides, persist a comparison target, reset session base SHAs, and push targets into running executors.
Those targets can cause comparison-remote fetches and cache invalidation without changing checkout.

New registration calls use typed observation options. Persist `comparison_policy=preserve` on new/restored rows from these paths.
All association, discovery, refresh, and orphan-sync callers consult that policy before comparison reconciliation.
Do not downgrade an existing link's policy or settings when another mention registers it.
Legacy explicit manual link and remote-review setup keep current comparison behavior. A later explicit comparison action follows the existing task contract.
Unlink clears only a comparison binding actually owned by that exact association; mention-created links never acquire such a binding automatically.

`github.task_pr.updated` starts CI evaluation. `gitlab.task_mr.updated` starts lifecycle evaluation.
Therefore, new observation links must persist all five per-association automation switches as false before publishing.
Do not inherit task-wide historical defaults. Ordinary status subscriptions can remain active, but they do not authorize agent wake or workflow changes.
Audit event, scheduled reconciliation, merged/closed, and options-updated paths. Each execution effect must read existing exact-association authority.
Existing automation actions can later enable those switches explicitly. This does not change `comparison_policy`.
A repeat link to an active association preserves any authority already granted; it neither enables nor disables it.

## Events and recovery

Reuse `github.task_pr.updated/deleted` and `gitlab.task_mr.updated/deleted` for built-in provider list/count state.
Plugin results invalidate the existing `ReviewProviderRegistration` task/workspace association subscriptions; see the plugin design.
Do not emit synthetic GitHub/GitLab rows for plugin associations.
Extend their payloads with association revision and policy/provenance. Detached results use existing deletion semantics.
Update `apps/web/lib/ws/handlers/github.ts` and `gitlab.ts`, their store slices, and provider projections through normal event flow.
Increment revisions for every committed provider-row mutation and include the committed revision in deletion events.
Clients ignore older association revisions, including success arriving after unlink. Hydration carries revisions and clears stale state through existing workspace generations.

Add a scoped `task.change_request_registration.updated` projection event for per-reference results, not another association event stream.
It contains task/workspace, origin IDs, intent ID, state and result revision. Message/queue read APIs include the same durable results.
A successful HTTP/WS response can reconcile the same revision if the event arrives late. A replayed queue receipt must not insert a phantom queue row.
Use existing message/queue refresh on reconnect; retrieve registration state with those reads, even after queue folds.
Never increment counts from optimistic reference status. Provider association state owns counts.

Add `POST /api/v1/tasks/:task_id/change-request-registrations/:id/retry` with `{action: retry | relink}` and an idempotent operation ID.
It authorizes task and intent, refreshes exact identity, and invokes the shared coordinator. `relink` creates a fresh intent; `retry` preserves the original one.
Unsupported targets return a typed explanation. No endpoint attaches a repository or grants credentials.

## UI and accessibility

Reuse `EntityReferenceChip`, its TipTap node, existing popover/menu, phone Drawer, provider indicators, and `@kandev/ui` tokens.
Desktop chip disclosure offers Link on send / Context only. The selected mode remains visible without hover.
Phone disclosure opens a touch drawer with the same choices and a Done action. Escape/Cancel closes without submission and restores focus.
Do not change picker keyboard selection or Enter-to-send semantics.

Submitted chips show Linking, Linked PR/MR, Context only, Not linked, or Unlinked beside the portable reference.
Failure details distinguish credentials, unavailable provider, unsupported scope, ambiguous identity, and persistence failure.
Retry never resends the message. Suppression offers Link again. Unsupported references offer Open reference and an explanatory Link action; disable impossible submission.
Queue editor copy states that accepted links remain until explicitly unlinked, even when the queue item is removed.

Existing task PR/MR lists expose first source and origin navigation in their details. Missing/deleted messages show “Original message unavailable”.
Use repository plus number in names, not number alone. Use polite live announcements for final per-reference results; avoid repeated announcements on replay.
Status uses text and icons, not color alone. Keep Tab order predictable and keyboard focus on the retry control after failure.
Use localization keys in all seven supported locales, plural counts, and pseudo-locale checks. Provider titles remain escaped data.
Phone controls have at least 44-pixel targets, safe-area clearance, internal drawer scrolling, and no horizontal page overflow.

## Compatibility, rollout, and evidence

Expand schema and server support before clients emit intent. Old clients continue context-only submission and explicit legacy association behavior.
No historical association scan or replay backfill occurs. Existing built-in association rows default to legacy comparison policy. Plugins without the new contract keep existing manual flows and context-only mentions.
Compatible plugin capability discovery also governs tool visibility and read completeness; built-in automation capabilities do not transfer to plugins.
Do not promote requirements/design statuses until all work orders pass in a later implementation turn.
At implementation, amend the existing MCP contract's required fields and link the observation exception from the comparison and composer contracts.
Preserve prior plan verification records; add this package as a successor reference without reopening completed work orders.

Log bounded stage/error/source fields with task/intent correlation in logs only. Do not log provider text, credentials, or entire prompts.
The [validation matrix](../../../plans/pr-mention-linking/validation-matrix.md) defines acceptance evidence and the [plan](../../../plans/pr-mention-linking/plan.md) defines delivery.
