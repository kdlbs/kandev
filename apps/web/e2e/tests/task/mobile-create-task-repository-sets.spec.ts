import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { useRegularMode } from "../../helpers/regular-mode";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

useRegularMode();

const SECOND_REPO_NAME = "Mobile Sets Target";
const SET_NAME = "Full-stack";

test.describe("Repository sets in the mobile task-create picker", () => {
  test("applying a set on a phone fills the picker with its members", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const dir = path.join(backend.tmpDir, "repos", "mobile-repository-sets");
    fs.mkdirSync(dir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: dir, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: dir, env: gitEnv });
    const second = await apiClient.createRepository(seedData.workspaceId, dir, "main", {
      name: SECOND_REPO_NAME,
    });
    await apiClient.createRepositorySet(seedData.workspaceId, SET_NAME, [
      seedData.repositoryId,
      second.id,
    ]);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await mobile.mobileFab.click();

    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();
    const repositoryChips = dialog.getByTestId("repo-chip-trigger");
    await expect(repositoryChips.first()).toBeVisible();

    // The menu renders as a safe-area-aware bottom sheet below 640px, which comes
    // from the shared DropdownMenu primitive rather than a separate mobile menu.
    await dialog.getByTestId("repository-sets-trigger").tap();
    const options = testPage.getByTestId("repository-set-option");
    await expect(options.first()).toBeVisible();
    await expect(options.first()).toContainText(SET_NAME);
    await options.first().tap();

    await expect(repositoryChips).toHaveCount(2);
    await expect(repositoryChips.nth(1)).toContainText(SECOND_REPO_NAME);

    if (prCapture.capturing) {
      // Let the bottom sheet finish dismissing so the asset shows the resulting
      // rows rather than a half-faded menu over them.
      await expect(options).toHaveCount(0);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "repository set applied on mobile");
    await prCapture.screenshot("mobile-repository-set-applied", {
      caption: "Applying a repository set on a phone fills the picker with both members.",
    });
  });
});
