# Architecture linter roadmap

[Roadmap](README.md) · Inventory at main `359b5ffdbb6`, 2026-09-27.

The [linter guide](../architecture-lint.md) defines enforcement and rule contribution requirements.
The [deprecation design](../specs/architecture-lint/system-design/deprecation-ledger.md) defines declaration registration.
This page records priorities, not new enforced rules.

## Existing protections

| Rule                          | Dated baseline count | Maintenance target                                                               |
| ----------------------------- | -------------------: | -------------------------------------------------------------------------------- |
| ARCH-RUNTIME-IMPORT           |                   61 | Reduce direct implementation imports after the facade supports real caller needs |
| ARCH-TASK-OFFICE-IMPORT       |                    7 | Reduce reverse dependencies without moving Office policy into Task               |
| ARCH-FRONTEND-ROOT-STATE-CAST |                   46 | Type one slice at a time                                                         |
| ARCH-RUN-SCHEDULER-OWNER      |                    0 | Preserve the single composition owner                                            |
| ARCH-RUNS-OFFICE-IMPORT       |                    6 | Resolve policy ownership before removing remaining edges                         |
| ARCH-FRONTEND-STATE-UI-IMPORT |                    0 | Keep state below components and routes                                           |
| ARCH-INBOX-HISTORY-ISOLATION  |                    0 | Preserve separation from operational pending actions                             |
| ARCH-DEPRECATION-LEDGER       |                   15 | Remove or account for legacy unregistered declarations                           |

These counts are distinct rule findings, not comparable units of complexity.
Zero baselines remain active protections. They are not candidates for deletion.

## Proposed additions

| ID      | Candidate                                                                 | Value                                                       | Gate before implementation                                                                                 |
| ------- | ------------------------------------------------------------------------- | ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| LINT-01 | Existing TanStack ESLint checks for stable clients and query dependencies | Catch common library misuse without a custom scanner        | Inspect the installed Query version, plugin compatibility, and actual violations. Select only useful rules |
| LINT-02 | Prevent dual state ownership for migrated resources                       | Stop old Zustand fields/actions returning after a migration | Define exact resource scope after the second migration. Do not ban all Zustand server state                |
| LINT-03 | Keep Runs model contracts below service and Office layers                 | Protect the extracted low-level package                     | Review allowed dependencies and fixtures before defining an import rule                                    |
| LINT-04 | Contract or event checks for one vertical slice                           | Detect cross-language or routing drift                      | First select the slice and executable contract. No repository-wide generator by default                    |

No rule in this table is approved for implementation by this tracking PR.
The next rule must protect an accepted invariant and provide an actionable replacement.
Prefer existing tooling when it covers the invariant reliably.
Avoid blanket bans on fetch, WebSocket requests, UI stores, or compatibility terms.

## Rule acceptance evidence

- A supported syntax inventory and intentional exclusions.
- Positive cases, negative cases, and misleading comment/string fixtures.
- Stable identities across harmless formatting and movement.
- Failure for missing, stale, or ambiguous registrations where applicable.
- An exact reviewed baseline and a shrink-only comparison against main.
- A useful diagnostic and focused regression tests.
- A measured runtime comparison, especially for whole-tree scans.

Parser complexity is a cost. Repeated lexical bugs require a tooling/design review before another scanner extension.
An import or syntax scanner cannot prove semantic ownership or replace integration tests.

## Monthly checks

Run from the repository root:

```bash
make lint-architecture
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main
```

Refresh `origin/main` before the comparison.
Read current counts directly from the JSON files:

```bash
node -e 'const fs = require("node:fs"); const dir = "config/architecture-lint"; for (const file of fs.readdirSync(dir).filter(f => f.endsWith(".json")).sort()) { const data = JSON.parse(fs.readFileSync(`${dir}/${file}`, "utf8")); console.log(`${data.rule ?? "COMPATIBILITY-LEDGER"}: ${data.entries.length}`); }'
```

Record the commit, results, elapsed time, false-positive reports, and upcoming ledger targets in the milestone review.
Do not overwrite a historical count without changing its inventory date and source commit.
