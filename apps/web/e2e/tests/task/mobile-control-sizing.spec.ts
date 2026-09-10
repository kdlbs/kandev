import { expect, test } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { expectTouchControl } from "../../helpers/control-sizing";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";
import { SessionPage } from "../../pages/session-page";

test("completed-session New Agent remains a touch target on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile control sizing completed session task",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");
  await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    sessionId: task.session_id,
    agentProfileId: seedData.agentProfileId,
    completedAt: new Date().toISOString(),
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expectTouchControl(session.completedSessionNewAgentButton());
});

test("keeps a combobox search field and its wrapper touch-sized on mobile", async ({
  testPage,
}) => {
  await testPage.setViewportSize({ width: 390, height: 844 });
  const mobile = new MobileKanbanPage(testPage);
  await mobile.goto();
  await mobile.mobileFab.tap();

  const dialog = testPage.getByTestId("create-task-dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByTestId("agent-profile-selector").tap();

  const wrapper = testPage.locator('[data-slot="command-input-wrapper"]:visible').last();
  const search = wrapper.locator('[data-slot="command-input"]');
  const searchGroup = wrapper.locator('[data-slot="input-group"]');
  await expect(search).toBeVisible();
  await expect(searchGroup).toBeVisible();

  for (const control of [search, searchGroup]) {
    await expect
      .poll(async () => (await control.boundingBox())?.height ?? 0, { timeout: 5_000 })
      .toBeGreaterThanOrEqual(44);
  }

  const [inputBox, groupBox] = await Promise.all([search.boundingBox(), searchGroup.boundingBox()]);
  expect(inputBox).not.toBeNull();
  expect(groupBox).not.toBeNull();
  expect(inputBox!.y).toBeGreaterThanOrEqual(groupBox!.y);
  expect(inputBox!.y + inputBox!.height).toBeLessThanOrEqual(groupBox!.y + groupBox!.height);
});
