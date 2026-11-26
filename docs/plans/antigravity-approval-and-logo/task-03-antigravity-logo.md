---
id: "03-antigravity-logo"
title: "Use the Antigravity arch"
status: complete
wave: 3
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-001.5
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 03 Use the Antigravity arch

## Summary

Use the rounded Antigravity mark from the provider-usage plugin in both built-in SVGs. The existing agent logo API serves the correct theme to the composer.

## In scope

- Replace the paths in both Antigravity SVGs with the reference arch. Preserve its viewBox and use the existing theme fills.
- Extend the asset test to verify both SVG variants and their default variant behavior.
- Exercise the actual logo API and the desktop and phone model selector. Use a mock session with an Antigravity provider identity.

## Scope limits

Use the current logo API and shared `AgentLogo` component. The existing composer layouts supply the icon placement and model interaction.

## Acceptance

- The light and dark endpoints serve valid SVGs with matching arch geometry and theme-specific fills. Unknown variants use the light asset.
- The model trigger on desktop and phone displays the corresponding SVG before the model label. The logo fits within the current provider icon slot.

## ASCII UI preview

### UI-02 Composer provider icon

```text
Desktop composer       [Antigravity arch] Gemini 4 Neon [v]
Phone composer         [Antigravity arch] Gemini 4 Neon [v]
```

The placeholder represents the reference SVG. [Full preview](plan.md#ui-02-composer-provider-icon)

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/settings/handlers -run '^Test(AntigravityACP_LogosNonEmpty|GetAgentLogoEndpoint)$' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/model-selector-provider-icon.spec.ts -- --grep 'Antigravity arch')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-model-selector.spec.ts -- --grep 'Antigravity arch')
```

After rebasing, the desktop and phone model-picker tests both passed and
produced fresh captures for the PR description.

## Files likely touched

- `apps/backend/internal/agent/agents/logos/antigravity_acp_light.svg`
- `apps/backend/internal/agent/agents/logos/antigravity_acp_dark.svg`
- `apps/backend/internal/agent/agents/antigravity_acp_test.go`
- `apps/web/e2e/tests/chat/model-selector-provider-icon.spec.ts`
- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`

## Dependencies

None.

## Risks

An SVG rendered in an image element needs its own fill color. The plugin sets that color inline through the surrounding theme. Verify the light and dark assets from a rebuilt backend.

## Parallelism

`sequential`

## Inputs

- [Antigravity identity design](../../specs/agents/system-design/antigravity-acp-agent.md#existing-identity)
- [Provider-usage plugin SVG](https://github.com/kdlbs/kandev-plugin-provider-usage/blob/671dd5b87a95095ba8c99da93941449b7ef62c60/ui/bundle.js#L200)

## Results

Both embedded assets now use the provider-usage Antigravity arch and source
viewBox, with black and white fills for light and dark themes. The asset test
checks XML validity, theme fill, title, viewBox, matching path geometry, and
unknown-variant fallback. The logo endpoint test checks default-light and
explicit light/dark responses.

Verification passed:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/settings/handlers -run '^Test(AntigravityACP_LogosNonEmpty|GetAgentLogoEndpoint)$' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/model-selector-provider-icon.spec.ts -- --grep 'Antigravity arch')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-model-selector.spec.ts -- --grep 'Antigravity arch')
```
