---
id: "01-render-clarification-markdown"
title: "Render consistent clarification Markdown"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-MARKDOWN-001
  - REQ-UI-INBOX-HISTORY-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-MARKDOWN-001.1
  - AC-UI-CLARIFICATION-MARKDOWN-001.2
  - AC-UI-CLARIFICATION-MARKDOWN-001.3
  - AC-UI-CLARIFICATION-MARKDOWN-001.4
  - AC-UI-CLARIFICATION-MARKDOWN-001.5
  - AC-UI-CLARIFICATION-MARKDOWN-001.6
  - AC-UI-CLARIFICATION-MARKDOWN-001.7
  - AC-UI-INBOX-HISTORY-001.14
  - AC-UI-INBOX-HISTORY-001.30
system_design:
  - ../../specs/ui/system-design/clarification-markdown.md
  - ../../specs/ui/system-design/inbox-history.md
---

# Task 01: Render consistent clarification Markdown

## Summary

Use the existing limited Markdown renderer for History question summaries and details.
Verify active-question reference links with the same valid Markdown input on desktop and phone.

## In scope

- Connect the shared renderer to History title, prompt, and option-label presentation.
- Keep summary links passive and full question links interactive.
- Preserve title-less prompts, literal fallbacks, metadata, and answer payloads.
- Extend targeted component and desktop/mobile E2E tests. Update the public sessions guide with the supported behavior.

## Out of scope

No context formatting, permission changes, new History fields, plugin chat changes, parser replacement, lifecycle changes, or new dependencies.

## Acceptance

1. History summaries and expanded question fields meet the linked Markdown criteria, including title-less prompt lists and safe HTTP/HTTPS references.
2. Reference, disclosure, and answer activation remain independent. Phone content stays within its surface and current History actions remain usable.
3. Targeted checks pass and the public guide describes only verified support. Record the original active-question case as unconfirmed if no reproduction becomes available.

## ASCII UI preview

Excerpt of [UI-02 and UI-03 in the plan](plan.md#ascii-ui-preview):

```text
UI-02 desktop
[v Choose storage]  Reason  Time  [Open task] [Copy identifier]
   Read storage guidance.       <- reference opens separately
   1. Check retention

UI-03 phone
[v Choose storage                    ]
   Reason  Time  [Open] [Copy]
   Read storage guidance before
   choosing.                    <- reference opens separately
   1. Check retention
```

The summary toggles the row. Only expanded reference links navigate.
Keep the phone action row, page scroll owner, summary ellipsis, and literal context.
Copy and spacing are illustrative. This excerpt maps to AC .1 through .7 of the Markdown requirement.
Read UI-01 in the plan before extending active-question checks.

## Implementation sequence

1. Read the linked specs and scoped frontend guidance. Install workspace dependencies if absent.
2. Add the History component and browser regressions. Observe failures caused by raw Markdown.
3. Wire `ClarificationMarkdown` into the row without changing source strings or broadening syntax. Keep title and prompt branches separate.
4. Extend active-question link checks and the phone regressions. Add the public guide section after behavior passes.
5. Run the commands below. Record actual results and rendered comparison with UI-01 through UI-03.

## Verification

Run from the repository root. Dependencies were installed during the design turn.
In a fresh checkout, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/clarification-markdown.test.tsx components/task/chat/clarification-input-overlay.test.tsx components/task/chat/messages/clarification-request-message.test.tsx components/task/chat/messages/kandev/ask-user-question-renderer.test.tsx components/inbox-history/inbox-history-row.test.tsx components/inbox-history/inbox-history-row.mobile.test.tsx components/inbox-history/inbox-history-row.tablet.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/clarification-markdown.tsx components/task/chat/clarification-input-overlay.test.tsx components/inbox-history/inbox-history-row.tsx components/inbox-history/inbox-history-row.test.tsx e2e/helpers/clarification-markdown.ts e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts e2e/tests/chat/inbox-history-superseded.spec.ts e2e/tests/chat/mobile-inbox-history-superseded.spec.ts)
(cd apps/web && env NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/inbox-history-superseded.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project chromium tests/chat/clarification.spec.ts -- --grep "renders lightweight markdown across active and resolved clarification text")
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-inbox-history-superseded.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "renders lightweight markdown without overflow and submits through formatted content")
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/clarification-markdown
```

The first managed E2E command builds fresh artifacts. Subsequent `--no-build` commands reuse those artifacts only while source stays unchanged.
After any source correction, rebuild through the managed runner before reusing artifacts.
If verification changes a further test file or helper, include it in its targeted test/lint command.
Run the repository coverage helper against the final local changed-file inventory:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const {validateCoverage} = require('./.github/scripts/pr-docs.cjs');
const readPaths = (args) => cp.execFileSync('git', args, {encoding: 'utf8'}).trim().split('\n').filter(Boolean);
const added = new Set(readPaths(['ls-files', '--others', '--exclude-standard']));
const paths = [...new Set([...readPaths(['diff', '--name-only', 'HEAD']), ...added])];
const changedFiles = paths.map(filename => ({filename, status: added.has(filename) ? 'added' : 'modified', additions: 1, changes: 1}));
const documents = [
  'docs/plans/clarification-markdown/plan.md',
  'docs/plans/clarification-markdown/task-01-render-clarification-markdown.md',
  'docs/specs/ui/requirements/clarification-markdown.md',
  'docs/specs/ui/requirements/inbox-history.md',
  'docs/specs/ui/system-design/clarification-markdown.md',
  'docs/specs/ui/system-design/inbox-history.md',
];
const fileContents = Object.fromEntries(documents.map(path => [path, fs.readFileSync(path, 'utf8')]));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

This command checks the unstaged/staged local delivery diff. The PR gate checks committed references at the PR head.

## Files likely touched

- `apps/web/components/inbox-history/inbox-history-row.tsx`
- `apps/web/components/inbox-history/inbox-history-row.test.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.test.tsx`
- `apps/web/components/task/chat/clarification-markdown.tsx` and its tests, only if a demonstrated presentation or coverage gap requires it
- `apps/web/e2e/tests/chat/inbox-history-superseded.spec.ts`
- `apps/web/e2e/tests/chat/mobile-inbox-history-superseded.spec.ts`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`
- `apps/web/e2e/helpers/clarification-markdown.ts`
- `docs/public/sessions-and-review.md`
- This plan, work order, and linked specification lifecycle fields after implementation

