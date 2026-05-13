import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// Matches the number of permission prompts emitted by the mock-agent
// `/e2e:multi-permission` scenario (apps/backend/cmd/mock-agent/scenarios.go).
// Both approval loops below depend on this — bump together if the scenario
// changes.
const MULTI_PERMISSION_COUNT = 3;

/**
 * Seed a task that runs the multi-permission scenario, then navigate to it.
 * The mock agent will request three permissions in sequence and block on each.
 * Returns the SessionPage plus the task so callers can correlate with the
 * sidebar (which keys off the task title).
 */
async function seedMultiPermissionTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title = "Multi-permission approval",
): Promise<{ session: SessionPage; task: Awaited<ReturnType<ApiClient["createTaskWithAgent"]>> }> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:multi-permission",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  return { session, task };
}

test.describe("Permission approval persistence", () => {
  test.describe.configure({ retries: 1 });

  // Most E2E tests run with auto-approve on so tools-needing-permission scenarios
  // (e.g. /e2e:read-and-edit) don't block the agent. This test specifically
  // exercises the approve UI, so it needs auto-approve OFF — restart the worker
  // backend with the override before this file's tests, then restore.
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  });

  test("approved prompts stay approved after the agent's turn ends", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedMultiPermissionTask(testPage, apiClient, seedData);

    // Approve all permission prompts as they appear. Each click unblocks
    // the agent which then emits the next prompt; the previous one's button
    // detaches before the next one mounts, so wait for the count to be back
    // at 1 between clicks.
    for (let i = 0; i < MULTI_PERMISSION_COUNT; i++) {
      await expect(session.permissionApproveButtons()).toHaveCount(1, { timeout: 30_000 });
      await session.permissionApproveButtons().first().click();
    }

    // After the agent finishes its turn, no permission action row should be
    // visible — the previous bug had them re-appear at turn-complete because a
    // safety-net loop overwrote the approved status with "complete".
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.permissionActionRows()).toHaveCount(0);

    // And the resolved state must survive a page reload — i.e. backend must
    // have persisted the approve decisions, not "complete".
    await testPage.reload();
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.permissionActionRows()).toHaveCount(0);
  });

  // While an agent is blocked on a permission_request, the sidebar entry for
  // that task swaps the running spinner for the amber pending-permission icon
  // (introduced in #882). The icon goes away once the prompt is resolved.
  test("sidebar shows pending-permission icon while a permission prompt is open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const taskTitle = "Sidebar pending permission";
    const { session } = await seedMultiPermissionTask(testPage, apiClient, seedData, taskTitle);

    // First permission prompt blocks the agent.
    await expect(session.permissionApproveButtons()).toHaveCount(1, { timeout: 30_000 });

    // Sidebar swaps the running spinner for the amber pending-permission icon.
    // No `.first()` — `e2eReset` runs before every test (and every retry), so
    // the worker workspace has exactly one task with this title; if a duplicate
    // ever appears, strict locator resolution should fail the test rather than
    // silently picking one row.
    const sidebarItem = session.sidebarTaskItem(taskTitle);
    await expect(sidebarItem.getByTestId("task-state-pending-permission")).toBeVisible({
      timeout: 10_000,
    });
    await expect(sidebarItem.getByTestId("task-state-running")).toHaveCount(0);

    // Approve all prompts; once the agent's turn ends, the icon is gone.
    for (let i = 0; i < MULTI_PERMISSION_COUNT; i++) {
      await expect(session.permissionApproveButtons()).toHaveCount(1, { timeout: 30_000 });
      await session.permissionApproveButtons().first().click();
    }
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(sidebarItem.getByTestId("task-state-pending-permission")).toHaveCount(0);
  });
});
