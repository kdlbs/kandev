import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import {
  seedMoveOverrideFixture,
  waitForMoveRequest,
  MOVE_INSTRUCTIONS,
} from "../workflow/workflow-step-move-overrides-helpers";

// @covers AC-TASKS-KEYBOARD-ACTIONS-002.5
test("uses nested task commands and the move drawer on a phone", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Mobile palette actions",
  );
  await expect(testPage.getByTestId("mobile-task-picker-trigger")).toBeVisible();
  await testPage.keyboard.press("Control+k");
  const palette = testPage.getByRole("dialog", { name: "Command Palette", exact: true });
  await expect(palette).toBeVisible();
  const search = palette.getByRole("combobox");
  await search.fill("Move to");
  const move = palette
    .getByRole("option")
    .filter({ has: testPage.getByText("Move to", { exact: true }) });
  await move.tap();
  const back = palette.getByRole("button", { name: "Back", exact: true });
  await back.tap();
  await expect(search).toHaveValue("Move to");
  await move.tap();
  const target = palette
    .getByRole("option")
    .filter({ has: testPage.getByText("Verify", { exact: true }) });
  expect((await target.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await target.tap();
  await expect(palette).toBeHidden();
  const drawer = testPage.locator('[data-slot="drawer-content"]:visible');
  await expect(drawer).toBeVisible();
  await testPage.getByTestId("workflow-move-instructions").fill(MOVE_INSTRUCTIONS);
  const submit = testPage.getByTestId("workflow-move-submit");
  await submit.scrollIntoViewIfNeeded();
  expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: "test-results/mobile-task-command-move.png" });
  const request = waitForMoveRequest(testPage, fixture.taskId);
  await submit.tap();
  expect((await request).postDataJSON()).toMatchObject({
    entry_options: { instructions: MOVE_INSTRUCTIONS },
  });
  await expect
    .poll(async () => (await apiClient.getTask(fixture.taskId)).workflow_step_id)
    .toBe(fixture.targetStepId);
  await expect(drawer).toBeHidden();
  await testPage.keyboard.press("Control+k");
  await expect(palette).toBeVisible();
  const archiveSearch = palette.getByRole("combobox");
  await archiveSearch.fill("Archive task");
  const archiveCommand = palette.getByRole("option", { name: "Archive task", exact: true });
  await expect(archiveCommand).toBeVisible();
  await archiveCommand.scrollIntoViewIfNeeded();
  const archiveReceivesCenterTap = await archiveCommand.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    const target = document.elementFromPoint(
      rect.left + rect.width / 2,
      rect.top + rect.height / 2,
    );
    return target === element || (target instanceof Node && element.contains(target));
  });
  expect(archiveReceivesCenterTap).toBe(true);
  await archiveCommand.tap();
  const confirm = testPage.getByRole("dialog", { name: "Archive task?", exact: true });
  await expect(confirm).toBeVisible();
  await expect(palette).toBeHidden({ timeout: 10_000 });
  await expect(testPage.getByRole("combobox")).toHaveCount(0);
  const cancel = confirm.getByRole("button", { name: "Cancel", exact: true });
  expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage);
  await testPage.screenshot({ path: "test-results/mobile-task-command-archive.png" });
  await cancel.tap();
});

test("opens the shared change workflow form from the phone command palette", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 360, height: 780 });
  const [task, destination] = await Promise.all([
    apiClient.createTask(seedData.workspaceId, "Phone palette change workflow", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    }),
    (async () => {
      const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Phone destination");
      const step = await apiClient.createWorkflowStep(workflow.id, "Incoming", 0);
      return { workflow, step };
    })(),
  ]);
  await testPage.goto(`/t/${task.id}`);

  await expect(testPage.getByTestId("mobile-task-picker-trigger")).toBeVisible();
  await testPage.keyboard.press("Control+k");
  const palette = testPage.getByRole("dialog").filter({ has: testPage.getByRole("combobox") });
  const search = palette.getByRole("combobox");
  await search.fill("Change workflow...");
  const command = palette
    .getByRole("option")
    .filter({ has: testPage.getByText("Change workflow...", { exact: true }) });
  await expect(command).toBeVisible();
  expect((await command.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await command.tap();

  const form = new ChangeWorkflowPage(testPage, true);
  await expect(form.phoneDrawer).toBeVisible();
  await form.chooseWorkflow(destination.workflow.id);
  await form.chooseStep(destination.step.id);
  const submit = form.form.getByTestId("change-workflow-submit");
  await submit.scrollIntoViewIfNeeded();
  expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage, "phone command palette change workflow");
  await form.form.getByTestId("change-workflow-cancel").tap();

  expect((await apiClient.getTask(task.id)).workflow_id).toBe(seedData.workflowId);
});
