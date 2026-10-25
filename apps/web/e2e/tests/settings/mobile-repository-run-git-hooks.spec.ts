import { test, expect } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import {
  failRepositoryUpdates,
  holdRepositoryUpdate,
  openRepositoryCard,
  saveButton,
  saveSettings,
} from "./repository-run-git-hooks-helpers";

test("Run Git hooks switch is reachable and persists on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  try {
    const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
    await expect(toggle).toHaveAttribute("aria-checked", "false");
    await expectTouchControl(testPage.getByTestId("repository-run-git-hooks").locator("label"));

    await toggle.click();
    await saveSettings(testPage);
    expect((await apiClient.getRepository(seedData.repositoryId)).run_git_hooks).toBe(true);
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, { run_git_hooks: false });
  }
});

test("Run Git hooks: a failed save keeps the opt-in unsaved on mobile", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  try {
    const toggle = await openRepositoryCard(testPage, seedData.workspaceId);
    const stopFailing = await failRepositoryUpdates(testPage);
    await toggle.click();
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

test("Run Git hooks: an edit made while a save is in flight stays unsaved on mobile", async ({
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
