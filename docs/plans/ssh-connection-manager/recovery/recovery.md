# Recovery of issue #3735

Recovery date: 2026-10-20. Status: recovery complete; approved implementation resumed locally. Acceptance remains incomplete.

## Source and identity

- Issue: https://github.com/kdlbs/kandev/issues/3735. Read through authenticated `gh issue view`. Open, no comments at recovery.
- Exact read-only source: http://100.105.155.17:9998. HTTP 200 after network escalation. No authentication material was requested.
- Workspace: `9c1e60d1-2028-4810-aef4-ee3ad4a6b074`.
- Task: `74961be3-8c1a-4c16-ac36-3eff5f7455e8`. Issue URL, number, original title, and description match.
- Only session returned: `e60c5f55-efb3-43f9-926d-0a985337679e`. All 1,007 messages were read with ascending cursor pagination.
- Source plan: `9be1aeeb-22a3-48f9-84a4-d1a96b414806`. Eight revision records. No plan comments.
- Current recovery task/session: `a2845dea-8086-4b3a-824c-65e1941d4cd8` / `13973a35-0812-478d-a620-2e36314c61f1`.

## Verified approvals

See [approval excerpts](approvals.json) for exact messages and timestamps.

1. Specification authorized on 19 September.
2. All three recommended choices accepted: unattended key availability, administrator-managed installation-wide connections, and assisted enrollment.
3. Single-hop ProxyJump explicitly included. Private-key export deferred. Automatic revocation excluded.
4. Revised package implementation authorized by `go for impl` on 22 September. A later `continue` arrived on 28 September.

The original design-only handoff is superseded by the explicit implementation message. A new planning approval is unnecessary. Current instructions prohibit commits, pushes, PRs, GitHub messages, and live remote access changes.

## Recovered artifacts

The [approved design patch](approved-design.patch) comes from persisted Git snapshot `e0d21e67-8451-4b00-b4f6-2414e64f7fb7`, dated 19 September at 18:29:39 UTC. It includes ten new artifacts and three documentation edits. New artifacts were restored from that patch. Current plan/work-order status and ADR approval text were subsequently updated, with the original bytes retained in the patch. The existing executor requirement/design edits applied. The executor index was reconciled with current main.

The [source task plan](source-task-plan.md) preserves historical progress and superseded decisions. Its latest update is 24 September at 17:03:26 UTC. Later messages contain additional progress. Treat this plan as evidence, not current instructions or proof of completion.

The [latest file manifest](source-file-manifest.json) comes from snapshot `ef441ffa-4336-4998-9176-63725e5856cc`, dated 15 October. Its `files_complete` is false and its `files` field is null. It lists implementation changes but contains no implementation diff.

Recovered 378 command records and 316 persisted command outputs. A constrained local transformer reconstructed 134 changed implementation files from literal file writes and bounded local edits against source base `195530e5766abf7f44f08e2d5ad436b22c28d748`. It did not execute recorded shell commands, tests, network requests, or arbitrary Python.

The [implementation patch](reconstructed-implementation.patch) records that reconstruction before current-main reconciliation. The [reconstruction manifest](reconstruction-manifest.json) records file provenance and initial merge conflicts. Twenty-one files required reconciliation. These artifacts are recovery evidence, not an authoritative final source-tree snapshot.

One redacted test-fixture initializer was inferred from its assertions and model type, then validated: `Secret{ID: id, Name: "identity"}` in the internal vault test. Formatter-dependent edits were normalized locally. Two failed historical edits were superseded by later successful recorded edits. The interrupted Traditional Chinese update was regenerated using the current repository script. Korean translations were added locally. No production private key or token was recovered.

## Last reliable state

Source branch: `feature/issue-3735-ssh-conne-74961b`. Last recorded HEAD/base: `195530e5766abf7f44f08e2d5ad436b22c28d748`. Changes remained uncommitted. No feature PR or implementation commit was established. The initial session base `794773dd6887ae5c326ee77757dedd7bafb1ecf7` predates the source's fast-forward to main.

Implementation covered shared transport, internal credential custody, versioned connections, trust receipts, enrollment, executor bindings, runtime references, settings, adoption, and translations. Work-order completion was not established.

