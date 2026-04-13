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

// ---------------------------------------------------------------------------
// Plan mode follow-up message tests
// ---------------------------------------------------------------------------

test.describe("Plan mode follow-up messages", () => {
  test.describe.configure({ retries: 1 });

  test("follow-up message in plan mode shows badge and stores plan_mode metadata", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, sessionId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Plan mode follow-up test",
    );

    // Enable plan mode after initial agent turn completes.
    await session.togglePlanMode();
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });

    // Send a follow-up message in plan mode.
    await session.sendMessage("Please add error handling to step 3 of the plan");

    // The follow-up message should have the plan mode badge.
    const planBadges = session.chat.getByText("Plan mode", { exact: true });
    await expect(planBadges.last()).toBeVisible({ timeout: 15_000 });

    // Wait for agent to complete and return to plan mode idle state.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });

    // Verify the stored message has plan_mode metadata (confirms frontend sent plan_mode: true
    // and backend stored it, which triggers InjectPlanMode in PromptTask).
    const { messages } = await apiClient.listSessionMessages(sessionId);
    const userFollowUp = messages.filter((m) => m.author_type === "user").pop();
    expect(userFollowUp).toBeDefined();
    expect(userFollowUp!.metadata?.plan_mode).toBe(true);
  });

  test("follow-up message without plan mode has no badge or plan_mode metadata", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session, sessionId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "No plan mode follow-up test",
    );

    // Send a follow-up without plan mode.
    await session.sendMessage("implement the feature now");
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // No plan mode badge.
    const planBadges = session.chat.getByText("Plan mode", { exact: true });
    await expect(planBadges).toHaveCount(0, { timeout: 5_000 });

    // Verify no plan_mode metadata on the stored message.
    const { messages } = await apiClient.listSessionMessages(sessionId);
    const userFollowUp = messages.filter((m) => m.author_type === "user").pop();
    expect(userFollowUp).toBeDefined();
    expect(userFollowUp!.metadata?.plan_mode).toBeFalsy();
  });

  test("plan comment via Run button sends to agent with plan mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    // Create a task that produces plan content via MCP.
    const { session, sessionId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Plan comment Run test",
      PLAN_SCRIPT,
    );

    // Toggle plan mode so the plan panel is visible with content.
    await session.togglePlanMode();
    await expect(session.planModeInput()).toBeVisible({ timeout: 10_000 });
    await expect(session.planPanel.getByText("Analyze requirements", { exact: false })).toBeVisible(
      {
        timeout: 15_000,
      },
    );

    // Select text in the plan editor to trigger the comment popover.
    const editor = session.planPanel.locator(".ProseMirror");
    await editor.click();

    // Select all text with keyboard.
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+a`);

    // Open comment popover via Cmd+Shift+C.
    await testPage.keyboard.press(`${modifier}+Shift+c`);

    // Fill in the comment textarea.
    const textarea = testPage.locator('textarea[placeholder="Add your comment or instruction..."]');
    await expect(textarea).toBeVisible({ timeout: 5_000 });
    await textarea.fill("Split step 2 into smaller sub-steps");

    // Click the "Run" button (use exact match to avoid matching sidebar task button).
    const runBtn = testPage.getByRole("button", { name: "Run", exact: true });
    await expect(runBtn).toBeVisible({ timeout: 5_000 });
    await runBtn.click();

    // The comment should appear in the chat formatted as plan comment markdown.
    await expect(session.chat.getByText("Comment 1:", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      session.chat.getByText("Split step 2 into smaller sub-steps", { exact: false }),
    ).toBeVisible({ timeout: 5_000 });

    // Plan mode badge should be visible on the comment message.
    const planBadges = session.chat.getByText("Plan mode", { exact: true });
    await expect(planBadges.last()).toBeVisible({ timeout: 10_000 });

    // Wait for agent to complete.
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });

    // Verify the comment message was stored with plan_mode metadata.
    const { messages } = await apiClient.listSessionMessages(sessionId);
    const commentMsg = messages
      .filter((m) => m.author_type === "user")
      .find((m) => m.content.includes("Split step 2"));
    expect(commentMsg).toBeDefined();
    expect(commentMsg!.metadata?.plan_mode).toBe(true);
  });
});
