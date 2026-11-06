---
created: 2026-10-16
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
legacy_specs: []
---

# Implementation Plan: GitHub manual merge permissions

## Overview

Show a manual merge action only when the effective mutation actor has write
access to the target repository. Complete one vertical work order covering
the existing repository reader, an access endpoint, shared UI, and regression tests.

The integration system owns this change because it owns provider identity,
repository access, and merge operations. UI status remains a projection of that contract.

## Confirmed diagnosis

The supplied screenshot matches
[svkozak/pi-acp#163](https://github.com/svkozak/pi-acp/pull/163).
Read-only GitHub queries on 2026-10-16 returned:

| Observation | Value |
| --- | --- |
| Authenticated CLI viewer | `zeval` |
| Target repository | `svkozak/pi-acp` |
| Target `viewerPermission` | `READ` |
| Target REST `permissions.push` | `false` |
| Source fork | `zeval/pi-acp`, viewer permission `ADMIN` |
| PR state and mergeability | `OPEN`, `MERGEABLE`, `CLEAN` |

Before this fix, `PRMergeButton` rendered when `canAttemptPRMerge(taskPR)` passed. That predicate
checks PR readiness but receives no actor or repository permission data.
`getGitHubMutationActor` supplied a label only. Missing merge-method settings
also left the button enabled. These paths explain the screenshot.

GitHub also returned `viewerCanMergeAsAdmin: true` for this read-only viewer.
Do not use that field as the repository write-access predicate.

The CLI observation confirms the reported account's repository role. It does
not inspect the other task's private connection configuration. No merge request,
repository change, or account switch was performed.

## Scope

### In scope

- Manual action availability for the resolved mutation actor and target repository.
- Separate denied, loading, and unknown states with translated explanations.
- Context invalidation, late-response rejection, and permission refresh without a merge.
- Desktop popover/details and phone status drawer/Review coverage.
- Existing successful direct and queued merge behavior, method fallback, and public usage guidance.

### Out of scope

- Automatic merge configuration, identity, retry, and queue recovery.
- GitLab, account management, permission grants, or branch-rule bypass.
- Persisted permission snapshots, migrations, and sidebar permission polling.
- Executing a merge against the reported PR.
- Unrelated visual polish or layout redesign.

## Technical approach

Reuse `repositoryDetailsReader.GetRepository` and `GitHubRepository` from
`apps/backend/internal/github`. Add explicit presence information for the
existing push permission. Keep contribution and fork behavior unchanged.

Add `GetRepoMergeAccessForWorkspace` in a focused service file and a GET route
at `/api/v1/github/repos/:owner/:repo/merge-access`. Resolve the same
personal-write client as `MergePRForWorkspace`. Return the existing safe
principal shape with an `allowed`, `denied`, or `unknown` result. Never retain
the result in a global repository cache or persisted PR record.

| Provider path | Identity and evidence | Intended behavior and verification |
| --- | --- | --- |
| Named CLI or PAT | Resolved human, target REST push permission | Explicit true allows. False denies. Client and service tests cover both. |
| App user OAuth | Resolved human, target REST push permission | Personal actor wins even when the workspace App can write. Test conflicting actors. |
| App installation | Resolved App, scoped target visibility and `git_write` plus `pull_request_write` capabilities | Support App-only manual actions. Test missing capabilities without requiring human permission fields. |
| Legacy CLI path | Existing resolver, human permission from `GHClient.GetRepository` | Keep legacy routing. Do not inspect ambient auth outside that resolver. |
| Unsupported or incomplete result | No reader, nil result, missing/null push field, or read failure | No actionable merge. Test unknown state and explicit recovery. |

Add `getRepoMergeAccess` in `github-pr-api.ts` and the shared domain hook
`use-repo-merge-access.ts`. Use `auth.user`, GitHub status connection generations,
and `subscribeIntegrationAvailability` for context invalidation. Do not fetch
inside a component. Clear previous results before accepting a new context.

Update `PRMergeButton` as the common permission gate. Keep `canAttemptPRMerge`
and status colors independent of actor access. Use the response actor for the
action attribution. Retain merge-method fallback only after access is allowed.
Refresh permission after a mutation error before another manual attempt.

The provider remains the authority for token restrictions and branch rules.
Repository access is necessary, but it does not guarantee that GitHub will
accept a merge. Preserve server-side scope checks and the existing write route.

## ASCII UI preview

UI-01: Merge-action region in an otherwise ready PR. The supplied screenshot
and source establish the current state. The preview changes only that region.

```text
CURRENT: desktop popover, read-only actor
[ Merge PR                                  ]
as zeval

PROPOSED: same region, denied
zeval cannot merge into svkozak/pi-acp.

PROPOSED: same region, permission loading
Checking merge permissions.

PROPOSED: same region, lookup unavailable
Merge permissions are unavailable.
[ Check again ]

PROPOSED: same region, allowed and PR ready
[ Squash and merge                          ]
as zeval
```

Desktop details use the same states in their existing action group. Phone
status keeps its inset drawer. Phone Review keeps its existing full-height
surface. Both show the same inline text and refresh control without hover.
The surface remains the sole scroll owner. No nested overlay is introduced.

Required structure: no primary or alternative merge control in denied or
unknown states, wrapped explanation, and a separate read-only refresh action.
Copy and spacing are illustrative. Use existing tokens and localized messages.
Refresh controls use 28px desktop height and at least 44px touch targets.
The nearest shipped examples are `PRStatusChipDrawer`, its
`ChangeRequestStatusDrawerContent`, and the mobile Review panel.

This preview covers AC-001.16, AC-001.17, and AC-001.19 under
`REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001`.

## Tests

The first RED test extends `pr-merge-button.test.tsx` with
`does not expose a merge before access is confirmed`. Keep the existing
null-status/null-method fixtures and assert no actionable merge control.
The current implementation renders a button, so the failure proves behavior
without relying on a missing endpoint, module, or selector.

| Acceptance criteria | Focused evidence |
| --- | --- |
| 001.15, 001.20 | `service_merge_access_test.go`: target READ plus fork ADMIN, target write, personal/App disagreement, App capabilities, workspace scope, no fallback after denial. |
| 001.15, 001.17 | `repository_merge_access_test.go` and controller/service tests: both GH/PAT providers, explicit true/false versus omitted/null permission, nil response, provider errors. |
| 001.16, 001.17, 001.20 | `pr-merge-button.test.tsx`: compact and standard views, all methods, loading/denied/error, method failure with confirmed access, no mutation from refresh. |
| 001.18 | `use-repo-merge-access.test.ts`: workspace/user/actor/repository/generation changes, response ordering, focus refresh, failure, recovery, and unmount. |
| 001.13, 001.19 | Existing outcome tests plus desktop/mobile E2E: rejected merge triggers permission refresh, permitted direct/queued results remain correct. |

## E2E tests

Add `tests/pr/pr-merge-permissions.spec.ts` for desktop status and details.
Add `tests/pr/mobile-pr-merge-permissions.spec.ts` for the phone status drawer
and Review. Use the real backend path with an explicit mock repository-access
fixture. Do not intercept the production access response as the sole E2E proof.

Seed the reported shape: ready PR, read-only target, writable source fork.
Assert the explanation and the absence of every merge method. Observe merge
requests before opening the surface and prove none occur during the interaction.
Test permission-read failure, read-only refresh, and recovery to permitted access.
Retain successful direct and queued submissions in isolated mock scenarios.
Phone tests assert wrapped text, drawer containment, zero document overflow,
and a refresh hit target of at least 44px.

Update explicit permission fixtures in the existing merge-queue E2E tests and
in `pr-detail-layout.spec.ts` and `mobile-pr-rerequest-review.spec.ts` where
they expect an enabled merge. Do not make every mock repository writable.

## Related delivery records

The original [merge queue plan](../github-pr-merge-queue/plan.md) and the
[queue status plan](../github-pr-merge-queue-status/plan.md) own existing
manual-action and display evidence. Their affected suites run with the new
permission fixtures. Their historical completion results are not evidence for
this new behavior and are not rewritten.

The [automatic merge reliability plan](../github-auto-merge-reliability/plan.md)
and [sidebar indicators plan](../github-sidebar-automation-indicators/plan.md)
remain outside this repair. Their automation ownership and outcome contracts
do not change.

## Work orders

- [x] [Task 01: Gate manual merge by target access](task-01-gate-manual-merge.md)

The user's later `continue` request authorized implementation. The primary
session owns this work order; no delegation was used.

## Verification results

Design validation completed on 2026-10-16:

- `python3 scripts/list-docs.py validate`: passed, 384 decisions and 1616 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed with this package and
  the proposed merge-button path supplied as a triggering change. All work-order
  requirement, acceptance, plan, and design references were covered.
- `git diff --check`: passed. `git status --short` confirmed both new plan files
  remain untracked and the two specification edits remain unstaged.

Implementation and verification are complete. The initial component regression
failed because an enabled merge button was present before access was confirmed.
The controller regression failed with the expected missing-route response.
After implementation, the backend GitHub package passed normally and with race
detection. All 669 GitHub frontend tests and 13 desktop/phone browser tests
passed. TypeScript, lint, translations, and documentation checks passed.
The denied and unavailable states were inspected in four screenshots.
See Task 01 for commands, local environment setup, logs, and capture locations.

CI found a footer test that assumed the popover had only one status region.
Its accessible-role query now targets the footer, so the permission notice can
coexist with the refresh announcement. The test failed before this correction
and passed afterward, along with the broader GitHub component suite.

The permission notice now capitalizes its displayed first letter and uses the
same horizontal inset as Review follow-up in the desktop popover and phone drawer.
The focused 16 component tests and three permission browser tests passed after
this correction. The phone capture waits for the drawer and scrolls the notice
into view. Account identifiers and permission behavior are unchanged.

## Risks

- GitHub can revoke access after a read. The mutation must retain provider enforcement.
- A green PR or writable fork must never substitute for target access.
- Adding a presence marker must not alter existing fork permission behavior.
- Shared E2E fixtures currently seed repository search data without write permissions.
- Unknown human token restrictions can still cause GitHub to reject an otherwise permitted action.

## Documentation decision

Update the existing requirement and design. No new ADR is needed because
ADR 0047 already owns actor resolution and isolation. The design preserves
the local access-check rationale and provider limitations.

The GitHub review guidance in `docs/public/integrations.md` now explains denied
and unavailable merge permissions and the read-only retry. Its primary content
type is a how-to guide. The root README and screenshot index do not require changes.
