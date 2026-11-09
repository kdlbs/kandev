---
status: draft
system: agents
created: 2026-11-08
requirements:
  - REQ-AGENTS-REMOTE-AUTH-ADMISSION-001
owners:
  - kandev
---

# Remote agent authentication admission design

## Ownership and mapping

Agents owns authentication method declarations and profile evidence.
Tasks supplies the actual executor selection, authorized actor, candidate profile mapping, and workflow guards.
Executors retains compute authorization and credential provisioning.

| Criteria | Design sections |
| --- | --- |
| AC-AGENTS-REMOTE-AUTH-ADMISSION-001.1, .2 | Catalog and effective settings |
| AC-AGENTS-REMOTE-AUTH-ADMISSION-001.3, .5 | Actor and authorization boundary; Workflow integration |
| AC-AGENTS-REMOTE-AUTH-ADMISSION-001.4, .7 | Provisioning and compatibility |
| AC-AGENTS-REMOTE-AUTH-ADMISSION-001.6 | Settings surface |

The [evidence record](../../../plans/claude-profile-admission/evidence.md) pins the investigation to source and upstream heads.
The [proposed decision](../../../decisions/2026-11-08-profile-auth-admission-evidence.md) records the credential-proof alternatives.

## Current mismatch

`backendapp.taskAgentExecutorCompatibilityValidator` receives the concrete agent profile but passes only the executor profile to `validateRemoteAgentCredentials`.
The latter accepts selected file IDs or nonempty `remote_auth_secrets` IDs.
It neither reads profile environment entries nor verifies secret usability.
`ClaudeACP.RemoteAuth()` currently advertises only `CLAUDE_CODE_OAUTH_TOKEN`.

Launch already resolves profile environment entries through `environment.Resolve` and lifecycle's scoped secret store.
`sshRemoteAgentEnv` forwards an explicitly resolved `ANTHROPIC_API_KEY`.
Thus a working API-key launch can fail the earlier workflow compatibility check.

## Catalog and effective settings

Add an env method for `ANTHROPIC_API_KEY` beside the existing OAuth method in `agents/claude_acp.go`.
Its generated method ID is `agent:claude-acp:env:ANTHROPIC_API_KEY`.
It has no OAuth setup script, file target, or new English label/hint.
The existing method ID and OAuth script remain stable.

Extend the backend compatibility validator to consume both selected profiles and a narrow credential reader.
Wire `secrets.NewUserVisibleStore(repos.Secrets)` at construction in `backendapp/services.go`.
Do not use the raw store, a host credential manager, or a process environment lookup as admission evidence.

Reuse `runtime/environment.Definition`, `Validate`, and `Resolve` for the selected profiles' environment declarations.
Preserve `OriginAgentProfile` and `OriginExecutorProfile`, including existing tier precedence and secret-conflict vetoes.
Do not compare decrypted values to resolve conflicting secret IDs.
Reuse profile entry shape rules: valid keys, no duplicate keys, and no simultaneous literal and secret value.
Extract a small shared validator only where existing private validation prevents reuse.

Decode `remote_credentials` and `remote_auth_secrets` before accepting any route.
Malformed JSON, incorrect value types, and malformed selected-family method references fail closed.
An unknown method never supplies positive evidence. Unrelated agent methods remain unrelated to the selected family's authentication proof.
Null or empty secret selections are unconfigured, not valid credentials.

Validate Global scope through `secrets.ValidateGlobalReference` on the user-visible store.
Resolve through that store's scoped Global reveal path to establish a nonempty usable value.
The same store already enforces internal-ID containment and caller visibility.
A store read or decrypt failure becomes a sanitized validation failure.
Do not introduce an ID-presence fallback when the store or actor is unavailable.

The admission result contains readiness or a stable failure class. It contains no resolved environment, secret ID, or credential value.
Resolved values exist only within the bounded validation call and are neither cached nor returned.
No provider process or network authentication is needed.

Quantifiers are deliberate:

1. All effective profile environment bindings required by the strict launch must resolve.
2. All explicitly configured authentication bindings for the selected agent must validate, including mixed valid/invalid methods.
3. After those checks, at least one selected file route or nonempty resolved catalog env method must exist.

Recognized profile env methods can satisfy the third condition without duplicating a secret on the executor profile.
Existing authorized literals remain ordinary profile inputs. New UI copy must recommend stored secret bindings for credentials.
Repository-only credentials and arbitrary env names do not qualify through this profile-only compatibility interface.
The final launch resolver remains authoritative for complete task/repository composition and managed runtime values.

## Actor and authorization boundary

Before the compatibility callback reads credentials, Tasks must authorize the resolved executor and its actual parent/profile ownership.
Use the existing `AuthorizeExecutorConfiguration` and executor identity machinery.
The service must establish a valid current actor through `executorCaller` and carry that identity into credential reads.
Retain active-user checks and caller precedence. Never substitute the task owner, workspace owner, or session creator for a denied caller.

