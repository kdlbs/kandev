import { test, expect } from "../../fixtures/test-base";

test.describe("Unread divider preference", () => {
  test("persists a disabled New divider from Task Actions", async ({
    testPage,
    apiClient,
    prCapture,
  }, testInfo) => {
    await apiClient.saveUserSettings({ unread_divider: true });
    await testPage.goto("/settings/preferences/task-behavior?tab=conversation");
    const toggle = testPage.getByRole("switch", { name: "Show New divider in transcripts" });

    await expect(toggle).toBeChecked();

    await toggle.scrollIntoViewIfNeeded();

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

  test("persists the unread task dot independently of the New divider", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.saveUserSettings({ unread_divider: false, unread_task_dot: false });
    await testPage.goto("/settings/preferences/task-behavior?tab=conversation");
    const divider = testPage.getByRole("switch", { name: "Show New divider in transcripts" });
    const dot = testPage.getByRole("switch", { name: "Show unread dot on tasks" });

    await expect(divider).not.toBeChecked();
    await expect(dot).not.toBeChecked();
    await dot.click();
    await expect(dot).toBeChecked();

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();

    await expect
      .poll(async () => (await apiClient.getUserSettings()).settings.unread_task_dot)
      .toBe(true);
    expect((await apiClient.getUserSettings()).settings.unread_divider).toBe(false);
  });
});
