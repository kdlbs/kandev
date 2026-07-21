import { test, expect } from "../../fixtures/test-base";
import { GITLAB_PROJECT, seedGitLabReview } from "../../helpers/gitlab";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { GitLabSettingsPage } from "../../pages/gitlab-settings-page";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("GitLab watch controls", () => {
  test("review watch dispatches exactly once and supports pause plus reset", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.configureGitLab(seedData.workspaceId);
    const watch = await apiClient.createGitLabReviewWatch({
      workspace_id: seedData.workspaceId,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      projects: [{ path: GITLAB_PROJECT }],
    });
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/gitlab/watches/review?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        );
        const body = (await response.json()) as {
          watches: Array<{ id: string; last_polled_at?: string }>;
        };
        return body.watches.find((item) => item.id === watch.id)?.last_polled_at ?? null;
      })
      .not.toBeNull();
    await seedGitLabReview(apiClient, seedData.workspaceId, 91, "Watch exact-once MR");

    const settings = new GitLabSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = settings.reviewWatches;
    const table = card.getByTestId("gitlab-watch-desktop-table");
    await expect(table.getByText(GITLAB_PROJECT, { exact: true })).toBeVisible();
    await assertLocatorWithinViewportX(card, "desktop review watches");
    await assertNoDocumentHorizontalOverflow(testPage, "desktop review watch settings");

    await table.getByRole("button", { name: "Check now" }).click();
    await expect(testPage.getByText(/Found 1 matching merge request/)).toBeVisible();
    const taskTitle = `[${GITLAB_PROJECT}!91] Watch exact-once MR`;
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1, {
      timeout: 20_000,
    });

    await settings.goto(seedData.workspaceId);
    await table.getByRole("button", { name: "Check now" }).click();
    await expect(
      testPage.getByText("No new merge requests matched", { exact: true }),
    ).toBeVisible();
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1);

    await settings.goto(seedData.workspaceId);
    await table.getByRole("button", { name: "Pause watch" }).click();
    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /save changes/i });
    await expect(save).toBeEnabled();
    await save.click();
    await expect(table.getByText("Paused", { exact: true })).toBeVisible();
    await expect(table.getByRole("button", { name: "Check now" })).toBeDisabled();

    await table.getByRole("button", { name: "Enable watch" }).click();
    await save.click();
    await table.getByRole("button", { name: "Reset watch" }).click();
    const reset = testPage.getByTestId("reset-watch-dialog");
    await expect(reset).toBeVisible();
    await expect(testPage.getByTestId("reset-watch-dialog-description")).toContainText(
      /delete 1 task/i,
    );
    await testPage.getByTestId("reset-watch-dialog-confirm").click();
    await expect(testPage.getByText(/Review watch reset; 1 task\(s\) deleted/)).toBeVisible();
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1, {
      timeout: 20_000,
    });
  });

  test("issue watch creates one visible task for a repeated match", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.configureGitLab(seedData.workspaceId);
    const now = new Date().toISOString();
    const watch = await apiClient.createGitLabIssueWatch({
      workspace_id: seedData.workspaceId,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      projects: [{ path: GITLAB_PROJECT }],
    });
    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/gitlab/watches/issue?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
        );
        const body = (await response.json()) as {
          watches: Array<{ id: string; last_polled_at?: string }>;
        };
        return body.watches.find((item) => item.id === watch.id)?.last_polled_at ?? null;
      })
      .not.toBeNull();
    await apiClient.mockGitLabAddIssues(seedData.workspaceId, GITLAB_PROJECT, [
      {
        id: 10_092,
        iid: 92,
        project_id: 101,
        title: "Watch exact-once issue",
        body: "Issue body",
        url: `https://gitlab.com/${GITLAB_PROJECT}/-/issues/92`,
        web_url: `https://gitlab.com/${GITLAB_PROJECT}/-/issues/92`,
        state: "opened",
        author_username: "reporter",
        project_namespace: "platform",
        project_path: GITLAB_PROJECT,
        labels: ["bug"],
        assignees: [],
        created_at: now,
        updated_at: now,
      },
    ]);

    const settings = new GitLabSettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = settings.issueWatches;
    const table = card.getByTestId("gitlab-watch-desktop-table");
    await assertLocatorWithinViewportX(card, "desktop issue watches");
    await table.getByRole("button", { name: "Check now" }).click();
    await expect(testPage.getByText(/Found 1 matching issue/)).toBeVisible();
    const taskTitle = `[${GITLAB_PROJECT}#92] Watch exact-once issue`;
    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1, {
      timeout: 20_000,
    });

    await settings.goto(seedData.workspaceId);
    await table.getByRole("button", { name: "Check now" }).click();
    await expect(testPage.getByText("No new issues matched", { exact: true })).toBeVisible();
    await kanban.goto();
    await expect(kanban.taskCardByTitle(taskTitle)).toHaveCount(1);
    await assertNoDocumentHorizontalOverflow(testPage, "desktop issue watch settings");
  });
});