An absent identity must not become the secret store's unscoped internal context.
Authentication-disabled operation requires the existing, positively established synthetic single-user identity.
Do not infer that mode from a missing identity.
For background callers, only existing explicit durable-actor resolution can supply identity.
Keep the existing local and methods-less branches intact when no remote credential proof or secret read is required.

Keep this preparation read-only. `PrepareExecutorAdmission` can perform admission writes and is not a preview helper.
Do not import lifecycle internals into Tasks or backendapp.
Use the public runtime environment package and the existing task authorization boundary.

Draft PR [#5184](https://github.com/kdlbs/kandev/pull/5184) is not a dependency.
This design admits the caller's currently accessible binding and denies another user's private key.
If owner grants later land, reconcile their exact binding resolver contract before extending admission.
Never recreate those grants or treat a shared profile as secret-use authority here.

## Workflow integration

`normalizeWorkflowChangeAgentOverrides` validates destination defaults as well as explicit replacements.
Keep grouping, one validation per concrete profile per request, source-profile error attribution, and task-selected executor resolution.
Authorize before the new secret reads in both this path and `validateWorkflowAgentOverrideProfiles` for creation.
No successful decision is reusable across actors or requests.

`ValidateWorkflowChange` and `MoveTaskWithOptions` continue to use the shared candidate-map preparation.
Preview does not persist that candidate. Apply repeats validation against current bindings.
Existing source workflow/step/timestamp guards and transactional writes remain intact.
The runtime separately revalidates credentials after a queued or deferred launch.

An API key's presence changes no routing decision.
Empty `agent_overrides` retains destination defaults. Explicit replacements remain task-owned.
Fresh Review and QA sessions remain independent when their configured policies require them.
Source conversation history and shared workflow/profile definitions remain unchanged.

## Provisioning and compatibility

`executor_ssh_credentials.go:resolveAuthSecrets` maps a selected method to its declared env variable.
`sshRemoteAgentEnv` obtains credentials solely from the resolved request environment.
Cover both agent-profile env and explicit executor-method routes through the actual SSH child configuration boundary.
An API-key-only request must never run the OAuth setup script or write `.credentials.json`.
An existing OAuth request retains its established script conditions and credential variable.
Selected file-method behavior for other agents remains supported without inventing a Claude file method.

The shared backend validator also serves other remote executor types.
Keep local/worktree behavior, dynamic-profile gating, virtual-agent rejection, and methods-less agent behavior intact.
Shared catalog tests cover other providers. Real provisioning evidence in this package is limited to the SSH route.
Do not claim Docker, Kubernetes, Sprites, or plugin runtime authentication parity from an SSH test.

Previously accepted bogus or inaccessible executor secret IDs will now fail admission.
That tightening is intentional. There is no database migration or profile rewrite.
Provider-side revocation cannot be detected through a local secret read.
Provider authentication failures remain execution-time errors and must not trigger a model/provider fallback.

## Settings surface

The existing `remote-auth-section.tsx` renders multiple env methods with localized `provideSecretForEnvVar` and `setEnvVarViaStoredSecret` keys.
The API-key method must use that generic path without a new raw `setup_hint` or `Label`.
Existing `remote-credentials-card.tsx` state preserves the two independent method IDs through save and reload.
No new page, picker, sharing control, or automatic credential selection is required.

Use the existing inline accordion on desktop and phone.
The closest shipped exemplar is that accordion's multiple-env-method rendering and `mobile-ssh-profile-connection-link.spec.ts` settings navigation.
The profile page remains the only vertical scroll owner. Existing secret pickers retain focus return and dismissal.
Phone actions need 44px targets and no horizontal overflow. Desktop keeps the current compact controls.
The plan's UI-01 sketch covers only the added method row.

## Failure handling and verification

Keep `workflow_agent_incompatible` and its source-profile attribution for compatibility denial.
Do not embed raw store errors in API errors or logs because they can include secret IDs.
Cancellation and deadlines retain their error identity and must not become successful admission.
No new metrics, credential caches, migrations, generated contracts, or fallback rules are needed.

The [work orders](../../../plans/claude-profile-admission/plan.md#work-orders) define targeted TDD and end-to-end evidence.
Source, scoped-store, workflow, and SSH-child tests are separate evidence levels.
An isolated mock-agent SSH test proves forwarding and workflow behavior, not real Anthropic authentication.

## Related contracts

- [Secret resolution](../../workspaces/system-design/repository-secrets.md)
- [Executor environment composition](../../executors/system-design/executor-profile-env-precedence-01.md)
- [Private compute](../../executors/system-design/per-user-ownership.md)
- [Workflow change](../../tasks/system-design/change-workflow.md)
- [Exact model policy](no-silent-model-fallback-01.md)
