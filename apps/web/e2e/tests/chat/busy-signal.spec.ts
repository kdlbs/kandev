import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";
import { typeWhileBusy } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

async function seedTaskAndWaitForIdle(
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

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Coarse RUNNING busy signal", () => {
  test.describe.configure({ retries: 1 });

  test("held-open background turn remains busy across input and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Coarse busy signal",
    );

    await session.sendMessage("/background 30s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Kicking off background work")).toBeVisible({
      timeout: 15_000,
    });
    // The foreground-idle frame follows this text in the mock's ordered ACP
    // stream. Allow the subsequent WS publication to settle before asserting
    // the stable composer contract.
    await testPage.waitForTimeout(500);

    // The private tracker may identify background work, but the public
    // contract remains coarse for the entire RUNNING turn.
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, "queue this follow-up");
    await testPage.getByTestId("submit-message-button").click();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).not.toBeVisible();
    await expect(testPage.locator('[data-placeholder^="Queue"]')).toBeVisible();
  });

  test("foreground generation continues to queue input", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Coarse busy foreground",
    );

    await session.sendMessage("/slow 10s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, "generating");

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, "queue foreground follow-up");
    await testPage.getByTestId("submit-message-button").click();
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
  });
});

test.describe.serial("Claude background prompt handoff experiment", () => {
  test.describe.configure({ retries: 1 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({
      KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF: "true",
    });
    const response = await fetch(`${backend.baseUrl}/api/v1/features`);
    expect(response.ok).toBeTruthy();
    expect(await response.json()).toMatchObject({
      claudeBackgroundPromptHandoff: true,
    });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("async subagent accepts a foreground turn and clears on completion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Experimental async subagent",
    );
    const agentName = await testPage.evaluate(() => {
      const store = (
        window as Window & {
          __KANDEV_E2E_STORE__?: {
            getState: () => {
              tasks: { activeSessionId: string | null };
              taskSessions: { items: Record<string, Record<string, unknown>> };
            };
          };
        }
      ).__KANDEV_E2E_STORE__;
      const state = store?.getState();
      const sessionID = state?.tasks.activeSessionId;
      const snapshot = sessionID
        ? (state?.taskSessions.items[sessionID]?.agent_profile_snapshot as
            | Record<string, unknown>
            | undefined)
        : undefined;
      return snapshot?.agent_name;
    });
    expect(agentName).toBe("mock-agent");

    await session.sendMessage("/async-subagent-lifecycle 12s");
    await expect(testPage.getByText("Foreground response after async launch.")).toBeVisible({
      timeout: 20_000,
    });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");

    await session.sendMessage("/slow 2s");
    await expect(testPage.getByText("/slow 2s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
    await waitForActiveSessionForegroundActivity(testPage, "generating");
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");
    await waitForActiveSessionForegroundActivity(testPage, null);
  });

  test("detached work accepts input and preserves the background state on reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const session = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Experimental detached work",
    );

    await session.sendMessage("/detached-background 20s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 20_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");

    await testPage.reload();
    await session.waitForLoad();
    await expect(session.agentStatus()).toBeVisible();
    await expect(session.idleInput()).toBeVisible({ timeout: 15_000 });
    await waitForActiveSessionForegroundActivity(testPage, "background");

    await session.sendMessage("/slow 2s");
    await expect(testPage.getByText("/slow 2s")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("queue-chip")).not.toBeVisible();
  });
});