The source reported focused Go tests, SQLite/PostgreSQL parity, targeted lint, production-only TypeScript, and locale checks passing. Full TypeScript previously exhausted 2 and 3 GiB heaps. Browser/native-build attempts had no verified successful final result. These are historical results, not current verification.

The last implementation command at 24 September 17:56:41 has no exit code in its record. It attempted to clear admission metadata after a failed initial managed dial. Do not infer that this final edit or its test completed on the source. The local
reconstruction includes this behavior; `TestManagedSSHFailedAdmissionCanRetry`
passed in the fresh lifecycle test selection.

Resume attempts failed with npm ENOENT for `/run/kandev/home/.kandev`, then ETARGET for `@agentclientprotocol/codex-acp@1.13.1` and `@agentclientprotocol/codex-acp@2.0.1`. The final session state is FAILED. These are startup failures, not feature-test failures.

## Read-only boundary and gaps

Repository-defined reads supplied workspace search, sessions, messages, stored shell outputs, plan revisions/comments, and Git snapshots. Tool discovery returned `unsupported call`; repository routes supplied the fallback.

Live workspace file/diff handlers call `GetOrEnsureExecution`, which can create an execution. They were not invoked. Existing SSH access to `zeval@100.105.155.17` failed with `Permission denied (publickey,password)` using batch mode and strict host checking. No host trust, credentials, services, tasks, sessions, repository, or authorized_keys were changed on the source.

An authoritative final source tree remains unavailable through the inspected read-only paths. If reconstruction cannot resolve a file, the minimum input is a sanitized archive or patch of this task's `/workspace` source. Exclude `.git`, private keys, tokens, credentials, and unrelated data. Existing read-only access to that source is an alternative.

## Related repair evidence

The related healing task is `88e52fc9-ba69-46fa-9efd-133a97bfe96d`, session
`0a9cfef0-76e9-4acd-8f79-0afd861b807c`. Read-only message search found references
to #3735. Only matching repair summaries were retrieved; credential-handling
commands were excluded. Messages `9439fb85-1c9b-4870-b5fc-21a6d4502862` and
`03d800fa-107c-430c-ac23-f79da5ac6804` on 28 September report a temporary repair,
a WAITING_FOR_INPUT session, and preserved workspace/history. These are agent
reports, not independent proof of the repair. Later startup failures in the
original task supersede this temporary state.

## Local reconciliation and remaining work

The isolated workspace started clean at `595df36ed5c`. A read-only remote SHA
query verified current main as `e34761417d28e561ce9bd616cfa78c1b723df9ad`; this
branch was fast-forwarded to that existing commit before applying the recovered
implementation. Fetch reported a pre-existing broken shared ref,
`refs/heads/feature/lead-pr-4371-to-merg-eeb`. The required main object was
available; the unrelated ref was not repaired or deleted.

Current main includes Remote Docker, per-user executor ownership, additional
runtime flags, coordinated settings save, and startup sequencing. Reconciliation
preserves those contracts while restoring shared SSH transport, internal vault,
connections/trust/enrollment, runtime admission, reachability and settings. The
feature remains disabled in prod, dev, and e2e profiles. Both runtime consumers
retain their existing legacy transport path. No automatic host-trust acceptance
or fallback around a changed fingerprint was added.

Initial reconciliation evidence is in [validation](validation.md): focused Go
and PostgreSQL tests, 58 frontend unit tests, seven browser cases, full
TypeScript, localization, lint, documentation and specification checks passed.
The continuation section records additional checks and fixes. Local fixes cover the
API registration collision, phone drawer containment, breadcrumb localization,
and maintainability findings. The complete approved package
exists, so implementation resumed without another planning cycle. All six work
orders remain in progress: recovered implementation is substantial, but partial
unit coverage and historical reports do not establish every acceptance criterion.

The continuation added protocol-level enrollment and restart fixtures, plus
UI ordering and secret-clearing fixes. The fixture executes the production
enrollment script in a temporary HOME. A fresh process reopens the database
and key vault without SSH_AUTH_SOCK or SSH_AGENT_PID. These checks cover direct
and single-hop transport, but do not replace the full backend/desktop runtime
acceptance against OpenSSH. See the continuation section in
[validation](validation.md) for exact results and limitations.

