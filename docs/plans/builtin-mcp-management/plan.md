---
created: 2026-10-22
status: completed
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
legacy_specs: []
---

# Implementation plan: Selectable built-in MCP packages

## Pi delivery follow-up (2026-11-05)

[Issue #5090](https://github.com/kdlbs/kandev/issues/5090) exposes a gap in the
completed runtime-binding work: ACP admission ignores successful project-file
delivery. The [repair package](../pi-builtin-mcp-delivery/plan.md) owns its
implementation and regression evidence and is now implemented. Earlier
completion results remain historical evidence. They do not certify Pi package
startup; the repair's [work order](../pi-builtin-mcp-delivery/task-01-recognize-project-delivery.md#results)
records the current Pi coverage.

## New-task MCP placement correction (2026-11-04)

PR #4762 merged at `a1013e89fc460b39b17003e5022d756bf480e3d2` before this
request. This correction uses `fix/task-create-mcp-package-placement-6d1b` from
verified current main `5d6adcbca18231a5aa3726ca9ee642bfa255cd4a`, which contains
that merge. The merged delivery branch is unchanged. The initial work is backed
up as a patch and retained stash before applying it to this new branch.

The user reported a separate package disclosure above Advanced settings and a
repeated package title. Work order 03 was reopened for this existing UI-02 management
scope. The correction is complete and locally verified. Put task-creation packages inside Advanced settings > MCP servers, using
the shipped desktop popover and phone drawer. Render one package-list heading.
Keep the task draft in the form owner so closing either disclosure does not
discard consent. Reset it on a new task or workspace change. No package default,
permission, persisted format, process-start timing, or existing-task flow changes.

After testing the isolated preview, the user authorized its shutdown and merge
of this follow-up once conflicts, bot findings, and failing or pending CI are
clear. This supersedes the historical no-merge checkpoints below. The preview
is stopped. Work order 03 also covers the valid review finding that slow
additional-server reads must not block the built-in picker. Exact-head remote
verification remains a delivery gate recorded in the saved task plan.

Desktop: New Task > Advanced settings > MCP servers > package list/detail and
additional servers in one popover. Phone: the same entry opens the shipped MCP
drawer, with one scroll body and its existing Close control. Package Back stays
inside the same surface and does not submit. The existing form footer creates
the task after the picker closes. Work order 03 names focused regressions.

## Follow-up hosted CI repair (2026-11-04)

The first follow-up fix head passed local checks and bot disposition, but hosted
Shard 26 failed in mobile navigation hydration and workflow-model test
sequencing. The scoped test-only repair waits for the actual restored commits
response and finishes the initial workflow launch before changing its model.
Both phone journeys pass repeated checks, and all five desktop navigation
cases pass. The package implementation and UI remain unchanged. See
[implementation results](implementation-results.md#hosted-ci-sequencing-repair-2026-11-04).
Exact-head CI and the final merge gate remain pending publication of this repair.

## Historical pre-merge CI and main integration (2026-11-04)

The user's current CI/conflict requests and later "continue" authorize repair,
rebasing, publication to the existing PR, and remote verification. No merge is
authorized. These requests supersede the older no-CI delivery checkpoints below.
The approved requirements, package boundaries, and implementation scope remain.

Before PR #4762 merged, its integration base was main
`e180953eae3e433ed91fd15844ad3fe4571b7b76`.
All 16 owned patches were preserved by the rebase, including the recovery-ledger
and navigation-readiness repairs. Main's clarification attachment schemas,
caller checks, and prevent-auto-start behavior are preserved. Earlier baseline
and publication sections are historical receipts. The placement correction above
records the active branch and its verified base.

Work orders 01, 02 and 03 remain implemented. Current-source compatibility
checks and delivery evidence are recorded in
[implementation results](implementation-results.md). Exact published-head CI and
mergeability remain separate delivery gates, tracked in the saved Kandev plan.
The existing user preview and main runtime are outside this repair.

## Active UI-02 save-feedback remediation (2026-11-02)

Work order 03 was reopened for the user's PR findings: Configuration saves but
has no visible next-start addition in Connections, and task controls omit the
settings change-aware treatment. The existing approved requirements and design
own this repair. No package authority or process-start behavior changes. See
[work order 03](task-03-management-experience.md#user-reported-save-feedback-correction-2026-11-02)
for the diagnosed cause, regression checks, and desktop/phone composition.
The repair is complete and locally verified. Earlier checkpoints remain historical
evidence; exact-head remote CI is pending publication of this checkpoint.

## User-requested adversarial review (2026-11-03)

A secondary read-only session reviewed the package boundaries and integration.
Three findings are repaired under work orders 02/03: legacy Walkthrough transport
aliases, future-start versus live execution intent, and executor-discovery E2E
verification. The original package authority and apply point stay unchanged.
Focused post-rebase race and browser checks pass. See
[implementation-results.md](implementation-results.md#secondary-adversarial-review-2026-11-03)
for review coverage limits, regressions and final publication evidence.

## Historical implementation checkpoint and verified baseline

Design approved by the user; implementation explicitly authorized with "go for it"
on 2026-10-24 and "continue impl" on 2026-10-25. Work order 01 is complete;
02 and 03 are complete. The latest request authorizes the remaining
approved implementation, beyond the historical partial publication checkpoint.
Implementation uses sequential TDD. The publication checkpoint below supersedes
the earlier instruction to retain this package only as uncommitted files.

Workspace: `/home/zeval/.kandev/tasks/unify-built-in-mcp-m_mk09846g/kdlbs-kandev`.
Original feature branch: `feature/builtin-mcp-packages-6d1b` (merged in PR #4762).
Verified base: `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`, actual squash merge of
[#3293](https://github.com/kdlbs/kandev/pull/3293) and fetched main on 2026-10-24.
The parent finished at `e053e8caa565e93564bd1a3e2c04b4f61f1eb362` before merge.

The 2026-10-22 draft started at authorized parent head
`193de56ba2597b335a97b1e5ab01d09340f22647`. At revision time the workspace branch
was `feature/unify-built-in-mcp-m-3po` at that original base, with no task-owned
commits and seven untracked design files. Creating a new owned branch at the actual
merge was the safe reconciliation: all prior branches and uncommitted documents
were preserved. No parent commits were replayed, and no shared main/parent branch
was modified. Isolated Git metadata avoided an unrelated broken shared ref.
Future PR target is **main**, not the now-merged parent delivery branch.

The saved Kandev plan preserves the old handoff as history and records this
revision. No reviewed implementation package has been found. Autopilot remains
off. No workers, additional tasks/sessions, commits, pushes, or merge are
part of this design turn. Recheck current base/contracts before implementation.

## Current user-review UI repair (2026-11-03)

Work order 03 completed the user's package-detail, missing saved selection
and empty exclusion-help feedback. Follow its current repair section and the
existing UI-02/AC-003.3/AC-004 contract. No runtime injection, permission or
exclusion semantics change. The existing implementation authorization applies.

## Registration cleanup authorized on 2026-11-02

The user requested completion of the registration cleanup after the latest main
rebase. Work order 01 is complete again with package-owned assembly and shared
Core tool builders. The configuration-chat assembly path and temporary server
used by ordinary tasks to copy Configuration tools are removed. Existing schemas,
caller capabilities, resource permissions, and non-native catalogs are preserved.
Work orders 02 and 03 remain complete. Publish the checked result to the existing
open PR #4762; no merge or CI/review waiting is authorized.

## Historical partial publication checkpoint (2026-10-24)

The coordinator requested publication of all task-owned work as a draft on
2026-10-24, followed by a stop without review handling, CI waiting, or merge.
This checkpoint contains completed work order 01 and partial work order 02.
Work order 03 has not started. It does not deliver the complete feature.

The owned branch was reconciled onto remotely verified main
`cc687bdce96bc22fb8a9ca87abe571b8309d3125`, which contains parent squash
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. No parent commits were replayed.
A file archive, binary patch, and named Git stash preserve the pre-rebase work.
The stash applied without conflicts; the separate package transport changes
retain main's concurrent server changes. Other workspaces were not modified.

Normal package selection and human management writers remain unwired. Initial
launch prompt/runtime preparation is connected behind the unwired policy reader;
automatic prompts, retained-context guidance, accepted session evidence, and
queued Walkthrough intent remain incomplete. The automatic prompt acceptance
test is preserved as unfinished TDD work. Desktop/phone UI, localization, and
public documentation are pending. Do not mark this draft ready or merge it.
Publication validation and known failures are recorded in
[publication-checkpoint.md](publication-checkpoint.md).

## Scope and decisions proposed for review

Own point 5: native package boundaries, names/defaults, Configuration on regular
tasks, safe pruning, built-in management, truthful evidence, and phone parity.
Use [requirements](../../specs/agents/requirements/builtin-mcp-management.md),
[system design](../../specs/agents/system-design/builtin-mcp-management.md), and
[proposed ADR](../../decisions/2026-10-22-builtin-mcp-tool-policy.md).

The proposed set is Core (`kandev`), Configuration (`kandev-configuration`), Rich
output (`kandev-rich-output`), Walkthroughs (`kandev-walkthroughs`), and Diagnostics
(`kandev-diagnostics`). They are distinct logical MCP servers in one agentctl
process. Configuration chats receive Core+Configuration. Regular tasks receive
Core and the three existing optional capabilities by default; Configuration
requires a human task opt-in. Workspace policy can block or set optional defaults.
Profiles retain permission/provider control; sessions have no separate override.
Application remains the next normal agent process start.

The user approved the five-package taxonomy, default-on optional task packages,
task-level choice, profile deferral, and process-start boundary on 2026-10-24.
Implementation follows that approved package.

Sibling saved plans were reread on 2026-10-24 and still contain scope, not shipped
interfaces. Exclusions belong to `a3588dfb-7c25-4f91-9a3d-665ce89c6605`;
assignment to `58d6bce2-eab4-4ac6-9ac4-50a31011e1e1`; management/evidence clarity to
`2adba1ca-619e-4bfc-b705-3bbc628df4dd`. Built-in package IDs never enter their
catalog selection/exclusion lists. This package adds its own section to the
existing Customize surface and does not implement those sibling behaviors.
Keep their workspaces untouched. The parent's
[testing guide](../workspace-mcp-configuration/testing-guide.md) remains baseline
context; `merge-readiness.md` is historical evidence, not extra product scope.

## Technical approach and sequence

1. Add the package catalog, disjoint tool ownership, validated manifest carrier,
   and isolated server views. Keep the existing default launch path functional
   while these internals are introduced; no package policy writer yet.
2. Deliver the complete backend path: human policy/preferences, execution binding,
   multi-server injection, package dispatch grants, permission provenance, prompts,
   recovery, and evidence. Activate package-mode starts only with negotiated helper
   support and complete enforcement. Never expose a save that runtime ignores.
3. Deliver workspace/task UI, configuration-chat behavior evidence, session detail,
   dependent actions, localization, focused E2E, and public documentation.

Task 01 is a compatible expansion with a real catalog/transport verification
boundary. Task 02 integrates all enforcement before choices become externally
writable. Task 03 exposes the supported contract in existing user journeys.
No generic QA or infrastructure-only task is needed. The design's compatibility
matrix specifies provider/transport limitations; shared mocks do not prove every
installed provider. Implementation uses `/tdd`; frontend work also uses `/e2e`.

## UI preview contract

Use the labelled structural views below and the native Kandev rich-output preview
in the task conversation. The task's explicit native-output instruction supersedes
the skill's ASCII default. English copy is illustrative; implementation uses
localized text and shipped controls. Control order, scope labels, evidence
separation, and phone navigation are requirements.

### UI-01: Workspace Built-in packages

Entry: Settings > Workspaces > selected workspace > MCP servers > Configured.
Before: workspace definitions and Add MCP server. Proposed: a Built-in section
with five full-row package entries before the unchanged definition list. Zero
user definitions still shows Built-in and accurate empty-definition help.

Desktop selects a package into the existing inline settings detail. Order:
Back to MCP servers; package name / Built-in; purpose and runtime ownership;
tool membership and eligible contexts; policy for regular tasks; fixed required
contexts; next-agent-start notice. Core has a Required reason, not a switch.
Configuration offers Blocked or Allow explicit task selection, and explains
that configuration chats include it automatically. Other packages offer Blocked,
Available for task selection, or On by default. Shared Save changes / Discard
owns persistence. Reset restores the draft defaults.

Phone replaces the list with one full-height detail. Fixed navigation and save
controls surround one scrolling body. Read-only shows values/reasons. Conflict
keeps draft with Reload saved values; denial remains unsaved. Required reasons,
shared-resource warnings, and final controls remain reachable without hover.

### UI-02: Task choice and session evidence

Entry: Composer MCP plug > Customize > Built-in packages. Desktop keeps the
existing dialog; phone uses the existing full-height drawer. Core reads Required.
The three default task packages read Inherit (On); Configuration reads Inherit
(Off). Optional choices are Inherit/On/Off, with effective value and workspace
limit. Choosing Configuration On reveals its settings/resource scope inline
before the existing Save action. Blocked packages show the workspace reason and
cannot be enabled. There is no independent session override. Copy states that
task choices affect future starts of this task's sessions. Provide the same
selection in authorized task creation before its first start.

Connections shows separate actual server entries per package. Package detail:
Built-in identity; desired selection and origin; current agent's bound selection;
next-start notice if different; current-session connection observation/time;
expected tools and observed tools, clearly labelled. An authorized Manage action
opens the correct workspace/task choice. An unselected package can be inspected
from Customize without pretending it has a connection. Legacy monolithic agents
show one legacy entry with Unknown package history where needed.

Phone uses list > package detail > tool detail within the same drawer, with Back
and Close, fixed header/actions, and one scroller. Desktop retains master/detail.
A viewport change preserves the draft. Use at least 44px touch targets, nominal
48px header close, and 28px ordinary desktop controls. Safe areas, keyboard focus,
long translated labels, loading, pending, unavailable, conflict, and denial states
apply to both views. Map UI-01/UI-02 to AC-001.1/.2/.3/.4 and AC-004.1 through .5.
All abbreviated acceptance IDs here use `AC-AGENTS-BUILTIN-MCP-`.

## Tests and acceptance traceability

These are proposed test names/paths unless marked existing. Backend paths are
relative to `apps/backend`, frontend paths to `apps/web`.

| Acceptance | Test file and named evidence |
| --- | --- |
| 001.2, 002.1/.2/.5 | `internal/mcp/server/builtin_packages_test.go`: `TestBuiltinPackageCatalog`, `TestBuiltinDefaultUnionParity`, `TestBuiltinConfigurationMembership`; existing full-surface catalog tests |
| 002.3/.4, 003.2 | `internal/mcp/profile/builtin_packages_test.go`: `TestBuiltinManifestValidation`, `TestBuiltinManifestRoundTrip`; server `TestBuiltinPackageSelectionMatrix`, `TestBuiltinPackageRebuild` |
| 001.3/.4, 002.3/.5 | `internal/agent/mcpconfig/builtin_policy_test.go`: `TestBuiltinPolicyAuthority`, `TestBuiltinSelectionPrecedence`; `internal/task/service/builtin_mcp_test.go`: `TestBuiltinTaskPreferenceAuthority`, `TestBuiltinGrantNotInherited` |
| 001.2/.3, 002.4/.5 | `internal/mcp/handlers/builtin_package_authorization_test.go`: `TestBuiltinRawActionGuard`, `TestBuiltinConfigurationCannotGrantPackages`, `TestBuiltinConfigurationDoesNotGrantTransfer`; scope principal tests |
| 002.3, 003.1/.3/.4 | settings/task store `builtin_mcp_policy_test.go` / `builtin_mcp_preferences_test.go`: `TestBuiltinPolicyPersistence`, `TestBuiltinPreferenceCAS`; HTTP `TestBuiltinPolicyHTTP`, `TestBuiltinTaskPreferencesHTTP` |
| 003.1 through .5 | `internal/orchestrator/executor/executor_builtin_mcp_test.go`: `TestBuiltinLaunchBinding`; lifecycle `builtin_mcp_packages_test.go`: `TestBuiltinRecoveryAndAcceptance`; carrier and old-helper tests |
| 001.2, 002.4, 003.5 | agentctl API `builtin_mcp_routes_test.go`: `TestBuiltinPackageRouteIsolation`; config `TestBuiltinPackageInjection`; process `TestBuiltinPackagePermissionProvenance`; mcpconfig `TestBuiltinReservedNameCollision` |
| 002.4/.5, 003.2 | server `builtin_prompt_packages_test.go`: `TestBuiltinPromptCatalogParity`; orchestrator `builtin_mcp_guidance_test.go`: `TestBuiltinQueuedIntentGuard` |
| 004.1/.2, 003.3 | `components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts`: `keeps desired, bound and observed packages separate`; stream/DTO ordering tests |
| 001.1/.3/.4, 004.3/.4/.5 | workspace `builtin-mcp-detail.test.tsx`; task `builtin-package-selector.test.tsx`; domain hook tests; Walkthrough availability tests |

Test all 16 optional-package combinations for eligible Kanban tasks, both Office
Rich output states, fixed configuration defaults, and all existing restricted
surfaces. Include workspace blocked/default/available precedence, task reset,
configuration-grant non-inheritance, schema equality for shared tools, duplicate
membership, generic-write self-grants, and direct calls on every disabled route.
No token savings estimate is inferred from the issue's historical tool count.

## E2E tests

`e2e/tests/settings/builtin-mcp-management.spec.ts` (chromium) covers UI-01/UI-02:
empty definitions; defaults/save/discard/reset; stale or denied writes; workspace
block suppressing task On; unblock preview; other-workspace isolation; pruning
Rich output; last bound execution versus next start; failed launch and missing
evidence; Walkthrough guard and retained content; reserved-name collision. Maps
to 001.1/.2/.3, 002.1/.3/.4, 003.1 through .5, and 004.1 through .3.

`e2e/tests/settings/builtin-mcp-configuration.spec.ts` (chromium) covers automatic
Core+Configuration in a configuration chat; regular task initially without it;
human task On before first start and on an existing task; real `tools/list` and
one harmless authorized configuration read through the package route; task
surface/title/question semantics retained; foreign resource denial; attempted
package self-grant; no Configuration inherited by an agent-created child. Maps
to 001.3/.4, 002.1/.2/.3/.5, 003.1/.2, and 004.2/.3.

`e2e/tests/settings/mobile-builtin-mcp-management.spec.ts` (mobile-chrome) covers
the same primary opt-in/pruning journey, blocked and denied choices, Back/Close,
focus, draft preservation across viewport changes, final row reachability, one
scroller, safe areas, and long translations. Use Pixel 5 plus 320/767px widths;
desktop tests cover narrow fine-pointer, 768px boundary, and 900px touch tablet.
Measure actual hitboxes/overflow. Maps to 004.3 through .5 and UI-01/UI-02.

Use isolated mock fixtures plus real built-in MCP calls for catalog/dispatch
proof. Mock healthy rows prove only presentation. Run suites sequentially, managed
builds, one worker per shard, retries zero. Do not run overlapping full suites or
use main runtime 9998 or old previews 48593/48595.

## Work orders

- [x] [Task 01: Package catalog and isolated registration](task-01-tool-group-contract.md)
- [x] [Task 02: Policy, task grants, and runtime delivery](task-02-policy-runtime-binding.md)
- [x] [Task 03: Package selection and responsive management](task-03-management-experience.md)

Execute 01 > 02 > 03 in the primary session after explicit authorization.
The historical filenames remain stable for links; their scopes are revised here.

## Verification results

2026-10-22 results apply only to the superseded three-group draft. They are
preserved in the saved task plan and must not be treated as evidence for this
revision. Revised checks on 2026-10-24 passed:

- `python3 scripts/list-docs.py validate`: 404 decisions and 1730 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual documentation-only
  paths are exempt; a prospective implementation diff is covered by all three
  work orders with zero reference errors. The source path was validator input
  only; no implementation file changed.
- Local-link and whitespace check across all seven untracked documents passed.
  `git diff --check` and work-order status inspection passed.

These are document checks, not evidence that the proposed feature works.

No product unit, browser, runtime, or provider tests run during this design-only
revision. All seven files remain unstaged/uncommitted; no production or permanent
test changes. Each work order supplies its later exact verification commands.

## Risks and review choices

Multiple logical servers increase connection and adapter work. Explicit package
capability negotiation prevents a false compatible fallback. Configuration has
real mutation authority, including shared resources; task selection needs a
human writer and backend dispatch proof. Tool ownership must be disjoint, and
client-qualified name changes require new discovery guidance. Legacy agents must
remain stable. Newly reserved names can collide with old user definitions; keep
those definitions intact and make remediation explicit.

Review the five package boundaries, default-on optional task packages, explicit
Configuration task opt-in, deferred profile/session editors, and next-start
application. These are review choices, not factual blockers or approval prompts.

## Current implementation verification (2026-10-25)

Runtime and human management wiring, atomic initial task consent, accepted-session
evidence, typed Walkthrough admission/dispatch, workspace and task package controls,
seven-language copy, and public documentation are implemented. Desktop and phone
browser verification passed. See [implementation-results.md](implementation-results.md)
for current evidence and limits; earlier checkpoint statements describe historical
partial work, not the current feature state.
