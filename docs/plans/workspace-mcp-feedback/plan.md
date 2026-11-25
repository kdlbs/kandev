---
created: 2026-10-05
status: complete
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
legacy_specs: []
---

# Workspace MCP User-Test Feedback

## Outcome

Give session MCP additions one entry point, readable inherited-origin names,
and accurate save feedback. Preserve additive selection, desired/applied
revision semantics, workspace boundaries, and the settings interlock.

This package extends the
[completed original package](../workspace-mcp-configuration/plan.md), especially
Tasks 07 and 08. It does not invalidate its historical verification results.
The user authorized implementation in a later turn with "go for fix".

## Confirmed root causes and reproduction

Evidence comes from PR #3293 at
`ffa6d403802250dc8ff9246fe0f827e68d28c519`, the user screenshot, source inspection,
and fresh browser/API requests against an isolated test instance.

1. `MCPSessionSelector` renders an outer disclosure containing
   `MCPSelectionPicker`, which owns another disclosure. Both desktop and phone
   paths require a second action before the selection list opens.
2. `originLabel` formats `origin.owner_id` directly. Names already exist in
   hydrated profile options and workspace repository records.
3. The forwarded test URL injects a bearer credential: the browser sends none,
   but the backend receives one. The interlock rejects bearer credentials even
   with a valid current browser token. It incorrectly returns the same stable
   error code used for a missing/stale token. The API client consequently
   triggers global reload recovery; MCP presenters also show a generic error
   because they ignore the handled-error marker.

Direct pod-local selection writes returned HTTP 200; the same forwarded write
returned HTTP 403. Current boot identity matched and the interlock token was
present and correct. Therefore the original save symptom did not establish a
backend restart or an MCP attachment failure.

Temporary instrumentation recorded only request path and credential/token
presence or match booleans. It was removed, the original test binary restored,
and the owned backend restarted on its original port. Evidence and a SQLite
baseline remain under `/tmp/kandev-pr3293-test-runtime/`; no main-instance data
was copied. The public forwarding configuration is outside this checkout.

## Contracts and traceability

| Outcome | Acceptance criteria | Work order |
| --- | --- | --- |
| Ordinary bearer rejection does not claim restart; genuine stale token produces one recovery surface | `AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9`, `001.11` | 01 |
| Idle additions save normally without bearer credentials; rejected writes do not appear applied | `AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1`, `005.5`, `006.5` | 01 |
| Origin summaries use contextual names and safe missing-name fallback | `AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7`, `003.8` | 02 |
| One session disclosure, native phone sheet, accessible localized controls | `AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3` through `006.6`, `006.9` | 02 |

The agent requirements add the previously unstated name and single-disclosure
outcomes. Platform requirements already exclude ordinary authorization failures
from restart recovery. Their design now distinguishes credential rejection from
stale browser tokens explicitly. No new ADR is needed: credential authorization,
scope composition, persisted IDs, and token transport remain unchanged.

## ASCII UI previews

### UI-01: Session MCP additions, expanded desktop and phone

Entry point: task session, idle agent. Current desktop structure:

```text
[Session MCP additions | 0 selected | v]
  [MCP servers | 0 selected | v]
    Team documentation    Profile: <UUID>
    Repository tools      Repository: <UUID>
    [ ] Issue tracker
```

Proposed desktop structure:

```text
[Session MCP additions | 0 selected | v]
  State: Applied / Pending / Deferred / Failed [Retry when failed]
  Inherited MCP servers
    Team documentation    Profile: MCP sandbox agent
    Repository tools      Repository: MCP sandbox repository
  Session additions
    [Search MCP servers..........................]
    [ ] Issue tracker
    [ ] Session extras
```

Proposed phone composition, directly after tapping the closed summary:

```text
+--------------------------------------------+
| Session MCP additions               [Close]|
|--------------------------------------------|
| State: Applied / Pending / Deferred / Failed|
| Inherited MCP servers                      |
| Team documentation                         |
|   Profile: MCP sandbox agent               |
| Repository tools                           |
|   Repository: MCP sandbox repository       |
| Session additions                          |
| [Search MCP servers......................] |
| [ ] Issue tracker                          |
| [ ] Session extras                         |
+--------------------------------------------+
```

The phone title is fixed; state, errors, search, and choices share one internal
scroll owner. Use the existing Drawer/mobile picker patterns, safe-area spacing,
44-pixel touch targets, focus return, and predictable dismiss behavior. Desktop
remains an inline disclosure. Saved selections survive viewport changes; search
survives closing and reopening the mounted picker. A full task-layout remount
resets its transient query. Names wrap
without horizontal overflow; inherited rows are summaries, not checkboxes.

### UI-02: Rejected selection write

