# PR/MR registration investigation

Date: 2026-10-09. This is source evidence for the draft package, not runtime test evidence.

## Workspace and scope

- Kandev: `/root/.kandev/tasks/kandev-is-my-product_5n2z7gip/kdlbs-kandev`, HEAD `a458174b023b33fe16c755bfed762025bf3706a5`.
- t3code: sibling `pingdotgg-t3code`, HEAD equals pinned `611132c171f3a821bd2e32f22261135cef6330ac`.
- Kandev task: `01f3df73-d3cd-405e-be79-f09b1a262a3e`; session: `0a3edad5-4e5a-4679-876d-778e85288c84`.
- Sibling `a3d0b82a-8d8e-4436-a319-8e472d8db11c` owns delegated work. Its files remain untouched.
- The source workspace also contained t3code analysis for task cards and checkpoints. It is outside this package and is not an implementation dependency.

No new task, session, native agent, production change, or permanent test was created for this package.

### Destination review

The package was copied into this task workspace and reviewed against Kandev HEAD `3fed5570cec533f468c25ed03c84967bdc972588`.
The source paths and revisions above remain historical evidence. This package uses the pinned source references below without the unrelated analysis files.
`task/models.Repository` has `ProviderRepoID` and `ProviderScope`. Its identity comment confirms that legacy built-ins still use host/owner/name.
Implementation must therefore verify immutable built-in IDs instead of assuming that every existing repository already supplies them.

## Plugin extension findings

The follow-up investigation corrects the original omission of plugin capability. Current source already has a live plugin mention bridge.

| Existing seam | Implication |
| --- | --- |
| `apps/backend/internal/plugins/mentions.go`: `MentionSourceBridge` | Active manifest reference sources join the same autocomplete and reauthorize submission. This is implemented, not hypothetical. |
| `internal/plugins/repository_provider_inspect.go` and `repository_provider.go` below `apps/backend` | Reuse active owner resolution and bounded `InvokeAction` dispatch; no Bitbucket/Forgejo parser belongs in the host. |
| `apps/backend/internal/plugins/action_handlers.go`, SDK `VerifiedActionContext` | Reuse authenticated, verified workspace/task selectors for backend registration; body fields are not authority. |
| [Plugin mutation ADR](../../decisions/2026-08-10-plugin-change-request-mutations.md) and [Plugin API](../plugins/PLUGIN-API.md) | Plugins own link storage/mutation and provide native review snapshots, task indicators and unlink callbacks. |
| [Bitbucket design](../../specs/integrations/system-design/bitbucket-plugin-02.md) | Existing manual link/unlink and branch detection must share tombstones with registration; preserve connection/context-root identity. |
| `apps/backend/cmd/plugin-fixture/` and packaged-plugin E2E tests | Use disposable conformance fixtures for this host capability; no production plugin implementation in the monorepo. |

Missing today: a negotiated, backend-callable registration contract for shared message/MCP paths. Frontend review callbacks cannot implement headless admission.
The [plugin design](../../specs/integrations/system-design/task-change-registration-plugins.md) specifies three versioned actions over existing HandleAction, not new MCP tools.
Plugin persistence has no shared SQL transaction with host admission. Operation receipts, revisions and exact readback replace the built-in atomic-write assumption.
Specific installed Bitbucket/Forgejo plugin revisions were not inspected; production adoption must declare and satisfy the new contract.


## Assumption check

Confirmed by the brief: default selected-reference linking; context-only choice; agent registration; exact identity; explicit unlink; observation-only effects; plan-only handoff.
Verified from code: provider-neutral MCP mutation exists; references reach prompts; association events reach UI and automation; comparison reconciliation has runtime effects.
Delegated design choices: acceptance means durable submission, including queue insertion/save; no repository auto-attachment; built-in GitHub/GitLab coverage plus the user-authorized compatible plugin extension; URL ergonomics extend the existing tool.
The user subsequently requested plugin support. The expanded package includes it through capability negotiation and existing plugin-owned associations.
No material product question remains. Implementation must verify transactional race handling and legacy migration with the work-order tests.

## Contract ownership and prior packages

Integrations owns provider-qualified task associations. UI and task admission are consumers of that contract.
This package adds admission intent and registration UX; it does not duplicate the established management API requirements.

