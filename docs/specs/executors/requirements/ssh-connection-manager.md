---
status: active
system: executors
created: 2026-09-19
owners:
  - kandev
---

# SSH connection manager requirements

## Overview

An administrator can establish reusable SSH access starting without a local key
or agent, then attach that connection to executor profiles. Executors owns the
identity-to-host connection and its lifecycle. Workspace secrets supplies
storage protection; authentication supplies management and task permissions.

Source: [issue #3735](https://github.com/kdlbs/kandev/issues/3735).
The maintainer selected unattended key availability, administrator-managed
installation-wide connections, and assisted enrollment on 2026-09-19.
This extends the [SSH executor](ssh-executor.md) behind the experimental
SSH Connection Manager feature flag, which remains off by default. Existing
external-file and agent modes retain their current contract. Implementation
and validation records live in the linked plans.

## Terms

- **Identity:** a managed key or a reference to an external key file or agent.
- **Connection:** one named remote host, port, login user, identity, and trusted
  host key, optionally reached through one separately trusted bastion connection,
  reusable by executor profiles.
- **Enrollment:** explicitly adding an identity's public key to the selected
  remote user's authorized keys using access the administrator already holds.
- **Ready:** the current connection revision has a trusted host key and has
  passed authentication. Executor-specific readiness is a separate check.

## Requirements

### REQ-EXECUTORS-SSH-CONNECTIONS-001: Managed identity custody

**Intent:** Administrators can create SSH credentials without shell setup.

- **AC-EXECUTORS-SSH-CONNECTIONS-001.1:** An administrator shall be able to
  generate an Ed25519 identity, name it, and view/copy its public key and
  fingerprint. Kandev shall never return its private key through user APIs,
  exports, logs, agent messages, or credential selectors.
- **AC-EXECUTORS-SSH-CONNECTIONS-001.2:** An administrator shall be able to
  import a supported private key, including an encrypted key with a one-time
  passphrase. Failed parsing or a wrong passphrase shall create no identity;
  the passphrase shall not be retained after the request.
- **AC-EXECUTORS-SSH-CONNECTIONS-001.3:** Managed identities shall work after
  an unattended backend restart without SSH_AUTH_SOCK or user unlock. The UI
  shall explain that Kandev protects the stored key and that backend host
  access can access it; importing a key shall not retain its original
  passphrase as an additional unlock requirement.
- **AC-EXECUTORS-SSH-CONNECTIONS-001.4:** Missing or unreadable protected key
  material shall produce an unavailable-identity result without creating a
  replacement key or falling back to another identity.
- **AC-EXECUTORS-SSH-CONNECTIONS-001.5:** Only administrators shall manage
  identities and connections. Other users shall consume them only through
  executor profiles they may use, without choosing a different host, login,
  identity, or trust value for that use. Auth-disabled installs retain their
  existing single-operator behavior.
- **AC-EXECUTORS-SSH-CONNECTIONS-001.6:** Deleting a referenced identity shall
  be refused with its connection/runtime dependencies identified to the
  administrator. Deleting an unused identity shall not claim to revoke a key
  already installed on a remote host. Replacing a key shall use a new identity.

### REQ-EXECUTORS-SSH-CONNECTIONS-002: Shared connection and host trust

**Intent:** Profiles can reuse a connection without duplicating credentials or pins.

- **AC-EXECUTORS-SSH-CONNECTIONS-002.1:** Administrators shall be able to save
  an incomplete named connection and resume setup later. Only a ready
  connection shall be selectable for a new executor binding.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.2:** An administrator shall inspect and
  explicitly trust each hop's observed host fingerprint before Kandev sends
  authentication credentials to that hop or changes its remote authorization.
  Observing a destination behind a bastion may authenticate to the already
  trusted bastion, but shall not authenticate to the untrusted destination. A changed fingerprint shall block authentication and
  enrollment until separately verified and explicitly trusted.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.3:** Host, port, user, identity, trust, and
  bastion selection/configuration changes shall invalidate previous readiness
  for every affected route. A result from an older revision
  shall not mark the changed connection ready or authorize enrollment.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.4:** Multiple profiles bound to one
  connection shall use that connection's current approved identity and host
  trust for new launches. Non-admin callers shall not override these fields.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.5:** A changed connection shall not redirect
  an existing or resumed runtime to another endpoint. Transport-affecting edits
  shall be refused while live or resumable runtimes depend on the connection.
  Existing established transports may finish; reconnecting a retained runtime with a different
  connection revision shall stop before remote actions and explain the conflict.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.6:** Connection deletion shall be refused
  while a profile, another connection using it as a bastion, or a retained
  runtime references it. Conflicting administrator
  edits shall be reported instead of silently overwriting newer settings.

- **AC-EXECUTORS-SSH-CONNECTIONS-002.7:** Managed connections shall support
  direct access and one ProxyJump bastion, with independently selected login
  users, identities, and explicitly trusted host keys. The destination identity
  shall not be forwarded to or silently reused for the bastion. Both hops shall
  be verified on each new connection, with failures identifying the failed hop.
- **AC-EXECUTORS-SSH-CONNECTIONS-002.8:** Chained jumps, cycles, and an
  incomplete/untrusted bastion shall not become a ready route. Chained or
  cyclic configurations shall be rejected before any network or remote work.
  Changing a bastion shall not redirect a live or resumed dependent runtime.

### REQ-EXECUTORS-SSH-CONNECTIONS-003: Explicit host enrollment

**Intent:** The administrator can authorize a key without an out-of-product agent setup.

- **AC-EXECUTORS-SSH-CONNECTIONS-003.1:** Enrollment shall offer public-key copy
  and optional installation through a one-time SSH password or an existing
  authorized identity. Kandev shall clearly state that existing access is
  required; it shall not bypass hosts that disallow password authentication.
- **AC-EXECUTORS-SSH-CONNECTIONS-003.2:** Before installation, Kandev shall show
  the exact host, login user, trusted fingerprint, and public key fingerprint
  and require an explicit installation action. The bootstrap password shall
  not be stored, replayed, added to command arguments, or included in diagnostics.
- **AC-EXECUTORS-SSH-CONNECTIONS-003.3:** Installation shall add only the
  selected public key for the selected login user, preserve all existing
  authorization entries and restrictions, avoid duplicate entries on retry,
  and never use privilege escalation. Unsafe ownership, permissions, symlinks,
  unsupported authorization layouts, or lock conflicts shall cause a clear
  failure without replacing existing files.
- **AC-EXECUTORS-SSH-CONNECTIONS-003.4:** Enrollment shall be successful only
  after a separate connection authenticates with the selected destination
  identity and the trusted host key, without the bootstrap credential.
- **AC-EXECUTORS-SSH-CONNECTIONS-003.5:** If installation may have succeeded
  but verification or transport fails, Kandev shall show that authorization
  may already have changed, retain the public-key identity, and offer a fresh
  check. It shall not silently remove the key, retry a password, or mark the
  connection ready. Restart shall not resume an enrollment mutation.

- **AC-EXECUTORS-SSH-CONNECTIONS-003.6:** An administrator shall be able to
  enroll a destination reached through a ready, trusted single-hop bastion.
  Bootstrap credentials and authorized-key changes shall apply only to the
  selected destination; destination verification shall use the same approved
  route with the destination identity. Setting up a bastion's own authorization
  shall be a separate, explicit direct-connection enrollment operation.

### REQ-EXECUTORS-SSH-CONNECTIONS-004: Guided setup and diagnostics

**Intent:** Desktop and phone users can complete the same setup and recovery flow.

- **AC-EXECUTORS-SSH-CONNECTIONS-004.1:** Settings shall expose SSH connections
  and identities under Executors, with a discoverable path from the executor editor. Setup
  shall guide target selection, host trust, identity selection/creation,
  authorization, and authentication checking, preserving non-secret drafts.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.2:** Preflight shall distinguish locally
  missing identity, unavailable protected storage, missing/unreachable/empty
  external agent, untrusted/changed host, network failure, and authentication
  rejection, identifying bastion versus destination when applicable. Authentication rejection shall not assert that the key is absent
  from authorized_keys when that cause is unproven.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.3:** Loading and failed steps shall show
  the next permitted action without discarding completed setup. Changing a
  prerequisite shall invalidate dependent results; duplicate submissions and
  stale responses shall not advance the flow. Secrets shall be cleared on
  submission completion, navigation, and dismissal.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.4:** Phones shall provide focused direct
  navigation through setup, accessible identity selection, reachable primary
  actions, at least 44px touch targets, and no horizontal document scrolling.
  Keyboard navigation, focus restoration, and all required actions shall work
  on desktop and phone. All copy shall be localized in supported languages.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.5:** Executor setup shall separately report
  connection readiness and executor-specific checks. A successful SSH login
  alone shall not claim that an agent or Docker daemon is ready.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.6:** SSH connections shall be a child of
  Executors, without a separate top-level settings entry. Authorized administrators
  shall reach it from the Executors page even when no executor profile exists,
  from SSH executor setup, and from settings search. Breadcrumbs and the active
  settings entry shall identify Executors as its parent on desktop and phone.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.7:** Connection setup shall offer identity
  selection and creation at the identity field, with advanced identity management
  available within SSH connections. Creating or cancelling an identity shall
  return to the connection with its saved non-secret details intact. Returning
  from connection management to the initiating managed SSH executor editor in
  the same browser session shall preserve its non-secret draft. Only an explicit
  selection may change the draft connection; executor testing and saving shall
  remain separate actions. Reload may discard unsaved executor drafts; secrets
  shall never be included in navigation or draft recovery state.

- **AC-EXECUTORS-SSH-CONNECTIONS-004.8:** Connection and identity collections
  shall show named rows with endpoint or credential-kind context and setup
  status. Opening a row shall show one focused detail view on desktop and
  phone. Separate records with identical names shall remain distinguishable
  without merging or deleting them. Executors shall expose SSH management as
  a secondary header action, including when no profile exists.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.9:** A saved connection shall initially show
  its endpoint, identity, optional bastion, and current saved verification state
  as a readable overview. Metadata fields shall become editable through an
  explicit Edit action. A verified connection shall not initially display
  bootstrap credential fields, a raw public-key block, or a destructive action.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.10:** Incomplete setup shall identify the
  next permitted step and distinguish saving details, trusting a host, and
  testing access. Key installation shall be an explicit optional action.
  Authentication rejection shall offer diagnostics and authorization options
  without assuming that installation is required. After verified enrollment,
  the form shall be replaced by its result and a connection-test action.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.11:** Opening access setup, diagnostics,
  identity creation, a menu, or a settings tab shall not itself authenticate,
  trust a host, install a key, or save an executor. Uncertain enrollment outcomes
  and changed host fingerprints shall remain visible until resolved. Leaving
  an enrollment form shall clear bootstrap secrets, including when its tab
  remains mounted. Reopening shall require fresh secret input.
- **AC-EXECUTORS-SSH-CONNECTIONS-004.12:** The manager shall use Kandev's shared
  settings hierarchy, header tabs, save behavior, typography, and control sizes.
  Ordinary desktop controls shall measure 28px at the standard root font, with
  a 1px rendering tolerance. Phone and coarse-pointer actions shall have at
  least 44px hit areas. Long names, public keys, and translated labels shall
  fit without horizontal document scrolling. Optional explanatory detail shall
  not obscure required trust, custody, failure, or deletion consequences.

### REQ-EXECUTORS-SSH-CONNECTIONS-005: Compatibility and rollout

**Intent:** Introduce managed access without replacing working operator configuration.

- **AC-EXECUTORS-SSH-CONNECTIONS-005.1:** Existing agent/file profiles, config
  aliases, IdentityAgent overrides, and ProxyJump settings shall continue to
  work unchanged. No private key shall be automatically copied into Kandev.
- **AC-EXECUTORS-SSH-CONNECTIONS-005.2:** Converting a legacy direct or single-hop profile
  shall require an administrator action, preserve the exact resolved endpoint
  and existing destination pin, and pass current connection/executor checks
  before binding. A single-hop conversion shall separately resolve and confirm
  the bastion identity and explicitly trust its host fingerprint; it shall not
  carry forward legacy acceptance of an unknown bastion key.
  Profiles shall not be merged because their host names match. Failed or
  unsupported conversion shall leave the original profile operational.
- **AC-EXECUTORS-SSH-CONNECTIONS-005.3:** The feature shall ship disabled in
  all runtime profiles. When disabled, new management, probes, enrollment,
  managed bindings, and managed runtime connections shall fail before secret
  resolution or remote work; legacy profiles shall retain existing behavior.
  Saved managed records shall remain available when re-enabled.
- **AC-EXECUTORS-SSH-CONNECTIONS-005.4:** Administrator mutations, trust
  decisions, enrollment results, and managed authentication failures shall
  produce attributable audit records without private keys, passwords, or
  raw remote output. A failed audit write shall prevent a new enrollment
  mutation from starting.
- **AC-EXECUTORS-SSH-CONNECTIONS-005.5:** Restoring a database without its
  matching protection key shall leave identities unavailable and explain the
  recovery requirement. Decryption failure shall never replace an existing
  identity or result in a successful connection status.
- **AC-EXECUTORS-SSH-CONNECTIONS-005.6:** Existing SSH management bookmarks
  shall open the equivalent nested settings view, preserving the selected tab,
  entity, creation state, and pending adoption or bastion-return context. The
  redirect shall not create records, trust hosts, enroll keys, or bind profiles.
  Feature and administrator restrictions shall apply at both locations.

## Out of scope

Per-user/workspace sharing policies, per-use passphrase unlock, private-key
export, automatic remote revocation or rotation, certificate authorities,
cloud-provider enrollment APIs, hardware-key generation, general interactive
keyboard authentication, sudo, custom AuthorizedKeysCommand management, and
chained ProxyJump connections are excluded. Single-hop ProxyJump and explicit
legacy conversion are included. The first managed
connection consumer is SSH; implementing the remote Docker runtime itself
remains [#3710](https://github.com/kdlbs/kandev/issues/3710).

## Related artifacts

- [System design](../system-design/ssh-connection-manager.md)
- [Implementation plan](../../../plans/ssh-connection-manager/plan.md)
- [Executor navigation follow-up](../../../plans/ssh-connections-executor-navigation/plan.md)
- [SSH management UX revision](../../../plans/ssh-connections-ux/plan.md)
