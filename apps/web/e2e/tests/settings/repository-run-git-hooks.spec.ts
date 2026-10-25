import { test, expect } from "../../fixtures/test-base";
import { openRepositoryCard, saveSettings } from "./repository-run-git-hooks-helpers";

test.describe("Repository Run Git hooks", () => {
  test("is off by default, explains the trust boundary, and persists the opt-in", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    try {
      const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      await expect(testPage.getByTestId("repository-run-git-hooks")).toContainText(
        "run with Kandev's permissions",
      );

      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await saveSettings(testPage);
      expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(true);

      const reloaded = await openRepositoryCard(testPage, seedData.workspaceId);
      await expect(reloaded).toHaveAttribute("aria-checked", "true");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { run_git_hooks: false });
    }
  });
});
