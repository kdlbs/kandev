---
id: "02-upstream-field-sourcing"
title: "Source outcome fields from GitHub"
status: done
wave: 2
depends_on: ["01-schema-and-activation"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/pr-outcome-attribution.md"
---

# Task 02: Upstream field sourcing

Read the five upstream facts from the existing GraphQL, gh CLI, and REST
pull-request paths. Mark a result as populated only when the path actually
observed the relevant fields.

## Implementation

- Extend the shared GraphQL field block with draft, changed files, merger,
  auto-merge, and the latest closed-event actor.
- Extend gh CLI and REST decoders with the fields those APIs expose.
- Keep closure attribution GraphQL-only because the other pull-request APIs do
  not expose a closing actor.
- Add field-level presence flags for optional draft and changed-file values.
- Keep no-op and partial paths unpopulated.

## Verification

Client and helper tests cover field selection, nullable decoding, explicit
`false` and `0`, closed-event ordering, and populated flags.
