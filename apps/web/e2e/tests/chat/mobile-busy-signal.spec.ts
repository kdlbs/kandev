import { test, expect } from "../../fixtures/test-base";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile coarse RUNNING busy signal", () => {
  test.describe.configure({ retries: 1 });

  test("held-open background turn remains busy across input and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile coarse busy signal",
      seedData.agentProfileId,
      {
        description: "/background 30s",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(testPage.getByText("Kicking off background work")).toBeVisible({
      timeout: 20_000,
    });
    await testPage.waitForTimeout(500);

    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, "queue this mobile follow-up");
    await testPage.getByTestId("submit-message-button").tap();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();
  });
});

test.describe.serial("Mobile Claude background prompt handoff experiment", () => {
  test.describe.configure({ retries: 1 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true",
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("background-only work keeps working visible while input sends immediately", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile experimental background handoff",
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
    await session.waitForChatIdle({ timeout: 30_000 });

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await editor.tap();
    await editor.fill("/detached-background 20s");
    await testPage.getByTestId("submit-message-button").tap();
    await expect(session.agentStatus()).toBeVisible({ timeout: 20_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");

    await editor.tap();
    await editor.fill("/slow 2s");
    await testPage.getByTestId("submit-message-button").tap();

    await expect(testPage.getByText("/slow 2s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
  });
});
