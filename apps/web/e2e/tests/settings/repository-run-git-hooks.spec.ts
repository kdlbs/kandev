import { test, expect } from "../../fixtures/test-base";
import {
  failRepositoryUpdates,
  holdRepositoryUpdate,
  openRepositoryCard,
  saveButton,
  saveSettings,
} from "./repository-run-git-hooks-helpers";

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

  test("a failed save keeps the opt-in unsaved and does not persist it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    try {
      const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
      const stopFailing = await failRepositoryUpdates(testPage);
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await saveButton(testPage).click();
      await expect(testPage.getByTestId("settings-floating-save")).toHaveAttribute(
        "data-status",
        "error",
      );
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(false);

      await stopFailing();
      await saveSettings(testPage);
      expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(true);
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { run_git_hooks: false });
    }
  });

  test("an edit made while a save is in flight stays unsaved", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    try {
      const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
      const { held, release } = await holdRepositoryUpdate(testPage);
      await toggle.click();
      await saveButton(testPage).click();
      await held;
      await toggle.click();
      release();

      await expect
        .poll(async () => (await apiClient.getRepository(seedData.repositoryId)).run_git_hooks)
        .toBe(true);
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      await expect(saveButton(testPage)).toBeEnabled();

      await saveSettings(testPage);
      expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(false);
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { run_git_hooks: false });
    }
  });
});