## Dependencies

None. Execute sequentially in the primary session.

## Risks

The original active-question link text is unknown. Avoid speculative escape normalization.
Test formatted summary truncation and title-less prompts explicitly.
Keep full reference links outside disclosure and answer buttons.

## Parallelism

`sequential`

## Inputs

- [Markdown requirements](../../specs/ui/requirements/clarification-markdown.md).
- [Markdown design](../../specs/ui/system-design/clarification-markdown.md).
- [History requirements](../../specs/ui/requirements/inbox-history.md), especially AC .14 and .30.
- [History design](../../specs/ui/system-design/inbox-history.md).
- Existing clarification renderer, History row tests, and desktop/mobile clarification and History E2E fixtures.

## Results

Completed on 2026-10-24 in the primary session. No subagents were used.

### Failure reproduction

The new History component cases failed on missing code, unformatted link labels, and title-less prompt formatting.
The extended active-question component case passed on the original renderer.
Both new History browser scenarios failed against the old production build on raw Markdown assertions before the renderer integration.

### Application checks

- The seven-file `pnpm exec vitest run` command above passed: 7 files, 65 tests.
- The scoped `pnpm exec eslint --max-warnings 0` command above passed, including the new shared E2E helper.
- `env NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` passed. The first default-heap attempt exhausted 4 GiB.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed. No new product copy or locale keys were required.
- `pnpm exec prettier --check` passed for the changed component, component tests, E2E helper, and four E2E specs.

### Browser checks

All commands used the managed runner, one worker, `--host`, and `--retries=0`.
The first green History run rebuilt the backend, web assets, and fixture plugin. Later runs reused that unchanged production build.
`scripts/run-quiet e2e --summary --` retained each log and exit code.

