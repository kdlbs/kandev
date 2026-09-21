import { expect, test } from "../../fixtures/test-base";
import {
  createFolderTestRepository,
  findFolderTestWorktree,
  mockFolderAvailability,
} from "../../helpers/open-task-folder";
import { SessionPage } from "../../pages/session-page";

// @covers AC-TASKS-OPEN-FOLDER-001.1, AC-TASKS-OPEN-FOLDER-001.3
test("task tools open the folder beside the IDE action and recover from errors", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Open task folder",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await mockFolderAvailability(testPage, true);
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  let attempts = 0;
  const requests: string[] = [];
  await testPage.route("**/api/v1/task-sessions/*/open-folder", async (route) => {
    requests.push(route.request().url());
    attempts++;
    await route.fulfill({
      status: attempts === 1 ? 500 : 200,
      json: attempts === 1 ? { error: "failed to open folder" } : { success: true },
    });
  });
  const folder = testPage.getByTestId("open-task-folder");
  await expect(folder).toBeVisible();
  await expect(folder).toHaveAccessibleName("Open folder");
  const editorBox = await testPage.getByTestId("editors-menu-list").boundingBox();
  const folderBox = await folder.boundingBox();
  expect(folderBox!.height).toBeCloseTo(28, 0);
  expect(folderBox!.x).toBeGreaterThanOrEqual(editorBox!.x + editorBox!.width);
  expect(folderBox!.x - editorBox!.x - editorBox!.width).toBeLessThan(20);
  await testPage.screenshot({ path: test.info().outputPath("desktop-folder-shortcut.png") });
  await folder.click();
  await expect(testPage.getByText("Failed to open folder", { exact: true })).toBeVisible();
  await folder.click();
  await expect.poll(() => attempts).toBe(2);
  expect(
    requests.every((url) => url.endsWith(`/task-sessions/${task.session_id}/open-folder`)),
  ).toBe(true);
  await expect(folder).toBeEnabled();
});

// @covers AC-TASKS-OPEN-FOLDER-001.2
test("desktop folder picker opens only the selected repository", async ({
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
    "Desktop folder picker",
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
  const trigger = testPage.getByTestId("open-task-folder");
  await trigger.focus();
  await testPage.keyboard.press("Enter");
  const picker = testPage.getByRole("dialog", { name: "Choose a folder" });
  await expect(picker).toBeVisible();
  await testPage.screenshot({ path: test.info().outputPath("desktop-folder-picker.png") });
  await picker.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(picker).not.toBeVisible();
  await expect(trigger).toBeFocused();
  expect(payloads).toEqual([]);
  await trigger.click();
  await picker.getByRole("button", { name: /Folder second repo/ }).click();
  await expect.poll(() => payloads).toEqual([{ worktree_id: worktreeId }]);
  await expect(picker).not.toBeVisible();
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
  await expect(testPage.getByTestId("open-task-folder")).toBeDisabled();
  await expect(testPage.getByRole("dialog", { name: "Choose a folder" })).not.toBeVisible();
  expect(requests).toBe(0);
});
