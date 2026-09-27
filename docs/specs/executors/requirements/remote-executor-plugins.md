---
status: active
system: executors
created: 2026-09-26
owners:
  - kandev
---

# Remote executor plugins requirements

## Overview

An installed plugin can supply remote environments for normal Kandev task sessions.
Users select an executor profile and retain the normal agent, terminal, file, and Git flows.
The executor system owns environment identity, lifetime, recovery, and profile compatibility.
The plugin system supplies installation, transport, and capability enforcement.

These requirements follow the assessment of [issue 3953](https://github.com/kdlbs/kandev/issues/3953).
It specifies core support and a test provider. It does not implement an AWS provider.

## Terminology

- **Provider:** An installed plugin's declared remote executor implementation.
- **Environment:** Remote compute and its workspace, identified independently from a connection.
- **Attachment:** A connection to an existing environment for the same session.
- **Bounded environment:** An environment with a provider-enforced expiry time.
- **Retained resource:** An owned resource whose deletion or absence has not been confirmed.

## Requirements

### REQ-EXECUTORS-PLUGIN-001: Provider admission and profiles

**Intent:** Operators can configure installed providers without changing Kandev for each vendor.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-001.1:** An enabled, compatible provider shall appear in executor settings and normal profile selectors with its name and availability.
- **AC-EXECUTORS-PLUGIN-001.2:** Profile creation and updates shall enforce the provider's configuration schema and existing executor-settings permissions. Secret values shall not appear in ordinary profile reads.
- **AC-EXECUTORS-PLUGIN-001.3:** Missing, disabled, incompatible, or unhealthy providers shall reject new launches without selecting another executor. Saved selections and profiles shall remain visible.
- **AC-EXECUTORS-PLUGIN-001.4:** Conflicting provider identities and undeclared provider calls shall be rejected. Existing plugins and built-in executor profiles shall retain their behavior.
- **AC-EXECUTORS-PLUGIN-001.6:** Before provider state is persisted, the host shall validate it against the provider's closed resource-state schema, including required fields, declared scalar types, enums, and numeric bounds. Undeclared or secret fields shall be rejected.
- **AC-EXECUTORS-PLUGIN-001.7:** Profile updates shall submit blank optional non-secret fields so users can clear their saved values. Secret fields shall retain the existing unchanged, replace, and explicit clear behavior.
- **AC-EXECUTORS-PLUGIN-001.8:** When no installed, active, compatible plugin declares an available provider, Kandev shall not offer a new plugin remote executor selection or dispatch provider operations. Existing inventory and saved selections shall remain readable and intact.

AC-EXECUTORS-PLUGIN-001.5 covered the initial release toggle. It is retired; the original
[implementation plan](../../../plans/remote-executor-plugins/plan.md) retains its historical references.

### REQ-EXECUTORS-PLUGIN-002: Provisioning and ordinary session behavior

**Intent:** Remote plugins supply environments while Kandev retains normal session behavior.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-002.1:** Retrying one admitted launch shall not allocate a second environment. A failed or canceled launch shall leave either confirmed cleanup or a visible retryable resource record.
- **AC-EXECUTORS-PLUGIN-002.2:** Before allocation, launch shall reject incompatible repositories, unsupported workspace sharing, and missing remote-to-Kandev connectivity. Rejection shall name the incompatible input.
- **AC-EXECUTORS-PLUGIN-002.3:** A successful launch shall use the host-selected runtime artifact and authenticated readiness check before agent startup. Setup progress and failures shall be visible.
- **AC-EXECUTORS-PLUGIN-002.4:** Supported sessions shall provide normal prompts, events, permission responses, terminal, files, and Git. Editor and preview actions shall require explicit effective support.

### REQ-EXECUTORS-PLUGIN-003: Connection authentication

**Intent:** Provider authentication does not weaken Kandev authentication or interrupt supported remote surfaces.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-003.1:** HTTP requests and WebSocket connections, including editor and preview traffic, shall apply provider authentication and preserve Kandev execution authentication.
- **AC-EXECUTORS-PLUGIN-003.2:** Expiring connection credentials shall refresh for subsequent requests and reconnects without allocating another environment. Refresh failure shall report unavailable transport without deleting the workspace.
- **AC-EXECUTORS-PLUGIN-003.3:** Transient connection credentials and bootstrap secrets shall not appear in inventory, browser responses, persisted profile configuration, or logs.
- **AC-EXECUTORS-PLUGIN-003.4:** Invalid endpoints, insecure remote connections, and attempts to replace host-owned authentication shall fail before credentials are sent. Existing local executor transport shall remain compatible.

### REQ-EXECUTORS-PLUGIN-004: Recovery and cleanup authority

**Intent:** Backend or plugin interruption does not lose ownership of remote resources.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-004.1:** After restart, Kandev shall recover the exact recorded environment or report missing, expired, incompatible, or unavailable. It shall not substitute a fresh workspace for required reuse.
- **AC-EXECUTORS-PLUGIN-004.2:** Ordinary agent Stop and backend shutdown shall preserve the environment. Archive, delete, reset, and failed-launch rollback shall use the existing authorized cleanup lifecycle. Task reset shall authorize task-write scope before reading environment state or invoking provider cleanup.
- **AC-EXECUTORS-PLUGIN-004.3:** Delayed status, checkpoint, or cleanup operations shall not alter a successor execution or a transferred environment. Unconfirmed cleanup shall preserve its retry information.
- **AC-EXECUTORS-PLUGIN-004.4:** Recovery shall resolve interrupted allocation by its original operation identity, including interruption before a resource handle was returned. Unknown outcomes shall block replacement allocation.
- **AC-EXECUTORS-PLUGIN-004.5:** Resumable conversation information shall remain intact after compute loss. Recovery shall make no new guarantee about remote transcript replay.

### REQ-EXECUTORS-PLUGIN-005: Plugin changes with retained resources

**Intent:** Plugin administration does not silently strand billable resources or erase recovery credentials.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-005.1:** Disable shall prevent new provider operations and preserve profiles, credentials, and retained resource records. The user shall see that remote resources can remain running.
- **AC-EXECUTORS-PLUGIN-005.2:** Uninstall shall be rejected while allocation outcomes or retained resources remain unresolved, including resources from stopped sessions. The rejection shall explain the cleanup path.
- **AC-EXECUTORS-PLUGIN-005.3:** Upgrade and rollback shall preserve provider identity and support the recorded state versions of retained resources. An incompatible change shall fail before replacing the working installation.
- **AC-EXECUTORS-PLUGIN-005.4:** Plugin crashes shall not prove remote resource loss. Restoring a compatible provider shall permit attachment and pending cleanup without changing session identity.

### REQ-EXECUTORS-PLUGIN-006: Workspace retention and expiry

**Intent:** Users understand whether their workspace survives compute loss and when it can disappear.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-006.1:** Profiles and sessions shall distinguish persistent, compute-bound, and unknown workspace retention. Omitted retention information shall mean unknown.
- **AC-EXECUTORS-PLUGIN-006.2:** Bounded profiles shall disclose their maximum lifetime before selection. A bounded environment shall include a parseable absolute expiry no later than its declared maximum lifetime. Active and idle sessions shall display their actual expiry time and workspace-loss consequence.
- **AC-EXECUTORS-PLUGIN-006.3:** Confirmed expiry shall show an explicit environment-expired outcome and disable resume into the lost workspace. A network error alone shall not establish expiry.
- **AC-EXECUTORS-PLUGIN-006.4:** Reconnecting, stopping, or suspending shall not imply an extended lifetime. The host shall not automatically replace an expired environment or treat Git commits as durable remote backups.

### REQ-EXECUTORS-PLUGIN-007: Desktop and phone parity

**Intent:** Users can configure, select, and inspect plugin executors from desktop and phone interfaces.

#### Acceptance criteria

- **AC-EXECUTORS-PLUGIN-007.1:** Desktop and phone users shall create, edit, save, reload, and select a provider profile with the same validation and permissions.
- **AC-EXECUTORS-PLUGIN-007.2:** Both interfaces shall explain provider absence, setup failure, cleanup failure, and expiry. Previously selected unavailable profiles shall remain identifiable.
- **AC-EXECUTORS-PLUGIN-007.3:** Phone settings shall use a focused page; temporary selection and status inspection shall use touch-accessible surfaces. Actions shall not depend on hover.
- **AC-EXECUTORS-PLUGIN-007.4:** New phone controls shall provide at least 44px touch targets, internal scrolling where needed, safe-area clearance, and no document horizontal overflow. Keyboard focus and dismissal shall remain usable.
- **AC-EXECUTORS-PLUGIN-007.5:** Host-owned copy shall use supported locales. Provider labels shall use the plugin localization contract, with declared fallback text when a locale is unavailable.
- **AC-EXECUTORS-PLUGIN-007.6:** Fine-pointer desktop profile controls shall retain the standard 28px control height. Phone and coarse-pointer controls shall meet the 44px touch-target minimum.

## Out of scope

- A production Lambda, SSH, Docker, or sandbox provider plugin.
- Moving built-in executors into plugins or replacing their profile formats.
- Local process sandbox wrappers, arbitrary provider protocols, or a second ACP implementation.
- Workspace sharing between sessions in the initial provider contract.
- Automatic snapshots, workspace migration, durable event replay, or automatic renewal through replacement compute.
- A new public tunnel service, cost dashboard, image builder, or provider marketplace publication.

## Implementation plans

- [Remote executor plugins](../../../plans/remote-executor-plugins/plan.md)
- [Remote executor plugin flag graduation](../../../plans/remote-executor-plugin-flag-graduation/plan.md)
