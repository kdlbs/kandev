/**
 * Mobile parity for the sidebar subtask tree.
 *
 * The mobile task-switcher sheet renders through the same `TaskSwitcher` /
 * `applyView` path as the desktop sidebar, so nested subtasks must show up in
 * the sheet on a mobile viewport. This guards the shared recursive renderer
 * against a mobile-only regression.
 *
 * Lives in `mobile-*.spec.ts` so the `mobile-chrome` Playwright project applies
 * the mobile device automatically.
 */
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile sidebar — nested subtasks", () => {
  test("multi-level subtasks render nested in the mobile task switcher sheet", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Build a 3-level chain via the harness (createTask caps kanban depth at 1).
    const stepOpts = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const root = await apiClient.seedTask(seedData.workspaceId, "Mobile Root Task", stepOpts);
    const child = await apiClient.seedTask(seedData.workspaceId, "Mobile Child Task", {
      ...stepOpts,
      parent_id: root.task_id,
    });
    await apiClient.seedTask(seedData.workspaceId, "Mobile Grandchild Task", {
      ...stepOpts,
      parent_id: child.task_id,
    });

    await testPage.goto(`/t/${root.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Open the task switcher sheet from the mobile session top bar. Scope all
    // assertions to the sheet — task titles also appear in the session header.
    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");

    // All three levels are listed in the sheet.
    await expect(sheet.getByText("Mobile Root Task")).toBeVisible({ timeout: 10_000 });
    await expect(sheet.getByText("Mobile Child Task")).toBeVisible({ timeout: 10_000 });
    await expect(sheet.getByText("Mobile Grandchild Task")).toBeVisible({ timeout: 10_000 });

    // The grandchild (depth 2) is DOM-nested inside the root block — mobile
    // renders the full tree, not a flat list.
    const grandchildBlock = sheet.locator(
      `[data-testid='sortable-task-block'][data-task-id='${root.task_id}'] [data-testid='sortable-task-block'][data-depth='2']`,
    );
    await expect(grandchildBlock).toHaveCount(1);
    await expect(grandchildBlock.getByText("Mobile Grandchild Task")).toBeVisible();
  });
});
