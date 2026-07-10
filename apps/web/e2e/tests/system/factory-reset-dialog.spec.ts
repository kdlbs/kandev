import { test, expect } from "../../fixtures/test-base";

test.describe("Factory Reset dialog", () => {
  test("confirm button only enables on the literal 'RESET' token; cancel closes the modal", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/system/database");
    await expect(testPage.getByTestId("system-database-card")).toBeVisible();

    await testPage.getByTestId("system-factory-reset-button").click();
    const dialog = testPage.getByTestId("system-factory-reset-dialog");
    await expect(dialog).toBeVisible();

    const confirm = testPage.getByTestId("system-factory-reset-confirm");
    const input = testPage.getByTestId("system-factory-reset-input");

    // Empty → disabled.
    await expect(confirm).toBeDisabled();

    // Wrong token → still disabled.
    await input.fill("WRONG");
    await expect(confirm).toBeDisabled();

    // Correct token → enabled. DO NOT click — that would wipe state.
    await input.fill("RESET");
    await expect(confirm).toBeEnabled();

    // Cancel closes the modal.
    await testPage.getByTestId("system-factory-reset-cancel").click();
    await expect(dialog).not.toBeVisible();
  });
});
