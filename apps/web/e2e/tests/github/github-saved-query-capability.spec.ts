import { test, expect } from "../../fixtures/test-base";
import {
  disableRandomUuid,
  expectSavedQuery,
  readSavedQuerySettings,
  restoreSavedQueries,
} from "../../helpers/github-saved-query-capability";

// @covers AC-INTEGRATIONS-GITHUB-SAVED-QUERY-CAPABILITY-001.1
test("desktop saves a query when randomUUID is unavailable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const label = "HTTP desktop review";
  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("test-user");
  const baseline = await readSavedQuerySettings(testPage, seedData.workspaceId);
  try {
    await disableRandomUuid(testPage);
    await testPage.goto("/github");
    expect(
      await testPage.evaluate(() => [typeof crypto.randomUUID, typeof crypto.getRandomValues]),
    ).toEqual(["undefined", "function"]);
    const query = testPage.getByPlaceholder(/Custom query/);
    await query.fill("is:open");
    await query.press("Enter");
    const trigger = testPage.getByTestId("github-saved-queries-menu");
    await trigger.click();
    const entry = testPage.getByRole("menuitem", { name: "Save current query", exact: true });
    await expect(entry).toBeEnabled();
    await entry.click();
    const dialog = testPage.getByRole("dialog", { name: "Save query", exact: true });
    await dialog.getByLabel("Name").fill(label);
    const save = dialog.getByRole("button", { name: "Save", exact: true });
    await expect(save).toBeEnabled();
    await save.click();
    await expect(dialog).toBeHidden();
    await expect(trigger).toContainText(label);
    await expectSavedQuery(testPage, seedData.workspaceId, label);
    await testPage.reload();
    await trigger.click();
    await expect(testPage.getByRole("menuitem").filter({ hasText: label })).toHaveCount(1);
  } finally {
    await restoreSavedQueries(testPage, seedData.workspaceId, baseline.saved_presets);
  }
});
