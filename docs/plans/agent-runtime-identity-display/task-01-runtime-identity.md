---
id: "01-runtime-identity"
title: "Clarify runtime identity on agent cards"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-IDENTITY-001
  - REQ-AGENTS-HOST-CLI-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-IDENTITY-001.1
  - AC-AGENTS-RUNTIME-IDENTITY-001.2
  - AC-AGENTS-RUNTIME-IDENTITY-001.3
  - AC-AGENTS-RUNTIME-IDENTITY-001.4
  - AC-AGENTS-RUNTIME-IDENTITY-001.5
  - AC-AGENTS-RUNTIME-IDENTITY-001.6
  - AC-AGENTS-HOST-CLI-001.1
system_design:
  - ../../specs/agents/system-design/runtime-identity-display.md
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 01: Clarify runtime identity on agent cards

## Summary

Make runtime identity understandable on the agent card. Preserve local discovery evidence
behind a diagnostic disclosure and prove the same outcome on desktop and phone.

## In scope

Presentation reducer, card metadata wiring, diagnostics, all locale catalogs, focused tests,
and the existing public agent reference's explanation of runtime versus local CLI discovery.

## Out of scope

Backend contracts, persistence, executable selection, runtime update policy, installation,
authentication, model discovery, and running processes.

## Acceptance

1. The card uses structural metadata to distinguish managed, native, harness-owned, custom,
   and unknown identity, with selected and observed versions kept separate.
2. Local CLI path/version live only in optional diagnostics; identity is always visible.
3. Targeted unit, desktop/mobile rendered, localization, and documentation checks pass.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview).

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

## Verification

Run from repository root; install workspace dependencies only once as needed.

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

## Files likely touched

- `apps/web/lib/agent-runtime-identity.ts` and `.test.ts` (new)
- `apps/web/components/settings/installed-agent-card.tsx` and `.test.tsx`
- `apps/web/components/settings/agent-runtime-summary.tsx` (new)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-tw,zh-hk,ja,ko,pseudo}/agents.json`
- `apps/web/e2e/tests/settings/agent-runtime-identity.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-agent-runtime-identity.spec.ts` (new)
- `apps/web/e2e/tests/settings/agent-runtime-identity-helpers.ts` (new)
- `apps/web/e2e/tests/settings/agent-host-cli.spec.ts`
- `docs/public/agents-and-profiles.md`
- Existing host CLI discovery requirement/design, aligning version placement with diagnostics.

## Dependencies

None. Execute sequentially in the primary conversation after an explicit implementation request.

## Risks

Misreading managed fallback or native OpenCode selection; late status arrival; long paths on phones.
If existing metadata cannot express a selected source, preserve unknown and report rather than
adding launch policy or inventing an executable identity.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/runtime-identity-display.md)
- [Design](../../specs/agents/system-design/runtime-identity-display.md)
- Existing `RuntimeUpdateCapabilities`, `presentHostCLIVersion`, and runtime-update E2E helpers.
- `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer` for implementation.

## Results

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

## PR review remediation

Corrected the harness-owner label in Japanese, Korean, Portuguese, and Chinese catalogs;
Traditional Chinese variants were generated with `pnpm run i18n:zh-hant`.
`pnpm run i18n:check` passed and the focused runtime/card Vitest command passed 29 tests.
The existing ambiguous-runtime table already covers the managed-fallback flag and OpenCode
name guard independently; the review thread was answered with those concrete cases.
Runtime behavior and the rendered composition remain as previously verified.
