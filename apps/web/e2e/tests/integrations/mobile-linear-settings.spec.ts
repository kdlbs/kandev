import { test, expect } from "../../fixtures/test-base";
import { LinearSettingsPage } from "../../pages/linear-settings-page";

test.describe("Linear settings on mobile", () => {
  test("workspace-scoped route scopes the credentials form", async ({ testPage, apiClient }) => {
    const other = await apiClient.createWorkspace("Mobile Linear Workspace");

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await settings.secretInput.fill("lin_api_mobile");
    await settings.saveButton.click();
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();

    await settings.gotoWorkspace(other.id);

    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.saveButton).toBeDisabled();
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toHaveCount(0);
  });
});
