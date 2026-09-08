import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Coordinator grants", () => {
  test("renders the create-grant form at a desktop viewport", async ({
    testPage,
    backend,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_COORDINATOR_TASK_AUTHORITY: "true",
    });

    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/coordinators`);
      await expect(testPage.getByTestId("coordinator-grants-page")).toBeVisible({
        timeout: 15_000,
      });

      await testPage.getByTestId("create-grant-button").click();
      const dialog = testPage.getByRole("dialog");
      await expect(dialog).toBeVisible();

      await testPage
        .getByTestId("grant-task-id-input")
        .fill("00000000-0000-4000-8000-000000000001");
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
    } finally {
      await releaseFeature();
    }
  });
});
