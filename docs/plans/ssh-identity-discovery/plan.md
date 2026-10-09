---
created: 2026-09-16
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-IDENTITY-DISCOVERY-001
system_design:
  - ../../specs/executors/system-design/ssh-identity-discovery.md
legacy_specs: []
---

# Implementation Plan: SSH Identity Discovery

## Overview

Replace the free-text identity file path in the shared SSH connection form with
a picker over the private keys the backend can actually use, keeping free-text
entry as an explicit choice and as the fallback whenever discovery is
unavailable.

Two work orders. The backend enumerates and classifies; the frontend renders
the choice. They are split because the endpoint has an independent verification
boundary — its security properties (fixed roots, no key material, admin gate)
are provable in Go tests without any UI.

## Scope

In scope:

- A fixed-root, admin-gated discovery endpoint on the backend host.
- A combobox identity-file control with a custom-path escape, shared by the SSH
  and Remote Docker connection cards.
- Five-locale copy for the new strings.

Out of scope:

- Key generation, host enrollment, or agent management. That is the larger gap
  tracked in kdlbs/kandev#3735; this package deliberately does not start it.
- Changing the admin gating of the existing SSH executor routes.
- Discovering keys on the remote host or in the browser's filesystem.
- Validating that a discovered key is authorized anywhere. Test Connection
  remains the only proof.

## Dependency order

| Wave | Work order | Depends on |
|------|-----------|------------|
| 1 | [Identity discovery endpoint](task-01-identity-discovery-endpoint.md) | — |
| 2 | [Identity file picker with custom path](task-02-identity-file-picker.md) | task-01 |

## ASCII UI previews

### UI-01: Identity file control, identity source = "file"

Entry point: Settings > Executors > (SSH \| Remote Docker) > Connection card.
The control replaces the current single-line text input. Only the affected
region is drawn; the surrounding Connection grid is unchanged.

Current behavior (evidence: `ssh-connection-form.tsx` lines 84-94):

```text
Identity file path
+---------------------------------------------------+
| ~/.ssh/id_ed25519                                 |
+---------------------------------------------------+
Passphrase-protected keys must be loaded into ssh-agent first.
```

Proposed, collapsed:

```text
Identity file path
+---------------------------------------------------+
| ~/.ssh/id_ed25519                              [v] |
+---------------------------------------------------+
Paths resolve on the machine running the Kandev backend.
```

Proposed, expanded:

```text
Identity file path
+---------------------------------------------------+
| ~/.ssh/id_ed25519                              [^] |
+---------------------------------------------------+
| Search keys...                                    |
+---------------------------------------------------+
| * ~/.ssh/id_ed25519              ssh-ed25519      |
|   ~/.ssh/id_rsa                  ssh-rsa          |
|   ~/keys/prod                    from ~/.ssh/config|
|   Custom path...                                  |
|   ---------------------------------------------   |
|   ~/.ssh/id_work                 encrypted        |  <- disabled
+---------------------------------------------------+
```

The separator and the trailing position of encrypted entries are the shared
combobox's own grouping, not a per-field choice: it renders selected, then
enabled, then a rule, then disabled.

Proposed, custom path chosen:

```text
Identity file path
+---------------------------------------------------+
| Custom path...                                 [v] |
+---------------------------------------------------+
+---------------------------------------------------+
| /opt/keys/deploy_ed25519                          |
+---------------------------------------------------+
Paths resolve on the machine running the Kandev backend.
```

Proposed, discovery unavailable (non-admin, or request failed) — identical to
current behavior, which is the requirement:

```text
Identity file path
+---------------------------------------------------+
| ~/.ssh/id_ed25519                                 |
+---------------------------------------------------+
Passphrase-protected keys must be loaded into ssh-agent first.
```

Proposed, empty result:

```text
Identity file path
+---------------------------------------------------+
| Select a key...                                [^] |
+---------------------------------------------------+
| No private keys found in ~/.ssh on the backend.   |
|   ---------------------------------------------   |
|   Custom path...                                  |
+---------------------------------------------------+
```

Structural requirements: the custom-path option is always present and is the
last selectable row;
encrypted entries are visible but not selectable and state why; the collapsed
trigger shows the current value verbatim. Column alignment, the `*` selection
marker, and the separator rule are illustrative, not a pixel specification.

Phone composition: same control and same option order. The combobox popover
already portals; on a phone the option list renders full-width below the
trigger with touch-sized rows, and the custom-path input appears beneath rather
than beside the trigger (the Connection grid is already single-column at `md`
and below). No desktop-only affordance is introduced, so one preview covers
both, per `docs/specs/guide/plans-and-work-orders.md#ascii-ui-previews`.

Criteria mapped: AC-…-001.4, .6, .7, .8, .10.

## Verification strategy

Targeted, per work order. No broad suite run is part of this package.

## Risks

- The endpoint is a filesystem-listing primitive over the backend user's home.
  Its fixed-root construction and the no-key-material rule are the mitigations
  and both are directly tested in task-01, not assumed.
- Adding a picker can imply the listed key is known-good. The copy must stay
  descriptive; Test Connection remains the only validity signal.
- Five-locale copy gates the build. `pnpm run i18n:check` is part of task-02's
  verification, not an afterthought.
