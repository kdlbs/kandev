import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Coordinator grants", () => {
  test("creates, persists, and revokes a coordinator grant at a desktop viewport", async ({
    testPage,
    backend,
    seedData,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR_TASK_AUTHORITY: "true",
    });
    const coordinatorTask = await apiClient.seedTask(seedData.workspaceId, "QA Coordinator Task", {
      workflow_id: seedData.workflowId,
    });

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/coordinators`);
      await expect(testPage.getByTestId("coordinator-grants-page")).toBeVisible({
        timeout: 15_000,
      });

      await testPage.getByTestId("create-grant-button").click();
      const dialog = testPage.getByRole("dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("grant-task-id-input").fill(coordinatorTask.task_id);
      await testPage.getByTestId("grant-cap-inspect").check();

      const submit = testPage.getByTestId("grant-create-submit");
      await expect(submit).toBeVisible();
      const [dialogBox, submitBox] = await Promise.all([
        dialog.boundingBox(),
        submit.boundingBox(),
      ]);
      expect(dialogBox, "create-grant dialog has no rendered bounds").not.toBeNull();
      expect(submitBox, "create-grant submit has no rendered bounds").not.toBeNull();
      expect(submitBox!.y + submitBox!.height).toBeLessThanOrEqual(
        dialogBox!.y + dialogBox!.height,
      );
      await assertNoDocumentHorizontalOverflow(testPage, "desktop coordinator grant dialog");

      await testPage.addStyleTag({
        content: '[data-testid="toast-container"] { display: none !important; }',
      });
      await prCapture.screenshot("coordinator-grants-create-dialog", {
        caption: "Desktop coordinator grant creation dialog with visible submit action",
      });

      await submit.click();
      await expect(dialog).toBeHidden();

      const grantRow = testPage
        .locator('[data-testid^="grant-row-"]')
        .filter({ hasText: coordinatorTask.task_id });
      await expect(grantRow).toBeVisible();
      await testPage.reload();
      await expect(grantRow).toBeVisible();

      await grantRow.locator('[data-testid^="revoke-grant-"]').click();
      await testPage.getByTestId("revoke-grant-confirm").click();
      await expect(grantRow).toBeHidden();
      await testPage.reload();
      await expect(grantRow).toHaveCount(0);
    } finally {
      await releaseFeature();
    }
  });
});
