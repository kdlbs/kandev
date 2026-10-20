import { expect, test } from "../../fixtures/test-base";

type UserSettingsResponse = {
  settings: { show_turn_changed_files?: boolean };
};

test("saves the turn changed-files preference from the mobile conversation settings", async ({
  testPage,
  apiClient,
  prCapture,
}) => {
  const initialResponse = await apiClient.rawRequest("GET", "/api/v1/user/settings");
  expect(initialResponse.ok).toBe(true);
  const initial = ((await initialResponse.json()) as UserSettingsResponse).settings;
  const originalValue = initial.show_turn_changed_files ?? true;

  try {
    await testPage.goto("/settings/preferences/task-behavior");
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).tap();
    const row = testPage.getByTestId("turn-changed-files-settings-row");
    const toggle = row.getByRole("switch", { name: "Show changed files after each turn" });
    await expect(toggle).toBeChecked();
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    await prCapture.screenshot("turn-changed-files-setting-mobile", {
      caption: "Turn changed-files preference in mobile conversation settings",
    });

    await toggle.tap();
    await expect(toggle).not.toBeChecked();
    await expect(testPage.getByTestId("settings-floating-save")).toBeVisible();
    const beforeSave = await apiClient.rawRequest("GET", "/api/v1/user/settings");
    expect(
      ((await beforeSave.json()) as UserSettingsResponse).settings.show_turn_changed_files,
    ).toBe(originalValue);

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Reset" })
      .tap();
    await expect(toggle).toBeChecked();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    await toggle.tap();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .tap();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();
    await testPage.reload();
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).tap();
    await expect(toggle).not.toBeChecked();
  } finally {
    const restore = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      show_turn_changed_files: originalValue,
    });
    expect(restore.ok).toBe(true);
  }
});
