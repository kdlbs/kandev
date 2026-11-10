import { test, proveRejectedMoveChain, proveRollback } from "../../helpers/cross-step-rollback";

// @covers AC-TASKS-KANBAN-TASK-REORDERING-001.13, AC-TASKS-KANBAN-TASK-REORDERING-001.28
for (const sameTarget of [false, true]) {
  test(`phone card-menu taps preserve authoritative ${sameTarget ? "equal" : "different"} target after old HTTP failure`, async ({
    page,
    apiClient,
    backend,
    seedData,
  }) => {
    await proveRollback({ page, apiClient, backend, seedData }, true, sameTarget);
  });
}

for (const newerFirst of [true, false]) {
  test(`two rejected phone tap moves return to source: ${newerFirst ? "newer-first" : "older-first"}`, async ({
    page,
    apiClient,
    backend,
    seedData,
  }) => {
    await proveRejectedMoveChain({ page, apiClient, backend, seedData }, true, newerFirst);
  });
}
