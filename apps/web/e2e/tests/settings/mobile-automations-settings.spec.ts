import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

test.describe("Automation deletion confirmation on mobile", () => {
  test("keeps cancellation safe and confirms deletion from the editor", async ({
    testPage,
    seedData,
    apiClient,
    prCapture,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Mobile Delete Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await expect(testPage.getByTestId("automation-editor")).toBeVisible({ timeout: 15_000 });

    const deleteButton = testPage.getByTestId("automation-delete-button");
    await deleteButton.tap();

    const confirmation = testPage.getByTestId("automation-delete-confirm-dialog");
    await expect(confirmation).toBeVisible();
    await expect(confirmation).toContainText(
      "This will permanently delete Mobile Delete Automation. This action cannot be undone.",
    );
    await expect(testPage.getByRole("alertdialog")).toHaveCount(1);

    const viewport = testPage.viewportSize();
    const dialogBox = await confirmation.boundingBox();
    expect(viewport).not.toBeNull();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height);

    for (const label of ["Cancel", "Delete"]) {
      const action = confirmation.getByRole("button", { name: label, exact: true });
      await expect(action).toBeVisible();
      const box = await action.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile automation deletion confirmation");
    await prCapture.screenshot("automation-delete-confirmation-mobile", {
      caption: "Mobile automation deletion confirmation",
    });

    await confirmation.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(confirmation).not.toBeVisible();
    await expect(deleteButton).toBeVisible();

    await deleteButton.tap();
    await testPage
      .getByTestId("automation-delete-confirm-dialog")
      .getByTestId("automation-delete-confirm")
      .tap();

    await expect(testPage).toHaveURL(/\/settings\/workspaces\/[^/]+\/automations$/, {
      timeout: 15_000,
    });
    await expect(testPage.getByText("Mobile Delete Automation")).not.toBeVisible({
      timeout: 10_000,
    });
  });
});
