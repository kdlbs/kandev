# PR #3293 final testing guide

**Resumed-machine status (2026-10-20):** the sandbox links and stop command in
this guide belong to the previous machine. That sandbox is not available here.
Use freshly built, isolated fixtures for current verification. The resumed
delivery ledger is in [merge readiness](merge-readiness.md); earlier counts
below remain historical.

Start with the [inheritance task](http://localhost:48595/t/f2a32026-ffcc-458a-8642-ab2fe4f4d9c7)
and open **MCP plug > Customize**. Allow **25 to 35 minutes** for the main sweep,
plus **10 to 15 minutes** for the optional real-provider checks.

This guide covers PR #3293 and its follow-up fixes. The pod-local links refer to
the original seeded sandbox, whose running build must be checked before manual
verification. Current integrated runtime and browser evidence is recorded in
[merge readiness](merge-readiness.md); older sandbox receipts do not establish
that the final PR head passed. Publishing the PR does not authorize merging it.

**Review repair coverage (2026-10-19):** sections 3, 4 and 8 cover the single
repository disclosure, shared choice metadata and mobile Connections geometry.
See the [repair package](../mcp-picker-mobile-repair/plan.md) for implementation
and regression evidence.

## Before testing

1. Forward pod-local port **48595** and refresh the page. Replace `localhost:48595`
   in these links if your local forwarding port differs. Do not use the main
   instance on **9998** for this sweep.
2. Use workspace **PR 3293 MCP sandbox**. Take a screenshot of current selected
   servers before changing them. Earlier manual tests changed some fixtures;
   their titles and descriptions do not define their current saved selections.
3. For create/edit/delete tests, use new definitions and tasks with a
   `manual-final-` prefix. Restore any existing selection or enabled state that
   you change. Delete only the scratch resources you created.
4. Test once on desktop, then use a phone or narrow viewport for section 8.
   Keep the browser console available for unexpected errors.

**Separate PR:** temporarily turning off inherited servers for one session is
not included. Inherited rows should remain read-only in this PR. Workspace-wide
disabling is a different operation and already exists.

**Deferred profile UI:** global agent profiles still use the legacy JSON editor.
The user explicitly moved global-profile catalog navigation and management/status
clarity to unstarted follow-up task `2adba1ca-619e-4bfc-b705-3bbc628df4dd`, based on
this PR. Workspace-owned profiles have the shared picker. The original sandbox
agents are global, so do not count their legacy editor as a successful catalog
picker test. Existing API/resolution support for workspace-scoped global-profile
associations is separate from the deferred UI. The original reproduction is in
the [consistency review](ux-consistency-review.md#functional-finding-global-profile-catalog-picker-is-absent).

### Review repair checks

In Configured, edit an existing executable with arguments containing spaces, an empty argument and a path. Change only its description, save, reopen and confirm each argument retains its own row. Add an environment value and bind a credential through Secret bindings; after saving and reopening, the reference remains selected and its value is never displayed. Repeat on a phone and check every control is reachable without horizontal scrolling.

In Marketplace, inspect a choice with required inputs. Add MCP server stays disabled until required values and secret references are complete. Unsupported variable substitutions or argument inputs explain why the choice is unavailable. In a direct selection list, a previously selected definition that was later disabled can be unchecked but cannot be newly selected. In Connections, an unreadable secret produces an unavailable row with a safe explanation while other MCPs continue to attach.

## Surface map

| Surface | Main check | Section |
| --- | --- | --- |
| Workspace navigation and breadcrumb | Consistent MCP servers label and correct workspace | 1 |
| Configured catalog and definition editor | CRUD, three setup modes, metadata and selection impact | 1 |
| Marketplace and installation review | Discovery versus saved definition versus runtime startup | 2 |
| Agent profile and repository settings | Scope ownership, save/discard and inheritance | 3 |
| New task Advanced settings | Compact picker and persisted task selections | 4 |
| Add agent/new session | Session additions and independent sibling sessions | 4 |
| Composer hover, Connections and tools | Current-agent evidence, reasons and navigation | 5 |
| Customize source sections and richer choices | Origins, metadata, autosave and persistence | 6 |
| Pending/apply/recovery states | Exact changes, timing, stable refresh and error ownership | 7 |
| Phone, tablet, keyboard and localization | Reachability and parity | 8 |
| Application forwarding and editor/port surfaces | Save through forwarding without leaking control credentials | 9 |
| Real provider/executor behavior | Actual tools, package preparation and reconnect support | 10 |

## 1. Workspace catalog and setup modes (5 minutes)

Open [MCP servers settings](http://localhost:48595/settings/workspaces/f8ec5269-f393-4653-a954-c71d8176f926/mcp-servers).

1. Confirm the tab, heading and breadcrumb say **MCP servers**. The breadcrumb
   names the owning workspace. **External MCP** remains a separate destination
   for exposing Kandev to outside clients.
2. Open **Add MCP server**. Create `manual-final-sse` in **Remote endpoint** mode,
   with a recognizable description and the local fixture endpoint
   `http://127.0.0.1:48596/docs/sse` using **Server-sent events**. Use the shared
   **Save changes** control, then reload and reopen the definition.
3. Edit its description. Verify the card shows the updated text, enabled state,
   runtime alias, readable mode/transport/source labels and selection usage.
   A new unused definition should say it is not selected anywhere.
4. Try a second definition with the same runtime name. Try incomplete remote
   setup, an empty existing-executable command and an unpinned npm version in
   separate drafts. Cancel invalid drafts rather than altering seeded servers.
5. Inspect **Existing executable** and **Managed npm package** modes. Confirm
   the former says the command must already exist, and the latter identifies
   an exact package/version and explains installation on first agent use.

**Expected:** valid edits persist; invalid or duplicate setup cannot save.
Saving a definition does not connect to its endpoint, execute its command or
download the managed package. Descriptions and metadata do not imply health.
Switching workspace must not expose another workspace's definitions.

**Disable/delete:** after selecting the scratch definition in a scratch task
in section 4, return here. Disable it: saved selections remain, but new delivery
excludes it. Re-enable it and confirm the saved selection is still present.
Delete only the scratch definition after examining the affected-scope warning
and confirming it. Cancel a deletion once to verify the definition remains.
Selection counts should use correct singular/plural wording.

## 2. Marketplace and review (3 minutes)

In the same settings page, open **Marketplace**.

1. Search for an entry and inspect its source, description and available package
   or remote choices. Open its review/setup surface without saving it.
2. If the entry has multiple choices, switch between them. Review transport,
   exact package/version or endpoint, required inputs and secret bindings.
   Unsupported choices should be unavailable with an explanation.
3. Add one supported entry only if you want a scratch definition for later
   real-provider testing. Confirm it appears under **Configured**, with the
   reviewed identity, version and source. Remove it during cleanup if unused.

**Expected:** browsing/review does not run the server. A remote option does not
install a package; a managed package is prepared in the task environment when
an agent first uses it. Registry metadata is publisher supplied, not a security
approval. Cache/outage messages must explain when previously saved results are
being shown; an unavailable upstream must not silently erase saved definitions.

Stale Registry and unsupported-package cases have controlled automated coverage.
Do not disable the main instance's network to manufacture an outage. If the
live registry is unavailable, record the actual warning and available results.

## 3. Repository and profile selection (4 minutes)

Open **Settings > Workspaces > PR 3293 MCP sandbox > Repositories**, then edit
a repository. Prefer a scratch repository; otherwise record and restore its
initial selections because they affect every task using that repository.

1. Confirm **MCP servers** starts collapsed with a selected count. Open it
   once: search and choices should appear directly, without a second MCP
   disclosure. Each choice should show its runtime alias, supplied description
   and readable setup-mode, transport and known catalog-source badges.
2. Select an enabled scratch definition. Use **Save changes**, reload and
   confirm it remains selected. Make another change and discard it: the saved
   selection should remain intact.
3. Confirm disabled definitions cannot be newly selected. Search for a name
   that matches nothing: the empty message should describe the search result.
4. On a workspace-owned agent profile, if available, perform the same save,
   discard and reload check on its **MCP servers** card. Confirm other profile
   controls, transport policy and custom TUI settings remain separate.

**Expected:** selections stay with the chosen owner and workspace. Repository
and profile settings use shared Save changes; they do not autosave like session
Customize. There should be no UUID-based origin labels.

For the sandbox's global Mock profiles, open **Settings > Agents > Mock >
profile** and confirm the known routing gap: **MCP Configuration / MCP servers
(JSON)** appears instead of the catalog picker. Seeded profile inheritance can
still be inspected in tasks, but that does not prove the missing editor works.

## 4. Create a task and add another agent (4 minutes)

1. Open **New Task**. Keep Advanced settings closed initially: the form should
   not show a large persistent MCP panel. Expand it and inspect the compact
   MCP label/count/help control beside the established advanced controls. Open
   its picker: choices should include descriptions and the same metadata badges
   as repository settings and session Customize.
2. Select `manual-final-sse` as a task addition and create a scratch task with
   the desired repository/profile. Open its plug > Customize. The server
   should appear under **Selected for this task**, not as a session checkbox.
3. Add another agent/session to that task. Choose a different enabled server
   as a session addition in the creation dialog. Confirm the new session has
   it, while the first session's own additions remain unchanged.
4. Inspect a server selected through more than one scope. It should be one
   inherited row with all sources explained, and one effective server. If
   your task has multiple repositories, contributors from each must be named.

**Expected:** task selections survive adding agents, changing the active
session and reloading. Session additions belong to that session. An empty
task/session selection does not subtract inherited servers. Canceling a creation
dialog must not create a task/session or save a partial selection.

## 5. Connections, hover and tool inspection (2 minutes)

Open the [mixed-transports task](http://localhost:48595/t/952e07bf-7c4f-4235-8fbe-069cd469b694).

1. Hover or keyboard-focus the plug on desktop. Read **Current agent** and,
   when applicable, **Saved changes**. Long names/statuses must align and wrap.
2. Click the plug and use **Connections**. Open a server, inspect its tools,
   open a tool and navigate back. Switching to Customize should retain useful
   connection navigation context.
3. Compare `kandev`, the remote Atlas definitions, an unsupported HTTP server
   and the missing-executable fixture, if they remain in this task's setup.

**Expected:** Kandev may be **Active** with tools loaded. **Delivered, connection
unverified** means configuration acceptance, not a successful connection.
Unsupported transports say **Not available** with a reason. An Active row must
not retain an obsolete unsupported-transport explanation. The mock advertises
SSE support, not Streamable HTTP; its missing-executable row can be Delivered
without proving the executable exists or ran.

Inspect a previous Atlas tool result or use a fixture call from section 10.
A successful tool call does not automatically turn every attachment row green.

## 6. Customize origins and richer choices (3 minutes)

Open the [inheritance task](http://localhost:48595/t/f2a32026-ffcc-458a-8642-ab2fe4f4d9c7)
or your scratch task, then **MCP plug > Customize**.

1. Read **Selected for this task**, **From repositories** and **From agent
   profile**. Only populated sections should appear. Check the actual current
   sources, not the old fixture description. Repository/profile names are
   plain text, with additional contributors under **Also selected by**.
2. Verify each inherited server appears once. Task selection determines its
   display group before repository, then profile. This is presentation order,
   not configuration precedence. Inherited rows have no removal checkbox.
3. Inspect **Additional servers**. Rows include name, runtime alias, a supplied
   description and readable mode/transport/catalog-source badges. Empty
   descriptions do not need a placeholder. **Custom** describes catalog
   provenance, not which task, repository or person selected the server.
4. Select one enabled additional server. Close/reopen, then reload. Its saved
   checkbox state must persist. Search should survive closing/reopening in
   the same session; a page reload may reset transient search text.
5. Scroll to the actual last choice. Check keyboard access, the whole selectable
   row and wrapped detail. Badges are labels, not extra buttons. Inherited
   definitions must not also appear as selectable additions.

**Expected:** no extra conversation bar, nested Customize disclosure, loading
flash or duplicate origin-chip row. Session changes autosave; no new Apply/Save
button is required. A rejected save must not appear saved or applied. Restore
your starting session selections when finished.

## 7. Pending changes, persistence and errors (4 minutes)

Use the [persistence task](http://localhost:48595/t/3746236f-2717-45de-b47a-5aad784486dc)
and [busy-turn task](http://localhost:48595/t/02a7c9e6-a20f-4a92-a980-7fc7e199578b).

1. On the persistence task, toggle an enabled additional server. While its
   application status refreshes, keep search focused and scroll partway down.
   Choices, search, focus and scroll must not be replaced by a loading screen.
2. On the busy-turn task, send `/sleep 20` to this isolated mock agent. During
   that turn, change an additional selection and close the overlay. The plug
   should show pending status while the current turn retains its prior setup.
3. Reopen Customize. When a recorded comparison is available, **Changes waiting
   to apply** identifies each add/remove/update separately. An unchecked server
   can still be waiting for removal. Searching the bottom list must not hide
   pending actions above it. A server retained by another scope is not removed.
4. After the turn ends, inspect the actual status. Applied clears accepted
   pending actions; next-start deferral explains that timing; failure retains
   saved choices and explains retry. None of these states claims server health.
5. Reload or switch between sessions. There must be no duplicate transcript,
   replacement task session or one sibling's choices leaking into another.
   Restore the selections you changed.

The [exact-change task](http://localhost:48595/t/c754e47d-2540-4e30-a53f-9103e907a568)
was seeded to demonstrate add/remove/update, but earlier testing may already
have applied those changes. Do not expect a fixed count of three today. Older
fixtures can have unavailable comparison: the UI must say exact changes cannot
be listed yet instead of inventing actions from connection status.

**Error ownership:** a normal network/authorization rejection stays local and
retains authoritative choices; it must not claim Kandev restarted. A genuine
stale page token uses the shared Reload required surface without a second
local save warning; on touch, the overlay closes so recovery is reachable.
Controlled E2E tests cover rejection, stale recovery, deferred and failed states.
Do not restart the main instance to manufacture these errors.

## 8. Phone, tablet, keyboard and language (3 minutes)

1. Repeat opening the plug, reading origins, selecting a scratch session
   addition and reloading on a phone. It should be one drawer with fixed
   title/view controls, one internal scroller and reachable final rows.
2. Inspect long names/descriptions at **320px**, normal phone width and
   **767px with a mouse**, plus a **900px touch tablet**. In Connections, each
   server row must contain its name, status and complete explanation without
   overlapping the next row. The touch title and description should be centered
   on the drawer, with Close contained and clear of text. No sideways page
   scroll or clipped badge/source text.
   Touch controls must remain large enough to activate; desktop controls retain
   the surrounding compact sizing.
3. Use Tab and Space/Enter to open and change a checkbox. Escape/Close should
   dismiss predictably and return focus to the plug. Hover must not be required
   to customize on a phone or touch tablet.
4. Check light and dark themes. Switch to another shipped language or pseudo
   in Settings > General > Appearance and reopen the feature. Labels should be
   translated and metadata/source text should still wrap. Restore your locale.

**Expected:** the same saved state and capabilities on desktop and touch.
Resizing must not create a second selection controller or reset a saved choice.
Task creation's picker remains inside Advanced settings; it does not turn into
the full session Customize screen.

## 9. Forwarding regression (when applicable)

If you access this isolated app through **Kandev's application port forwarding**,
repeat section 6's save/reload check through that URL. This covers the original
reported save failure caused by an authenticated control header reaching the
nested application. Direct pod port-forwarding alone does not test that hop.

**Expected:** valid browser saves succeed without a false 403/Reload required
pair. Normal port/editor views remain usable. Consumed control credentials must
not reach the downstream app, while ordinary application authentication remains
supported. Backend auth/rotation/proxy/interlock and race tests are the evidence
for credential containment; do not expose real credentials in screenshots or
attempt to validate this by sending them to a public echo service.

## 10. Optional runtime smoke checks (10 to 15 minutes)

The mock sandbox verifies selection and presentation. It does not certify
reconfiguration support for every real Claude/Codex/Cursor/OpenCode adapter or
successful npm installation in every executor.

For local fixtures, once the appropriate definition is in the accepted setup,
send one of these messages and expand the resulting tool row:

```text
e2e:mcp:atlas-docs:lookup_demo_docs({})
e2e:mcp:atlas-reviews:read_demo_review({})
e2e:mcp:atlas-outage:lookup_demo_docs({})
```

Use [the disabled/outage task](http://localhost:48595/t/efa94fbe-e6ca-49cf-a93f-0e739ec2862d)
for the third message. Start its agent first if the task is still unstarted.
The outage fixture deliberately returns HTTP 503. The archived definition's
enabled state may have changed in earlier tests; record it before testing.

For a real-provider pass, use an isolated scratch task and a provider/executor
already installed in the environment:

1. Select a reviewed server compatible with that provider. Start the agent and
   invoke one harmless tool. Confirm the result, not just a Delivered label.
2. For managed npm, verify the exact configured version is prepared in the task
   environment on first use, without modifying repository manifests/lockfiles.
   For an existing executable, test both a present command and an absent one.
3. Change a session addition while idle and during a turn. Confirm the actual
   adapter's result: safe reconnect in the same conversation, next-start
   deferral or an explicit failure. Do not infer reconnect support from the
   presence of a checkbox or the mock's successful path.

**Expected:** disabled definitions remain saved but are excluded from new
delivery; unsupported/missing resources are explained. Failed application does
not replace the last accepted baseline or discard the transcript. A generic
tool-call error does not automatically rewrite every connection row's health.

## Finish and record results

| Result | What to record |
| --- | --- |
| Pass | Section, desktop/phone/provider used, and one sentence of observed behavior |
| Fail | Starting selection, exact action, expected result, actual application notice and connection label |
| Blocked | Missing provider/fixture capability or known global-profile gap; do not count it as passed |
| Reset | Scratch resources deleted and previous selections, enabled states and locale restored |

Record screenshots with the relevant panel open. Exclude secret values, tokens
and credential-bearing output. Do not delete seeded servers to clean up scratch
tests. The nested repository disclosure and global-profile gap are documented
separately from the deferred inherited-session disabling feature.

For merge readiness, use the current committed branch and its eventual CI/review
results in addition to this manual sweep. The latest origin-section change has
54 targeted unit checks, 13 desktop and 10 phone E2E checks, plus six live-preview
rendered modes. Catalog/marketplace and forwarding evidence lives in their
respective completed work orders; those results are not a fresh all-PR audit.

To stop only this isolated environment after testing:

```sh
python3 /tmp/kandev-pr3293-test-runtime/stop.py
```

Next: open the inheritance task and complete section 6 before changing settings.
