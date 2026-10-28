---
id: "17-profile-contract"
title: "Profile schema keywords, typed values, and secret references"
status: pending
wave: 2
depends_on:
  - "01"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-PROFILE-001
  - REQ-EXECUTORS-PLUGIN-PROFILE-002
  - REQ-EXECUTORS-PLUGIN-PROFILE-003
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-PROFILE-001.1
  - AC-EXECUTORS-PLUGIN-PROFILE-001.2
  - AC-EXECUTORS-PLUGIN-PROFILE-001.3
  - AC-EXECUTORS-PLUGIN-PROFILE-001.4
  - AC-EXECUTORS-PLUGIN-PROFILE-001.5
  - AC-EXECUTORS-PLUGIN-PROFILE-001.6
  - AC-EXECUTORS-PLUGIN-PROFILE-001.7
  - AC-EXECUTORS-PLUGIN-PROFILE-001.8
  - AC-EXECUTORS-PLUGIN-PROFILE-002.1
  - AC-EXECUTORS-PLUGIN-PROFILE-002.2
  - AC-EXECUTORS-PLUGIN-PROFILE-002.3
  - AC-EXECUTORS-PLUGIN-PROFILE-002.4
  - AC-EXECUTORS-PLUGIN-PROFILE-002.5
  - AC-EXECUTORS-PLUGIN-PROFILE-003.1
  - AC-EXECUTORS-PLUGIN-PROFILE-003.2
  - AC-EXECUTORS-PLUGIN-PROFILE-003.3
  - AC-EXECUTORS-PLUGIN-PROFILE-003.4
  - AC-EXECUTORS-PLUGIN-PROFILE-003.5
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-profile-contract.md
---

# Task 17: Profile schema keywords, typed values, and secret references

## Summary

Close skeleton findings 1 to 3: accept `default`, `minLength`, `maxLength`, and `pattern` in the profile schema, validate profiles in the host before any provider call, add additive `typed_config`, and add additive `secret_refs` beside `secret_values`. Runs after task 01 and can proceed beside tasks 02 to 08.

## Size

M: ~450 source, ~800 test lines.

## In scope

- Secret references carry the existing store ID only (no version, no revision counter); reuse the inventory's `secret_references`; replacing a secret creates a new ID and a retained environment keeps the old one through the existing retention check.
- `unknown_property` for undeclared keys (exempting the six existing keys of `kandevExecutorProfileKeys` plus the new host keys, only for providers declaring a capability) and `profile_too_large` over the 4096-byte config plus secret-references budget, at save and launch; existing over-limit profiles are not rewritten.
- Manifest keywords, one shared value-validation function, `validateProfileAgainstSchema` at save and launch, defaults in the profile editor.
- `typed_config` and `secret_refs` messages, gated by `typed_profile_values` and `secret_references`; recorded reference IDs and recovery re-resolution.

## Out of scope

- Nested objects, arrays, other JSON Schema keywords, and changes to secret storage.

## Acceptance

- A manifest and profile that use none of the new keywords behave byte-for-byte as before.
- The host rejects an invalid profile with stable field errors and never calls the provider; no secret value appears anywhere but the transient `secret_values`.

## Failing-first tests

- Validator table: defaults versus type, enum, bounds, length, and pattern; secret with a default; bad pattern (fails: keywords unknown).
- Linear-time pattern test at a 1 MiB input with a time bound.
- Default change after an upgrade leaves an existing environment's snapshot unchanged.
- Secret rotated or deleted between allocation and recovery; value never in refs, errors, logs, or state.
- `profile_too_large`: measured by one shared function (compact JSON of the filled provider-facing config without the six kandev keys (the document-set keys are measured), plus the secret ID array), identical at save and launch; local and before any provider call and before the echo gate; a resume of an environment allocated with an empty negotiated set is not re-measured after a plugin upgrade adds a capability.
- Unparseable integer stored: `invalid_type`, no provider call. Undeclared `typed_config` use: `conflict`.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -race -trimpath ./internal/plugins/... ./pkg/pluginsdk ./cmd/plugin-fixture -count=1)
```

```bash
(cd apps/backend && go test -race -trimpath ./internal/agent/runtime/lifecycle -count=1)
```

## Likely files and risks

- `apps/backend/internal/plugins/manifest/executor_provider.go`
- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- `apps/backend/internal/agent/runtime/lifecycle/executor_plugin.go`
- The profile editor in `apps/web` shows defaults; strings there go through `t()` in seven catalogs.
- Risk: the new schema keywords change the editor rendering; keep it additive and covered by the existing editor tests.
