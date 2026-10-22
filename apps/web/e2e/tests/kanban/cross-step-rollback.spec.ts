import fs from "node:fs";
import { expect } from "../../fixtures/test-base";
import { test, proveRollback, withRollbackFixture } from "../../helpers/cross-step-rollback";

// @covers AC-TASKS-KANBAN-TASK-REORDERING-001.13, AC-TASKS-KANBAN-TASK-REORDERING-001.28
for (const sameTarget of [false, true]) {
  test(`desktop drag preserves authoritative ${sameTarget ? "equal" : "different"} target after old HTTP failure`, async ({
    page,
    apiClient,
    backend,
    seedData,
  }) => {
    await proveRollback({ page, apiClient, backend, seedData }, false, sameTarget);
  });
}

test("disposable rollback fixture cleans up after a callback failure", async ({
  apiClient,
  backend,
  seedData,
}) => {
  let root = "";
  let workspaceId = "";
  await expect(
    withRollbackFixture({ apiClient, backend, seedData }, async (owned) => {
      root = owned.root;
      workspaceId = owned.workspaceId;
      throw new Error("Injected fixture callback failure");
    }),
  ).rejects.toThrow("Injected fixture callback failure");
  expect(fs.existsSync(root)).toBe(false);
  expect(
    (await apiClient.listWorkspaces()).workspaces.some((workspace) => workspace.id === workspaceId),
  ).toBe(false);
});
