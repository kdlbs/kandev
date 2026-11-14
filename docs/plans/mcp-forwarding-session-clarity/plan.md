---
created: 2026-10-06
status: complete
requirements:
  - REQ-EXECUTORS-PROXY-CREDENTIAL-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
system_design:
  - ../../specs/executors/system-design/proxy-control-credential-containment.md
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
legacy_specs: []
---

# MCP Forwarding and Session Clarity

## Overview

Repair Kandev's consumed-control-credential forwarding and make session MCP
customization understandable. The reported save failure is a real Kandev proxy
bug, not a selection-picker failure or an external proxy that this repo cannot
repair. Keep the previously delivered composer, stable refresh, and origin fixes.

Executors own control authentication at application forwarding; Agents own the
independent selection state and copy. These are distinct reusable contracts,
so the package links both existing system boundaries without duplicating them.

## Evidence and root causes

On 2026-10-06 a fresh browser at `kandev.soitgoes.cc:38383` sent no Authorization
and sent the SPA interlock. Selecting Session extras returned 403 `forbidden`,
retained the saved checkbox state, and produced no reload banner. Test backend
logs record the user's repeated selection 403s. The :38383 tunnel listener is
owned by the main backend PID 3009187; the nested test app remains PID 2107007.

The gateway's authenticated agentctl transport supplies a control bearer header.
`api/port_proxy.go` leaves it on the downstream request after `instanceAuth`
accepts it. The destination Kandev interlock correctly rejects bearer writes.
`handleVSCodeProxy` shares the request projection and the same containment gap.
A temporary real-auth/real-interlock Go reproduction returned 204 directly and
403 through agentctl. The temporary test was removed. Evidence is retained under
`/tmp/kandev-pr3293-test-runtime/port-proxy-followup-diagnostic.log` and
`mcp-ui-followup-evidence.json`, without real credentials.

The screenshot also exposes three concrete copy defects: the generic idle
promise contradicts deferred-next-start state; This scope does not name the
session; Desired revision is implementation detail in the primary choice flow.
Inherited rows are legitimately read-only under additive scope selection.
The existing state is saved revision 3 from the earlier reversible live test;
rejected clicks do not save another revision. No claim of a new applied change.

## Scope

Repair consumed Authorization projection for port/editor proxies, preserve
control admission and stream fencing, and clarify session copy/forbidden-save
feedback. Keep the existing desktop dialog and phone/coarse-pointer drawer,
visible plug markers, origin names, query persistence, and stable loaded rows.

No changes to inherited selection semantics, provider reconnect capability,
backend MCP persistence, application bearer rejection, main-instance deployment,
new application-auth channels, marketplace/built-in policy, PR publication,
or speculative visual redesign. Arbitrary additional UI issues not demonstrated
by the screenshot/reproduction remain unclassified.

## Technical approach

Task 01 marks successful control-auth consumption in trusted Gin state. The
existing `credentialScopedRequest` creates an outbound clone with consumed
Authorization removed, preserving its authenticated-generation invalidation.
Both proxy handlers already use it. Auth-disabled application headers retain
current behavior. The gateway-to-agentctl credential remains mandatory.

| Boundary | Behavior | Evidence |
| --- | --- | --- |
| Static/dynamic instance auth | Consume accepted control header before app | Real proxy integration |
| Rotating control-server auth | Preserve admission; no header leakage | Auth and credential tests |
| Disabled agentctl auth | Preserve app Basic/Bearer | Mixed authenticated/disabled cases |
| Invalid/revoked control auth | Refuse before app, keep stream fencing | Existing rotation tests |
| Browser HTTP/stream/WS | Preserve app data and negotiation | Proxy integration and gateway suites |

Task 02 reuses the existing picker with a session-specific section label. Timing
belongs only to authoritative state notices; creation copy makes no idle promise.
Remove the visible revision counter and simplify the deferred explanation. A
classified ordinary forbidden save describes a connection that cannot save and
retains previous choices. Other failures and handled reload keep their owners.
All copy is translated in every shipped locale. Keep mobile layout and geometry.

## ASCII UI preview

### UI-01: Customize on desktop or touch

Current: `[Applies on next start] [Desired revision 3]`, followed by a promise
that changes apply at idle and a section named This scope.

```text
MCP servers                                  [Close]
[Connections] [Customize]
[Applies on next start]
Saved changes apply when the agent starts again.
Choose MCP additions for this session.

Inherited MCP servers
  Issue tracker     Task: <readable task name>
  Repository tools  Repository: <readable repo name>

Add to this session
[Search MCP servers...............................]
[ ] Session extras
```

