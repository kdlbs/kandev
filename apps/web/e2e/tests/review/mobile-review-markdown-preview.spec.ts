import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MARKDOWN_FILE = "mobile-review-preview.md";

test.describe("Review Markdown preview on mobile", () => {
  test.describe.configure({ timeout: 120_000 });

  test("renders changed Markdown in the mobile Review dialog", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    fs.writeFileSync(
      path.join(repoDir, MARKDOWN_FILE),
      "# Mobile review preview\n\nRendered content.",
    );

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Review Markdown Preview E2E",
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

    await testPage.getByRole("button", { name: "Changes" }).tap();
    const changesPanel = testPage.getByTestId("mobile-changes-panel");
    await expect(changesPanel).toBeVisible();
    await expect(
      testPage.getByTestId(`file-row-${MARKDOWN_FILE.replace(/[/\\]/g, "-")}`),
    ).toBeVisible({ timeout: 20_000 });
    await changesPanel.getByRole("button", { name: "Review", exact: true }).tap();

    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible();
    const header = dialog.locator(
      `[data-testid="review-file-header"][data-file-path="${MARKDOWN_FILE}"]`,
    );
    await header.getByRole("button", { name: `More actions for ${MARKDOWN_FILE}` }).tap();
    await testPage
      .getByTestId("review-file-actions-menu")
      .getByRole("menuitem", { name: "Preview markdown" })
      .tap();

    await expect(dialog).toBeVisible();
    const preview = dialog.getByTestId("review-markdown-diff-preview");
    await expect(preview).toBeVisible({ timeout: 15_000 });
    await expect(preview.locator("h1")).toHaveText("Mobile review preview");
    await header.getByRole("button", { name: `More actions for ${MARKDOWN_FILE}` }).tap();
    await expect(
      testPage.getByTestId("review-file-actions-menu").getByRole("menuitem", { name: "Show diff" }),
    ).toBeVisible();
    await prCapture.screenshot("rendered-markdown-preview", {
      caption: "Mobile rendered Markdown preview from a review diff",
    });
  });
});
