import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { withIsolatedRenameFixture } from "./sidebar-task-rename-fixture";
import { SessionPage } from "../../pages/session-page";
import { checkRenameOwnership } from "./sidebar-task-rename-ownership-helpers";

// @covers AC-UI-EDIT-FOCUS-001.7
for (const scenario of [
  { entry: "sidebar", sameTask: false },
  { entry: "sidebar", sameTask: true },
  { entry: "sidebar", sameTask: false, failure: true },
  { entry: "commands", sameTask: true },
] as const) {
  test(`${scenario.entry} preserves ${scenario.sameTask ? "reopened" : "different-task"} Rename after old ${"failure" in scenario ? "failure" : "success"}`, async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    await checkRenameOwnership({ testPage, apiClient, seedData, backend }, testInfo, scenario);
  });
}

test("cleans owned Rename resources after a failed flow without touching the worker seed", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const failure = new Error("Intentional Rename flow failure");
  let ownedRoot = "";
  let ownedWorkspace = "";
  await expect(
    withIsolatedRenameFixture({ testPage, apiClient, seedData, backend }, async (owned) => {
      expect(owned.workspaceId).not.toBe(seedData.workspaceId);
      expect(owned.repositoryPath).not.toBe(seedData.repositoryPath);
      ownedRoot = path.dirname(owned.repositoryPath);
      ownedWorkspace = owned.workspaceId;
      const task = await apiClient.createTask(owned.workspaceId, "Cleanup Rename task", {
        workflow_id: owned.workflowId,
        workflow_step_id: owned.startStepId,
        repository_ids: [owned.repositoryId],
        agent_profile_id: owned.agentProfileId,
        executor_profile_id: owned.worktreeExecutorProfileId,
      });
      await testPage.goto(`/t/${task.id}`);
      await new SessionPage(testPage).waitForLoad();
      throw failure;
    }),
  ).rejects.toBe(failure);
  expect(fs.existsSync(ownedRoot)).toBe(false);
  expect(
    (await apiClient.listWorkspaces()).workspaces.some(
      (workspace) => workspace.id === ownedWorkspace,
    ),
  ).toBe(false);
});
