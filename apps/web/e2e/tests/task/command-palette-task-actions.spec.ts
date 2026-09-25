import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { ChangeWorkflowPage } from "../../pages/change-workflow-page";
import {
  seedMoveOverrideFixture,
  waitForMoveRequest,
  expectMoveInstructionsDelivered,
  MOVE_INSTRUCTIONS,
} from "../workflow/workflow-step-move-overrides-helpers";

async function chooseCommand(page: Page, label: string) {
  await page.keyboard.press("Control+k");
  const palette = page.getByRole("dialog").filter({ has: page.getByRole("combobox") });
  await palette.getByRole("combobox").fill(label);
  const command = palette
    .getByRole("option")
    .filter({ has: page.getByText(label, { exact: true }) });
  await expect(command).toHaveAttribute("aria-selected", "true");
  await page.keyboard.press("Enter");
}

// @covers AC-TASKS-KEYBOARD-ACTIONS-002.1, AC-TASKS-KEYBOARD-ACTIONS-002.2, AC-TASKS-KEYBOARD-ACTIONS-001.3
test("moves the current task from the keyboard and retains failed instructions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Keyboard palette move",
  );
  await chooseCommand(testPage, "Move to");
  const palette = testPage.getByRole("dialog").filter({ has: testPage.getByRole("combobox") });
  await palette.getByRole("combobox").fill("Verify");
  await expect(
    palette.getByRole("option").filter({ has: testPage.getByText("Verify", { exact: true }) }),
  ).toHaveAttribute("aria-selected", "true");
  await testPage.keyboard.press("Enter");
  const input = testPage.getByTestId("workflow-move-instructions");
  await expect(input).toBeFocused();
  await input.fill(MOVE_INSTRUCTIONS);
  await testPage.screenshot({ path: "test-results/desktop-task-command-move.png" });
  const endpoint = `**/api/v1/tasks/${fixture.taskId}/move`;
  await testPage.route(
    endpoint,
    (route) =>
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ error: "Move rejected for retry coverage" }),
      }),
    { times: 1 },
  );
  const failed = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/tasks/${fixture.taskId}/move`) && response.status() === 400,
  );
  await input.press("Control+Enter");
  await failed;
  await expect(input).toHaveValue(MOVE_INSTRUCTIONS);
  await expect(testPage.getByTestId("workflow-move-submit")).toBeEnabled();
  expect((await apiClient.getTask(fixture.taskId)).workflow_step_id).toBe(fixture.sourceStepId);
  const request = waitForMoveRequest(testPage, fixture.taskId);
  await input.press("Control+Enter");
  expect((await request).postDataJSON()).toMatchObject({
    workflow_step_id: fixture.targetStepId,
    entry_options: { instructions: MOVE_INSTRUCTIONS },
  });
  await expect
    .poll(async () => (await apiClient.getTask(fixture.taskId)).workflow_step_id)
    .toBe(fixture.targetStepId);
  await expectMoveInstructionsDelivered(fixture.session);
  const { messages } = await apiClient.listSessionMessages(fixture.primarySessionId);
  expect(
    messages.filter(
      (message) => message.author_type === "user" && message.content.includes(MOVE_INSTRUCTIONS),
    ),
  ).toHaveLength(1);
});

// @covers AC-TASKS-KEYBOARD-ACTIONS-002.3, AC-TASKS-KEYBOARD-ACTIONS-002.4
test("uses sidebar metadata actions and preserves destructive confirmation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Keyboard palette actions",
  );
  await chooseCommand(testPage, "Pin");
  await chooseCommand(testPage, "Unpin");
  await chooseCommand(testPage, "Rename");
  const rename = testPage.getByRole("dialog", { name: "Rename task" });
  await rename.getByRole("textbox").fill("Renamed by keyboard");
  await rename.getByRole("textbox").press("Enter");
  await expect
    .poll(async () => (await apiClient.getTask(fixture.taskId)).title)
    .toBe("Renamed by keyboard");
  await chooseCommand(testPage, "Priority");
  const palette = testPage.getByRole("dialog").filter({ has: testPage.getByRole("combobox") });
  await palette.getByRole("combobox").fill("High");
  await testPage.keyboard.press("Enter");
  await expect.poll(async () => (await apiClient.getTask(fixture.taskId)).priority).toBe("high");
  await chooseCommand(testPage, "Delete");
  const confirm = testPage.getByRole("alertdialog");
  await expect(confirm).toContainText("Renamed by keyboard");
  await confirm.getByRole("button", { name: "Cancel", exact: true }).click();
  expect((await apiClient.getTask(fixture.taskId)).title).toBe("Renamed by keyboard");
});

test("shows destination colors and moves immediately with the modified Enter shortcut", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Immediate keyboard move",
  );
  await chooseCommand(testPage, "Color");
  const red = testPage.getByRole("option", { name: "Red", exact: true });
  await expect(red.locator(".bg-red-500")).toBeVisible();
  await testPage.keyboard.press("Escape");
  await testPage.keyboard.press("Escape");
  await chooseCommand(testPage, "Move to");
  const target = testPage
    .getByRole("option")
    .filter({ has: testPage.getByText("Verify", { exact: true }) });
  await expect(target.locator(".rounded-full")).toBeVisible();
  await expect(target.locator(".tabler-icon-arrow-right")).toBeVisible();
  await testPage.getByRole("combobox").fill("Verify");
  const request = waitForMoveRequest(testPage, fixture.taskId);
  await testPage.keyboard.press("Control+Enter");
  expect((await request).postDataJSON()).toMatchObject({ workflow_step_id: fixture.targetStepId });
  await expect(testPage.getByTestId("workflow-move-instructions")).toHaveCount(0);
  await expect
    .poll(async () => (await apiClient.getTask(fixture.taskId)).workflow_step_id)
    .toBe(fixture.targetStepId);
});

test("opens the shared change workflow form from the command palette", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const fixture = await seedMoveOverrideFixture(
    testPage,
    apiClient,
    seedData,
    "Keyboard change workflow",
  );
  const destination = await apiClient.createWorkflow(seedData.workspaceId, "Keyboard destination");
  const destinationStep = await apiClient.createWorkflowStep(destination.id, "Incoming", 0);
  await testPage.reload();

  await chooseCommand(testPage, "Change workflow...");
  const form = new ChangeWorkflowPage(testPage);
  await expect(form.desktopDialog).toBeVisible();
  await form.chooseWorkflow(destination.id);
  await form.chooseStep(destinationStep.id);
  await form.form.getByTestId("change-workflow-cancel").click();

  expect((await apiClient.getTask(fixture.taskId)).workflow_id).toBe(fixture.workflowId);
  await expect(testPage).toHaveURL(new RegExp(`/t/${fixture.taskId}$`));
});
