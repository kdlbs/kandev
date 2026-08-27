/**
 * Regression test for the sidebar "stuck generating" spinner: when a task's
 * session settles out of RUNNING (agent turn completes -> WAITING_FOR_INPUT,
 * task -> COMPLETED), the sidebar row must stop showing the yellow generating
 * spinner (`task-state-running`) from a live task.updated stream — without ever
 * opening the completed task.
 *
 * The bug (backend): the turn-activity record was detached while the session was
 * still RUNNING, so the recomputed task-level foreground_activity aggregate cached
 * "generating"; the session then settled with only a session-level state_changed
 * event, so the task aggregate was never republished and the spinner kept spinning.
 * Fixed by republishing the task aggregate when a session leaves RUNNING.
 *
 * The browser must be mounted before the agent starts so the client observes the
 * generating task.updated and retains that cached aggregate through settle. A
 * post-settle fresh load only hydrates the durable settled state and would pass
 * against an unfixed parent revision.
 */
import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/** Hold RUNNING long enough for the sidebar spinner assertion, then finish. */
const SETTLE_SCRIPT = [
  'e2e:message("starting settle turn...")',
  "e2e:delay(4000)",
  'e2e:message("settle turn done")',
].join("\n");

async function waitForSessionWaitingForInput(
  apiClient: ApiClient,
  taskId: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions[0]?.state ?? "";
      },
      { message, timeout: 60_000 },
    )
    .toBe("WAITING_FOR_INPUT");
}

test.describe("Sidebar spinner clears when a session settles", () => {
  test("completed task drops the generating spinner without opening it", async ({
    apiClient,
    seedData,
    testPage,
  }) => {
    test.setTimeout(120_000);

    // Mount the sidebar on a different task first so the client is live before
    // the agent runs and can receive the generating → settled task.updated stream.
    const navTask = await apiClient.createTask(seedData.workspaceId, "Nav Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${navTask.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.sidebar).toBeVisible({ timeout: 10_000 });

    const settledTask = await apiClient.createTask(seedData.workspaceId, "Settled Turn", {
      description: SETTLE_SCRIPT,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });

    const settledRow = session.sidebarTaskItem("Settled Turn");
    await expect(settledRow).toBeVisible({ timeout: 15_000 });

    // Start only after the live sidebar has observed the task row. Creating
    // and starting in one request can settle the short mock turn before the
    // client receives the generating task.updated event under CI load.
    const ensured = await apiClient.ensureTaskSession(settledTask.id);
    expect(ensured.session_id).toBeTruthy();

    // Client must observe generating while the session is still RUNNING.
    await expect(settledRow.getByTestId("task-state-running")).toBeVisible({
      timeout: 30_000,
    });
    const runningStatus = settledRow.getByTestId("task-state-running");
    await expect
      .poll(async () =>
        runningStatus.evaluate((element) => ({
          tagName: element.tagName,
          svgAnimated: element.querySelector("svg")?.classList.contains("animate-spin") ?? false,
        })),
      )
      .toEqual({ tagName: "SPAN", svgAnimated: false });

    await waitForSessionWaitingForInput(
      apiClient,
      settledTask.id,
      "settled task session should finish its turn",
    );

    // Core regression: the settle task.updated must clear the spinner the
    // client already painted — never reopen the completed task.
    await expect(settledRow.getByTestId("task-state-running")).toHaveCount(0, {
      timeout: 15_000,
    });

    // Settled turn with no pending clarification buckets into "review".
    await expect(settledRow.getByTestId("task-state-turn-finished")).toBeVisible({
      timeout: 10_000,
    });
  });
});