Managed executor launch/reconnect now passes against real OpenSSH, including a
health request through the restored tunnel after the startup recovery fix.
Remaining work includes the complete executor adoption/browser matrix, dependency
identification when identity deletion is refused, managed orphan-sweep integration,
desktop-specific startup acceptance, and a final acceptance-by-acceptance audit.
The current identity deletion response returns `ssh_resource_in_use` without
listing connection/runtime dependencies. This is an implementation gap for
AC-EXECUTORS-SSH-CONNECTIONS-001.6, not a test-environment limitation.
The initial Docker blocker was removed after the user authorized tool
installation. Docker and Buildx now run the disposable OpenSSH tests; see the
authorized-environment section in [validation](validation.md). PostgreSQL was found in the Nix
store; both recovered PostgreSQL tests passed against a temporary Unix-socket
server, which was then stopped and removed. Those cases do not establish the
full proposed PostgreSQL transaction matrix.
A sanitized final source archive would improve reconstruction confidence but is
not needed to continue local work on the recovered implementation.

## Implementation trace

- Shared connection ownership: `internal/sshconnections/service.go` checks
  `org.config.manage`; `runtime.go`, `binding.go`, and the task repository binding
  transaction limit runtime use to the saved executor binding and route stamp.
  Current executor `OwnerUserID` and existing ownership guards were preserved.
- Key generation/import: `identity.go` generates Ed25519 keys and imports
  supported SSH keys. A passphrase is used only to decode the input.
  `internal/secrets/ssh_identity.go` stores encrypted material under a reserved
  internal secret identity, within the metadata transaction. Generic secret
  visibility filtering excludes those records.
- Agent and service behavior: managed authentication builds an in-memory signer
  from the vault; it does not require an inherited `SSH_AUTH_SOCK`. Explicit
  file/agent references remain external. `credential.go` bounds agent socket
  operations and reports unavailable or empty agents. Missing/mismatched
  protection keys fail closed. Desktop and service composition use the same
  backend service; complete process-restart acceptance remains open.
- Reusable profiles and migration: `adoption*.go`, `binding.go`,
  `executor_ssh_managed.go`, and `executor_binding.go` preserve legacy inline
  settings until explicit adoption succeeds. Managed metadata records route
  and identity generations; transport edits are blocked by runtime references.
- Trust and enrollment: `trust.go`, `probe.go`, and `route.go` bind observations
  and explicit trust to the actor and saved route. Single-hop routes use distinct
  pinned hops. `enrollment.go` journals before remote access; `enroll.sh` checks
  ownership, permissions, symlinks and locking before an idempotent append.
  Recovery retains an unknown outcome instead of replaying a password.
- Actionable errors: `preflight.go` identifies target, identity, trust, agent
  and bastion prerequisites. The localized settings components map bounded
  diagnostic codes to recovery instructions.
- Current-main API compatibility: enabling the recovered manager initially
  collided with the existing identity-discovery route. A regression test
  reproduced the startup panic. New management routes now use
  `/api/v1/ssh/managed`; `/api/v1/ssh/identities` retains its discovery contract.

Paths above are relative to `apps/backend` unless explicitly identified as
frontend. These are code-trace findings, not a claim that every acceptance case
has passed. See the validation record for tested boundaries.

No commit, push, PR, GitHub message, live enrollment, live authorized_keys change, or
old-instance mutation was performed during recovery or local resumption.

## Latest reliable state (2026-10-25)

The user explicitly requested implementation of the remaining work after draft
PR #4778. Managed orphan cleanup, identity dependency details, browser migration,
native desktop startup/restart, and PostgreSQL transaction coverage are now
complete locally. The [dated acceptance record](acceptance-2026-10-25.md) maps
the requirements to evidence and records environment failures, cleanup, and
platform limits. It supersedes earlier remaining-gap lists for this package.

At the 2026-10-25 handoff, branch `feature/recover-and-resume-s-1mt` had committed
head `20661f71af0c8ac42f233d8de418411088ced639` and an uncommitted continuation.
Draft PR https://github.com/kdlbs/kandev/pull/4778 contained the previously pushed
commits. The user authorized committing and pushing the continuation on
2026-10-26. CI and review comments remain deferred. No main or seeded
instance, old recovery source, or live SSH authorization was changed.
