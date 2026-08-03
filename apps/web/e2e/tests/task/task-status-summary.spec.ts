import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const TARGET_TITLE = "Inactive summary target";

test.describe("Task status summary", () => {
  test("updates an inactive row from bounded clarification, error, and PR state", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const stepOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    };
    const navTask = await apiClient.seedTask(
      seedData.workspaceId,
      "Summary navigation",
      stepOptions,
    );
    const targetTask = await apiClient.seedTask(seedData.workspaceId, TARGET_TITLE, stepOptions);
    const navSession = await apiClient.seedTaskSession(navTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const targetSession = await apiClient.seedTaskSession(targetTask.task_id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });

    await testPage.goto(`/t/${navTask.task_id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const targetRow = session.sidebarTaskItem(TARGET_TITLE);
    await expect(targetRow).toBeVisible({ timeout: 15_000 });

    await apiClient.seedSessionMessage(targetSession.session_id, {
      type: "clarification_request",
      content: "Which approach should I use?",
    });
    await expect(targetRow.getByTestId("task-state-waiting-for-input")).toBeVisible({
      timeout: 15_000,
    });

    const occurredAt = new Date().toISOString();
    await apiClient.seedTaskSession(targetTask.task_id, {
      sessionId: targetSession.session_id,
      state: "WAITING_FOR_INPUT",
      metadata: {
        last_agent_error: {
          message: "Inactive agent failed",
          occurred_at: occurredAt,
        },
      },
    });
    await expect(targetRow.getByTestId("task-agent-error-icon")).toBeVisible({ timeout: 15_000 });

    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: targetTask.task_id,
      owner: "kandev-e2e",
      repo: "summary-fixtures",
      pr_number: 42,
      pr_url: "https://github.test/kandev-e2e/summary-fixtures/pull/42",
      pr_title: "Bounded summary fixture",
      head_branch: "feature/summary",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
      review_state: "changes_requested",
      checks_state: "failure",
      mergeable_state: "dirty",
      required_reviews: 1,
      checks_total: 1,
      checks_passing: 0,
    });
    await expect(targetRow.getByText("#42", { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(targetRow.getByTestId("task-agent-error-icon")).toBeVisible();
    await expect(targetRow.getByTestId("task-state-waiting-for-input")).toBeVisible();

    // The navigation task is deliberately selected throughout: every update
    // above must arrive through task.status_summary.updated, not an inactive
    // session detail subscription.
    await expect(session.activeSidebarTaskItem("Summary navigation")).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(navSession.session_id).toBeTruthy();
  });
});
