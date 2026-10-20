import { expect, test } from "../../fixtures/test-base";

type UserSettingsResponse = {
  settings: { show_turn_changed_files?: boolean };
};

test("saves and reloads the turn changed-files preference", async ({
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
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
    const row = testPage.getByTestId("turn-changed-files-settings-row");
    const toggle = row.getByRole("switch", { name: "Show changed files after each turn" });
    await expect(toggle).toBeChecked();
    await prCapture.screenshot("turn-changed-files-setting-desktop", {
      caption: "Turn changed-files preference in desktop conversation settings",
    });

    await toggle.click();
    await expect(toggle).not.toBeChecked();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    const savedResponse = await apiClient.rawRequest("GET", "/api/v1/user/settings");
    expect(savedResponse.ok).toBe(true);
    expect(
      ((await savedResponse.json()) as UserSettingsResponse).settings.show_turn_changed_files,
    ).toBe(false);

    await testPage.reload();
    await testPage.getByRole("tab", { name: "Conversation", exact: true }).click();
    await expect(toggle).not.toBeChecked();
  } finally {
    const restore = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      show_turn_changed_files: originalValue,
    });
    expect(restore.ok).toBe(true);
  }
});