Current: ordinary forwarded authorization rejection produces both a global
`Reload required` alert and local `Could not save the MCP server.` copy.

Proposed ordinary failure, desktop inline or inside the phone sheet:

```text
Session MCP additions
  Could not save the MCP selection.
  [ ] Issue tracker                 (previous saved selection retained)
```

Proposed genuine stale token:

```text
[Reload required ... Reload page]   (shared application recovery surface)
Session MCP additions              (no duplicate local save error)
```

On phones, close the sheet when shared reload recovery owns the failure so the
global recovery control remains visible.

One disclosure, name-based origins, one phone scroll owner, and correct error
ownership are requirements. Drawing spacing, row order, and illustrative copy
are not pixel specifications. Actual copy uses i18next and every shipped locale.

## Work orders and dependency order

Execution is sequential in the primary session; waves do not authorize agents.

- [x] [Task 01: Repair rejection feedback](task-01-repair-rejection-feedback.md)
- [x] [Task 02: Simplify the session picker](task-02-simplify-session-picker.md), after 01

## Verification strategy

Task 01 proves credential classification, handled-error suppression, and
authoritative selection rollback using Go, API client/hook tests, and focused
desktop/phone browser cases. Task 02 changes the existing task journeys to
assert names and one-action list access, then checks sheet containment,
scrolling, keyboard interaction, retained search on reopening, and saved
selection continuity across viewport changes.

Each work order specifies exact commands. Run E2E projects sequentially with
the guarded runner, rebuilding production artifacts. Do not overlap full
suites or increase workers. Final results are recorded below.

## Risks and exclusions

- Forwarded saves remain forbidden while a forwarding service injects bearer
  credentials. Product error correction does not remove that operational
  prerequisite. A direct local port-forward supports browser testing now.
- Never allow a browser token to bypass the bearer prohibition or remove
  authentication headers globally in the client. Do not log credentials.
- Missing hydrated names use localized scope labels; do not broaden inventory
  fetches or expose another workspace's names.
- Keep existing async application polling and prior applied revision behavior;
  saving a selection and provider attachment remain different outcomes.
- No built-in Kandev tool pruning, marketplace redesign, main-instance changes,
  proxy configuration changes, new persistent tasks, commits, pushes, or merge.

## Results

Both work orders are complete. RED cases reproduced bearer misclassification,
duplicate recovery errors, rejected-write checkbox promotion, UUID labels,
nested disclosures, and a phone sheet covering shared recovery. All final
regressions passed after the last production change:

- Go middleware and agent settings handler suites passed.
- 32 tests across the origin resolver, API client, and selection editor passed.
- Guarded browser suites passed sequentially: 4 phone and 3 desktop tests.
- Typecheck, targeted ESLint, localization checks/ratchet, and generated
  Traditional Chinese/pseudo catalogs passed.
- Public documentation tests passed (62 tests), with 47 pages validated.
  Specification/catalog validation passed (348 decisions, 1334 specifications),
  as did specification lint and whitespace checks.

Installed the tested backend/web artifacts into the owned isolated runtime at
`127.0.0.1:48593`, retaining its database after a SQLite backup. Backend PID
1696869 is healthy. Fresh desktop and phone checks verified readable repository
and task names, no UUID fallback or inner trigger, one phone sheet, a 44-pixel
Close target, and no horizontal overflow. Automated journeys also verified
profile names, keyboard operation, positive saves, and recovery behavior.

A fresh browser write through `kandev.soitgoes.cc:38383` returned HTTP 403 with
`error_code: forbidden`, showed only the selection error, kept the prior
selection, and produced zero reload alerts. Forwarding still injects bearer
credentials; it is not repaired by this package. Use a direct port-forward to
test successful saves. Main :9998 remains PID 3009187 and was not modified.
The verification browser was closed. Changes remain uncommitted; PR-wide CI
and review completion remain separate pending work.

## Later user steering

The user subsequently requested replacing the existing-session bar with the
composer plug and reported repeated loading flashes. The
[composer customization package](../composer-mcp-customization/plan.md)
supersedes this package's inline existing-session composition. Historical
results above remain valid for the delivered version; they do not claim the
new interaction or stable pending refresh is implemented.


## Follow-up forwarding diagnosis (2026-10-06)

The user's renewed save-failure report confirmed that :38383 is Kandev's own
main-runtime tunnel. Its authenticated agentctl proxy passes a consumed control
credential to the nested test application. The earlier characterization as only
an external forwarding limitation is superseded. Existing direct browser/test
results remain historical; this application package did not repair that executor
boundary. See [the follow-up package](../mcp-forwarding-session-clarity/plan.md)
for control-credential containment and clearer session timing/error copy. No
main/test runtime restart or new production edit occurred in this diagnosis.
