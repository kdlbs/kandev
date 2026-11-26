---
created: 2026-11-26
status: complete
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Recover a missing managed OpenCode runtime

## Overview

Give operators a direct path from the configured OpenCode card to the installer
when the selected managed runtime is missing. One work order connects the card
to the existing install catalog and verifies the flow on desktop and phone.

## Diagnosis

The running backend, `735a07998744`, contains #5530. The persisted selection was
managed v2, package `@opencode/cli`, default `2.3.2`, revision 6. The backend
resolved `/root/.npm` as its npm cache. The exact execution tree and all other
trees for the selected package were absent. A standalone executable was present.
The retained logs establish the missing state at startup. The reason that the
cache became empty for OpenCode is unknown.

The previous fix needs at least one cached execution tree for the selected
package. With zero, detection reports unavailable and host utility bootstrap
skips the probe. The saved OpenCode card remains visible with its profiles.
That card currently has no link to the separate install catalog.

An isolated backend from the same binary reproduced the state with the same
selection, an empty npm cache, and a v1 executable fixture on PATH. The
existing install endpoint for OpenCode prepared v2.3.2 and discovery reported
availability immediately and after restart. The persisted selection kept its
original revision. A separate disposable CLI run confirmed ACP initialization.

## Scope

- Open the OpenCode installer from the configured card when the selected
  managed runtime is missing.
- Reuse exact-version installation, progress, retry, and discovery refresh.
- Document the recovery entry under OpenCode in the agent guide.

### Out of scope

- Startup downloads, changes to runtime detection, and runtime selection migration.
- Native runtime maintenance and remote or container installation changes.

## Technical approach

1. Derive the link in the Agents settings page from its existing saved agent,
   discovery, available agent, runtime status, and permission data. Show it
   only for configured, unavailable, managed OpenCode with an install command.
2. Add the link to `InstalledAgentCard` and pass the agent hint to the existing
   `AgentInstallCatalog` from the Browse page. The matching install card leads
   the list when present. Keep the other catalog entries in their usual order.
3. The catalog uses its existing install route and job lifecycle. The server
   reads the exact current selection on submission. After success, let
   refreshed discovery remove the recovery entry and keep the saved profiles
   accessible.

The existing Install agents entry remains available during status loading,
after an unavailable status request, and when the target is already repaired.
The local [adoption decision](../../decisions/2026-09-27-opencode-runtime-adoption.md)
governs selection and source priority.

## ASCII UI preview

UI-01. Settings > Agents, configured OpenCode with a missing managed runtime.

```text
Desktop before
OpenCode  [Configured] [Not installed]             [v] [New profile]

Desktop after
OpenCode  [Configured] [Not installed] [Reinstall] [v] [New profile]

Phone after
OpenCode
[Configured] [Not installed]
[Reinstall] [v] [New profile]
Saved profiles
```

UI-02. Install agents, opened through Reinstall. The OpenCode card leads.

```text
Browse available agents
OpenCode
<selected install command>
[Install]

Job running     [Installing...]  <job output>
Job failed      [Retry]          <job error and output>
```

The phone action opens the full Settings catalog using the existing Settings
scroll region. The card uses a single column on a phone and the existing grid
on desktop. Use the adjacent Install agents control as the sizing precedent
and the phone Browse page as the navigation precedent. Keep the Reinstall
hit target at least 44px on phones and coarse pointers. The preview specifies
action order and destination. Spacing and the job output are illustrative.

## Verification

- AC-AGENTS-OPENCODE-V2-001.4 and .13. The existing backend
  `TestOpenCodeInstallPreparesSelectedManagedRuntime` and
  `TestOpenCodeACPManagedIsInstalled` cases cover exact preparation and
  available versus missing cache states.
- AC-AGENTS-OPENCODE-V2-001.14, UI-01 and UI-02. Extend
  `agents-browse-page.spec.ts` in `chromium` and
  `mobile-agents-browse-page.spec.ts` in `mobile-chrome`. Start at the
  configured card and confirm that the link leads to the selected OpenCode
  installer. Assert the retry and recovery outcomes through the existing job
  state. The work order lists the exact commands.

## Work orders

- [x] [Task 01. Open the installer from the OpenCode card](task-01-opencode-reinstall-entry.md)

## Verification results

The focused Go detection and install cases passed. Web typecheck, lint, i18n
completeness, and formatting checks passed. Desktop E2E passed 4 cases and
mobile E2E passed 2 cases, including missing-runtime recovery, retry, and
profile retention. Public documentation validation passed for 56 documents,
and its test suite passed 63 cases. The real-package and isolated-backend
checks reproduced the missing cache and verified recovery. Specification
catalog, specification lint, and local PR documentation coverage checks passed.
The review follow-up also verified post-install availability after reload and
no saved-profile mutations in both Settings E2E flows.

## Risks

- npm or its registry can prevent a repair. The install card must keep the
  failure output and retry available.
- A status response can arrive after the catalog data. Use both current
  availability and managed source data before rendering the targeted action.
