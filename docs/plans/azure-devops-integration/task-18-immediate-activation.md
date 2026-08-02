---
id: "18-immediate-activation"
title: "Immediate connection activation"
status: done
wave: 11
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 18: Immediate Connection Activation

## Acceptance

- Saving new or changed Azure credentials performs one bounded auth probe,
  persists the result, and returns config with current `lastOk`,
  `lastCheckedAt`, and `lastError`.
- A successful save invalidates integration availability after the authoritative
  response so sidebar/home/settings consumers render Azure active within one
  second; the 90-second poll remains recovery only.
- A failed probe preserves the saved credential/config, returns unhealthy
  status with a useful error, and does not briefly expose Azure as active.

## Verification

- `go test ./internal/azuredevops -run 'TestSetConfig(ProbesImmediately|PersistsProbeFailure)'` from `apps/backend`.
- `pnpm --filter @kandev/web test -- --run components/azure-devops/azure-devops-settings.test.tsx hooks/domains/integrations/use-integration-availability.test.ts` from `apps`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/service.go`
- `apps/backend/internal/azuredevops/service_test.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/components/azure-devops/azure-devops-settings.test.tsx`
- `apps/web/hooks/domains/integrations/use-integration-availability.test.ts`

## Dependencies

None.

## Parallelism

Sequential. It changes the authoritative connection mutation contract used by
every later Azure feature.

## Inputs

- Spec: immediate activation What, Failure Modes, and save scenario.
- Existing `invalidateIntegrationAvailabilityAfter` and shared health poller.

## Risks

- Do not launch an unobserved background probe: consumers need the probe result
  in the mutation response or a reliable completion signal.

## Output Contract

Report RED/GREEN tests, response semantics, timing evidence, files changed,
residual risks, and update task/plan status.
