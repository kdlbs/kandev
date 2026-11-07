import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { visitOtherBrowserTab } from "../../helpers/external-focus";

// Full Chromium supports native foreground changes in headless mode.
test.use({ channel: "chromium" });

// @covers AC-UI-EDIT-FOCUS-001.6
test("phone Rename retains the draft and caret across a browser tab switch", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone focus", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  await testPage.goto(`/t/${task.id}`);
  await new SessionPage(testPage).waitForLoad();
  await testPage.getByTestId("mobile-task-picker-trigger").tap();
  const row = testPage
    .getByRole("dialog", { name: "Tasks" })
    .locator(`[data-task-row-id="${task.id}"]`);
  await row.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByRole("menuitem", { name: "Rename", exact: true }).tap();
  const dialog = testPage.getByRole("dialog", { name: "Rename task" });
  const input = dialog.getByRole("textbox");
  await input.fill("Release notes");
  await input.evaluate((el) => (el as HTMLInputElement).setSelectionRange(7, 7));
  await visitOtherBrowserTab(testPage, async () => {
    await expect(input).toHaveValue("Release notes");
    expect((await apiClient.getTask(task.id)).title).toBe("Phone focus");
  });
  await testPage.keyboard.type(" draft");
  await expect(input).toHaveValue("Release draft notes");
  await dialog.getByRole("button", { name: "Save", exact: true }).tap();
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).title)
    .toBe("Release draft notes");
});
