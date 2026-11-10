# MCP sandbox testing guide

For the current full-PR sweep, use the [final testing guide](../workspace-mcp-configuration/testing-guide.md). The scenarios below are historical seed descriptions; saved selections may have changed.

Forward pod-local port **48595**. These are five real tasks in the existing
`PR 3293 MCP sandbox` workspace. Allow **15 minutes** for the full pass; start
with case 01 for a two-minute check. The six added MCP definitions use local
fixtures only. Existing tasks, selections and main instance 9998 are preserved.

The clearer UX from [the plan](plan.md) is deployed. Customize separates
**From task setup** from **Additional servers**. Application notices explain
what stays in use and what happens next, using icons and restrained colors.
Hover and Connections distinguish **Saved changes** from **Current agent**.

## 01: Inheritance across two repositories (2 minutes)

[Open the inheritance task](http://localhost:48595/t/f2a32026-ffcc-458a-8642-ab2fe4f4d9c7).

1. Click the composer plug, then **Customize**. Inspect Atlas shared
   documentation, Atlas issue tracker and Atlas review notes.
2. Check origins: shared documentation comes from **Atlas inherited tools**,
   **Atlas API repository**, and this task. Issue tracker comes from
   **Atlas operations repository**. Names must be readable, with no UUIDs.
3. Switch to **Connections**. Each runtime name appears once. Open the existing
   `lookup_demo_docs` tool call in the transcript, or send
   `e2e:mcp:atlas-docs:lookup_demo_docs({})` and expand its tool row.

**Expected:** the fictional Atlas documentation is returned. Duplicate scope
selections produce one effective server. Inherited choices cannot be removed
from the session picker. Kandev can be Active; Atlas servers remain Delivered
because their direct connection evidence is not observed by Kandev. The tool
call succeeding does not turn the attachment row green automatically.

## 02: Mixed transports and missing executable (2 minutes)

[Open the mixed-transport task](http://localhost:48595/t/952e07bf-7c4f-4235-8fbe-069cd469b694).

1. Hover or focus the plug on desktop. Compare `kandev`, `atlas-docs`,
   `issue-tracker`, and `atlas-missing-cli`.
2. Click the plug and inspect the corresponding connection rows. On a phone,
   tap the plug to inspect the same states without hover.

**Expected:** Kandev is Active, Atlas docs is Delivered, and Issue tracker
says **Not available** with an explanation that this agent does not support
HTTP MCP servers. The mock agent advertises SSE but not Streamable HTTP.
The missing executable is currently Delivered by this mock path; that proves
configuration acceptance, not installation or launch. This limitation is part
of the case, not a fake successful executable.

Delivered explains that the connection is unverified. Active Kandev must not
display its obsolete HTTP-filter reason.

## 03: Session additions and reload persistence (3 minutes)

[Open the persistence task](http://localhost:48595/t/3746236f-2717-45de-b47a-5aad784486dc).

1. Open plug **Customize**, search for `Atlas review notes`, and select it.
2. Close and reopen the overlay, then reload the page and reopen it. Check the
   choice remains selected and inspect the application notice.
3. Once the state says applied, send
   `e2e:mcp:atlas-reviews:read_demo_review({})` and expand the tool result.

**Expected:** save succeeds with no save warning or false Reload required banner.
Selection survives reload; transcript, profile and session identity are retained.
Search and focus remain stable during background refresh without a loading
flash. The saved state and current agent evidence may differ until the adapter
accepts an update. If application remains pending or reports failure, record
that state; do not interpret saved selection as a successful connection.

**Reset:** restore Atlas review notes to its selection before this test. Record
that starting choice before step 1. Profile/repository selections remain inherited.

## 04: Change tools during a long turn (3 minutes)

[Open the busy-turn task](http://localhost:48595/t/02a7c9e6-a20f-4a92-a980-7fc7e199578b).

1. Send `/sleep 20` to hold this local mock agent's turn open for 20 seconds.
2. During that turn, open plug **Customize** and select Atlas review notes.
   Close the overlay and inspect the clock marker and desktop hover.
3. After the turn finishes, reopen the plug and inspect the saved application
   state. If applied, send `e2e:mcp:atlas-reviews:read_demo_review({})`.

**Expected:** a save can succeed while application waits. The current turn keeps
its previous configuration. Pending, applied, next-start and failure states
must stay distinct; only a recorded applied transition removes the clock.
No transcript is duplicated and no second task session appears. The
hover separates **Saved changes** from **Current agent**, making this visible.

**Reset:** after the turn finishes, restore Atlas review notes to its selection
before this test. Record that starting choice before step 1.

## 05: Disabled selection and endpoint outage (3 minutes)

[Open the disabled/outage task](http://localhost:48595/t/efa94fbe-e6ca-49cf-a93f-0e739ec2862d).

1. Inspect **Customize** before starting the agent. Atlas archived documentation
   was selected on the task before it was disabled; it must remain saved while
   being excluded from new delivery.
2. Start this task's agent using its normal composer/start control. Inspect
   **Connections**: Atlas issue tracker is inherited from the operations
   repository; the disabled definition must not be delivered.
3. Send `e2e:mcp:atlas-outage:lookup_demo_docs({})` and expand the tool result.

**Expected:** the local outage endpoint returns HTTP 503 and the scripted call
fails. A saved/delivered configuration is not proof the endpoint worked.
The attachment row may remain Delivered because a generic tool-call error is
not a provider-reported server attachment failure. Other rows keep their own
states. No unrelated selection or main-instance setting changes.

To test enable/disable explicitly, use [workspace MCP settings](http://localhost:48595/settings/workspaces/f8ec5269-f393-4653-a954-c71d8176f926/mcp-servers)
and change only **Atlas archived documentation**, then return to this task.
Restore it to disabled afterward. Definitions already delivered to a running
turn must not be silently replaced.

## What to record

For an unexpected result, record the case number, action, saved application
notice, and connection row label/reason. A screenshot with the plug overlay
open is enough to make the difference visible. Note desktop versus phone.
Do not include tokens, secret values, or raw agent output containing credentials.

Next-start and explicit reconnect-failure presentation use controlled
browser reports. The five live tasks do not manufacture those states or claim
coverage for real Claude/Codex/Cursor adapters.

## Ownership and shutdown

The existing app is on loopback 48593, repaired entry point 48595, original
HTTP fixture 48594, and added local SSE fixture 48596. Forward only 48595.
The source base is `ffa6d403802250dc8ff9246fe0f827e68d28c519` plus the uncommitted repair and UX changes. A fresh frontend build was deployed
on 2026-10-07; existing preview processes and the main instance were retained.
The scenario ledger is `/tmp/kandev-pr3293-test-runtime/complex-scenarios.json`.
The additive seed script checkpoints owned IDs and can be rerun without
duplicating its records; it does not restore changes made during testing.

Stop all owned test services while retaining the data:

```bash
python3 /tmp/kandev-pr3293-test-runtime/stop.py
```

## Seeding verification

Six definitions, two profiles, two local repositories, and five tasks were
created through the owned sandbox API. Four new agents reached Waiting for
input; the disabled/outage case is prepared for manual start. Runtime reports
confirm SSE configuration delivery and typed HTTP transport exclusion. A
recorder-free browser rehearsal and API persistence results are recorded in
the runtime evidence files; no DOM state or attachment history was fabricated.

Verified outcomes: named multi-repository origins, a real local SSE tool result,
selection save 200 and reload persistence, applied revision after idle, one
phone drawer without horizontal overflow, and restored session selections.
A 12-second busy-turn rehearsal recorded Pending during the turn, Applied
afterward, removal of the clock marker, and the same single session. The seed
rerun preserved five unique tasks without duplicate records. The outage case's
disabled selection and local 503 were checked; manual start remains for the
guided test. The original session's pending state was preserved.

The preview also produced an existing terminal-pane WebSocket 404 during the
rehearsal. This guide verifies MCP controls and does not claim terminal coverage.
Detailed evidence is `/tmp/kandev-pr3293-test-runtime/complex-scenarios-rehearsal.json`
and `busy-scenario-rehearsal.log`. The shutdown script includes the added SSE
fixture and retains its process-identity guards.

## UX delivery verification (2026-10-07)

The updated hover, Connections and Customize surfaces are deployed at 48595.
Forty unit tests and sixteen distinct desktop/phone browser cases passed;
TypeScript, scoped lint, seven-language completeness and documentation checks
passed. Controlled application reports prove next-start/failure presentation;
live save/reload and a 12-second busy turn prove actual pending-to-applied timing.

Screenshots cover desktop and phone in light/dark themes. Current saved choices
in all four selection scopes match the snapshot captured before UX deployment.
No seed reset was run. Main 9998 and the existing preview processes were retained.
Own diagnostic browser closed. Exact live outcomes, screenshots and artifact
hashes are in `/tmp/kandev-pr3293-test-runtime/ux-rehearsal.json` and
`ux-deployment.json`. Inherited disabling remains a separate follow-up.
