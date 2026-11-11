import fs from "node:fs";
import { expect } from "../../fixtures/test-base";
import {
  test,
  proveRejectedMoveChain,
  proveRollback,
  withRollbackFixture,
} from "../../helpers/cross-step-rollback";

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
  let profileId = "";
  const deleteExecutor = apiClient.deleteExecutor.bind(apiClient);
  apiClient.deleteExecutor = async (executorId) => {
    let orphaned = false;
    try {
      const profile = await apiClient.rawRequest(
        "GET",
        `/api/v1/executors/${executorId}/profiles/${profileId}`,
      );
      orphaned = profile.status === 200;
      expect(profile.status).toBe(404);
    } finally {
      await deleteExecutor(executorId);
      if (orphaned) await apiClient.deleteExecutorProfile(profileId);
    }
  };
  try {
    await expect(
      withRollbackFixture({ apiClient, backend, seedData }, async (owned) => {
        profileId = owned.profileId;
        root = owned.root;
        workspaceId = owned.workspaceId;
        throw new Error("Injected fixture callback failure");
      }),
    ).rejects.toThrow("Injected fixture callback failure");
  } finally {
    apiClient.deleteExecutor = deleteExecutor;
  }
  expect(fs.existsSync(root)).toBe(false);
  expect(
    (await apiClient.listWorkspaces()).workspaces.some((workspace) => workspace.id === workspaceId),
  ).toBe(false);
});

for (const newerFirst of [true, false]) {
  test(`two rejected desktop drag moves return to source: ${newerFirst ? "newer-first" : "older-first"}`, async ({
    page,
    apiClient,
    backend,
    seedData,
  }) => {
    await proveRejectedMoveChain({ page, apiClient, backend, seedData }, false, newerFirst);
  });
}
