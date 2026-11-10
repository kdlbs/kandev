---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-TASK-AGENT-TAB-RECONCILIATION-002
system_design:
  - ../../specs/ui/system-design/task-agent-tab-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Unicode Session Rename Parity

## Overview and evidence

Preserve legal server names while editing task and preview Agent tabs. The
server limits names to 120 runes; the frontend slices UTF-16 at 120 and uses
DOM maxLength=120. The parent diagnostic reproduced two behavioral failures:
120 compass emoji become 60, and 119 ASCII plus an emoji sends a lone high
surrogate. Two ASCII controls pass. No runtime incident is claimed.

Confirmed scope: code-point parity, existing trimming/reset/focus/failure
semantics, no server or delivery-order changes. No material unresolved choice.
One sequential work order, executed in this session under explicit authorization
to skip the separate design approval checkpoint. No delegation.

## Scope and technical approach

The existing UI task-agent-tab requirement/design pair owns the edit contract
(AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.5 and .6). Backend RenameSession retains
canonical authority. Add a session-owned code-point limiter in
`apps/web/components/task/use-session-rename.ts`; use it for commit and both
session input consumers. Add only an optional draft limiter to the shared
`tab-rename-input.tsx`, retaining terminal maxLength behavior. No public API,
copy, schema, lifecycle, permission, task-title, or global input-policy changes.

## ASCII UI previews

UI-01: Desktop Agent tab context menu > Rename, current name selected.

```text
[ Agent tab: <complete current name selected> ]
           Enter / click outside: commit
           Escape: cancel
```

The same inline composition remains. Before: a legal Unicode name can be
shortened on unchanged Enter. After: it is preserved and edits accept 120 code
points. Spacing is illustrative; no geometry or focus behavior changes.

UI-02: Phone/tablet existing Sessions pill > native session picker.

```text
[ Current session v ]
  Sessions
  [ * <canonical session name> ]
  [   <other session name> ... ]
```

Reuse MobileSessionsPicker and its current sheet/selection/scroll ownership.
No session rename action exists here. Verify Unicode labels and touch selection;
do not invent a rename entry point. No new touch or layout rules are needed.

## Work orders

- [x] [Task 01: Preserve Unicode names](task-01-preserve-unicode-names.md)

## Verification and risks

Run the exact commands in Task 01. RED must fail on behavior before production
changes. Coverage belongs at actual input/hook/API/store boundaries plus shipped
desktop browser controls. Native phone checks cover display/navigation only.
Public docs impact review: no commands, copy, navigation or API contract changes;
internal limit parity is clarified in the owning design. Refresh main and open
PR inventory/actual overlapping patches before edits and delivery; preserve
PR1512/4013 unrelated hunks. Stop after normal commit with both hooks active,
clean worktree, ready_for_parent_validation. No push/PR without later authority.

## Results

Done on 2026-10-22. See the [work-order results](task-01-preserve-unicode-names.md#results)
for exact commands, failures, coverage boundaries and receipt paths.

- Unit RED: 9 behavioral failures / 9 controls passed. Native desktop RED
  observed a 60-emoji write when submitting an unchanged legal 120-emoji name.
- GREEN: 57 focused tests; 2 desktop and 1 scoped native phone browser checks, no
  retries; scoped ESLint zero warnings; typecheck passed with CI's documented
  8 GiB heap; doc catalog, full spec lint, PR-doc coverage and diff checks passed.
- One session-only limiter serves both input consumers and the committer.
  Terminal defaults and server authority remain intact. Public reference copy
  clarifies desktop rename and the existing Unicode code-point limit.
- Final inventory: 69 open PRs, no competing fix. Main advanced one disjoint
  chat-mentions commit to c971923bb9505a0030de2539884ebf35457729ab; original
  base is retained for parent validation. Later delivery requires combined
  current-main validation, including dependency refresh.
- Stop after the normal hooked commit: ready_for_parent_validation. No push/PR.

## Parent evidence correction and known limitation

The original de5ba1ff phone screenshot showed a restart error despite the test
passing. Parent review exposed a separate native Session-actions trigger defect:
a tap retargets a click onto Restart session, sending stop and launch before
Escape. It is preserved as a known unfixed defect, with exact diagnostic logs
and source trace in the [work-order correction](task-01-preserve-unicode-names.md#parent-checkpoint-phone-evidence-correction).
It has no Unicode source connection. No clean mobile menu claim remains.

Per the parent's explicit scope decision, this package verifies only canonical
Unicode labels and real touch selection on phones. The strengthened scenario
requires a real selected-session subscription, zero lifecycle mutation requests,
zero toasts and no horizontal overflow; it omits the unrelated lifecycle menu.
Rename absence is source-traced only. Fresh phone evidence and captures replace
the invalid menu-flow evidence; no mobile menu/Radix production change is made.

## Delivery remediation: focus protocol and insertion preservation

Delivery was separately authorized after the parent checkpoint. CI at 8fa4fd22
failed the phone test's overly broad launch assertion: the existing resumption
hook sent a promptless `session_focus` request for the known live primary session.
A normal informational Kandev update notice was also visible. Earlier zero-launch
and zero-toast local results remain historical observations, not universal product
invariants. The corrected test retains complete outgoing lifecycle payloads and
permits only that exact focus shape and fixture identity. Explicit/unclassified
launches, stops, restarts, deletes and primary changes remain failures. Only the
source-supported default informational update notice is allowed; lifecycle errors
and other feedback remain failures. The separate native menu defect remains known
and outside this package.

Exact-head review also identified that inserting into a full name clipped its
existing ending. The limiter now uses the browser's insertion-end position to
constrain inserted text and preserve the unselected suffix. The shared terminal
input keeps native maxlength behavior. Focused actual input/API/store regressions
and shipped desktop typing checks cover beginning and middle insertion.
