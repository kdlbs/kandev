import { test, expect } from "../../fixtures/test-base";

test.describe("Unread divider preference on mobile", () => {
  test("persists a disabled New divider from Task Actions", async ({
    testPage,
    apiClient,
    prCapture,
  }, testInfo) => {
    await testPage.goto("/settings/general/task-actions");
    const toggle = testPage.getByRole("switch", { name: "Show New divider in transcripts" });

    await expect(toggle).toBeChecked();

    await prCapture.screenshot(`unread-divider-preference-${testInfo.project.name}`, {
      caption: `Unread divider preference (${testInfo.project.name})`,
      fullPage: true,
    });
    await toggle.click();
    await expect(toggle).not.toBeChecked();
    expect((await apiClient.getUserSettings()).settings.unread_divider).toBe(true);

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.unread_divider)
      .toBe(false);
  });
});