| Existing contract/package | Relationship |
| --- | --- |
| [Task change management](../../specs/integrations/requirements/task-change-link-mcp.md) and [design](../../specs/integrations/system-design/task-change-link-mcp.md) | Preserve mutation/read/authorization guarantees; add URL/default target and observation options. |
| [Neutral MCP plan](../provider-neutral-change-request-mcp/plan.md), [original association plan](../manage-task-pr-and-m/plan.md), [identity repair](../mcp-pr-link-single-user/plan.md) | Existing implementation evidence remains unchanged; later implementation adds successor links and reconciles API examples. |
| [Entity reference requirement](../../specs/ui/requirements/entity-reference-composer.md) and [design](../../specs/ui/system-design/entity-reference-composer.md) | Preserve picker, validation, snapshots, Markdown fallback and editor scope. New intent is explicit and additive. |
| [Queue admission](../../specs/tasks/system-design/queue-admission.md) and [queue edit](../../specs/ui/system-design/message-queue-edit.md) | Reuse receipt/fingerprint and edit lease/revision boundaries. |
| [Initial brief](../../specs/tasks/system-design/initial-task-brief.md) | Keep title ownership, first prompt selection, and description snapshot. |
| [Provider-aware automation](../../specs/integrations/requirements/provider-aware-review-automation.md) | Tool availability does not grant unsupported provider or execution authority. |
| [Unlink menus](../../specs/integrations/requirements/github-pr-unlink-menus.md) and [sync coordination](../../specs/integrations/requirements/github-task-pr-sync-coordination.md) | Reuse exact association controls and stale-response handling. |
| [Remote contributions](../../specs/tasks/system-design/remote-contribution-tasks.md), [comparison ADR](../../decisions/2026-08-19-repository-qualified-comparison-targets.md) | Preserve explicit review setup; add a persistent observation-only exception. |

The existing MCP design contains an outdated example naming retired provider-specific tools. The new package uses current source names.
Correct that example with the URL/schema work; do not treat it as authority to reintroduce retired tools.

## Kandev source findings

All paths in this table are repository-relative.

| Source | Verified behavior and design consequence |
| --- | --- |
| `apps/backend/internal/mcp/server/server.go` | Registers manage/get/automation tools; management currently requires task/provider/repository/number. Extend this schema, not the catalog with another tool. |
| `apps/backend/internal/mcp/handlers/task_change_link.go` | Validates operation, scope, explicit identity and replacement. Keep backend checks independent from agentctl validation. |
| `apps/backend/internal/backendapp/task_change_link_coordinator.go` | Dispatches GitHub/GitLab; `taskRepository` requires membership in the task and same workspace. GitHub identity uses trusted user or disabled-auth fallback. |
| `apps/backend/internal/github/service_pr_watch.go` | Exact repository/number lookup includes detached rows. URL link restores detached rows and reconciles comparison even on an existing-row return. |
| `apps/backend/internal/github/models.go`, `store.go` | `TaskPR` has source and detached state. Existing identity preserves multiple PRs; do not replace siblings. |
| `apps/backend/internal/github/service_pr_unwatched.go` | Existing orphan sweep observes unwatched links. Use it; no registration-specific polling daemon. |
| `apps/backend/internal/gitlab/service_task_mr_link.go`, `store.go` | Configured-host URL parser validates returned MR identity. Association reconciles comparison. Unlink deletes row and exact automation/watch state. Add tombstone behavior. |
| `apps/backend/internal/azuredevops/service_task_pr.go`, `controller.go`, `store_task_pr.go` | REST association and persistence exist. Current neutral coordinator still supports only GitHub/GitLab; no complete exact neutral unlink path. |
| `apps/backend/internal/task/service/service_comparison_target.go` | Matches exact head repository and branch; explicit calls can clear manual comparison override and replace the target. |
| `apps/backend/internal/task/service/service_comparison_target_projection.go` | Reconciliation resets session base SHAs, publishes task state, and pushes comparison targets. Executor comparison can fetch a remote. |
| `apps/backend/internal/orchestrator/event_handlers_github.go`, `event_handlers_gitlab.go` | Association update events start automation evaluation. New links need explicit false switches and authority checks before event publication. |
| `apps/backend/internal/task/handlers/message_handlers.go` | Reference validation precedes creation. `CreateMessageIdempotent` and queued feedback paths define durable acceptance; dispatch follows persistence. |
| `apps/backend/internal/task/handlers/message_handlers_initial_task_brief.go` | Initial brief adds validated reference context to the composed snapshot. Reuse its transaction; do not mutate description/title ownership. |
| `apps/backend/internal/orchestrator/message_meta.go` | `WithEntityReferences` snapshots references; `EntityReferenceContext` warns titles are untrusted. No association call exists here. |
| `apps/backend/internal/orchestrator/handlers/queue_handlers.go`, `messagequeue/` | Queue receipt identities, entity-reference metadata, leases and revisioned saves already exist. Dispatch/send-now carry references. |
| `apps/backend/internal/mentions/provider_github.go`, `provider_gitlab.go`, `provider_azure.go`, `internal/entityrefs/` | Registry-owned validation and opaque refs provide context identity; provider node IDs are not interchangeable with PR numbers. |
| `apps/backend/internal/sysprompt/sysprompt.go` | Shared Kandev context and options are the injection seam. Preserve restricted-mode tool visibility and barriers. |
| `apps/web/components/task/chat/use-entity-reference-composer.ts`, `messages/entity-reference-chip.tsx`, `tiptap-entity-reference-extension.tsx` | Picker selection inserts reference data. Add a visible per-reference choice without changing picker send semantics. |
| `apps/web/hooks/message-request.ts`, `use-message-handler.ts`, `hooks/domains/session/use-queue.ts` | Carry structured reference requests and stable admission identity; extend fingerprints and recovery, not browser-side attachment calls. |
| `apps/web/lib/api/domains/github-api.ts`, `hooks/domains/github/task-pr-mutations.ts`, `use-task-pr.ts` | Existing association/unlink API and workspace-generation guards remain the task-list path. |
| `apps/web/lib/ws/handlers/github.ts`, `gitlab.ts` | Existing provider events update task stores. Add revision/provenance data and reuse projection consumers. |

