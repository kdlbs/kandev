import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createFolderTestRepository,
  findFolderTestWorktree,
  mockFolderAvailability,
} from "../../helpers/open-task-folder";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// @covers AC-TASKS-OPEN-FOLDER-001.2, AC-TASKS-OPEN-FOLDER-001.4
test("phone Files action opens the selected repository folder", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  const repository = await createFolderTestRepository(
    apiClient,
    seedData.workspaceId,
    backend.tmpDir,
  );
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Phone folder picker",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId, repository.id],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await mockFolderAvailability(testPage, true);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  if (!task.session_id) throw new Error("Task has no session");
  const worktreeId = await findFolderTestWorktree(
    apiClient,
    task.id,
    task.session_id,
    repository.id,
  );
  const payloads: unknown[] = [];
  await testPage.route("**/api/v1/task-sessions/*/open-folder", async (route) => {
    payloads.push(route.request().postDataJSON());
    await route.fulfill({ json: { success: true } });
  });
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  const trigger = testPage.getByTestId("files-workspace-actions");
  await trigger.tap();
  const action = testPage.getByRole("menuitem", { name: "Open workspace folder" });
  await action
    .locator("xpath=ancestor::*[@role='menu'][1]")
    .evaluate((element) =>
      Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      ),
    );
  expect((await action.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await action.tap();
  const picker = testPage.getByRole("dialog", { name: "Choose a folder" });
  await expect(picker).toBeVisible();
  await picker.evaluate((element) =>
    Promise.all(
      element
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished.catch(() => undefined)),
    ),
  );
  const choice = picker.getByRole("button", { name: /Folder second repo/ });
  expect((await choice.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const [choiceBox, pickerBox] = await Promise.all([choice.boundingBox(), picker.boundingBox()]);
  expect(choiceBox!.y).toBeGreaterThanOrEqual(pickerBox!.y);
  expect(choiceBox!.y + choiceBox!.height).toBeLessThanOrEqual(pickerBox!.y + pickerBox!.height);
  expect(pickerBox!.y + pickerBox!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
  await assertNoDocumentHorizontalOverflow(testPage, "folder picker");
  await testPage.screenshot({ path: test.info().outputPath("mobile-folder-picker.png") });
  await choice.tap();
  await expect.poll(() => payloads).toEqual([{ worktree_id: worktreeId }]);
  await expect(picker).not.toBeVisible();
  await expect(trigger).toBeFocused();
});

test("missing host folder opener disables the action before any picker opens", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Unavailable folder opener",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await mockFolderAvailability(testPage, false);
  let requests = 0;
  await testPage.route("**/api/v1/task-sessions/*/open-folder", async (route) => {
    requests++;
    await route.fulfill({ json: { success: true } });
  });
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  await testPage.getByTestId("files-workspace-actions").tap();
  await expect(testPage.getByRole("menuitem", { name: "Open workspace folder" })).toBeDisabled();
  await expect(testPage.getByRole("dialog", { name: "Choose a folder" })).not.toBeVisible();
  expect(requests).toBe(0);
});