| Project and scenario | Result | Final log |
| --- | --- | --- |
| `chromium`, complete `tests/chat/inbox-history-superseded.spec.ts` | 2 passed | `/tmp/kandev-run.e2e.JIxfoY31.log` |
| `chromium`, selected active Markdown scenario in `tests/chat/clarification.spec.ts` | 1 passed | `/tmp/kandev-run.e2e.XJhwOjxh.log` |
| `mobile-chrome`, complete `tests/chat/mobile-inbox-history-superseded.spec.ts` | 2 passed | `/tmp/kandev-run.e2e.Dkobgkst.log` |
| `mobile-chrome`, selected active Markdown scenario in `tests/chat/mobile-clarification.spec.ts` | 1 passed | `/tmp/kandev-run.e2e.M5mfbbsl.log` |

History was checked with keyboard and touch activation. Reference popups used a stubbed destination and closed after assertions.
The stored question text and pending status survived inspection and reference activation.
Phone tests checked single-line summary styles, long-reference bounds, document overflow, and retained 44px row-action targets.
Desktop and phone captures were inspected against the plan previews. The temporary capture directory is `/tmp/kandev-clarification-6516d6ea`.

This NixOS session required a temporary command wrapper to supply existing Chromium libraries from `/nix/store`.
It also removed inherited `KANDEV_INTERNAL_CONFIG_FILE` and `KANDEV_INTERNAL_CONFIG_HOME_FILE`, then set `KANDEV_SERVER_HOST=127.0.0.1` for isolated fixtures.
These were command-local environment settings. Repository runtime defaults and E2E fixture code were not changed.
Initial browser setup failures occurred before the feature assertions and are not counted as reproduction evidence.

### Documentation and contract gates

- `node --test scripts/validate-public-docs.test.mjs`: 63 passed.
- `node scripts/validate-public-docs.mjs`: 48 published pages validated.
- `python3 scripts/list-docs.py validate`: final run validated 384 decisions and 1618 specifications.
- `python3 scripts/lint-spec-files.test.py`: final run passed all 36 tests. `python3 scripts/lint-spec-files.py --all` passed after lifecycle updates.
- The work-order reference coverage preflight returned `covered` for the actual final runtime diff, with no errors. `git diff --check` passed.
- Public docs updated: `docs/public/sessions-and-review.md`, within its existing how-to guide.

### PR review remediation

Greptile identified that fenced code uses a whitespace-preserving span, so an override limited to inline `code` elements did not keep summaries on one line.
The summary now applies no-wrap styling to every Markdown descendant. Expanded content retains its line breaks.
The existing desktop and phone History scenarios include fenced code and assert both presentations.

- Before the correction, the selected desktop and phone scenarios failed on summary height: `/tmp/kandev-run.e2e.mGgHoR01.log` and `/tmp/kandev-run.e2e.pvInIQPv.log`.
- After the correction, both complete History specs passed with `--host --retries=0`: 2 desktop tests in `/tmp/kandev-run.e2e.VdXTXrAS.log` and 2 phone tests in `/tmp/kandev-run.e2e.zU8lXQ19.log`.
- The seven-file Vitest command above passed again: 65 tests. The changed component, helper, and two History specs passed scoped ESLint and Prettier checks. The 8 GiB TypeScript command passed again.
- The existing single-line summary contract and public documentation remain accurate. No syntax, link policy, or payload contract changed.

### CI browser corrections

CI exposed two existing test problems. The sidebar test reloaded after an optimistic visibility update, before persistence completed.
It now checks the saved workspace layout before reloading. Recovery feedback tests now activate the failure banner's Retry button on desktop and phone.
The separate stopped-session card can still be inspecting the session at that point.

Verification used the original failing CI process selections, with updated catalog hashes for the changed tests.
A task-local script used the repository's `partitionShard` and guarded raw runner. Each replay used two CPUs, one browser worker, and zero retries.

- Shard 24, process 4: 34 tests passed, including the corrected desktop recovery scenario. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/replay-24-logs/kandev-run.e2e.EDYtjkvb.log`.
- Shard 17, process 4: 31 tests passed, including the corrected sidebar persistence scenario. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/replay-17-logs/kandev-run.e2e.NN3dInrN.log`.
- The selected phone recovery scenario passed. Log: `/tmp/kandev-run.e2e.lWZOoOv6.log`.
- Scoped ESLint, Prettier, and the 8 GiB TypeScript command passed for the three changed browser specs.

