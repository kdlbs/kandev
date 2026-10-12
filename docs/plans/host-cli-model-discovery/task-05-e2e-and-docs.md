---
id: "05-e2e-and-docs"
title: "E2E coverage and public docs"
status: complete
wave: 5
depends_on:
  - "04-frontend"
plan: "plan.md"
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-004
acceptance_criteria:
  - AC-AGENTS-HOST-CLI-001.1
  - AC-AGENTS-HOST-CLI-002.1
  - AC-AGENTS-HOST-CLI-002.3
  - AC-AGENTS-HOST-CLI-004.1
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
---

# Task 05: E2E coverage and public docs

## Summary

Prove the user-facing flows against the mock agent's host CLI surfaces on
desktop and phone, and document the version line, discovery note, and
custom model entry in the public agents guide.

## In scope

- `agent-host-cli.spec.ts` (chromium): agent card shows the mock CLI
  version; profile selector shows the discovery note; a custom model ID is
  typed, saved, and survives reload.
- `mobile-agent-host-cli.spec.ts` (mobile-chrome): same selector flow in the
  viewport-contained popover.
- `docs/public/agents-and-profiles.md`: version display and dynamic model
  list/custom entry sections.

## Out of scope

- Backend or frontend behavior changes beyond test IDs.
- Any CLI install or update flow (unchanged, already documented elsewhere).

## Acceptance

- Both specs pass on the managed runner against a fresh build.
- Public docs validation passes.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/settings/agent-host-cli.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-agent-host-cli.spec.ts)
(node scripts/validate-public-docs.mjs)
```

## Files likely touched

- `apps/web/e2e/tests/settings/agent-host-cli.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-host-cli.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 04.

## Risks

- The mock CLI list must stay a subset of the mock bridge list so existing
  model-count assertions in other specs are unaffected.

## Parallelism

`sequential`

## Inputs

- Plan E2E matrix; existing settings specs under `e2e/tests/settings/`;
  `/e2e` and `/mobile-parity`.

## Results

Implemented as scoped: `agent-host-cli.spec.ts` (chromium) and
`mobile-agent-host-cli.spec.ts` (mobile-chrome), both driving the mock
agent's host-CLI version and `codex app-server`-shaped model listing, and
both passing against a full local build. `docs/public/agents-and-profiles.md`
gained a "Claude Code and Codex CLI version and model discovery" section;
`node scripts/validate-public-docs.mjs` passes.
