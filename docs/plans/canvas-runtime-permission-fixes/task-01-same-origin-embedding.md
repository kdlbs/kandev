---
id: "01-same-origin-embedding"
title: "Support same-origin canvas embedding"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.7
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.8
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.10
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 01: Support same-origin canvas embedding

## Summary

Add the host-owned `'self'` source to runtime framing policy. This fixes custom
DNS, IP, and port access without maintaining a hostname list or weakening the
opaque sandbox.

## In scope

- Add the keyword in `BuildContentSecurityPolicy`, outside exact-origin parsing.
- Preserve configured launcher/Tauri origins, current network policy, and
  relative runtime paths. No request-header-derived ancestor values.
- Real-browser regression through two HTTPS aliases and a foreign parent.
- Document same-origin hosting in public canvas/security guidance.

## Out of scope

New DNS/configuration keys, cross-origin deployment setup, initial grants,
startup acknowledgement, dialog changes, or desktop packaging mutations.

## Acceptance

1. `TestBuildContentSecurityPolicyIncludesSelf` fails on the old policy, then
   passes with existing sandbox, normalization, and token tests preserved.
2. Real entry content executes through each custom same-origin HTTPS host;
   unrelated and nested foreign ancestors stay blocked, including spoofed
   forwarded-header requests. Localhost and Tauri origins remain supported.
3. The fix does not add wildcard ancestors, reflect inbound headers, or modify
   `allow-same-origin`, network grants, or stored release artifacts.

## Verification

From the repository root, after the plan's one-time dependency install:

```bash
(cd apps/backend && go test ./internal/plugins/webapp/...)
(cd apps/web && pnpm e2e:run --project chromium tests/canvas/canvas-host-origins.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Create the policy unit RED first. The browser regression must assert content
execution, not the outer host's existing Ready label. Close test proxies,
contexts, and certificate directories in failure cleanup. No production DNS,
live runtime, or public task interaction is permitted.

## Files likely touched

- `apps/backend/internal/plugins/webapp/policy.go`, `policy_test.go`, `runtime_test.go`
- `apps/backend/internal/plugins/webapp/runtime.go`, `apps/backend/internal/plugins/provider.go` only if needed to preserve explicit origins
- New `apps/web/e2e/tests/canvas/canvas-host-origins.spec.ts`
- New `apps/web/e2e/tests/canvas/canvas-origin-fixture.ts`
- `docs/public/canvases.md`, `docs/public/security.md`

## Dependencies

None. Start with the current spec and repository head; no other open PR is a prerequisite.

## Risks

An E2E proxy that rewrites CSP can make the test pass without proving the fix.
An HTTPS-only trust bypass in test code must remain confined to disposable
browser contexts. Cross-alias embedding is not same-origin embedding.

## Parallelism

`sequential`

## Inputs

- Plugin design: Browser host and Browser security boundary.
- Existing runtime/security tests and `FrameAncestorsForConfig`.
- TLS fixture mechanics in `apps/web/e2e/helpers/plugin-git-credentials.ts`.
- [Plan evidence and E2E matrix](plan.md).

## Results

Pending.