The replay worktree combined the feature with the current main tree. The backend build used the command-local override `MAKEFLAGS=GOFLAGS=-buildvcs=false` because Go's VCS lookup failed from the temporary worktree.
Local runs selected unused E2E port ranges after initial fixture-startup failures. Setup failures are excluded from feature regression evidence.
The corrections adjust test synchronization and action selection; the product contracts and public guide remain accurate.

Remote CI and final review completion remain external delivery checks until the corrected tests are pushed and checked.

### SSH CI diagnostics

The corrected sidebar and recovery checks passed remotely at `097fc8f7eed`.
The SSH interruption scenario failed again, and its diagnostics recorded `ENOENT` for the remote agent log.
The collector had used the host's view of a Docker mount. It now reads the validated session log inside the fixture container.
The remote command returns at most 512 KiB. The collector discards the first potentially partial line, redacts credentials before writing, and keeps the existing aggregate artifact budget.
Remote command errors retain only their error code or a generic message, so command output cannot enter metadata.

The three task commits were rebased onto main at `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab` to update the current SSH fixture.
The rebased tree matched the previously tested synthetic merge exactly before the diagnostic changes.
The new reader regressions failed before the collector change; all 13 diagnostic tests then passed.
Local Docker is unavailable. CI must supply the SSH reproduction and remote log evidence; the diagnostic change alone does not establish the runtime failure's cause.

### Additional CI browser corrections

CI at `b9616d7ac96` passed all SSH container shards but found three browser-test failures.
The ownership test assumed every executor had a profile array. API responses omit empty arrays, so the ownership poll could throw before the save completed. Both catalogue reads now use an empty-array fallback.
The phone history test now waits for the outer drawer's animation before tapping its action. Its prior wait inspected an inner element that does not own the animation.
The repository-options test now waits for each repository picker to unmount before opening the next surface. CI showed the options popover disappearing, followed by a 53-second wait for its missing download control.

The original shard 1, process 4 selection reproduced the ownership exception locally: 43 passed, one failed, and one did not run. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/diagnostics-replay-1-logs/kandev-run.e2e.DfwUYR0g.log`.
The corrected selections passed with two CPUs, one worker, and zero retries:

- Shard 1, process 4: 45 passed. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/diagnostics-replay-1-logs/kandev-run.e2e.o870sew9.log`.
- Shard 10, process 2: 45 passed and one skipped. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/diagnostics-replay-10-logs/kandev-run.e2e.Kr1k5iXZ.log`.
- Shard 19, process 1: 41 passed. Log: `/tmp/kandev-pr-6516d6ea-33_nf3t6/diagnostics-replay-19-logs/kandev-run.e2e.ueGOXfs2.log`.
- Scoped ESLint, Prettier, TypeScript, and specification lint passed after the final changes.

### Secondary review correction

The user-requested secondary review found no blockers and one optional CI reliability issue.
The remote diagnostic call could wait indefinitely for Docker. It now sets a five-second timeout with `SIGKILL` for each Docker attempt. The existing retry uses the same timeout. Other SSH test commands keep their prior behavior.
The stalled-child regression failed before the change after waiting 5.08 seconds. It now covers both the initial attempt and a retry, confirms partial credential-bearing output was emitted, and checks that collection returns promptly without retaining that output or replacing the original test failure.
The successful remote-tail case also runs an executable Docker fixture. All 15 diagnostic tests, scoped ESLint, Prettier, TypeScript, and specification checks passed for this correction.
Real Docker is unavailable locally, so fresh SSH container CI remains required after the push.

### Mobile automation CI correction

The mobile webhook test could match the creation fallback while the saved automation was still loading. Holding `automation.get` confirmed that the broad `Webhook` selector tapped `Or use a webhook instead` and reproduced the missing-field assertion.
Both visits now wait for the trigger card and tap its exact `Webhook` disclosure. The existing persistence, touch-target, overflow, and deletion assertions are retained.
The controlled loading case, scoped lint, Prettier, and TypeScript passed. The original 36-test CI process selection also passed after the correction, with two CPUs, one worker, and zero retries.
The first failing CI attempt did not retain an action trace. Its screenshot and DOM match the controlled failure's collapsed-card state.

### Remaining limit

The original active-question link text was not provided. Standard links pass through the actual active overlay on desktop and phone.
This verifies the supported contract without claiming a reproduction of the original active-question example.
