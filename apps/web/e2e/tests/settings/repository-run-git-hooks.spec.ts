import { test, expect } from "../../fixtures/test-base";
import {
  enableManagedGitIsolation,
  failRepositoryUpdates,
  holdRepositoryUpdate,
  openRepositoryCard,
  saveButton,
  saveSettings,
} from "./repository-run-git-hooks-helpers";

let releaseIsolation: (() => Promise<void>) | undefined;

test.beforeAll(async ({ backend }) => {
  releaseIsolation = await enableManagedGitIsolation(backend);
});

test.afterAll(async () => {
  await releaseIsolation?.();
});

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

  test("a failed save shows the stored value and keeps other unsaved edits", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    try {
      const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
      const stopFailing = await failRepositoryUpdates(testPage);
      const setup = testPage.getByPlaceholder(/#!\/bin\/bash/).first();
      await setup.fill("echo unsaved-edit");
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "true");
      await saveButton(testPage).click();
      await expect(testPage.getByTestId("settings-floating-save")).toHaveAttribute(
        "data-status",
        "error",
      );
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      await expect(setup).toHaveValue("echo unsaved-edit");
      expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(false);

      await stopFailing();
      await toggle.click();
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
