---
id: "01-backend-preference"
title: "Backend archive confirmation preference"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/archive-confirmation.md"
---

# Task 01: Backend Archive Confirmation Preference

## Acceptance

- The user settings GET, PATCH, boot payload, and WebSocket event expose `confirm_task_archive`.
- Missing stored values resolve to `true`; explicit `false` values persist and round-trip.

## Verification

`rtk go test ./internal/user/... ./internal/backendapp/...`

## Files Likely Touched

- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/controller/controller.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/user/**/*_test.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/helpers_test.go`

## Inputs

Spec sections: Data model, API surface, Persistence guarantees, and default-enabled scenario. Follow the existing `show_release_notification` default-true pattern.

## Output Contract

Report contract/defaulting changes, tests run, files touched, blockers, and risks; mark this task and the plan entry done.
