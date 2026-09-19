import { test, expect } from "../../fixtures/test-base";
import {
  GitHelper,
  makeGitEnv,
  openTaskSession,
  createStandardProfile,
} from "../../helpers/git-helper";
import { expectTouchControl } from "../../helpers/control-sizing";
import path from "node:path";

async function openMobileChangesPanel(page: Parameters<typeof openTaskSession>[0]) {
  await page.getByRole("button", { name: "Changes" }).tap();
  await expect(page.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
}

async function expandSection(page: Parameters<typeof openTaskSession>[0], testId: string) {
  const toggle = page.getByTestId(`${testId}-collapse-toggle`);
  await expect(toggle).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(
      async () => {
        if ((await toggle.getAttribute("aria-expanded")) === "true") return true;
        await toggle.tap();
        return (await toggle.getAttribute("aria-expanded")) === "true";
      },
      { timeout: 15_000 },
    )
    .toBe(true);
}

test.describe("Mobile commit file navigation", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test.beforeEach(({ backend }) => {
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.exec("git reset --hard HEAD");
    git.exec("git clean -fd");
  });

  test("uses touch targets for commit expansion and opens a selected file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const profile = await createStandardProfile(apiClient, "Mobile Commit File Navigation Profile");
    await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Commit File Navigation",
      profile.id,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const session = await openTaskSession(testPage, "Mobile Commit File Navigation");
    await session.waitForChatIdle({ timeout: 30_000 });

    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    const filePath = "mobile/deeply/nested/commit-file-navigation-with-a-long-name.ts";
    const commitMessage = "Add mobile commit navigation with a deliberately long message";
    git.createFile(filePath, "export const mobile = true;\n");
    git.stageAll();
    const sha = git.commit(commitMessage);

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "commits-section");
    const row = testPage.getByTestId(`commit-row-${sha.slice(0, 7)}`);
    await expect(row).toBeVisible({ timeout: 20_000 });
    await expect(row.getByText(commitMessage)).toHaveAttribute("title", commitMessage);

    const toggle = row.getByTestId("commit-toggle");
    await expect(toggle).toHaveClass(/min-h-11/);
    await expectTouchControl(toggle);
    await toggle.tap();
    const inlineFile = row.getByTestId(`commit-file-${filePath.replaceAll("/", "-")}`);
    await expect(inlineFile).toBeVisible({
      timeout: 15_000,
    });
    await expect(inlineFile).toContainText("commit-file-navigation-with-a-long-name.ts");
    await expect(inlineFile).toContainText("mobile/deeply/nested");
    await expect(inlineFile).toHaveAttribute("aria-label", filePath);
    await expectTouchControl(inlineFile);
    await expectTouchControl(row.getByTestId(`commit-open-${sha.slice(0, 7)}`));
    await inlineFile.tap();
    const detail = testPage.getByTestId("commit-detail-content");
    await expect(detail).toBeVisible({ timeout: 15_000 });
    const indexToggle = detail.getByTestId("commit-file-index-toggle");
    await expectTouchControl(indexToggle);
    const indexEntry = detail.getByTestId("commit-file-index-entry");
    await expect(indexEntry).toHaveAttribute("aria-label", filePath);
    await expect(indexEntry).toContainText("commit-file-navigation-with-a-long-name.ts");
    await expect(indexEntry).toContainText("mobile/deeply/nested");
    await expectTouchControl(indexEntry);
    const fileHeader = detail.locator(`section[data-file-path="${filePath}"]`).getByRole("button", {
      name: `Collapse ${filePath}`,
    });
    await expectTouchControl(fileHeader);
    await expect(fileHeader).toBeFocused();
    const noDocumentOverflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    );
    expect(noDocumentOverflow).toBe(true);
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible();
  });
});
