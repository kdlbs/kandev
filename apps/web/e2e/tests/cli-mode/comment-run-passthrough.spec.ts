import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";

/**
 * Regression for CLI passthrough: prompts sent via `message.add` (plan/editor
 * comment Run, passthrough composer, etc.) must reach PTY stdin with a submit
 * key so the mock TUI echoes "Processed: <text>".
 *
 * This spec exercises the passthrough composer UI; comment Run uses the same
 * `useRunComment` → `message.add` → `promptPassthrough` backend path.
 *
 * Related coverage:
 * - ACP plan comment Run: apps/web/e2e/tests/task/comment-run-not-queued.spec.ts
 * - Passthrough composer Enter: apps/web/e2e/tests/cli-mode/passthrough-toolbar.spec.ts
 * - Backend integration: TestPromptTask_PassthroughRoutesToPTYStdin
 */
test.describe("CLI mode: message.add submits to PTY", () => {
  test.describe.configure({ retries: 1 });

  async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
    const profile = await apiClient.createAgentProfile(agents[0].id, name, {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: true,
    });
    return profile.id;
  }

  async function openPassthroughTask(
    testPage: Page,
    apiClient: ApiClient,
    seedData: {
      workspaceId: string;
      workflowId: string;
      startStepId: string;
      repositoryId: string;
    },
    profileId: string,
    taskTitle: string,
  ): Promise<{ session: SessionPage; taskId: string; sessionId: string }> {
    const created = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      taskTitle,
      profileId,
      {
        description: "seed passthrough prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!created.session_id) throw new Error("createTaskWithAgent did not return session_id");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();
    const card = kanban.taskCardByTitle(taskTitle);
    await expect(card).toBeVisible({ timeout: 20_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    await session.expectPassthroughHasText("Processed:", 20_000);

    return { session, taskId: created.id, sessionId: created.session_id };
  }

  test("passthrough composer send shows Processed in terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "CLI message.add");
    const { session } = await openPassthroughTask(
      testPage,
      apiClient,
      seedData,
      profileId,
      "CLI message.add Task",
    );

    await testPage.getByTestId("passthrough-toggle-composer").click();
    await expect(testPage.getByTestId("passthrough-composer")).toBeVisible({ timeout: 5_000 });

    const followUp = "passthrough message.add submit e2e";
    const editor = testPage.getByTestId("passthrough-composer").locator(".tiptap.ProseMirror");
    await editor.fill(followUp);
    await testPage.getByTestId("submit-message-button").click();

    await expect(testPage.getByTestId("passthrough-composer")).toBeHidden({ timeout: 10_000 });
    await session.expectPassthroughHasText(`Processed: ${followUp}`, 20_000);
  });

  test("message.add via API (comment Run path) shows Processed in terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "CLI comment Run API");
    const { session, taskId, sessionId } = await openPassthroughTask(
      testPage,
      apiClient,
      seedData,
      profileId,
      "CLI comment Run API Task",
    );

    const commentText = "passthrough comment run via message.add";
    await apiClient.addUserMessage(taskId, sessionId, commentText);

    await session.expectPassthroughHasText(`Processed: ${commentText}`, 20_000);
  });
});
