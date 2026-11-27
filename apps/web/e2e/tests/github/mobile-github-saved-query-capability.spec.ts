import { test, expect } from "../../fixtures/test-base";
import { MobileGitHubPage } from "../../pages/mobile-github-page";
import {
  disableRandomUuid,
  expectSavedQuery,
  readSavedQuerySettings,
  restoreSavedQueries,
} from "../../helpers/github-saved-query-capability";

// @covers AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.1
test("phone saves a query when randomUUID is unavailable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const label = "HTTP phone review";
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const baseline = await readSavedQuerySettings(testPage, seedData.workspaceId);
  try {
    await disableRandomUuid(testPage);
    const github = new MobileGitHubPage(testPage);
    await github.goto();
    expect(
      await testPage.evaluate(() => [typeof crypto.randomUUID, typeof crypto.getRandomValues]),
    ).toEqual(["undefined", "function"]);
    const query = testPage.getByPlaceholder(/Custom query/);
    await query.fill("is:open");
    await query.press("Enter");
    await github.mobileMenuButton.tap();
    const entry = github.mobileSidebar.getByRole("button", { name: "Save current query" });
    await expect(entry).toBeEnabled();
    await entry.tap();
    const dialog = testPage.getByRole("dialog", { name: "Save query", exact: true });
    await dialog.getByLabel("Name").fill(label);
    const save = dialog.getByRole("button", { name: "Save", exact: true });
    await expect(save).toBeEnabled();
    await save.tap();
    await expect(dialog).toBeHidden();
    await expect(github.mobileMenuButton).toContainText(label);
    await expectSavedQuery(testPage, seedData.workspaceId, label);
    await testPage.reload();
    await github.mobileMenuButton.tap();
    await expect(github.savedQueryByLabel(label)).toHaveCount(1);
  } finally {
    await restoreSavedQueries(testPage, seedData.workspaceId, baseline.saved_presets);
  }
});
