---
id: "02-authenticated-webhook-manifest"
title: "Extend webhook declarations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/voice-extraction-host.md"
---

# Task 02: Extend Webhook Declarations

## Acceptance

- Manifest webhooks support optional `access: public|authenticated` and `max_body_bytes`, with legacy
  declarations resolving to public access and a 4 MiB limit.
- Validation rejects unknown access modes, public limits above 4 MiB, and authenticated limits above
  16 MiB, while preserving existing key uniqueness and method behavior.
- Install, registry, and JSON/YAML round-trip tests preserve the new fields. Protobuf and the Go plugin
  SDK remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/manifest ./internal/plugins
```

Follow RED-GREEN-REFACTOR, beginning with legacy-default and invalid-limit cases.

## Files Likely Touched

- `apps/backend/internal/plugins/manifest/*`
- `apps/backend/internal/plugins/types.go`
- plugin install/registry serialization tests

## Inputs And Risks

- Spec authenticated-webhook API and compatibility section.
- Do not reinterpret omitted fields during YAML/JSON round trips in a way that breaks old packages.
