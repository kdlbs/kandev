import { test, expect } from "../../fixtures/test-base";

/**
 * Covers the Reset action on a GitHub review watch. The flow:
 *   1. Watch creates a task for a mock PR (via trigger).
 *   2. User clicks Reset on the settings page → confirmation dialog shows
 *      the preview count ("delete N task(s)").
 *   3. Confirm → backend cascade-deletes the task, wipes dedup, and
 *      immediately re-imports the same PR.
 *
 * The same controller wiring backs Jira / Linear / Sentry / GitHub-issue
 * watches (`watchreset.Run`), so this single spec exercises the shared
 * orchestration end-to-end. Per-integration store coverage lives in the
 * Go unit tests.
 */
test.describe("GitHub review watch reset", () => {
  test("preview + reset endpoints delete tasks and clear polling cursor", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 5101,
        title: "Reset me",
        state: "open",
        head_branch: "feature/reset-me",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "Reset Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #5101"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    // Preview reports the count the dialog will surface. workspace_id is
    // required — the reset endpoints reject cross-workspace IDs with 404.
    const wsQuery = `workspace_id=${seedData.workspaceId}`;
    const preview = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/watches/review/${watch.id}/reset/preview?${wsQuery}`,
    );
    expect(preview.status).toBe(200);
    expect(await preview.json()).toMatchObject({ taskCount: 1 });

    // Reset cascades the delete and returns the count it actually removed.
    const reset = await apiClient.rawRequest(
      "POST",
      `/api/v1/github/watches/review/${watch.id}/reset?${wsQuery}`,
    );
    expect(reset.status).toBe(200);
    expect(await reset.json()).toMatchObject({ tasksDeleted: 1 });

    // Reset immediately queues the matching PR again, so a replacement task
    // appears without waiting for a manual trigger or poll tick.
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.filter((t) => t.title.includes("PR #5101")).length;
        },
        { timeout: 15_000 },
      )
      .toBe(1);

    // Watch's polling cursor reflects the immediate re-check that reset
    // performed. last_polled_at sits on the watch row exposed by the list
    // endpoint.
    const listRes = await apiClient.rawRequest(
      "GET",
      `/api/v1/github/watches/review?workspace_id=${seedData.workspaceId}`,
    );
    expect(listRes.status).toBe(200);
    const list = (await listRes.json()) as {
      watches: Array<{ id: string; last_polled_at: string | null }>;
    };
    const refreshed = list.watches.find((w) => w.id === watch.id);
    expect(refreshed).toBeDefined();
    expect(refreshed!.last_polled_at).toBeTruthy();

    // The reset-triggered re-import already reserved the PR, so a manual
    // trigger has nothing new to create.
    const trigger = await apiClient.triggerReviewWatch(watch.id);
    expect(trigger.new_prs).toBe(0);
  });

  test("settings page reset flow shows preview count and deletes the task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    await apiClient.mockGitHubAddPRs([
      {
        number: 5201,
        title: "UI reset",
        state: "open",
        head_branch: "feature/ui-reset",
        base_branch: "main",
        author_login: "contributor",
        repo_owner: "testorg",
        repo_name: "testrepo",
        requested_reviewers: [{ login: "test-user", type: "User" }],
      },
    ]);

    const inboxStep = await apiClient.createWorkflowStep(seedData.workflowId, "UI Reset Inbox", 0);
    const watch = await apiClient.createReviewWatch(
      seedData.workspaceId,
      seedData.workflowId,
      inboxStep.id,
      seedData.agentProfileId,
      { repos: [{ owner: "testorg", name: "testrepo" }] },
    );

    await apiClient.triggerReviewWatch(watch.id);
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.find((t) => t.title.includes("PR #5201"));
        },
        { timeout: 15_000 },
      )
      .toBeTruthy();

    await testPage.goto("/settings/integrations/github");

    // The settings page aggregates every workspace's watches into a flat
    // table — find the row for our PR by the repo name it shows, then click
    // its Reset button. Scoping the testid lookup to the row ensures the
    // test fails loudly if a future selector change clones the reset button
    // onto another row.
    const watchRow = testPage.getByRole("row", { name: /testorg\/testrepo/i });
    const resetButton = watchRow.getByTestId("watch-reset-button");
    await expect(resetButton).toBeVisible({ timeout: 10_000 });
    await resetButton.click();

    // Dialog opens, preview loads, body shows "delete 1 task" wording.
    const dialog = testPage.getByTestId("reset-watch-dialog");
    await expect(dialog).toBeVisible();
    const description = testPage.getByTestId("reset-watch-dialog-description");
    await expect(description).toContainText(/delete 1 task/i);

    await testPage.getByTestId("reset-watch-dialog-confirm").click();

    // Dialog closes on success.
    await expect(dialog).toBeHidden({ timeout: 10_000 });

    // Success toast surfaces the deletion count.
    await expect(testPage.getByText(/deleted 1 task/i)).toBeVisible({ timeout: 10_000 });

    // The matching PR is recreated after the reset clears the old task.
    await expect
      .poll(
        async () => {
          const { tasks } = await apiClient.listTasks(seedData.workspaceId);
          return tasks.filter((t) => t.title.includes("PR #5201")).length;
        },
        { timeout: 15_000 },
      )
      .toBe(1);
  });
});