Desktop retains the current dialog. Phone retains the single full-height drawer
with fixed header/tabs and one scroll body; coarse-pointer tablet remains bounded.
Use the existing MCPSessionSelectionBody explorer composition as the mobile
exemplar. State text, selection/error logic and filtering remain shared. Controls
stay 28px fine-pointer and at least 44px touch; no nested sheet or hover-only action.

### UI-02: Genuine forbidden save

```text
This connection cannot save MCP selections.
Your previous choices are unchanged.
[ ] Session extras          remains unchecked
```

The error is local, the saved-state notice describes the previous authoritative
state, and there is no false Reload required. Fixing the proxy makes ordinary
valid browser writes succeed; this error remains for genuine rejection. Labels
are illustrative; AC-006.13 requires consistent timing, session scope, and no
primary revision counter. AC-006.12 preserves evidence/error ownership.

## Tests

Task 01 uses real agentctl auth plus the real interim settings interlock as an
HTTP application. Desired regression: authenticated forwarding succeeds with no
control header reaching the app. It must fail before the correction with the
observed 403. Static, rotating, invalid and disabled cases prevent global stripping
or skipped authentication from satisfying the test. Include upgrade/stream paths.

Task 02 adds pure error classification tests and browser assertions for deferred
copy, explicit session label, absent revision counter, and retained choices on
ordinary/handled rejection. Existing hook/origin regressions remain applicable.

## E2E tests

Task MCP desktop and phone suites retain real save/origin/closed-pending journeys.
Add deferred-state copy assertions through controlled GET responses and update
forbidden-error expectations. Task 01's live proxy integration proves the control
boundary; preserve gateway transport and port-forward-dialog regressions.
Do not count direct-browser tests as proof that the :38383 live hop was upgraded.

## Work orders

- [x] [Task 01: Contain proxy control credentials](task-01-contain-proxy-credentials.md)
- [x] [Task 02: Clarify session customization](task-02-clarify-session-copy.md)

Sequential primary-session work; no delegation. Task 02 can be checked
independently, but end-to-end forwarding must pass before delivery is claimed.

## Verification results

Both work orders are implemented and verified. Permanent regression first
reproduced static/dynamic forwarding 403, then passed with trusted control-header
consumption. Final auth/credential/port/editor tests passed under the race
detector; full backend lint reported zero issues. HTTP, streamed and upgraded
requests retain application data and authenticated-generation fencing.

- 56 frontend tests across seven files passed. Typecheck and targeted ESLint passed.
- 14 port-forward, seven desktop/tablet MCP and seven phone MCP browser journeys
  passed, sequentially with one worker and no retries (28 total).
- Full interlock and gateway suites passed without exclusions in an isolated
  shell/config environment. Earlier shell-marker failures were environmental;
  the first shared-cache lint/build failures were retried with an owned cache.
- Every shipped locale, pseudo check and i18n ratchet passed. Unrelated generated
  catalog normalization was removed while preserving prior edits.
- 62 public-doc tests, 47 published pages, 349-decision/1336-spec catalog validation,
  all-spec lint and whitespace passed. Initial design gates are retained in history.

Owned deployment uses final native/backend and web artifacts, a private SQLite
backup, retained selected IDs, and recorded source/artifact hashes. :48593 is
the underlying app; :48595 is a fresh owned production-API/authenticated proxy.
Deployed desktop and phone checks passed explicit scope/timing copy, absent
revision noise, readable origins, one drawer, touch geometry/no overflow, real
save 200, reload persistence, zero local errors/reload banners, and restoration
200 to the original selected IDs. The observed live state remains pending idle;
tests of deferred copy use controlled GET fixtures, without falsifying live state.

Historical main/test diagnosis PIDs were no longer live at repair inspection.
Only owned sandbox processes were restored. Current main :9998 PID 27220 retains
its start identity. The old :38383 path was not upgraded or configured by this
work. Diagnostic browsers are closed; owned test app, fixture and repaired
preview remain running. Stop with `python3 /tmp/kandev-pr3293-test-runtime/stop.py`.

Detailed evidence and delivery manifest live under
`/tmp/kandev-pr3293-test-runtime`. Sources and artifacts are uncommitted;
no pushes, PR changes, delegation or main-instance mutation occurred.

## Risks

- Changing the nested test binary cannot update the main session's agentctl hop.
  Keep :9998 unchanged and validate with an owned repaired proxy. Deployment to
  the active :38383 path requires a separate explicitly authorized runtime step.
- Strip only a credential authenticated and consumed by agentctl; disabled-auth
  applications may intentionally use Authorization.
- Do not infer credential consumption from a rotation source's presence or break
  the exact authenticated generation's stream cancellation.
- Unsupported live MCP reconnect remains truthful next-start deferral; improving
  copy must not fabricate hot application or provider connection evidence.
