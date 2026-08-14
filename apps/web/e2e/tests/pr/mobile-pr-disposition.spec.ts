// Filename starts with "mobile-" so the mobile-chrome project (Pixel 5
// emulation) exercises the touch path. The desktop topbar PRTopbarButton and
// the chat-bar PRStatusChip are both unreachable here (see
// mobile-pr-ci-chip.spec.ts) — a single closed-unmerged PR is the exact
// scenario where the chip hides itself entirely (pr-status-chip.tsx only
// shows for a task with at least one OPEN PR). The bottom-nav "Review" tab
// (session-mobile-layout.tsx -> MobileReviewPanel) is the surface these
// specs exercise instead: it stays reachable for any linked PR regardless of
// state and renders the same PRDetailContent the desktop PR detail panel
// does, which is where the disposition control now also lives.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const OWNER = "acme";
const REPO = "demo";
const PR_NUMBER = 601;

test.describe("mobile PR disposition control", () => {
  test("records superseded with a URL via the bottom-nav Review tab", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR Disposition",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: OWNER,
      repo: REPO,
      pr_number: PR_NUMBER,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/${PR_NUMBER}`,
      pr_title: "Mobile disposition closed unmerged",
      head_branch: "feat/mobile-disposition",
      base_branch: "main",
      author_login: "test-user",
      state: "closed",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    // A single closed-unmerged PR: the compact chat-bar chip is unreachable
    // by design (pr-status-chip.tsx hides for zero open PRs) and the desktop
    // topbar button never mounts on this viewport — the bottom-nav Review
    // tab is the only path in.
    await expect(session.prTopbarButton()).toHaveCount(0);
    await expect(session.prStatusChip()).toHaveCount(0);

    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    const panel = testPage.getByTestId("mobile-review-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });

    const dispositionRow = panel.getByTestId("pr-disposition-row");
    await expect(dispositionRow).toBeVisible({ timeout: 10_000 });

    const select = dispositionRow.getByTestId("pr-disposition-select");
    const selectBox = await select.boundingBox();
    expect(selectBox, "disposition select has no bounding box").not.toBeNull();
    if (selectBox) expect(selectBox.height).toBeGreaterThanOrEqual(44);

    await select.tap();
    await testPage.getByRole("listbox").getByRole("option", { name: "Superseded" }).tap();

    const urlInput = dispositionRow.getByTestId("pr-disposition-superseded-url");
    await expect(urlInput).toBeVisible();
    const urlInputBox = await urlInput.boundingBox();
    expect(urlInputBox, "disposition URL input has no bounding box").not.toBeNull();
    if (urlInputBox) expect(urlInputBox.height).toBeGreaterThanOrEqual(44);
    await urlInput.fill(`https://github.com/${OWNER}/${REPO}/pull/602`);

    const saveButton = dispositionRow.getByTestId("pr-disposition-save");
    const saveBox = await saveButton.boundingBox();
    expect(saveBox, "disposition save button has no bounding box").not.toBeNull();
    if (saveBox) expect(saveBox.height).toBeGreaterThanOrEqual(44);
    await saveButton.tap();

    await expect(dispositionRow.getByTestId("pr-disposition-select")).toContainText("Superseded", {
      timeout: 10_000,
    });

    await testPage.reload();
    await session.waitForLoad();
    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    const panelAfterReload = testPage.getByTestId("mobile-review-panel");
    await expect(panelAfterReload).toBeVisible({ timeout: 15_000 });
    await expect(
      panelAfterReload.getByTestId("pr-disposition-row").getByTestId("pr-disposition-select"),
    ).toContainText("Superseded", { timeout: 10_000 });
  });

  test("offers no disposition control for an open PR on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile PR Disposition Open",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      owner: OWNER,
      repo: REPO,
      pr_number: 603,
      pr_url: `https://github.com/${OWNER}/${REPO}/pull/603`,
      pr_title: "Mobile disposition open PR",
      head_branch: "feat/mobile-disposition-open",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await testPage.getByRole("button", { name: "Review", exact: true }).tap();
    const panel = testPage.getByTestId("mobile-review-panel");
    await expect(panel).toBeVisible({ timeout: 15_000 });
    await expect(panel.getByTestId("pr-disposition-row")).toHaveCount(0);
  });
});
