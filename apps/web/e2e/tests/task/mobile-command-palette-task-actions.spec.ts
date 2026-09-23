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
  const palette = testPage.getByRole("dialog").filter({ has: testPage.getByRole("combobox") });
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
  await testPage.keyboard.press("Control+k");
  await testPage.getByRole("combobox").fill("Archive task");
  await testPage
    .getByRole("option")
    .filter({ has: testPage.getByText("Archive task", { exact: true }) })
    .tap();
  const confirm = testPage.getByRole("dialog", { name: "Archive task?", exact: true });
  await expect(confirm).toBeVisible();
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
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Phone palette change workflow",
  );
  const destination = await apiClient.createWorkflow(seedData.workspaceId, "Phone destination");
  const destinationStep = await apiClient.createWorkflowStep(destination.id, "Incoming", 0);
  await testPage.reload();

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
  await form.chooseWorkflow(destination.id);
  await form.chooseStep(destinationStep.id);
  const submit = form.form.getByTestId("change-workflow-submit");
  await submit.scrollIntoViewIfNeeded();
  expect((await submit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(testPage, "phone command palette change workflow");
  await form.form.getByTestId("change-workflow-cancel").tap();

  expect((await apiClient.getTask(fixture.taskId)).workflow_id).toBe(fixture.workflowId);
});
