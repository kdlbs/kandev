import { execSync } from "node:child_process";
import path from "node:path";
import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import type { ApiClient } from "../../helpers/api-client";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";

async function waitForReviewTasksInBackend(
  apiClient: ApiClient,
  workspaceId: string,
  reviewStepId: string,
  titles: string[],
) {
  await expect
    .poll(
      async () => {
        const { tasks } = await apiClient.listTasks(workspaceId);
        return titles.filter((title) =>
          tasks.some((task) => task.title === title && task.workflow_step_id === reviewStepId),
        ).length;
      },
      {
        timeout: 60_000,
        intervals: [500, 1_000, 2_000],
        message: `Expected ${titles.length} PR tasks to exist in the Review step`,
      },
    )
    .toBe(titles.length);
}

async function waitForReviewTaskCards(
  testPage: Page,
  kanban: KanbanPage,
  reviewStepId: string,
  titles: string[],
) {
  async function visibleCardCount() {
    const visible = await Promise.all(
      titles.map((title) => kanban.taskCardInColumn(title, reviewStepId).isVisible()),
    );
    return visible.filter(Boolean).length;
  }

  const expected = titles.length;
  await expect
    .poll(visibleCardCount, { timeout: 10_000, intervals: [500, 1_000] })
    .toBe(expected)
    .catch(async () => {
      // The watcher creates tasks from a background poll. If task.created events
      // arrive before the kanban subscription is fully attached, backend state is
      // correct but one card can be absent until SSR rehydrates the board.
      await testPage.reload();
      await kanban.board.waitFor({ state: "visible", timeout: 15_000 });
    });

  await expect
    .poll(visibleCardCount, {
      timeout: 90_000,
      intervals: [500, 1_000, 2_000],
      message: `Expected all ${expected} PR task cards (${titles.join(", ")}) to appear in the Review column`,
    })
    .toBe(expected);
}

