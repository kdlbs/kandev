import { test, expect } from "../../fixtures/test-base";
import { expectTouchControl } from "../../helpers/control-sizing";
import { openRepositoryCard, saveSettings } from "./repository-run-git-hooks-helpers";

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
