import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

/**
 * Regression: maximize a tab on Task A → switch to Task B via the sidebar →
 * switch back to Task A. The dockview layout was returning broken with the
 * centre group shrunk because the per-env saved layout had been overwritten
 * with the (2-column) maximize overlay while still maximized, and the
 * maximize state was only half-restored on the way back.
 *
 * Reproduces with sidebar clicks (no page reload), exercising
 * `performLayoutSwitch` rather than `tryRestoreLayout`.
 */
async function seedTaskWithSession(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  return session;
}

test.describe("Maximize survives sidebar task switch", () => {
  test.describe.configure({ retries: 1 });

  test("switching back to a maximized task via the sidebar keeps the layout healthy", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Task A — will be maximized.
    const sessionA = await seedTaskWithSession(testPage, apiClient, seedData, "Maximize Task A");
    await sessionA.expectDefaultLayout();

    // Task B — created up front so it's visible in the sidebar of Task A.
    const taskB = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Maximize Task B",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!taskB.session_id) throw new Error("Task B creation did not return a session_id");

    // Maximize the terminal group on Task A.
    await sessionA.clickMaximize();
    await sessionA.expectMaximized();

    // Switch to Task B via the sidebar (client-side, no page reload).
    await sessionA.clickTaskInSidebar("Maximize Task B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}(?:\\?|$)`), { timeout: 10_000 });
    await sessionA.waitForLoad();
    // Task B starts with the default layout — sanity check before the bug.
    await sessionA.expectDefaultLayout();

    // Switch back to Task A via the sidebar.
    await sessionA.clickTaskInSidebar("Maximize Task A");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 10_000 });
    await sessionA.waitForLoad();

    // Bug invariants:
    //  1. The layout must be healthy — no zero-width groups, no large gap on
    //     the right (the user-visible "centre group is shrunk" symptom).
    //  2. The maximize state must be preserved across the round-trip — the
    //     same expectation we already hold for full page reloads.
    await sessionA.expectLayoutHealthy();
    await sessionA.expectMaximized();
  });
});
