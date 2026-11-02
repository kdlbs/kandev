---
id: "01-preserve-dismissal"
title: "Preserve prompt preview dismissal intent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-ALIAS-001
  - REQ-UI-PROMPT-ALIAS-002
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
acceptance_criteria:
  - AC-UI-PROMPT-ALIAS-001.1
  - AC-UI-PROMPT-ALIAS-002.4
---

# Task 01: Preserve dismissal intent

## Summary

Reject delayed opens from before dismissal until fresh trigger intent. Preserve
ordinary hover/focus, immediate click/keyboard toggles, and native phone preview.

## In scope

- Permanent causal timing regression using real production component and Radix.
- Local host preview reconciliation and positive interaction controls.
- Desktop and existing native-phone browser verification; owning docs.

## Out of scope

Shared UI primitive policy, prompt parsing, task mentions, plugins, persistence,
backend, permission changes, push, and PR creation before parent validation.

## Acceptance

- Pending hover/focus cannot undo Escape, outside, or toggle dismissal.
- Fresh hover/focus/click/Enter/Space opens; content transit and unmount are safe.
- Existing phone tap opens native Drawer; dismissal and subsequent tap work.

## ASCII UI preview

### UI-01: Desktop prompt inspection

Entry: a saved-prompt alias in a transcript or Host prompt row.

```text
[@daily] -> hover/focus delay or activation -> [saved definition]
Escape / outside / toggle close -> [@daily] (closed)
Older open callback -> [@daily] (still closed)
New pointer/focus entry or activation -> [saved definition]
```

### UI-02: Phone prompt inspection

Entry: tap the existing 44px alias trigger.

```text
Transcript [@daily] -> tap -> bottom Drawer
+--------------------------+
| Custom prompt: daily     | fixed existing header
| Saved definition         | internal scroll body
+--------------------------+
Dismiss -> transcript [@daily]; tap again -> Drawer
```

Structure and behavior are required; spacing is illustrative. The existing
Drawer is the nearest shipped mobile exemplar, appropriate for temporary
inspection. It retains its sole content scroller, dynamic viewport bound,
safe-area padding and touch targets. No composition or copy changes.
Both views map to AC-UI-PROMPT-ALIAS-001.1 and 002.4.

