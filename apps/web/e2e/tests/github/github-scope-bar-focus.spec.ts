import { test, expect } from "../../fixtures/test-base";

// @covers AC-INTEGRATIONS-GITHUB-MOBILE-001.1
test("desktop keyboard saves and cancels return focus to the saved-query trigger", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const response = await testPage.request.get("/api/v1/github/workspace-settings", {
    params: { workspace_id: seedData.workspaceId },
  });
  expect(response.ok()).toBe(true);
  const baseline = await response.json();
  try {
    await testPage.goto("/github");
    const trigger = testPage.getByTestId("github-saved-queries-menu");
    const dialog = testPage.getByRole("dialog", { name: "Save query", exact: true });
    for (const action of ["Cancel", "Save"]) {
      await trigger.focus();
      await trigger.press("Enter");
      const saveQuery = testPage.getByRole("menuitem", { name: "Save current query", exact: true });
      await expect(saveQuery).toBeEnabled();
      await testPage.keyboard.press("End");
      await expect(saveQuery).toBeFocused();
      await testPage.keyboard.press("Enter");
      await expect(dialog.getByLabel("Name")).toBeFocused();
      await dialog.getByLabel("Name").fill("Keyboard review queue");
      const button = dialog.getByRole("button", { name: action, exact: true });
      await button.focus();
      await button.press("Enter");
      await expect(dialog).toBeHidden();
      await expect(trigger).toBeFocused();
    }
    await expect(trigger).toContainText("Keyboard review queue");
  } finally {
    const restored = await testPage.request.put("/api/v1/github/workspace-settings", {
      data: { workspace_id: seedData.workspaceId, saved_presets: baseline.saved_presets ?? [] },
    });
    expect(restored.ok()).toBe(true);
  }
});
