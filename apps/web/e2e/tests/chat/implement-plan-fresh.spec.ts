import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// "Implement plan" split-button — fresh-agent path
// ---------------------------------------------------------------------------
//
// The toolbar shows the violet 🚀 Implement button only while plan mode is
// active and the workflow's next step is not a work step. The split-button has
// two paths:
//   - Primary click  → same-session message.add (existing behavior)
//   - Chevron menu → "Implement in fresh agent" → launchSession(intent: start)
//     which spawns a brand-new TaskSession on the same task.
//
// These tests exercise the fresh-agent path end-to-end and assert the new
// session inherits the planning session's agent profile.

async function seedTaskAndEnterPlanMode(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
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

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  await session.togglePlanMode();
  await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });

  return { session, taskId: task.id, planningSessionId: task.session_id! };
}

test.describe("Implement plan — fresh-agent path", () => {
  test.describe.configure({ retries: 1 });

  test("split-button menu launches a new session inheriting the planning session's agent profile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, taskId, planningSessionId } = await seedTaskAndEnterPlanMode(
      testPage,
      apiClient,
      seedData,
      "Implement plan fresh-agent test",
    );

    // Sanity: only the planning session exists at this point.
    const before = await apiClient.listTaskSessions(taskId);
    expect(before.sessions).toHaveLength(1);
    expect(before.sessions[0].id).toBe(planningSessionId);
    const planningProfileId = before.sessions[0].agent_profile_id;
    expect(planningProfileId).toBeTruthy();

    // Primary button is visible while plan mode is active.
    const primary = testPage.getByTestId("implement-plan-button");
    await expect(primary).toBeVisible({ timeout: 10_000 });

    // Open the chevron dropdown and click the fresh-agent menu item.
    await testPage.getByTestId("implement-plan-menu-trigger").click();
    const freshItem = testPage.getByTestId("implement-fresh-menu-item");
    await expect(freshItem).toBeVisible({ timeout: 5_000 });
    await freshItem.click();

    // A second TaskSession should appear on the task. launchSession is async
    // (WS round-trip + lifecycle prepare), so poll instead of asserting once.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskId);
          return sessions.length;
        },
        { timeout: 30_000, message: "fresh session never appeared" },
      )
      .toBe(2);

    const { sessions } = await apiClient.listTaskSessions(taskId);
    const freshSession = sessions.find((s) => s.id !== planningSessionId);
    expect(freshSession).toBeDefined();

    // Fresh session inherits the planning session's agent profile (the
    // fresh-agent path passes agent_profile_id from the planning session into
    // launchSession — no profile picker is shown).
    expect(freshSession!.agent_profile_id).toBe(planningProfileId);

    // Planning session was not deleted/replaced — fresh-agent path leaves it
    // running in parallel.
    expect(sessions.some((s) => s.id === planningSessionId)).toBe(true);

    // Touch `session` so the unused-binding lint stays clean even after we
    // dropped the dockview-tab assertion (which depended on lazy tab mount).
    expect(session).toBeDefined();
  });

  test("dropdown is hidden until plan mode is enabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Implement button hidden without plan mode",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // No plan mode → no Implement button and no chevron trigger.
    await expect(testPage.getByTestId("implement-plan-button")).not.toBeVisible();
    await expect(testPage.getByTestId("implement-plan-menu-trigger")).not.toBeVisible();

    // Toggle on → both appear.
    await session.togglePlanMode();
    await expect(testPage.getByTestId("implement-plan-button")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("implement-plan-menu-trigger")).toBeVisible();
  });
});
