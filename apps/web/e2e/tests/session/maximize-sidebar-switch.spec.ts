import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const TERMINAL_MARKER = "KANDEV_E2E_MAXIMIZE_SIDEBAR_5567";

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

    // Type into the terminal so it's actually connected before we maximize —
    // the existing maximize tests in session-layout.spec.ts use the same
    // pattern. Without it, clickMaximize can race with the terminal panel
    // mounting.
    await sessionA.typeInTerminal(`echo ${TERMINAL_MARKER}`);
    await sessionA.expectTerminalHasText(TERMINAL_MARKER);

    // Task B — created after Task A is settled so the sidebar is stable.
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
    const taskBRow = sessionA.taskInSidebar("Maximize Task B");
    await expect(taskBRow).toBeVisible({ timeout: 15_000 });
    await sessionA.clickTaskInSidebar("Maximize Task B");
    await expect(testPage).toHaveURL(new RegExp(`/t/${taskB.id}(?:\\?|$)`), { timeout: 15_000 });
    await sessionA.waitForLoad();
    await expect(sessionA.idleInput()).toBeVisible({ timeout: 30_000 });
    // Task B starts with the default layout — sanity check before the bug.
    await sessionA.expectDefaultLayout();

    // Switch back to Task A via the sidebar.
    const taskARow = sessionA.taskInSidebar("Maximize Task A");
    await expect(taskARow).toBeVisible({ timeout: 15_000 });
    await sessionA.clickTaskInSidebar("Maximize Task A");
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await sessionA.waitForLoad();

    // Bug invariants:
    //  1. The layout must be healthy — no zero-width groups, no large gap on
    //     the right (the user-visible "centre group is shrunk" symptom).
    //  2. The maximize state must be preserved across the round-trip — same
    //     expectation we already hold for full page reloads.
    await expect(sessionA.terminal).toBeVisible({ timeout: 15_000 });
    await sessionA.expectLayoutHealthy();
    await sessionA.expectMaximized();
  });
});
