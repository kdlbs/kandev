---
id: "03-agent-discoverability"
title: "Agent plugin discoverability"
status: completed
wave: 3
depends_on: ["02-contract-reconciliation"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/authoring-experience.md"
---

# Task 03: Agent plugin discoverability

## Acceptance

1. Root, backend, and web `AGENTS.md` each link directly to the canonical guide
   and identify the contract authorities relevant to their scope.
2. The existing `create-kandev-plugin` skill follows the canonical guide,
   reflects implemented Host writes, and sends agents through choose recipe →
   edit manifest → implement → validate → package → smoke test.
3. All focused documentation, harness, backend package, and frontend plugin-host
   checks pass, with exact results recorded in the plan.

## Verification

```sh
rg -n "docs/public/plugins-authoring.md|PLUGIN-API.md|apps/web/lib/plugins/types.ts|pkg/pluginsdk|plugin.proto|internal/plugins/manifest" AGENTS.md apps/backend/AGENTS.md apps/web/AGENTS.md .agents/skills/create-kandev-plugin/SKILL.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make -C apps/backend e2e-plugin-package
(cd apps/backend && go test ./cmd/plugin-pack ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/plugins/pkgtar ./pkg/pluginsdk)
(cd apps && pnpm --filter @kandev/web test -- lib/plugins/host-api.test.ts lib/plugins/host.test.ts lib/plugins/registry.test.ts lib/ws/plugin-bridge.test.ts)
git diff --check
```

## Files likely touched

- `AGENTS.md`
- `apps/backend/AGENTS.md`
- `apps/web/AGENTS.md`
- `.agents/skills/create-kandev-plugin/SKILL.md`
- this plan and its task files

## Results

- Root, backend, and web AGENTS now link directly to the canonical guide and
  identify the frontend pair, backend SDK, proto, manifest, and package
  authorities.
- `create-kandev-plugin` now routes through the same guide, current Host write
  behavior, unavailable-API notes, and layered validation scope.
- Stale-reference audit and `git diff --check` passed.
