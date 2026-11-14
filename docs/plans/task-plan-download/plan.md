---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-TASK-PLAN-DOWNLOAD-001
system_design:
  - ../../specs/ui/system-design/task-plan-download.md
legacy_specs: []
---

# Implementation Plan: Download Task Plan Draft

## Overview

One sequential vertical work order adds a local Download action to the main
Plan header, proves its draft wiring and actual downloads on desktop and phone,
and documents its use. UI owns this interaction; Tasks retains persistence.
The qualified source evidence is
`/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/discovery042/qualification.json`.
No material product question remains: the request fixes the content, filename,
disabled state, boundaries, failure feedback, and surface.

## Scope

### In scope

- A localized accessible Download control for the current main Plan draft,
  sharing the existing text download helper and error feedback pattern.
- Mounted consumer regression, desktop and phone browser evidence, responsive
  geometry, and a concise public how-to paragraph near Plan panel usage in
  `docs/public/sessions-and-review.md`.

### Out of scope

- Revision/preview downloads, clipboard, other formats, added frontmatter,
  title prefixes, new overlays or download protocols, backend or workspace
  writes, and save/lifecycle/editor refactors.
- Redesigning Implement, Revisions, recorder or other existing controls.
- Production changes, permanent tests, installs, checks, builds, or hooks in
  this design turn. No delegates or extra sessions.

## Technical approach

`TaskPlanPanel` passes `state.draftContent` into `PlanPanelHeader`; use that
prop, not saved `plan.content`. Add a small `PlanDownloadButton` if needed for
header size. Disable on `draftContent.trim().length === 0`, but pass the
original string and fixed `plan.md` with `isBinary: false` to
`triggerFileDownload`. Catch synchronous errors using the established localized
toast. Reuse `task:download` and `task:failedToDownloadFile`. The fixed filename
is program data; add an explained i18n exemption only if the detector needs it.
No helper, Blob lifetime, or native protocol change is needed.

## ASCII UI preview

**UI-01: Main Plan header**, current draft is nonempty. The header remains
fixed above the existing independently scrolling editor. The explicit action
order is structural; exact spacing and icon treatment are illustrative.

```text
Desktop fine pointer:
+-----------------------------------------------------------+
|                         [Implement v] [Download] [History] |
+-----------------------------------------------------------+
| Current Plan draft ...                                    |

Phone, Plan destination:
+---------------------------------+
| [Implement v] [Download] [Hist] |
+---------------------------------+
| Current Plan draft ...          |
|                                 |
|       existing navigation       |
+---------------------------------+
```

On phone and coarse pointers the new control's actual hit area is at least
44px; desktop fine pointer is 28px high. A narrow fine-pointer phone also gets
the phone target. Keep all actions reachable and contained, allowing local
header composition to fit without page-level horizontal scrolling; no new
overlay. At narrow desktop panel widths, the header's action row scrolls
locally so each action can be reached. Empty or whitespace draft leaves Download visibly disabled. A
synchronous initiation failure uses existing localized error feedback without
changing content. These views map to criteria `.1` through `.5`; actual desktop
and phone capture and geometry checks are required, not inferred from sketches.

## Tests

Extend `apps/web/components/task/task-plan-panel-header.test.tsx` with a
mounted header and mocked shared helper: use saved content distinct from a
Unicode/multiline draft, `activeSessionId=null`, and assert the exact helper
arguments, no `attemptSave`, no implement call, and continued displayed draft
ownership in the real panel E2E. Include empty and whitespace disabled, updated draft at activation,
and synchronous helper error with localized feedback. This verifies the
consumer, not the helper's Blob internals. Existing draft, save-ack, and
session-switch tests cover adjacent owners and should stay passing.

## E2E tests

Add focused desktop `apps/web/e2e/tests/task/plan-download.spec.ts` and
configured `mobile-chrome` `apps/web/e2e/tests/task/mobile-plan-download.spec.ts`
using isolated existing fixtures and the actual main Plan route. Wait for the
download event, check suggested `plan.md` and exact UTF-8 file bytes against
the visible current draft, including a live edit, Unicode and newlines; verify
content remains visible, keyboard and touch activation, and screen containment.
Measure the new control's height on desktop, both dimensions on phone/coarse,
and narrow phone with a fine pointer. Check header containment and document
scroll width, capture and inspect desktop/phone screenshots. Assert only the
action's calls in mounted tests; do not infer absence of independent editor
autosave from whole-page traffic. Explicitly inspect test describe retry
overrides, discover both project tests and run each with one worker and zero
retries. No live developer instance.

## Work orders

- [x] [Task 01: Download the current Plan draft](task-01-download-plan-draft.md)

## Verification results

After ROOT's later execution release, the mounted-header RED failed on the
missing Download button. Targeted Plan tests, lint, i18n, typecheck, spec and
public-doc checks passed; fresh same-worktree web, backend and fixture builds
preceded the browser runs. After the narrow-header correction, Chromium passed
three tests and mobile-chrome passed two, each with one worker and zero retries.
Captures and command receipts are
recorded in the [work order](task-01-download-plan-draft.md#results).

## Risks

- `PanelHeaderBarSplit` clips its action slot; translated labels and narrow
  widths require the Plan header's local scroll behavior and rendered proof.
- Editor serialization can normalize test input; compare downloaded bytes to
  the actual current draft rather than assuming keystrokes equal Markdown.
- Public `sessions-and-review.md` has concurrent unrelated PR edits. Refresh
  the complete open-PR inventory before publication and preserve live contents.
