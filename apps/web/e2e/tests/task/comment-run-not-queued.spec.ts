import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

/** MCP script that creates plan content the agent can interact with. */
const PLAN_SCRIPT = [
  'e2e:thinking("Creating plan...")',
  "e2e:delay(100)",
  'e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"## Plan\\n\\n1. Analyze requirements\\n2. Implement solution\\n3. Write tests","title":"Implementation Plan"})',
  "e2e:delay(100)",
  'e2e:message("Plan created.")',
].join("\n");

/** Create a task with agent, navigate to session, wait for idle. */
async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

  return { session, taskId: task.id, sessionId: task.session_id! };
}

/**
 * Open the plan comment popover, type a comment, and click Run.
 * Returns the comment text so callers can assert on it.
 */
async function addPlanCommentAndRun(
  testPage: Page,
  session: SessionPage,
  commentText: string,
): Promise<void> {
  const editor = session.planPanel.locator(".ProseMirror");
  await editor.click();

  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await testPage.keyboard.press(`${modifier}+a`);
  await testPage.keyboard.press(`${modifier}+Shift+c`);

  const textarea = testPage.locator('textarea[placeholder="Add your comment or instruction..."]');
  await expect(textarea).toBeVisible({ timeout: 5_000 });
  await textarea.fill(commentText);

  const runBtn = testPage.getByRole("button", { name: "Run", exact: true });
  await expect(runBtn).toBeVisible({ timeout: 5_000 });
  await runBtn.click();
}

// ---------------------------------------------------------------------------
// Tests: Comment "Run" should send directly when agent is idle, not queue
// ---------------------------------------------------------------------------

test.describe("Comment run sends directly when agent is idle", () => {
  test.describe.configure({ retries: 1 });

  test("plan comment Run after agent was previously running sends directly, not queued", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // 1. Create a task that produces plan content. Agent runs and finishes.
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Comment queue bug test",
      PLAN_SCRIPT,
    );

    // 2. Toggle plan mode — the plan panel should show content.
    await session.togglePlanMode();
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });
    await expect(session.planPanel.getByText("Analyze requirements", { exact: false })).toBeVisible(
      {
        timeout: 15_000,
      },
    );

    // 3. Run a slow command so the session goes through RUNNING -> WAITING_FOR_INPUT again.
    //    This ensures isAgentBusy was recently true.
    await session.sendMessage("/slow 3s");
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });

    // 4. Agent is now idle. Add a plan comment and click Run.
    await addPlanCommentAndRun(testPage, session, "Refactor step 1 to use dependency injection");

    // 5. The comment should NOT be queued — no queue indicator should appear.
    const queueIndicator = testPage.getByTitle("Cancel queued message");
    await expect(queueIndicator).not.toBeVisible({ timeout: 3_000 });

    // 6. The comment should appear in the chat as a direct message.
    await expect(
      session.chat.getByText("Refactor step 1 to use dependency injection", { exact: false }),
    ).toBeVisible({ timeout: 15_000 });

    // 7. The agent should start processing (proves it was sent as message.add, not queued).
    await expect(session.agentStatus()).toBeVisible({ timeout: 15_000 });

    // 8. Wait for agent to complete.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });
});
