---
id: "13-registration-onboarding-protocol"
title: "Registration onboarding protocol"
status: done
wave: 2
depends_on: ["12-registration-persistence"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 13: Registration Onboarding Protocol

## Acceptance

1. Manifest construction uses a preallocated registration ID in every Kandev URL, defaults private,
   supports explicit public visibility, and preserves exact permission/event policy.
2. Existing-App import verifies GitHub App identity before persistence, bounds secret inputs and
   provider responses, and returns actionable secret-safe errors including duplicate registration.
3. Owner URL, origin validation, one-hour state expiry/replay, conversion failure, and import
   mismatch are covered by focused tests.

## Verification

```bash
rtk go test ./internal/github -run 'Test.*(Manifest|Conversion|Import|PublicBaseURL)'
```

Run from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/github/deployment_app_manifest.go` (rename/generalize)
- `apps/backend/internal/github/deployment_app_manifest_test.go`
- `apps/backend/internal/github/deployment_app_conversion_client.go` (rename/generalize)
- `apps/backend/internal/github/deployment_app_conversion_client_test.go`
- `apps/backend/internal/github/app_client.go`
- `apps/backend/internal/github/app_client_test.go`
- `apps/backend/internal/github/app_registration_import.go` (new)
- `apps/backend/internal/github/app_registration_import_test.go` (new)

## Dependencies

Task 12.

## Inputs

- Spec: **GitHub App Policy**, registration catalog API, registration state machine, and security.
- GitHub App Manifest and App identity clients already present on the branch.

## Output Contract

Report protocol/validation behavior, tests run, files touched, blockers/risks, and update this task
plus `plan.md` to done.