## Pinned t3code comparison

Source base: `https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/`.
Local HEAD matched this revision. The following source files were inspected locally.

| Pinned source | Pattern to reuse or distinguish |
| --- | --- |
| [RuntimeInstructions.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/server/src/provider/RuntimeInstructions.ts) | Registers every created/worked PR and stack layer, excludes background, verifies final linked set, reports failure. Its monitoring guidance is outside this feature. |
| [tools.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/server/src/mcp/toolkits/pullRequests/tools.ts) | URL or repository/number; calling-thread default; distinct watch/unwatch operations. |
| [handlers.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/server/src/mcp/toolkits/pullRequests/handlers.ts) | `resolveTarget`, authoritative invocation context, idempotent link dispatch. URL wins outright there; Kandev instead rejects conflicting explicit assertions. |
| [threadPullRequest.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/packages/contracts/src/threadPullRequest.ts) | Persistent identity/source/snapshot and optional watch state. Kandev reuses its existing provider stores. |
| [Orchestrator.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/server/src/orchestration-v2/Orchestrator.ts) | Separate link and watch commands mutate durable state. No event-sourcing rewrite is needed in Kandev. |
| [usePullRequestLinking.ts](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/web/src/hooks/usePullRequestLinking.ts) | Capability-aware manual linking, exact identity and failure handling. |
| [ThreadStatusIndicators.tsx](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/web/src/components/ThreadStatusIndicators.tsx), [Sidebar.tsx](https://github.com/pingdotgg/t3code/blob/611132c171f3a821bd2e32f22261135cef6330ac/apps/web/src/components/Sidebar.tsx) | Linked-set indicators and navigation motivate visible results. Kandev already has provider-specific task lists. |

Automatic association of selected composer mentions is this Kandev proposal. These sources do not establish that t3code implements identical composer behavior.

## Decisions and rejected alternatives

- Link at submission, not dispatch: this matches the visible Send promise and keeps queued work discoverable. Explicit unlink handles later removal.
- Persist a narrow intent receipt: it closes the acceptance/write crash gap. A fire-and-forget frontend call cannot do that.
- Persist observation policy: a one-call flag cannot prevent later sync from retargeting comparison.
- Keep provider rows: a generic contribution graph adds another authority and migration burden without a required outcome.
- Reject conflicting URL assertions: silently preferring either input can link the wrong repository.
- Keep unsupported repositories as context: registering a repository implicitly changes the task's workspace and exceeds association authority.
- Use immutable provider repository IDs: path snapshots alone cannot distinguish a renamed repository from a different repository at its old path.
- Preserve selection IDs: comparing only final reference values loses deliberate reselection inside a queue edit.
- Retain nullable plugin revision fences: provider outages must not reject an authorized message. Recovery cannot gain restoration authority from later observations.
- Suppress stale plugin fences: cross-store admission cannot atomically observe unlink. A fresh Link again is safer than advancing a retry's revision.

The design records these feature-scoped alternatives. No new global ADR is necessary; existing comparison and admission decisions remain linked dependencies.
