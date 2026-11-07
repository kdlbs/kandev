---
created: 2026-11-02
status: done
requirements:
  - REQ-AGENTS-REPOSITORY-LOCAL-MODE-001
system_design:
  - ../../specs/agents/system-design/repository-local-session-modes.md
legacy_specs: []
---
# Plan: Repository-local session modes

## Overview and scope

Allow profile authors to retain repository-local mode IDs while a host probe
runs in a scratch directory. Preserve executor confirmation before prompting.
The [requirement](../../specs/agents/requirements/repository-local-session-modes.md)
and [design](../../specs/agents/system-design/repository-local-session-modes.md)
own the behavior and boundaries. Model fallback and new persistence fields are
outside this package.

## Technical approach

The profile picker stores the selected mode through the existing profile form.
The ACP host probe applies advertised modes and skips IDs absent from a present
advertisement. The task executor remains authoritative for actual startup.
The public profile guide explains custom mode entry and executor confirmation.
The ACP regression fixture supplies the required mode-select name for the newer
SDK used by the merge tree.

| Provider evidence | Host probe | Task startup |
| --- | --- | --- |
| Advertised requested mode | Apply and capture options | Apply and confirm |
| Present list without requested mode | Retain host default | Apply and confirm in repository |
| Missing advertisement | Attempt requested mode | Apply and confirm |
| Rejected or unconfirmed task mode | Host result is advisory | Fail before prompt |

## ASCII UI preview

UI-01: Profile settings mode picker, ready discovery (AC-001.1, .5, .6).

```text
Desktop: Start model [model]   Start mode [reviewer v]   [refresh]
Phone:   Start model [model] [refresh]
         Start mode  [reviewer v]
Open:    [Search modes...]
         reviewer  Custom
         [Use typed ID as mode ID]
```

The phone picker remains within the settings scroll owner. The controls and
labels are existing localized components. Disabled discovery prevents editing.
Spacing above is illustrative.

## Tests and E2E

- AC-001.1, .5, .6: `mode-combobox.test.tsx`, desktop
  `agent-profile-custom-mode.spec.ts`, and mobile
  `mobile-profile-custom-mode.spec.ts` cover selection and persistence.
- AC-001.2, .3: `acp_probe_capabilities_test.go` covers legacy and ConfigOptions
  advertisements, including grouped, absent, and empty modes.
- AC-001.4: existing lifecycle `session_test.go` covers explicit mode failure
  and confirmation before prompting.

## Work orders

- [Task 01: Preserve repository-local modes](task-01-preserve-repository-local-modes.md)

## Verification results

The initial contribution and maintainer follow-up passed focused Go and Vitest
checks, desktop and mobile E2E, web typecheck, and changed-file lint. The newer
ACP SDK in the PR merge tree exposed a missing required name in the test fixture.
The same test failed on merge tree a9c2fb9506 before the fixture correction.
After correction, the focused test passed three times and the full utility
package passed under the race detector on that merge tree. The focused branch
probe tests also passed under the race detector. Catalog validation, full spec
lint, and the trusted-base PR documentation evaluator passed. Public guide
validation covers the added profile instructions.

## Risks

A host-default snapshot may not contain repository-mode-dependent options.
Mode availability remains a task startup decision, and explicit modes fail
closed when the executor cannot confirm them.
