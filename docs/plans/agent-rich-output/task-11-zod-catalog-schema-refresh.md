---
id: "11-zod-catalog-schema-refresh"
title: "Zod catalog schema refresh"
status: done
wave: 11
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.2
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Task 11: Zod catalog schema refresh

## Outcome

Keep the rich-output catalog's generated MCP schema aligned with the supported
Zod version when upgrading the direct web dependency. Preserve catalog
validation semantics and the locked workspace dependency graph.

## Scope

- Update the direct `apps/web` Zod pin and synchronized workspace lockfile.
- Regenerate the MCP rich-output catalog schema with that Zod version.
- Keep the catalog component and property contract unchanged.

## Acceptance

1. The web manifest and workspace lockfile resolve Zod 4.6.6.
2. The generated MCP catalog schema matches the output from the pinned Zod
   version and preserves the existing nullable-number property semantics.

## Verification

```sh
cd apps/web && pnpm exec tsx scripts/generate-rich-output-catalog.ts --check
```

## Results

- Updated the direct Zod pin from 4.3.6 to 4.6.6 and synchronized the workspace
  lockfile; refreshed the generated backend schema whose nullable-number
  representation changed from `anyOf` to the equivalent JSON Schema type list.
- `pnpm exec tsx scripts/generate-rich-output-catalog.ts --check`: passed with
  Zod 4.6.6 installed from the frozen workspace lockfile.
- `pnpm exec vitest run lib/rich-output scripts/generate-rich-output-catalog.test.ts`:
  8 files and 159 tests passed.
- `pnpm run typecheck`: the initial run exhausted Node's 4 GiB heap limit. One
  rerun with `NODE_OPTIONS` appended per invocation as
  `--max-old-space-size=6144` passed, including `rich-output:check` and
  `tsc --noEmit`.

## Current upstream integration

The normal merge of upstream `3a740c6911a340e73ade7edad6f4b234035f2fc9`
keeps the current `@kdlbs/plugin-sdk` workspace name alongside the Zod upgrade.
The only text conflict was in the lockfile importer; regenerating that lockfile
with the repository's pnpm 9.15.9 preserves the incoming dependency declarations
and resolves the compatible Zod consumers to 4.6.6.

- Frozen-lockfile installation passed.
- The generated catalog and guidance check passed without further artifact edits.
- The same focused selection passed 8 files and 161 tests on the merged source.
- The full typecheck passed with the existing process-local 6 GiB Node heap
  allowance. No system configuration or check threshold changed.
- The feature delta against that upstream commit remains the manifest, lockfile,
  generated schema and existing delivery documentation. No replacement PR,
  schema policy, UI behavior or operating change is introduced.

## Current upstream reconciliation: 922971904aca

The next normal merge preserves upstream
`922971904aca81254db41d753d1ed65be66d8d83`, including its new OpenCode plugin
workspace and dependency graph. The only text conflict is the web
`@json-render/react` peer resolution in the lockfile. It keeps the existing
Zod 4.6.6 pin; regeneration with repository pnpm 9.15.9 also deduplicates
compatible Zod consumers to that version. Incoming workspace declarations and
other dependency versions remain unchanged.

- Frozen-lockfile installation passed for all nine workspace projects.
- The existing generator check passed for both schema and guidance. The
  previously generated nullable-number schema remains current without another
  generated-file edit.
- The existing rich-output and generator selection passed 8 files and 161 tests
  in 11.20 seconds.
- Full typechecking passed with the same process-local 6 GiB Node heap allowance.
- The main-relative contribution remains the same six manifest, lockfile,
  generated-schema and documentation paths. No new tests, runtime behavior,
  operating settings or replacement PR are introduced.

The earlier upstream integration results retain their original revision scope.
Hosted checks and maintainer approval remain separate from these source checks.
