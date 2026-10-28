---
status: in_progress
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-003
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 03: Load an operator provider-error rule catalog

## Outcome and scope

Add `routingerr/externalcatalog` (document, validation, matching), the
`routingerr` adapter between built-in rules and the keyword/context fallback,
the `providerErrors.catalogFile` startup setting and its startup loader. Keep
built-in rule order, the default-off fallback, saved policies, safety guards
and model order unchanged. No reload, watcher, remote source, signature or
operator activation is added. Converting an external CSV into this format is
separate work.

## Acceptance

- Unset path performs no file I/O and keeps built-in results.
- JSON and YAML catalogs classify an otherwise unmatched diagnostic through
  `Classify`; built-in matches win; terms/context need the enabled fallback.
- Eligible matches reach the dynamic circuit scope and backoff consumers.
- Invalid catalogs are rejected whole with a stable warning and no content.
- Focused package checks, normal hooks and independent review of the exact delta.
