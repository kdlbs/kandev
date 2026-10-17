# Architecture linter roadmap

[Roadmap](README.md) · Historical inventory at main `359b5ffdbb6`, 2026-09-27.
Current root-state-cast measurement: 45 entries at main
`3fed5570cec533f468c25ed03c84967bdc972588`, 2026-10-09.

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
The 46 root-state-cast findings above remain the dated 2026-09-27 count. The
current measurement is 45; other counts in this table have not been remeasured.

## Implemented protections

| ID | Protection | Merged evidence |
| --- | --- | --- |
| LINT-01 | Official TanStack Query rules: `@tanstack/query/exhaustive-deps` (error), `@tanstack/query/no-rest-destructuring` (warn), `@tanstack/query/no-unstable-deps` (error), and `@tanstack/query/stable-query-client` (error). | [#4012](https://github.com/kdlbs/kandev/pull/4012), `a1e2edadb9cd40a08d23a0f9b72665146ec3fea5` |
| LINT-02 | Guard the four migrated System Query snapshots against direct Zustand mirrors ([design](../specs/architecture-lint/system-design/migrated-system-query-owner.md)). | [#4357](https://github.com/kdlbs/kandev/pull/4357), `9ad5964ca0a6bdce1e32b462450f7ba78f9cf2b8` |

LINT-02 covers About SystemInfo, database statistics, the backup list, and the
disk-usage snapshot. It does not move the System job stream or other resources
to Query.

## Proposed additions

| ID | Candidate | Value | Gate before implementation |
| --- | --- | --- | --- |
| LINT-03 | Keep Runs model contracts below service and Office layers | Protect the extracted low-level package | Review allowed dependencies and fixtures before defining an import rule |
| LINT-04 | Contract or event checks for one vertical slice | Detect cross-language or routing drift | First select the slice and executable contract. No repository-wide generator by default |

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
