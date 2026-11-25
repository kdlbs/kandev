# Internal triage: #3735

Source: https://github.com/kdlbs/kandev/issues/3735
Original title: [Feature]: SSH connection manager — key generation, agent handling, and host enrollment from zero
Intake owner: zeval. Reverified on 2026-09-19 from GitHub's issue page: sole assignee, no competing owner. Claim timestamp: 2026-09-19T14:32:04.959823+00:00; intake eyes reaction ID: 514704201 (preserved from intake).
Intake task: 4b90f67b-010a-454e-9f68-74fe712cf037.
Current task/session: 74961be3-8c1a-4c16-ac36-3eff5f7455e8 / e60c5f55-efb3-43f9-926d-0a985337679e.
Inspected checkout: 794773dd6887ae5c326ee77757dedd7bafb1ecf7, matching live main at inspection. Source is an issue; no source PR/head SHA.

**Recommendation:** retain as a valid enhancement, normal feature priority, and stop at Triage Decision. The limitation is explicit in the existing SSH contract, not evidence of a regression. Authorize specification only after maintainer scope decisions below; do not implement yet.

**Reported / expected:** A user starting without SSH credentials must generate and authorize a key externally, arrange an agent accessible to the backend or use an unencrypted file, then test and trust the host before saving. The requested outcome is in-product identity creation/enrollment and protected-key use, reusable connections and host trust, and actionable prerequisite checks.

