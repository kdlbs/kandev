import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import type { Page } from "@playwright/test";
import path from "node:path";

async function openMobileChangesPanel(testPage: Page) {
  await testPage.getByRole("button", { name: "Changes" }).tap();
  await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
}

async function expandSection(testPage: Page, sectionTestId: string) {
  const toggle = testPage.getByTestId(`${sectionTestId}-collapse-toggle`);
  await expect(toggle).toBeVisible({ timeout: 10_000 });
  if ((await toggle.getAttribute("aria-expanded")) === "false") {
    await toggle.tap();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
  }
}

async function expectDiffText(testPage: Page, text: string, timeout = 45_000) {
  await testPage.waitForFunction(
    (searchText: string) => {
      for (const container of document.querySelectorAll("diffs-container")) {
        const shadow = container.shadowRoot;
        if (shadow?.textContent?.includes(searchText)) return true;
      }
      return false;
    },
    text,
    { timeout },
  );
}

test.describe("Mobile changes panel", () => {
  test.describe.configure({ retries: 1, timeout: 120_000 });

  test("renders timeline surface and opens Diff/Review/file/commit overlays", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Changes Surface",
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
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-committed.txt", "base commit marker");
    git.stageAll();
    git.commit("mobile commit seed");
    git.createFile("mobile-unstaged.txt", "UNSTAGED_MARKER");
    git.createFile("mobile-staged.txt", "STAGED_MARKER");
    git.stageFile("mobile-staged.txt");

    await openMobileChangesPanel(testPage);

    // Regression: mobile Changes tab should be timeline summary, not inline merged diff.
    await expect(testPage.locator("diffs-container")).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-changes-panel").getByRole("tab")).toHaveCount(0);
    await expect(testPage.getByTestId("unstaged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("staged-files-section")).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");

    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    const committedTab = testPage.getByRole("tab", { name: /^Committed/i });
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).toBeVisible();
    await expect(committedTab).toBeVisible();
    await committedTab.tap();
    await expect(committedTab).toHaveAttribute("aria-selected", "true");
    await closeButton.tap();

    await mobileChangesPanel.getByRole("button", { name: "Review" }).tap();
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await testPage.keyboard.press("Escape");
    await expect(reviewDialog).not.toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("file-row-mobile-unstaged.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "UNSTAGED_MARKER");

    // Regression: mobile diffs must use word-wrap (overflow="wrap") so long
    // lines are readable without horizontal scroll on touch devices.
    const overflowAttr = await testPage.waitForFunction(
      () =>
        document
          .querySelector("diffs-container")
          ?.shadowRoot?.querySelector("[data-diffs]")
          ?.getAttribute("data-overflow"),
      { timeout: 10_000 },
    );
    expect(await overflowAttr.jsonValue()).toBe("wrap");

    await closeButton.tap();

    await expandSection(testPage, "commits-section");
    await testPage.locator("[data-testid^='commit-row-']").first().tap();
    await expect(testPage.getByText("Commit Changes")).toBeVisible({ timeout: 10_000 });
    await closeButton.tap();
  });

  test("selecting a diff from Files panel switches to Changes and opens file sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Files to Changes",
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
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("from-files-panel.txt", "FILES_PANEL_MARKER");

    await testPage.getByRole("button", { name: "Files" }).tap();
    const filesPanel = testPage.getByRole("tabpanel", { name: /Diff files/i });
    await expect(filesPanel).toBeVisible({ timeout: 10_000 });
    await expect(filesPanel.getByText("from-files-panel.txt")).toBeVisible({ timeout: 20_000 });
    await filesPanel.getByText("from-files-panel.txt").first().tap();

    // Regression: selecting a file diff in Files should deep-link into mobile Changes diff sheet.
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("mobile-diff-sheet-close")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "FILES_PANEL_MARKER");
  });

  test("tapping a staged file row opens file diff sheet", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Staged File Diff",
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
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("mobile-staged-only.txt", "STAGED_ONLY_MARKER");
    git.stageFile("mobile-staged-only.txt");

    await openMobileChangesPanel(testPage);
    await expandSection(testPage, "staged-files-section");

    await testPage.getByTestId("file-row-mobile-staged-only.txt").tap();
    await expect(testPage.getByText("File Changes")).toBeVisible({ timeout: 10_000 });
    await expectDiffText(testPage, "STAGED_ONLY_MARKER");
    await testPage.getByTestId("mobile-diff-sheet-close").tap();
  });

  test("Diff sheet auto-selects Committed tab when no uncommitted changes exist", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Auto-Select Source",
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
    await expect(session.idleInput()).toBeVisible({ timeout: 45_000 });

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    git.createFile("committed-only.txt", "committed content");
    git.stageAll();
    git.commit("committed-only seed");

    await openMobileChangesPanel(testPage);
    await expect(testPage.getByTestId("commits-section")).toBeVisible({ timeout: 20_000 });

    const mobileChangesPanel = testPage.getByTestId("mobile-changes-panel");
    await mobileChangesPanel.getByRole("button", { name: "Diff" }).tap();
    const closeButton = testPage.getByTestId("mobile-diff-sheet-close");
    await expect(closeButton).toBeVisible({ timeout: 10_000 });

    // Single source → no tab bar rendered; title shows source name instead.
    await expect(testPage.getByRole("tab", { name: /^Uncommitted/i })).not.toBeVisible();
    await expect(testPage.getByRole("tab", { name: /^Committed/i })).not.toBeVisible();
    await expect(testPage.getByText("Committed Changes")).toBeVisible({ timeout: 10_000 });

    await closeButton.tap();
  });
});
