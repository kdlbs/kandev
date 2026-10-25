---
id: "01-publish-composer-contract"
title: "Publish composer contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 01: Publish Composer Contract

## Acceptance

- Public plugin types define composer surface/state/capability/results exactly as the
  spec, with backward-compatible additions to existing slot props.
- Internal capability lifecycle utilities make stale/unmounted capabilities
  fail closed without exposing draft values or editor handles.
- Host API tests cover teardown, prop updates, and stale capability behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins/host-api.test.ts lib/plugins/host-lifecycle.test.ts lib/plugins/host.test.ts
cd apps/web && pnpm run typecheck
```

Follow RED-GREEN-REFACTOR: add contract/lifecycle failures first, then the smallest public types and
host wiring. Do not add composer surface adapters in this task.

## Files Likely Touched

- `apps/web/lib/plugins/types.ts`
- `apps/web/lib/plugins/host-api.ts`
- `apps/web/lib/plugins/host.ts`
- focused tests beside those files
- a new internal composer-capability utility and test
- `docs/plans/plugins/PLUGIN-API.md`

## Inputs And Risks

- Spec API Surface and ADR mounted-capability decision.
- Avoid a Zustand draft slice, public editor handle, or snapshot-only state. Strict Mode teardown must
  not notify or mutate after revocation.
