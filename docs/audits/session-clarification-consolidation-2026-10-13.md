# Session and clarification E2E consolidation

Forty desktop cases in `session-tab-management.spec.ts` and
`clarification.spec.ts` repeat four setups whose assertions can run in existing
journeys. Keep all forty scenarios in thirty-six cases. Each removed case's
original title becomes a successfully executed `test.step` in its keeper.

| Disposition | Original case                                                         | Keeper and preserved evidence                                                                                                                                                                                                                                                                                                                  |
| ----------- | --------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Consolidate | `deleting the active session switches focus to the remaining session` | `deleting the active shared-environment session keeps Changes data visible`: retain the context-menu confirmation, absent modal dialog, removed/surviving tabs, same task URL and exact surviving backend session ID. The keeper already selects the same deletion target and checks local and PR Changes after deletion.                      |
| Consolidate | `keeps dirty Changes status after sibling hydration`                  | `does not restore removed Changes files after sibling hydration`: before removing the real file, reload and observe a received `session.git.event` for each sibling, then assert the file remains visible in each. Retain the separate post-removal capture reset, reload, both sibling events, environment-store checks and absent file rows. |
| Consolidate | `options render label and description on separate rows`               | `select option (happy path)`: check all three labels/descriptions and the original bounding-box inequality before clicking the real option and observing the completed answer.                                                                                                                                                                 |
| Consolidate | `emptying the custom draft clears the answer and reverts the stepper` | `typing in the custom input auto-selects it and deselects the picked option`: after the existing custom draft becomes the only selected answer, clear it and retain the unanswered step, zero answered count and inactive custom-input assertions.                                                                                             |

The production owners remain `use-session-actions.ts`, `session-tab.tsx`,
the environment-owned Git status path, `clarification-input-overlay.tsx`,
`clarification-overlay-parts.tsx` and `use-clarification-group.ts`.
The browser journeys still exercise their real bindings, backend sessions and
transport. Existing unit tests for answer clearing and Git status ordering
remain useful independent contracts; they do not replace these browser checks.

The session cases preserve the contracts in
[session delete feedback](../specs/ui/requirements/session-tab-delete-feedback.md)
and [environment-owned Git status](../specs/tasks/system-design/environment-owned-git-status.md).
The clarification request, race, failure and submit-feedback cases remain
separate, as do focused/global keyboard handlers, inactive-session deletion,
negative recreation dwell and mobile coverage. No product code, fixture,
timeout, retry configuration or support seam is changed.

## Verification

Run the two complete files sequentially in the same pinned CI runtime image,
with existing builds, one worker and `--retries=0 --reporter=blob,list`.
All baseline cases pass; all candidate cases pass. Neither run executes a
retry or skips a case. The existing suite-level retry configuration remains
unchanged. Blob comparison uses project, source file, full suite/title path
and repeat index, and verifies all four removed identities as executed steps.

| Local sample | Cases | Summed test seconds |
| ------------ | ----: | ------------------: |
| Baseline     |    40 |             378.013 |
| Candidate    |    36 |             336.768 |

This saves 41.245 summed seconds (10.91%) in one local sample. It is not a
prediction of full-suite wall time or evidence that the 50% hosted target is
met. Logs, exact baseline sources, blob reports and the identity comparison
are retained under ignored `e2e-focus/next-session-clarification-*`.
