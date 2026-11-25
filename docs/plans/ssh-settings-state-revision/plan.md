---
created: 2026-11-23
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
legacy_specs: []
---

# SSH settings states and recovery

## Evidence

A replacement lab SSH server had a new fingerprint. The original Ready connection still belonged to one retained runtime. Test correctly rejected the mismatch. Trust correctly protected that runtime. The UI then listed several possible dependencies and gave the operator no way forward. A separately verified lab connection worked. Only its display name should identify it as ready for this exercise.

The previous Access view combined a progress strip, a bordered surface and a separate icon-backed result. Starting installation replaced the Access body. Diagnostics gave one check a card and leaves its peer unframed. Making these elements wider did not address their hierarchy.

## Delivery

1. [Trust conflict and recovery](task-01-trust-recovery.md) makes the changed-key case usable through the existing create and trust flows.
2. [Stable Settings rows](task-02-settings-rows.md) applies Kandev's Settings patterns to trust, installation and diagnostics. Compare against Editors settings and Kubernetes diagnostics with the same viewport, theme and sidebar.

## ASCII UI previews

UI-01. Desktop, Access after a protected host key change. One expanded Settings row holds all the evidence needed for this decision.

```text
Server trust                                     Host key changed
  Saved fingerprint                              SHA256:...
  Fingerprint reported by server                  SHA256:...
  The saved key is in use. To connect to the replacement
  server, create a separate connection and verify its key.
                                                 [New connection]
```

UI-02. Desktop, Access with a trusted host. Compact completed facts sit above the task. In Access, put the local setup check alongside the live login workflow. Retain incoming Diagnostics links. Results expand beneath the check that produced them. Delete opens confirmation directly in the detail header. In the executor editor, a successful test shows a check mark and names the SSH login and platform checks.

```text
Server trust                  Trusted                 View details
SSH access                    Not checked         [Test SSH access]
  Install public key
```

Phone opens the active task first. Keep secondary facts in Host details. At a blocked review, keep both fingerprints expanded and wrap them. Place New connection directly below the explanation. During installation, show the selected method, then its input, review and outcome in the same section. The page owns scrolling.

Keep the active heading and command position recognizable through pending and completion. Use inline progress during a request and a single result on completion. Keep tab changes immediate, respect reduced motion, and avoid page-wide fades, animated height and permanently empty result boxes. Labels and spacing in these sketches are illustrative.

## Review and verification

Use isolated fixtures for a changed key held by a retained runtime and for a trust conflict caused by a concurrent operation. A screenshot cannot establish which owner holds a route. Show only facts the operation confirms. Record desktop and phone screens for pending, blocked, review and success, check keyboard focus and duplicate announcements, and run the commands in the work orders.


## Verification results

All 23 desktop Chromium scenarios and all 14 mobile Chromium scenarios passed.
The mobile run completed 11 scenarios before interruption. The remaining
three passed in a single-worker systemd scope with an 8 GB memory limit.
The SSH component suite passed 87 tests. Web type checking, changed-file lint,
formatting, localization and specification validation passed. Both work orders
include the results of direct testing on the disposable SSH server.
