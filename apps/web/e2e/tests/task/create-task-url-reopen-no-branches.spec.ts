import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";

// Regression test for the user-reported bug:
//
//   1. Open the create-task dialog, switch to GitHub URL mode, enter a URL.
//   2. Submit the task - the URL flow registers the repo in the workspace as
//      a remote (provider-backed) repository, without a local clone.
//   3. Reopen the dialog - the repo now appears in the workspace dropdown.
//      Pre-fix: picking it showed "no branches" with the
//      "No branches available for this repository." tooltip, because the
//      backend tried to list branches from an empty local_path and the
//      frontend cached the failure as a successful (empty) load.
//
// Fix: branch listing for a provider-backed repo now goes through the
// GitHub API (mocked in e2e) instead of the local clone. The local clone is
// only consulted when source_type=local. This test deliberately uses a fake
// owner/repo so the orchestrator's background `gh repo clone` fails - that
// keeps local_path empty, proving the remote-first path is what's serving
// the branches.
test.describe("Create-task URL flow - branches after reopen", () => {
  test.describe.configure({ retries: 1 });

  test("repo added via GitHub URL still lists branches when re-picked from the workspace dropdown", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // Fake owner/repo so the background clone in the orchestrator fails with
    // 404 and local_path stays empty. The bug only repros when local listing
    // can't help - that's the path the fix has to keep working.
    const owner = "kandev-e2e-no-such-owner";
    const repo = "kandev-e2e-no-such-repo";
    const repoFullName = `${owner}/${repo}`;
    const taskTitle = "URL repo reopen bug";

    await apiClient.mockGitHubAddBranches(owner, repo, [
      { name: "main" },
      { name: "develop" },
      { name: "feature/test" },
    ]);

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // ── First open: enter URL, submit task ──
    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("toggle-github-url").click();
    await testPage.getByTestId("github-url-input").fill(`https://github.com/${repoFullName}`);

    await testPage.getByTestId("task-title-input").fill(taskTitle);
    await testPage.getByTestId("task-description-input").fill("/e2e:simple-message");

    const startBtn = testPage.getByTestId("submit-start-agent");
    await expect(startBtn).toBeEnabled({ timeout: 15_000 });
    await startBtn.click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    // Wait for the workspace to surface the URL-added repo so the second
    // dialog open sees it in the dropdown.
    await expect
      .poll(
        async () => {
          const res = await apiClient.rawRequest(
            "GET",
            `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
          );
          const body = (await res.json()) as { repositories?: Array<{ name: string }> };
          return (body.repositories ?? []).some((r) => r.name === repoFullName);
        },
        { timeout: 15_000, intervals: [200, 500, 1000] },
      )
      .toBe(true);

    // ── Second open: pick the same repo from the workspace dropdown ──
    await kanban.createTaskButton.first().click();
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("repo-chip-trigger").first().click();
    await testPage
      .getByRole("option", { name: new RegExp(`^${repoFullName}\\b`) })
      .first()
      .click();

    // The branch chip is now populated from the mocked GitHub branches even
    // though local_path is empty. Asserting against the chip's current text
    // tolerates whichever default branch the autoselect heuristic picks
    // (main > master > develop); the dropdown contents are what proves the
    // full list arrived.
    const branchChip = testPage.getByTestId("branch-chip-trigger").first();
    await expect(branchChip).toBeEnabled({ timeout: 10_000 });
    await expect(branchChip).toContainText("main", { timeout: 10_000 });

    // Open the dropdown and verify every mocked branch is selectable.
    await branchChip.click();
    for (const name of ["main", "develop", "feature/test"]) {
      await expect(testPage.getByRole("option", { name: new RegExp(`^${name}$`) })).toBeVisible({
        timeout: 5_000,
      });
    }
  });
});
