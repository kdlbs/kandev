import { test, expect } from "../../fixtures/test-base";
import { openPhoneList } from "./mobile-list-helpers";

test("keeps a rename dialog and its draft open while crossing the phone breakpoint", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Phone orientation target", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });

  await openPhoneList(testPage);
  const row = testPage
    .getByTestId("mobile-tasks-list-scroll")
    .getByTestId("sidebar-task-item")
    .filter({ hasText: task.title });
  await expect(row).toBeVisible();
  await row.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByRole("menuitem", { name: "Rename", exact: true }).tap();

  const dialog = testPage.getByRole("dialog", { name: "Rename task" });
  const input = dialog.getByRole("textbox");
  const draft = "Phone orientation draft";
  await expect(dialog).toBeVisible();
  await input.fill(draft);

  await testPage.setViewportSize({ width: 852, height: 393 });
  await expect(testPage.getByTestId("tasks-list-view")).toBeVisible();
  await expect(dialog).toBeVisible();
  await expect(input).toHaveValue(draft);

  await testPage.setViewportSize({ width: 393, height: 852 });
  await expect(testPage.getByTestId("sidebar-view-chip-row")).toBeVisible();
  await expect(dialog).toBeVisible();
  await expect(input).toHaveValue(draft);
});