**Evidence:**
- `apps/web/components/settings/ssh-connection-form.tsx` offers agent or file-path identity sources, with no generation, unlock, or enrollment flow.
- `ssh-connection-card.tsx:228` gates Save on a successful connection test, observed fingerprint, explicit trust, and an unchanged tested target. It persists `host_fingerprint` with the executor configuration.
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go:378` uses a configured IdentityAgent socket or the backend's SSH_AUTH_SOCK. Missing/unreachable sockets fail; file authentication uses ParsePrivateKey and explicitly rejects passphrase-protected keys with ssh-add guidance.
- `docs/public/executors.md:407` documents these restrictions. SSH requirement AC-EXECUTORS-SSH-EXECUTOR-001.4 explicitly excludes Kandev passphrase handling and password/keyboard-interactive authentication.
- Qualification: merged PR [#2992](https://github.com/kdlbs/kandev/pull/2992), present locally as 0057e4cb7, already permits per-host IdentityAgent sockets. Thus inherited SSH_AUTH_SOCK is not mandatory if another accessible agent socket is configured. This still requires external setup. SSH config aliases are optional; direct host/user fields work.
- Existing requirement .5 still mentions IdentitiesOnly, whereas current design/docs explicitly exclude it. This documentation drift does not alter the credential-management finding.

**Duplicates / related work:** GitHub searches covered SSH titles, connection-manager/key-generation terms, and references to 3735. No direct duplicate found. The issue has no linked or closing PR and no discussion comments in its complete visible timeline.
- [#3710](https://github.com/kdlbs/kandev/issues/3710) remains open and links open PR [#3783](https://github.com/kdlbs/kandev/pull/3783), head `b7edaee8f00bf31ee0d0f52467842cbccc0d3312`. Its description adds remote Docker over existing SSH auth and admin-gated discovery of existing identity files. This overlaps with onboarding, not managed key generation/enrollment; coordinate future scope. PR claims were read, not independently tested.
- Open PR [#3747](https://github.com/kdlbs/kandev/pull/3747), head `67c76e43f31549b68656c608fa4cb25d4f2c6301`, concerns continuous SSH host reachability; no evidence it supplies the requested credential lifecycle.
- [#927](https://github.com/kdlbs/kandev/pull/927) introduced the existing SSH executor. Related work does not establish that this request is complete. REST detail requests were rate-limited; public pages, successful search requests, and git refs supplied the evidence.

**Reproduction / validation:** Passed `/usr/local/go/bin/go test ./internal/agent/runtime/lifecycle -run '^(TestBuildAuthMethods|TestBuildAuthMethodsIdentityAgentOverridesEnvironment|TestIdentityAgentNoneDisablesEnvironmentFallback)$' -count=1` (0.138s after dependency build). Temporary fixtures cover missing/unreachable agents, missing key files, encrypted-key rejection, explicit socket precedence, and disabled agents. No live remote enrollment or browser/service-launch reproduction was performed. Working tree was clean.

**Impact / confidence:** High confidence in the current limitation and creation gate; medium confidence in duplicate-search completeness. It blocks first-time SSH onboarding until external setup, with extra friction for service, desktop, and headless backends. Existing configured users retain working paths. No demonstrated credential compromise or data loss. Managed keys and remote authorization would introduce significant credential custody and access-control decisions, especially for Docker-capable accounts.

**Questions for maintainer before specification:**
1. Must the first release support unattended service/headless restart as well as desktop use, and when must users unlock protected keys again?
2. Who owns identities and reusable connections: installation admin, individual user, or workspace; who may share, use, rotate, export, and delete them?
3. Is initial enrollment limited to copying a public key, or must it mutate authorized_keys using password/existing access? What bootstrap access is assumed when password authentication is disabled?
4. Must #3783 integrate shared connections immediately, and how should existing profile-specific identities and host pins migrate?

No GitHub comments, reviews, reactions, assignments, pushes, merges, or closures were made. No specification, implementation plan, production code, or permanent tests were written. This report is internal to the existing Kandev task.

## Specification phase

Maintainer authorized specification in this session: “go for spec on this one”. Intake ownership reverified: zeval remains sole assignee. No implementation or GitHub publication authorized.

- [x] Resolve material credential lifecycle, ownership, and enrollment decisions.
- [x] Write requirements and system design.
- [x] Write implementation plan and work orders.
- [x] Validate and hand off the design package before implementation.

The earlier triage disposition is historical; the current authorized phase is specification. A formatting defect in the agent-written reproduction paragraph was repaired without changing its findings.

### Confirmed specification decisions

The maintainer selected all three recommended options in direct chat (the earlier question-tool request expired):
- Managed identities use Kandev-protected secret storage and remain usable after unattended backend restarts. Backend host access is the security boundary.
- Administrators manage installation-wide identities and connections. Users consume them through available executor profiles without obtaining private key material.
- Enrollment supports public-key copy and optional installation using a one-time SSH password or an existing authorized identity, after explicit host trust. Passwords are never retained.

Ownership: the executors system owns reusable SSH connectivity and profile consumption; secrets remains the encryption/storage provider. Preserve legacy external agent/file profiles. Resolve routine delivery sequencing against current main and related PR heads; no implementation is authorized.

### Initial requirements and decision recorded (ProxyJump exclusion superseded below)

Created draft `docs/specs/executors/requirements/ssh-connection-manager.md` (REQ-EXECUTORS-SSH-CONNECTIONS-001 through -005) and accepted custody ADR `docs/decisions/2026-09-19-managed-ssh-credential-custody.md`, reflecting the three confirmed choices. Existing profiles remain compatible; explicit conversion and direct managed connections form the first release. Private key export, automatic revocation, and new managed ProxyJump are excluded. System design and work orders are in progress.

Baseline update: local source remains 794773dd6887ae5c326ee77757dedd7bafb1ecf7. Current main advanced to 9e48a71dbe502408f3678f6bfefd9ee87490c3cc; fetched and inspected diff (repository checkout options, not SSH custody). Related PR heads remain #3783 b7edaee8f00bf31ee0d0f52467842cbccc0d3312 and #3747 67c76e43f31549b68656c608fa4cb25d4f2c6301. No branch checkout or merge performed; intake owner remains zeval alone.

### Design package complete

Created and validated:
- `docs/specs/executors/requirements/ssh-connection-manager.md`: five REQ-EXECUTORS-SSH-CONNECTIONS requirements, 30 acceptance criteria.
- `docs/specs/executors/system-design/ssh-connection-manager.md`: proposed transport/store/API/enrollment/runtime/UI contracts.
- `docs/decisions/2026-09-19-managed-ssh-credential-custody.md`: records the three maintainer-confirmed custody/ownership/enrollment choices.
- `docs/plans/ssh-connection-manager/plan.md` and six sequential pending work orders: foundation -> identities -> trusted connections -> enrollment -> executor bindings -> recovery/rollout.

Preserved legacy contracts and added draft-extension links; updated executor ownership notes. Concrete behavior: protected managed keys remain available after restart; management uses org.config.manage; enrollment trusts the host before credentials, preserves authorized_keys, and requires a separate destination-key login. Transport edits are refused while live/resumable runtime references exist. New managed connections support direct access and one ProxyJump bastion, with separate explicit host trust and identity selection for each hop; legacy direct/single-hop adoption is included. Chained jumps remain excluded. Private key export, automatic revocation/rotation, per-user/workspace sharing, and remote Docker runtime implementation are excluded. Remote Docker can consume the same proposed transport interface; recheck its PR before implementation.

Feature gate: features.sshConnectionManager / KANDEV_FEATURES_SSH_CONNECTION_MANAGER, proposed default false in all profiles and restart required. No code was changed to add the flag yet. Full desktop/phone previews, failure states, exact unit/PostgreSQL/Playwright commands, and documentation impact are in the package.

Validation: catalog validation passed (292 decisions, 1040 specifications); 36 spec-linter tests passed; all-spec lint and diff checks passed; custom traceability/link audit confirms all 30 ACs assigned to the six work orders; whitespace/status check includes all ten new artifacts. No new product tests or browser runs were performed because this turn changes documentation only. Public docs changes are assigned to implementation.

Handoff: design only. All work orders remain pending; await a later explicit implementation request. No subagents, commits, pushes, GitHub comments/reviews, merges, closures, or new persistent tasks.

### Maintainer revision: single-hop ProxyJump included

The maintainer explicitly requested including existing single-hop ProxyJump capability in the first managed-connection release. Updated requirements, system design, custody ADR, manifest, relevant work orders, and synchronized desktop/phone previews. Added AC-EXECUTORS-SSH-CONNECTIONS-002.7, -002.8, and -003.6 (30 total ACs).

A destination may reference a ready direct bastion connection. Each hop owns its user, identity, and explicitly trusted pin. Destination observation may authenticate to the already trusted bastion, but never to the untrusted destination. Destination enrollment passwords/authorized_keys changes apply only to the destination. The bastion's own enrollment is a separate direct setup action. No agent forwarding, implicit identity reuse, chained jumps, or direct-route fallback. Readiness, trust receipts, admission, adoption, deletion, and restart compare/protect both route records. Bastion transport edits invalidate dependent routes and are refused while dependent live/resumable runtimes exist.

Existing legacy bastion behavior can accept unknown host keys; adoption requires explicit bastion trust rather than inheriting that weaker behavior. Tests now cover distinct hop identities/pins, route-only reachability, each host-key mismatch, stale dependency revisions, destination-only enrollment, chain/cycle rejection, and restart.

Validation rerun: catalog and all-spec lint passed; 36 spec-linter tests passed; all 30 ACs map to pending work orders; links, whitespace, diff checks, and preview synchronization passed. Documentation only; no implementation or publication. Private-key export remains outside this release; automatic revocation remains out of scope as the maintainer requested.

### Implementation authorized (2026-09-22)
User explicitly requested implementation of the revised six-work-order package, including single-hop ProxyJump. Verified issue #3735 remains open with sole assignee zeval. Implementation begins with transport extraction and storage foundations; no GitHub publication authorized.

### Implementation progress (2026-09-22)
Fast-forwarded the existing feature branch from 794773dd6887ae5c326ee77757dedd7bafb1ecf7 to current main 195530e57, preserving the design documents and resolving the executor index conflict. #3747 is now merged (head fb81c2a9e94ff88bedfee55dc97de2a41c8b1d2b); #3783 remains open (head ac8cc4c93794c4c51881be7090e5c6f3bcf396c2).

In-progress backend work: shared SSH transport extraction with legacy adapters; separately pinned managed hops; credential-free host observation; transactional installation-owned secret vault; default-off feature registration; versioned connection store and single-hop/reference guards; authorized identity generation/import with idempotency; audited identity deletion; route-bound one-use trust receipts; managed-authentication readiness proof. No work order is complete, and the service is not wired into product routes or executors yet.

Focused transport/lifecycle tests, backend flag contracts, frontend flag contract (3 tests), and SQLite tests pass. Provisioned disposable PostgreSQL and ran new connection/vault parity plus existing secret-store tests successfully using a keyword DSN. Race/lint checks are in progress. No GitHub publication, commits, or completion signal.

## Implementation progress (2026-09-24)

Continuing the authorized implementation in this session; source #3735 remains owned solely by zeval (reverified today). No publication, push, or task movement.

Implemented managed identity custody, trusted single-hop transport, observation/trust receipts, enrollment journal and safe append script, interrupted-enrollment recovery, preflight and readiness invalidation. Settings UI and discovery are partially implemented. Focused service tests pass; the earlier SQLite/PostgreSQL persistence and secret checks passed.

Added atomic executor binding guards and runtime admission: binding and reference updates share the executor transaction; persisted runtime route references protect transport revisions before remote effects; launch/resume/persisted cleanup use generation-checked managed dialing. New guard, rollback, admission, and lifecycle regression tests pass. Existing focused SSH lifecycle tests pass.

Still in progress: reference lifecycle cleanup, preparation/readiness/background probes, executor selector and adoption, enrollment/recovery UI, mobile picker behavior, all translations, public documentation, and final integration/browser checks. Full frontend typecheck exhausted the available heap at both 2 and 3 GiB; no successful full typecheck is claimed. This is partial progress, not step completion.
