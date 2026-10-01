---
created: 2026-09-30
status: done
requirements:
  - REQ-AGENTS-MINIMAX-001
  - REQ-AGENTS-MINIMAX-002
  - REQ-AGENTS-MINIMAX-003
system_design:
  - ../../specs/agents/system-design/minimax-code.md
legacy_specs: []
---

# Implementation Plan: Native MiniMax

## Overview

Deliver #3996 sequentially in the authorized unattended primary session. The
user explicitly authorized continuing through design, implementation, commit,
PR, review remediation and normal-policy merge without routine checkpoints.

## Scope

Native agent and subscription setup, dynamic native model catalog, profile
selection, terminal passthrough, isolated runtime metadata and public docs.
Exclude wrappers, new transports, Office routing and credential relocation.

## Technical approach

Add MiniMaxACP to agents and registry using mcode directly. Verify installation
with --version, pin the official npm install recipe, retain native ACP IDs and
translate only terminal model syntax. Reuse LoginAgent with localized region
help, ACP sessionmodel, permission handlers and executor session mounts.

| Provider | Transport | Model shape | Behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- | --- | --- |
| MiniMax Code | native ACP | typed category=model, encoded provider/model/variant | native session selection | published 0.5.10 handshake, protocol fixture | visible error, no account fallback |
| MiniMax Code terminal | native CLI | provider/model#variant | decoded explicit profile model | argv tests | invalid input retained, native rejection |

## ASCII UI preview

UI-01: Settings > Agents > MiniMax (login required), shared desktop/phone content:

```text
MiniMax   [MCP] [Login required]
[Login] [New Profile]
  Sign in to minimax-acp
  MiniMax account login. China: mcode login.
  Global: mcode login --region global.
  [full wrapped command; native terminal; Ctrl+C opens shell]
  [Done]
```

UI-02: Profile start model picker (authenticated native catalog):

```text
Profile: MiniMax M3
Start model: [MiniMax-M3 - thinking v]
[Save]
```

These are content previews. MiniMax login reuses the shipped quick terminal:
a bounded desktop dialog and full-screen phone surface, dynamic viewport
height, safe-area padding, a flexing terminal and a visible Done action. The
command preview truncates within its width. Existing profile model popovers
retain their touch selection and tap-outside dismissal behavior. AC-001.2,
AC-001.3 and AC-002.2 are checked by desktop/mobile rendered flows.

## Tests

Agent/registry tests map AC-001.1/2 and AC-002.1/2/3 and AC-003.1/2 to native
metadata, availability, argv and credential isolation. Protocol shape tests
exercise typed config models in shared adapter/utility code. Isolated native
probes establish initialize and unauthenticated behavior; mock endpoint probes
provide runtime evidence without claiming subscription access.

## E2E tests

`settings/minimax-agent.spec.ts` (chromium) and
`settings/mobile-minimax-agent.spec.ts` (mobile-chrome) cover localized login
help and selecting/saving encoded profile models using fixture capabilities.

## Work orders

- [x] [Task 01: Native MiniMax runtime](task-01-native-runtime.md)
- [x] [Task 02: Setup, protocol and documentation](task-02-setup-validation.md)

## Verification results

Implementation validation passed: affected Go packages and changed-package
lint; login unit tests, frontend typecheck/lint/i18n; desktop and phone login,
model selection/save/reload E2E; public docs and specification/harness checks.
Native 0.5.10 probes verified initialize, auth_required, model selection, text,
MCP tool execution, active cancel and subsequent load against an explicit local
BYOK endpoint. No subscription credentials were available; native policy did
not emit permission requests during the probe, so handler evidence comes from
shared ACP regression tests. Both implementation work orders are complete.
CI/review disposition and final merge evidence are tracked by
[PR #4110](https://github.com/kdlbs/kandev/pull/4110) and Kandev task
`04d255ef-97fa-4d3c-9618-8abf0db8745c`.

## Risks

Published CLI requires a supported Node version and optional SQLite install
scripts. Subscription model discovery cannot be verified live without an account.
OAuth file copying is not portable because native credential keys include the
absolute auth-home identity. CLI and ACP model syntax differ.

## Review remediation validation

The setup work order now covers localized install/passthrough metadata, single
capability-probe ownership after profile login, complete phone command display,
and an actual native task using the saved model. Installation and task tests
use isolated executable fixtures; published CLI probes and live OAuth limits
remain recorded separately. Current-head remote review/check and merge evidence
is tracked in the persistent Kandev plan and PR #4110.
