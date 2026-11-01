import { expect, type Page } from "@playwright/test";
import { firstSessionId } from "../../helpers/session";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * The active background count is evidence that a workload still runs. When the
 * agent process that owns it is replaced, the count clears and completed
 * history stays reachable (REQ-AGENTS-BACKGROUND-WORK-010.1 and .5). Desktop
 * and phone share this flow through their own spec files.
 */
export async function expectProcessEndClearsBackgroundWork(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Background process end",
    seedData.agentProfileId,
    {
      description: "/detached-background 120s",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const activeChip = testPage.getByTestId("background-work-chip");
  await expect(activeChip).toBeVisible({ timeout: 30_000 });
  await expect(activeChip).toContainText("Background Job");

  const sessionId = await firstSessionId(apiClient, task.id);
  const activeWorkloadCount = async (): Promise<number> => {
    const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
    return workloads.filter((w) => ["running", "waiting", "unknown"].includes(w.state)).length;
  };
  // The provider shape may register more than one workload; only the fact that
  // work is active before the process ends matters here.
  await expect
    .poll(activeWorkloadCount, {
      message: "the detached launch should register active background work",
      timeout: 30_000,
    })
    .toBeGreaterThan(0);

  // Restart the session exactly as the UI does: stop the owning process, then
  // resume the conversation under a new execution.
  await apiClient.stopSession({ session_id: sessionId });
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((s) => s.id === sessionId)?.state ?? "";
      },
      { message: "session should settle after the stop", timeout: 30_000 },
    )
    .toMatch(/^(WAITING_FOR_INPUT|IDLE|COMPLETED|FAILED|CANCELLED)$/);
  // The resume waits for agent readiness; the default WS budget is shorter than
  // that readiness window on a loaded host.
  await apiClient.launchSession(
    {
      task_id: task.id,
      intent: "resume",
      session_id: sessionId,
    },
    90_000,
  );

  // The workload can no longer report completion, so it settles and the active
  // count disappears instead of staying wrong for the new process. Poll the
  // authoritative projection first, then assert the rendered chip.
  await expect
    .poll(activeWorkloadCount, {
      message: "background workload should settle after the process ends",
      timeout: 60_000,
    })
    .toBe(0);
  await expect(activeChip).toHaveCount(0);
  await expect(testPage.getByTestId("background-work-history-chip")).toBeVisible();
}
