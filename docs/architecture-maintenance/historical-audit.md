# Historical audit disposition

[Roadmap](README.md) · Original assessment date: 2026-09-27; disposition refreshed 2026-10-09.

The original workspace contains 15 untracked Markdown files under `docs/architecture-review/2026-07-30/`.
They total 2,622 lines. This PR leaves them unchanged and excludes them from its published files.
Other workspaces do not have those files. No current work order depends on them.

The audit records useful hypotheses, but its measurements and priorities describe an earlier checkout.
The Query assessment explicitly reviewed August commits. It is not a review of the merged pilot.
This assessment compares its proposals with current source and merged delivery records, not a complete repeat of the original audit.

## File disposition

| Historical file                              | Disposition                                         | Current destination or prerequisite                                                                    |
| -------------------------------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| `README.md`                                  | Historical overview, not current architecture       | This roadmap provides current delivery status. Re-measure old source counts before reuse               |
| `PRIORITIZATION.md`                          | Superseded priority order                           | The next finite milestone in this roadmap replaces its immediate sequence                              |
| `01-transport-role-clarity.md`               | Reassess                                            | Inventory one duplicated HTTP/WS operation and its clients before retiring a transport                 |
| `02-executable-contract-catalog.md`          | Reassess                                            | Prove one cross-language contract slice before choosing generation tooling                             |
| `03-deepen-agent-runtime.md`                 | Partly protected, still open                        | Runtime import rule exists. Remaining findings need caller-level investigation                         |
| `04-unify-task-read-model.md`                | Needs fresh domain design                           | Preserve intentional Task/Office differences until their semantics are agreed                          |
| `05-collapse-run-scheduling.md`              | Partly superseded                                   | Scheduler ownership and shared models now have explicit boundaries. Office policy remains Office-owned |
| `06-typed-domain-events.md`                  | Reassess                                            | Select one event family, including auth routing and gateway projection                                 |
| `07-frontend-reconciliation.md`              | Partly explored                                     | SystemInfo, database statistics, backups, and the disk-usage snapshot now have separate Query owners; the System job stream remains in Zustand. Other resources need individual inventory. |
| `08-typed-domain-store-composition.md`       | Implementation complete, delivery pending           | Features, Azure DevOps, Jira, and Linear are typed. The Jira/Linear increment is under [PR #4375](https://github.com/kdlbs/kandev/pull/4375); wait for merge evidence before closing this audit item. |
| `09-modular-backend-composition.md`          | Reassess                                            | Find a current construction-order problem before introducing module bundles                            |
| `10-code-host-capability-seam.md`            | Reassess                                            | Inventory current plugin/integration contracts and real provider differences                           |
| `11-frontend-feature-locality.md`            | Reassess                                            | State-to-UI imports are resolved. Broad folder moves remain unapproved                                 |
| `12-architecture-fitness-and-deprecation.md` | Foundation implemented, old absence claims obsolete | Architecture rules and the compatibility ledger now exist. TanStack Query rules and the four-owner guard are tracked in the linter roadmap. |
| `13-tanstack-query-pr-1512-assessment.md`    | Historical PR assessment, pilot proposal superseded | Use the merged SystemInfo design and the resource migration tracker                                    |

The earlier proposal to move all run processing into Runs is not the current migration instruction.
The shared-contract extraction deliberately retained Office launch, continuation, refusal, and gate policy.
The earlier Query mega-PR is not the vehicle for future migrations.

## Architecture overview refresh

The outdated overview identified in the original audit was refreshed in
[PR #4011](https://github.com/kdlbs/kandev/pull/4011), merge commit
`d2b66efd7e48fc518a9d62131d6d1392397b43db`. This closes that candidate; it
does not authorize another broad overview rewrite without new source evidence.

## Retention decision

The July files are not required for the new tracker or any next work item.
They remain local historical material, not a second maintained source of truth.
Nothing was deleted. Publishing an archive is a separate choice, not part of this PR.
New work must use repository-relative links to documents available on main, not absolute paths in this workspace.
