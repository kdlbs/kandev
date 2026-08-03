---
spec: docs/specs/plugins/authoring-experience.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Plugin Authoring Experience

## Overview

Make the existing public authoring page the single discovery entry point,
reconcile nearby documentation against the current frontend and backend
contracts, and route agents there from scoped guidance and the existing plugin
skill. The change is documentation-first and adds no plugin runtime or
validation code.

## Canonical developer guide

Rewrite `docs/public/plugins-authoring.md` around the author workflow while
keeping its stable public slug:

1. lifecycle diagram and package/data layout;
2. plugin shapes plus security/capability boundaries;
3. storage decision table;
4. complete current frontend and backend matrices linked to authoritative
   source;
5. six current-contract recipes;
6. exact build/test/package/install/smoke sequence and validation scope;
7. common mistakes and explicit unavailable-surface notes.

The guide will use small copy-pasteable snippets and link larger examples to the
in-tree fixture and official template. Broken references to absent example
repositories will be removed.

The workflow will document existing validation honestly:

- plugin repository formatting, unit tests, vet/lint, and builds;
- `plugin-pack` package creation and generated `checksums.txt`;
- package/archive/manifest/host-executable checks performed by installation;
- `make -C apps/backend e2e-plugin-package` and the focused `plugin-pack` test
  as the maintained in-tree package smoke validation;
- install and capability smoke tests in a disposable development instance.

It will state that there is no standalone exhaustive package checker in this
branch and distinguish build-time, pack-time, install-time, and runtime checks.

## Contract and example reconciliation

Update the public plugin overview, manifest guide, marketplace cross-links,
`docs/plugins-example.md`, the frozen plugin contract notes, and plugin spec
where they contradict the current implementation. Key corrections are:

- implemented Host task/message writes are not described as reserved;
- `ui.bundle` examples include the required leading slash;
- only actually mounted component slots are listed as supported;
- missing example repositories are not presented as maintained code;
- unsupported task-panel, task-menu, per-user storage, rich-text, and Kanban
  card APIs are identified as unavailable rather than assigned speculative
  signatures.

Prefer documentation edits. Change source files only if a stale documentation
comment inside a fixture or type file would otherwise remain directly
misleading; do not change runtime types, behavior, schemas, or tests.

## Agent and contributor discoverability

Update `AGENTS.md`, `apps/backend/AGENTS.md`, and `apps/web/AGENTS.md` with a
direct canonical-guide link, the authority map appropriate to each scope, and
the workflow: choose recipe → edit manifest → implement → validate → package →
smoke test.

Update `.agents/skills/create-kandev-plugin/SKILL.md` so agents read the same
canonical guide and contract map, use implemented Host writes correctly, and
follow the documented validation boundaries. Keep the skill procedural and the
public page explanatory; do not duplicate full matrices in the skill.

## Tests and validation

- Public-doc structure and local links:
  `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`.
- Backend package/example evidence:
  `make -C apps/backend e2e-plugin-package` and focused tests for
  `cmd/plugin-pack`, `cmd/plugin-fixture`, `internal/plugins/manifest`,
  `internal/plugins/pkgtar`, and `pkg/pluginsdk`.
- Frontend contract/lifecycle evidence: focused plugin host, API, registry, and
  WebSocket bridge tests.
- Harness/doc hygiene: targeted stale-reference searches plus
  `git diff --check`.

No new browser E2E is planned because no product UI behavior changes.

## Implementation waves

Wave 1:
- [x] [task-01-canonical-guide](task-01-canonical-guide.md)

Wave 2:
- [x] [task-02-contract-reconciliation](task-02-contract-reconciliation.md)

Wave 3:
- [x] [task-03-agent-discoverability](task-03-agent-discoverability.md)

## Risks

- A summary matrix can drift into a competing contract. Keep signatures
  abbreviated, link every section to authoritative files, and avoid copied DTO
  field inventories.
- The external template can drift independently. Treat it as scaffold/build
  workflow, not as the API source of truth.
- Install-time checks must not be described as a standalone authoring validator;
  explicitly state which command checks which layer.
- The system is still experimental according to public coverage evidence, so
  wording must not imply a stability promotion.

## Verification results

- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — validated 41 published docs pages.
- `(cd apps/backend && go test ./cmd/plugin-pack ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/plugins/pkgtar ./pkg/pluginsdk)` — 167 passed in 5 packages.
- `make -C apps/backend e2e-plugin-package` — produced `.build/kandev-plugin-e2e-1.0.0.tar.gz` for linux-amd64.
- `(cd apps && pnpm --filter @kandev/web test -- lib/plugins/host-api.test.ts lib/plugins/host.test.ts lib/plugins/registry.test.ts lib/ws/plugin-bridge.test.ts)` — 4 files and 50 tests passed. Happy-DOM emitted expected external-stylesheet warnings.
- `git diff --check` — passed.
