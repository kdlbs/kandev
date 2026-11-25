---
created: 2026-11-22
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
legacy_specs: []
---

# Desktop SSH configuration layout

## Evidence and scope

The user completed enrollment and found the configuration pages poorly adapted
to desktop. In the isolated lab at commit `7af863b3e5fe`, a fresh Chromium
session visited Executors, Overview, Access, installation and Diagnostics at
1280, 1440 and 1920 pixels with the settings sidebar open. On all three,
Access, installation and Diagnostics were 768px wide. Their available panels
were 928, 1088 and 1568px respectively. At 1440px the enrollment form ended
at x=1104, Edit started at x=1323, and Close setup sat below the card on the
opposite side from Review installation. Executors already lays profiles out
in columns. The practice connection now displays SSH verified.

Change the saved connection detail layout. Reuse the Executors page's responsive
grouping and shared Settings typography, buttons and rows. The sidebar,
collections, SSH service and identity custody retain their existing contracts.
Treat the recent rejection as evidence that the location of Install public key
matters. A trusted host alone does not establish whether installation is needed.

## Delivery and validation

1. [Connection details and diagnostics](task-01-detail-frame.md) establishes
   the shared detail frame and verifies both diagnostics actions.
2. [Access and enrollment](task-02-access-enrollment.md) uses that frame for
   method choice, review and recovery. Verify the navigation and command paths
   with the work orders' targeted desktop and phone Playwright commands.

Risks include disabling the existing settings Save coordinator, remounting a
password field across a width transition, and moving the focused control when
a result arrives. Keep one form and the settings page's single scroll owner.
Include a sidebar toggle, 200% zoom, long localized labels and coarse input in
rendered checks. Update public instructions if the documented steps change.

## ASCII UI previews

UI-01, saved connection on Diagnostics. Desktop, settings sidebar open.

```text
Back to connections
Enrollment practice                            Edit  ...
SSH verified  127.0.0.1
Overview    Access    [Diagnostics]

Setup checks                       | Live SSH test
[Check prerequisites]              | [Test SSH access]
Destination                        | Login result
  Missing agent    [View identity] |
```

UI-02, Access after trust and during optional installation. On wide screens
show the available actions together. Selecting Install replaces the Access body.

```text
Check SSH access
[Test SSH access]  [Install public key]       Host details

Install public key
( ) Password       ( ) Existing identity      ( ) Install manually
SSH password       [..........................]
                            Close setup   [Review installation]
```

Phone uses the existing Settings detail page. Each tab shows one task.
On Diagnostics, the setup result precedes the live login section. During
enrollment, the choices come before the selected input and review action.

```text
Access / Install public key
( ) Password
( ) Existing identity
( ) Install manually
SSH password [........]
[Review installation]
Close setup
```

## Verification results

Implemented locally on November 22 after the user authorized two Sol
implementation agents. The saved-connection layout uses a single named
container. Enrollment methods reflow at 54rem of available space. The error
and recovery actions share the Access surface. Diagnostics retains each result
under the action that produced it.

- 89 focused component and hook tests passed. Desktop and phone Playwright
  passed 35 distinct repository cases across access, diagnostics, management
  layout and connection setup. One additional disposable full Chromium check
  passed at actual 200% browser zoom with the same typed, focused field.
- Desktop initially timed out waiting for Settings to load. The separate zoom
  probe initially specified incompatible browser viewport and scale options.
  Both passed on targeted rerun after correcting the probe. On mobile, the
  820px saved-destination test exceeded its timeout and teardown deadline.
  That test passed alone with the original assertions and timeout.
- The backend, production web and pseudo-locale E2E bundles built. Web
  typecheck, changed-file Oxlint and Prettier, i18n checks and new-copy
  ratchet passed. Public docs validation, specification catalog/lint and the
  two-work-order coverage check passed.
- Reviewed rendered desktop and phone screens. Local captures are under
  `apps/web/e2e-artifacts/ssh-desktop-layout/` (ignored). The browser suite
  used disposable data and UI fixtures. This revision changes presentation and
  command feedback. The existing real OpenSSH results are recorded separately
  in the [access experience verification](../ssh-access-experience/verification.md).
  The manual lab was not redeployed in this implementation turn.
