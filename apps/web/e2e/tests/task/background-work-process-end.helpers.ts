import { expect, type Page } from "@playwright/test";
import { firstSessionId } from "../../helpers/session";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
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

  const sessionId = await firstSessionId(apiClient, task.id);
  const runningWorkloadCount = async (): Promise<number> => {
    const { workloads } = await apiClient.listBackgroundWorkloads(sessionId);
    return workloads.filter((w) => w.state === "running").length;
  };
  // The composer chip is running-only, so wait for the authoritative running
  // projection before asserting its rendered shadow. CI runs several subshards
  // concurrently, so registration can exceed a single-process local budget.
  await expect
    .poll(runningWorkloadCount, {
      message: "the detached launch should register a running background workload",
      timeout: 60_000,
    })
    .toBeGreaterThan(0);

  const activeChip = testPage.getByTestId("background-work-chip");
  await expect(activeChip).toBeVisible();
  await expect(activeChip).toContainText("Background Job");

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

  // The workload can no longer report completion, so it settles and the
  // running-only composer chip disappears instead of staying wrong for the new
  // process. Poll the authoritative projection first, then assert the chip.
  await expect
    .poll(runningWorkloadCount, {
      message: "background workload should leave the running summary after the process ends",
      timeout: 60_000,
    })
    .toBe(0);
  await expect(activeChip).toHaveCount(0);
  await expect(testPage.getByTestId("background-work-history-chip")).toHaveCount(0);

  // History remains reachable through the independent desktop or phone entry.
  if ((testPage.viewportSize()?.width ?? 0) < 768) {
    await testPage.getByTestId("mobile-session-nav-panels").tap();
    await testPage.getByTestId("mobile-background-work-open").tap();
    await expect(testPage.getByTestId("background-work-mobile-panel")).toBeVisible();
  } else {
    await testPage.getByTestId("dockview-add-panel-btn").first().click();
    await testPage.getByTestId("add-panel-background-work").click();
    await expect(testPage.getByTestId("background-work-overview")).toBeVisible();
  }
  await expect(testPage.getByTestId("background-work-tab-history")).toHaveAttribute(
    "data-state",
    "active",
  );
  await expect(testPage.getByTestId(/^background-workload-card-/).first()).toBeVisible();
}
