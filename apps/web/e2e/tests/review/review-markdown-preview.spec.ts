import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MARKDOWN_FILE = "review-preview.md";

test.describe("Review Markdown preview", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens a changed Markdown file in rendered mode from the desktop Review dialog", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    fs.writeFileSync(path.join(repoDir, MARKDOWN_FILE), "# Review preview\n\nRendered content.");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Review Markdown Preview E2E",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();

    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.stageFile(MARKDOWN_FILE);

    await session.clickTab("Changes");
    await expect(session.changes.getByTestId(`file-row-${MARKDOWN_FILE}`)).toBeVisible({
      timeout: 20_000,
    });
    await session.changes.getByRole("button", { name: "Diff", exact: true }).click();
    await testPage.getByRole("button", { name: "Expand review" }).click();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${MARKDOWN_FILE}"]`,
    );
    await expect(header.getByRole("button", { name: "Preview markdown" })).toBeVisible();
    await header.getByRole("button", { name: "Preview markdown" }).click();

    await expect(dialog).not.toBeVisible();
    const preview = testPage.getByTestId("markdown-preview");
    await expect(preview).toBeVisible({ timeout: 15_000 });
    await expect(preview.locator("h1")).toHaveText("Review preview");
  });
});
