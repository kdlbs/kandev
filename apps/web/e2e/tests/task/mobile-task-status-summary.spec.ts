import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const TARGET_TITLE = "Mobile inactive summary target";

test.describe("Mobile task status summary", () => {
  test("updates the native task switcher from bounded status revisions", async ({
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
      "Mobile summary navigation",
      stepOptions,
    );
    const targetTask = await apiClient.seedTask(seedData.workspaceId, TARGET_TITLE, stepOptions);
    await apiClient.seedTaskSession(navTask.task_id, {
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
    await testPage.getByTestId("mobile-session-menu").click();
    const sheet = testPage.getByRole("dialog");
    const targetRow = sheet.getByTestId("sidebar-task-item").filter({
      hasText: TARGET_TITLE,
    });
    await expect(targetRow).toBeVisible({ timeout: 15_000 });

    await apiClient.seedSessionMessage(targetSession.session_id, {
      type: "clarification_request",
      content: "Choose a mobile-safe approach",
    });
    await expect(targetRow.getByTestId("task-state-waiting-for-input")).toBeVisible({
      timeout: 15_000,
    });

    await apiClient.seedTaskSession(targetTask.task_id, {
      sessionId: targetSession.session_id,
      state: "WAITING_FOR_INPUT",
      metadata: {
        last_agent_error: {
          message: "Mobile inactive agent failed",
          occurred_at: new Date().toISOString(),
        },
      },
    });
    await expect(targetRow.getByTestId("task-agent-error-icon")).toBeVisible({ timeout: 15_000 });

    await apiClient.mockGitHubAssociateTaskPR({
      workspace_id: seedData.workspaceId,
      task_id: targetTask.task_id,
      owner: "kandev-e2e",
      repo: "mobile-summary-fixtures",
      pr_number: 43,
      pr_url: "https://github.test/kandev-e2e/mobile-summary-fixtures/pull/43",
      pr_title: "Mobile bounded summary fixture",
      head_branch: "feature/mobile-summary",
      base_branch: "main",
      author_login: "e2e",
      state: "open",
      review_state: "approved",
      checks_state: "success",
      mergeable_state: "clean",
      required_reviews: 1,
      checks_total: 1,
      checks_passing: 1,
    });
    await expect(targetRow.getByTestId(`pr-task-icon-${targetTask.task_id}`)).toHaveAttribute(
      "data-pr-state",
      "open",
      { timeout: 15_000 },
    );

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    expect(viewport!.width).toBeGreaterThan(0);
  });
});