test.describe("PR watcher dockview layout stability", () => {
  /**
   * Verifies the dockview layout stays correct when switching between
   * PR tasks auto-created by the review watcher.
   *
   * Flow:
   *   1. PR watcher detects 3 PRs and auto-creates tasks in the background
   *   2. Open PR task 1 from the kanban homepage
   *   3. Toggle plan mode (adds plan panel)
   *   4. Switch to PR task 2 via sidebar → should have default layout
   *   5. Switch to PR task 3 via sidebar → should have default layout
   *
   * Setup:
   *   Review step with auto_start_agent on_enter — all 3 tasks get primary
   *   sessions immediately, so sidebar navigation takes the fast (synchronous) path.
   */
  test("layout remains correct when switching between PR watcher tasks with plan mode", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // The review watcher auto-starts a mock agent for each of the 3 PR tasks,
    // so 3 git checkouts + agent boots run concurrently up front. Under CI shard
    // contention that inherent workload, plus three subsequent session
    // navigations, can outlast the default budget — give the whole flow headroom.
    test.setTimeout(180_000);

    // --- Register the GitHub repo so the PR watcher can resolve it to a real
    // local path (the seed repo created in test-base.ts has no provider info,
    // and clone-from-real-GitHub fails for the mocked testorg/testrepo). ---
    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    await apiClient.createRepository(seedData.workspaceId, repoDir, "main", {
      name: "GitHub Test Repo",
      provider: "github",
      provider_owner: "testorg",
      provider_name: "testrepo",
    });

    // Create the PR head branches in the local seed repo so the executor's
    // git checkout for auto-started review tasks succeeds (in production these
    // branches would have been fetched during clone).
    const gitEnv = makeGitEnv(backend.tmpDir);
    for (const branch of ["fix/auth", "feat/dashboard", "docs/update"]) {
      execSync(`git branch -f ${branch} main`, { cwd: repoDir, env: gitEnv });
    }

    // --- Seed workflow ---
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "PR Watcher Layout Workflow",
    );

    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 0);

    // Configure auto-start so the review watcher immediately launches mock agents
    // for all 3 PR tasks. By the time the test reaches the sidebar navigation
    // steps, tasks 2 and 3 already have a primarySessionId in Query snapshots
    // so handleSelectTask takes the fast (synchronous) path instead of the slow
    // HTTP + WS round-trip that times out in CI.
    await apiClient.updateWorkflowStep(reviewStep.id, {
      events: { on_enter: [{ type: "auto_start_agent" }] },
    });

    await apiClient.saveUserSettings({
      workspace_id: seedData.workspaceId,
      workflow_filter_id: workflow.id,
      enable_preview_on_click: false,
    });

    // --- Seed mock GitHub data ---
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // Add 3 PRs with requested_reviewers so the review watcher picks them up
    await apiClient.mockGitHubAddPRs([
      {
        number: 101,
        title: "Fix auth bug",
        state: "open",
        head_branch: "fix/auth",
        base_branch: "main",
        author_login: "alice",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 10,
        deletions: 2,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
      {
        number: 202,
        title: "Add dashboard",
        state: "open",
        head_branch: "feat/dashboard",
        base_branch: "main",
        author_login: "bob",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 80,
        deletions: 5,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
      {
        number: 303,
        title: "Update docs",
        state: "open",
        head_branch: "docs/update",
        base_branch: "main",
        author_login: "charlie",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 30,
        deletions: 10,
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    // Navigate to kanban BEFORE creating the review watch so the WS is
    // subscribed when task.created/task.updated events fire.
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    // --- Create review watch (triggers initial poll → auto-creates 3 tasks) ---
    await apiClient.createReviewWatch(
      seedData.workspaceId,
      workflow.id,
      reviewStep.id,
      seedData.agentProfileId,
      {
        repos: [{ owner: "testorg", name: "testrepo" }],
        prompt: "Review {{pr.link}}",
      },
    );

    // --- Wait for all 3 PR tasks to appear in the Review column ---
    const prTask1Title = "PR #101: Fix auth bug";
    const prTask2Title = "PR #202: Add dashboard";
    const prTask3Title = "PR #303: Update docs";

    // Poll for all 3 cards together in one shared budget rather than three
    // independent per-card waits. The cards are created concurrently and render
    // out of order under load; waiting for the whole set at once avoids
    // cumulative per-card timeout pressure and settles as soon as all 3 appear.
    const prTaskTitles = [prTask1Title, prTask2Title, prTask3Title];
    await waitForReviewTasksInBackend(apiClient, seedData.workspaceId, reviewStep.id, prTaskTitles);
    await waitForReviewTaskCards(testPage, kanban, reviewStep.id, prTaskTitles);

    // --- Click PR task 1 to enter session view ---
    await kanban.taskCardInColumn(prTask1Title, reviewStep.id).click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Verify initial default layout for task 1
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
    await expect(session.sidebar).toBeVisible();

    // Wait for the mock agent to complete and the layout to be stable before toggling
    // plan mode. Without this, an in-flight layout restore could swallow the panel add.
    // Use `waitForChatIdle` (vs. raw `idleInput().waitFor`) so the helper's
    // reload-and-retry recovery covers the rare case where the WS-driven idle
    // signal misses its window under shard pressure.
    await session.waitForChatIdle({ timeout: 30_000 });

    // --- Toggle plan mode on task 1 ---
    await session.togglePlanMode();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
    await expect(session.chat).toBeVisible();
    await expect(session.sidebar).toBeVisible();

    // --- Switch to PR task 2 via sidebar ---
    await session.clickTaskInSidebar(prTask2Title);
    // With auto_start_agent configured, task 2 already has a primarySessionId in
    // Query snapshots, so handleSelectTask takes the synchronous fast path and
    // setActiveSession is called immediately.
    await expect(
      testPage
        .getByRole("navigation", { name: "breadcrumb" })
        .getByText(prTask2Title, { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    // Task 2: full layout intact — sidebar, chat accessible, changes panel visible,
    // plan panel must NOT leak from task 1, and layout fills the viewport
    await expect(session.sidebar).toBeVisible({ timeout: 15_000 });
    await session.expectNoLayoutGap();
    await expect(session.planPanel).not.toBeVisible({ timeout: 10_000 });
    // Changes panel verifies the right column rendered
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    // Chat tab may be renamed (e.g. "#1 Mock • Mock Default") — use stable testid
    await session.clickSessionChatTab();
    await expect(session.chat).toBeVisible({ timeout: 10_000 });

    // --- Switch to PR task 3 via sidebar ---
    await session.clickTaskInSidebar(prTask3Title);
    // Same fast-path navigation as task 2
    await expect(
      testPage
        .getByRole("navigation", { name: "breadcrumb" })
        .getByText(prTask3Title, { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    // Task 3: same layout checks
    await expect(session.sidebar).toBeVisible({ timeout: 15_000 });
    await session.expectNoLayoutGap();
    await expect(session.planPanel).not.toBeVisible({ timeout: 10_000 });
    await session.clickTab("Changes");
    await expect(session.changes).toBeVisible({ timeout: 10_000 });
    // Chat tab may be renamed (e.g. "#1 Mock • Mock Default") — use stable testid
    await session.clickSessionChatTab();
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
  });
});
