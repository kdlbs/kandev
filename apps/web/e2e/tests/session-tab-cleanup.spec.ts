import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

async function createTaskAndNavigate(
  testPage: import("@playwright/test").Page,
  apiClient: import("../helpers/api-client").ApiClient,
  seedData: import("../fixtures/test-base").SeedData,
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

  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 30_000, message: "Waiting for session to finish" },
    )
    .toBe(true);

  const kanban = new KanbanPage(testPage);
  await kanban.goto();
  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 10_000 });
  await card.click();
  await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
    timeout: 15_000,
  });

  return { task, session };
}

test.describe("Session tab cleanup", () => {
  test("single-session task shows only named agent tab without star or number", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    await createTaskAndNavigate(testPage, apiClient, seedData, "Single Session Tab Task");

    // Should have exactly one session tab (the named agent tab)
    const sessionTabs = testPage.locator("[data-testid^='session-tab-']");
    await expect(sessionTabs).toHaveCount(1, { timeout: 10_000 });

    // The generic "Agent" permanent tab should NOT exist
    // (it uses permanentTab tabComponent which has no data-testid, identified by text)
    const permanentAgentTab = testPage.locator(
      ".dv-default-tab:not(:has([data-testid^='session-tab-'])):has-text('Agent')",
    );
    await expect(permanentAgentTab).toHaveCount(0);

    // The single session tab should NOT show a star icon
    const star = sessionTabs.first().locator(".tabler-icon-star");
    await expect(star).toHaveCount(0);

    // The single session tab should NOT show a number badge
    // (number badges are small spans with bg-foreground/10 containing a digit)
    const numberBadge = sessionTabs.first().locator("span.rounded");
    await expect(numberBadge).toHaveCount(0);
  });
});