Full preview: [plan](plan.md#ascii-ui-preview).

## Verification

Run from repository root. The managed runner builds fresh production assets.
The second browser command may reuse the first command's unchanged build.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/prompt-mention-preview.test.tsx components/task/chat/messages/prompt-mention-components.round2.test.tsx lib/prompts/prompt-mention-segments.test.ts)
(cd apps/web && pnpm exec eslint components/task/chat/messages/prompt-mention-components.tsx components/task/chat/messages/prompt-mention-preview.test.tsx e2e/tests/chat/chat-prompt-mention-hover.spec.ts e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --project chromium e2e/tests/chat/chat-prompt-mention-hover.spec.ts --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run local PR-documentation coverage from the repository root with Node 24 on
PATH (the workspace shell supplies it), including untracked files:

```bash
node <<'NODE'
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");
const { validateCoverage } = require("./.github/scripts/pr-docs.cjs");
const git = (...args) => execFileSync("git", args, { encoding: "utf8" });
const changedFiles = [...new Set([
  ...git("diff", "--name-only", "-z").split("\0"),
  ...git("ls-files", "--others", "--exclude-standard", "-z").split("\0"),
].filter(Boolean))];
const paths = [
  "docs/plans/prompt-preview-dismissal/plan.md",
  "docs/plans/prompt-preview-dismissal/task-01-preserve-dismissal.md",
  "docs/specs/ui/requirements/prompt-alias-rendering.md",
  "docs/specs/ui/system-design/prompt-alias-rendering.md",
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, "utf8")]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

After committing, repeat coverage using `git diff <base> HEAD --name-only` and
`git show HEAD:<path>` for the same four documents. Inspect new files and
cross-reference coverage before done.

## Files likely touched

- `apps/web/components/task/chat/messages/prompt-mention-components.tsx`
- `apps/web/components/task/chat/messages/prompt-mention-preview.test.tsx`
- `apps/web/e2e/tests/chat/chat-prompt-mention-hover.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts`
- Owning requirements/design and this package.

## Dependencies

None. Clean current live main base: `32575f4c88fe3c734e23fec1253792704cac7480`.
The supplied initial base `28d7927c19321dba52022de6ec567d4f121c19cb` is its parent;
only sibling PR 4669 intervenes. Preserve those hunks.

## Risks

Do not block fresh intent or pointer transit while rejecting stale callbacks.
No permanent tests may mock HoverCard, Radix timers, or controlled open state.

## Parallelism

`sequential`

## Inputs

- Owning UI requirements and system design, preview dismissal section.
- Preserved parent diagnostic under `/tmp/kandev-loop-attempt14/`.
- Existing prompt hover/browser and phone Host facade fixture cases.

## Results

- Independent permanent RED: actual StateProvider, PromptMentionChip, shared
  HoverCard, installed Radix, and controlled happy-dom clock; only the touch
  viewport hook mocked. Handle 63552 exited 1 with one behavioral failure and
  two passing controls. Pointer hover schedules open; click opens; Escape closes;
  after 1,000 ms the old callback produced `aria-expanded=true`, expected false.
  Initial root-shell invocation lacked pnpm (exit 127); excluded from RED.
- Related second-pointer-click RED: handle 28821 exited 1, twelve passing cases
  and one expected false/actual true failure. Outside pointer-down dismissed
  the trigger before click toggled it open again. Trigger exclusion prevents
  that double transition without changing true outside dismissal.
- Final focused Vitest command: three files, 40 tests passed (53582 exit 0),
  including 13 interaction/lifecycle cases. Covers delayed hover/focus, Escape,
  outside dismissal, click/Enter/Space toggles, ordinary delayed entry, fresh
  entry after dismissal, leave/reentry, content transit/interaction and unmount.
- Scoped ESLint with warnings forbidden and Prettier check passed. The first
  lint invocation found the new describe callback exceeded the function-length
  limit; splitting the interaction groups resolved it. No rule bypass.
- Typecheck with Node 24 and `NODE_OPTIONS=--max-old-space-size=8192` passed
  (25340 exit 0). Initial default 4 GiB run (27279) exhausted its JavaScript
  heap, exit 134, before TypeScript diagnostics. The successful run retains the
  existing shared resource guard.
- i18n check passed (20020 exit 0); it reports existing orphan catalog entries.
  No new user-facing copy or translation keys.
- Fresh managed production build, desktop Chromium: two cases passed, retries
  zero (80168 exit 0). Actual mouse/keyboard events plus a controlled browser
  clock prove pending-hover dismissal, fresh hover, second click, Enter, Space,
  and Escape through shipped transcript controls. Component RED is the causal
  gate; browser checks were added after it and run on the correction.
- Native phone managed fixture: one case passed, retries zero (50304 exit 0).
  The shipped Host alias opens a real Drawer, dismisses, opens again by tap,
  and dismisses again. The fixture backend/database/plugin are disposable;
  no real plugin installation or operator instance is modified.
- Optional phone capture run (44572 exit 0) also passed. Its screenshot caught
  opening motion; added the existing finite-animation settling helper only
  when captures are enabled. Final capture run 23597 exited 0, one case passed
  with retries zero. Inspected the settled phone screenshot: native inset
  Drawer, fixed title above definition, and safe-area clearance match UI-02.
- Documentation catalog and full spec lint passed (30718 exit 0). Changed-path
  PR-documentation coverage preflight passed with no errors and both owning
  requirements accepted. An initial preflight enumerated untracked paths from
  the web subdirectory and omitted the plan; corrected enumeration from root
  passed. No claimed product defect from that setup-only result.
- Public docs assessed using docs-maintainer: existing preview instructions
  remain accurate. This restores dismissal intent with no new control, copy,
  navigation, API, or configuration, so only internal owning docs change.
- Current main retained: `32575f4c88fe3c734e23fec1253792704cac7480`.
  Refreshed 70-open-PR inventory; PR 4664 remains at
  `4ad4556ed24db1fff02f434024b6cb7914e69484`, full paginated patch unchanged.
  Its upper-file task-mention work does not repair this preview interaction.
- Limits: clock-controlled component/browser reproduction is not live-incident
  evidence. No broad suites or production instance exercised. Stop at normal
  hook commit for independent parent validation, before push/PR.
