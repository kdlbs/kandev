---
created: 2026-10-22
status: implemented
requirements:
  - REQ-AGENTS-RUNTIME-IDENTITY-001
  - REQ-AGENTS-HOST-CLI-001
system_design:
  - ../../specs/agents/system-design/runtime-identity-display.md
  - ../../specs/agents/system-design/host-cli-model-discovery.md
legacy_specs: []
---

# Implementation Plan: Agent runtime identity display

## Overview

Show the selected structured session runtime prominently and move detected local CLI evidence
into diagnostics. One sequential vertical work order covers presentation logic, localized UI,
public explanation, and desktop/phone evidence. The user confirmed this direction after asking
why a CLI path needed to appear in the main card.

## Scope

In scope: runtime identity on agent settings cards, optional diagnostics disclosure, unknown
states, localization, and responsive verification. Excluded: launch policy, runtime updates,
authentication, installation, model discovery, and remote installation ownership.

## Technical approach

Use `InstalledAgentCard`'s existing runtime metadata and a pure presentation reducer.
Keep `presentHostCLIVersion` for local discovery inside diagnostics. Existing update controls
and runtime policy rows retain their existing responsibility. No API or database change.

| Runtime shape | Intended identity | Evidence/fallback |
| --- | --- | --- |
| Copilot ACP with detected host CLI | Managed package and effective version | Reducer + desktop/mobile fixture |
| Managed npm ACP or app-server | Managed package and effective version | Same structural metadata; unit tests |
| Native host with managed fallback | Native CLI | Status source/owner; never fallback version |
| Harness-owned updater | Harness-owned CLI | self_update metadata; unit test |
| Custom/virtual/unrecognized | Custom or unknown, as evidenced | No guessed managed identity |

## ASCII UI preview

### UI-01: Agent settings card, managed runtime

Entry: Settings > Agents. Desktop keeps header actions beside the identity.
Phones give identity the full card width and place actions below it.

```text
Copilot                 [MCP] [Configured]
Session runtime: Managed by Kandev
@github/copilot · <selected version>
For new sessions
> Diagnostics
[existing profile content and actions]
```

### UI-02: Phone agent settings card

```text
Copilot [MCP] [Configured]
Session runtime: Managed by Kandev
@github/copilot · <selected version>
For new sessions
> Diagnostics
[Update] [Collapse] [Setup/New profile]
[existing profile content]
```

Identity precedes actions and uses the full content width. The page owns scrolling.

Expanded diagnostics:

```text
v Diagnostics
  Detected local CLI: /opt/homebrew/bin/copilot
  Local CLI version: <observed version or unknown>
```

Required structure: runtime identity always visible; local discovery only in diagnostics.
Spacing and example values are illustrative. On phones long values wrap and the diagnostics
summary has a 44px hit target. Identity width must exceed 75% of card width. Existing page scrolling and profile controls remain.
Native state reads `Session runtime: Native CLI`; unknown state reads
`Session runtime: Unknown`. Missing managed version reads `Unknown version`.
No empty diagnostics disclosure is rendered.
Maps to AC-AGENTS-RUNTIME-IDENTITY-001.1 through .6.

## Tests

`lib/agent-runtime-identity.test.ts`: managed selection beats detected path; native fallback
is separate; self-update is distinct; missing/ambiguous metadata fails to unknown; effective
version is not replaced by local/observed/latest versions (AC .1, .2, .4, .6).
`components/settings/installed-agent-card.test.tsx`: retained version/unknown-reason coverage
and initially closed diagnostics (AC .3, .4). Browser tests own keyboard and touch expansion;
existing profile collapse tests retain coverage of the separate profile region.

## E2E tests

New `tests/settings/agent-runtime-identity.spec.ts` (chromium) and
`tests/settings/mobile-agent-runtime-identity.spec.ts` (mobile-chrome) cover AC .1 through .6:
Copilot managed identity with detected path, selected native fallback case, initially closed
diagnostics, expansion, long-path readability, no horizontal overflow, no mutation requests.
Use existing runtime-update API fixture patterns. Capture collapsed/expanded states on both.

## Work orders

- [x] [Task 01: Clarify runtime identity on agent cards](task-01-runtime-identity.md)

## Verification

Run from the repository root. Install dependencies once if this worktree has not been bootstrapped.
The managed E2E runner builds current web/backend artifacts; run the two projects sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/agent-runtime-identity.test.ts components/settings/installed-agent-card.test.tsx)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/agent-runtime-identity.ts components/settings/installed-agent-card.tsx components/settings/agent-runtime-summary.tsx e2e/tests/settings/agent-runtime-identity-helpers.ts e2e/tests/settings/agent-runtime-identity.spec.ts e2e/tests/settings/mobile-agent-runtime-identity.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run tests/settings/agent-runtime-identity.spec.ts tests/settings/agent-host-cli.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-runtime-identity.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Completed 2026-10-22 in the primary conversation, without delegation.

- Workspace dependency installation passed with the frozen lockfile.
- Focused Vitest: 29 tests passed across runtime identity and existing card tests.
- Targeted ESLint passed with zero warnings, including the new E2E files.
- Typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`; the first attempt
  exhausted Node's default 4 GiB heap. No type errors were reported by the successful run.
- `pnpm run i18n:check` passed for every shipped locale and pseudo.
- Chromium: 3 tests passed (managed/native runtime identity plus existing host CLI discovery).
- Mobile Chrome: 2 tests passed, including full-width identity, 44px diagnostics target,
  touch expansion, long-path wrapping, and no horizontal page overflow.
- Final desktop run reused the fresh mobile build with `--no-build`; both identity states
  and the existing host CLI flow passed after the responsive layout change.
- Inspected desktop and phone screenshots. Phone actions sit below full-width identity.
  A new geometry test first failed against the squeezed layout, then passed after the fix.
- Public-doc validator: 54 pages passed; validator tests: 63 passed.
- Specification catalog validation, specification lint, documentation coverage preflight,
  and diff whitespace check passed. The design-turn specification-linter tests also passed (36).
- Public reference and existing host CLI requirement/design now describe version/path
  placement inside Diagnostics. No runtime selection, API, persistence, or authentication changes.

Rendered captures were produced by the existing `prCapture` fixture. Development instances
were not touched; the managed Docker E2E runner tore down its isolated instances.


## Risks

- Runtime ownership and detected host CLI are different facts; native fallbacks and OpenCode
  source selection require explicit tests to prevent a new misleading label.
- Status can load later than the card. Unknown must remain honest until source metadata arrives.
- Native versions can be unknown without invalidating runtime identity.
- Public docs impact: update the existing reference in `docs/public/agents-and-profiles.md`
  during implementation. This design turn changes internal docs only.

## PR review remediation

Corrected the harness-owner label in Japanese, Korean, Portuguese, and Chinese catalogs;
Traditional Chinese variants were generated with `pnpm run i18n:zh-hant`.
`pnpm run i18n:check` passed and the focused runtime/card Vitest command passed 29 tests.
The existing ambiguous-runtime table already covers the managed-fallback flag and OpenCode
name guard independently; the review thread was answered with those concrete cases.
Runtime behavior and the rendered composition remain as previously verified.
