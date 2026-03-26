import { type Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";
import { KanbanPage } from "../pages/kanban-page";

/**
 * Seed a task + session with a clarification scenario and navigate to the session page.
 * Does NOT wait for idle input — the agent will be blocked on the clarification MCP call.
 */
async function seedClarificationTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  scenario: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/s/${task.session_id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  return session;
}

test.describe("Clarification flow", () => {
  test.describe.configure({ retries: 1 });

  test("select option (happy path)", async ({ testPage, apiClient, seedData }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Clarification Happy Path",
      "clarification",
    );

    // Wait for clarification overlay to appear (agent calls ask_user_question MCP tool)
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Verify the question text appears
    await expect(session.clarificationOverlay()).toContainText("Which database");

    // Click the PostgreSQL option
    await session.clarificationOption("PostgreSQL").click();

    // Agent receives the answer and completes its turn
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });

    // Verify the answer was reflected in chat
    await expect(session.chat).toContainText(/You answered|User selected/);
  });

  test("skip clarification", async ({ testPage, apiClient, seedData }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Clarification Skip",
      "clarification",
    );

    // Wait for clarification overlay
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Click skip button
    await session.clarificationSkip().click();

    // Agent should complete its turn
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("timeout and reconciliation", async ({ testPage, apiClient, seedData }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Clarification Timeout",
      "clarification-timeout",
    );

    // Wait for clarification overlay to appear
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Wait for agent to time out (5s) and complete its turn.
    // The "timed out" text appears in chat and the deferred notice should show.
    await expect(session.chat).toContainText("timed out", { timeout: 30_000 });
    await expect(session.clarificationDeferredNotice()).toBeVisible({ timeout: 10_000 });

    // User responds after agent has moved on — this triggers the event fallback path
    await session.clarificationOption("PostgreSQL").click();

    // Orchestrator resumes agent with new turn via ClarificationAnswered event
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("plan mode + clarification does not leave pointer-events stuck on body", async ({
    testPage,
  }) => {
    // Navigate to kanban board and open the task create dialog
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    // Fill title
    await testPage.getByTestId("task-title-input").fill("Plan Mode Clarification PE");

    // Fill description with clarification scenario so the agent starts and
    // calls the ask_user_question MCP tool.
    const descriptionInput = dialog.getByRole("textbox", {
      name: "Write a prompt for the agent...",
    });
    await descriptionInput.click();
    await descriptionInput.fill("/e2e:clarification");

    // With a description present, the footer shows a split button with dropdown.
    // Open the chevron dropdown and click "Start task in plan mode".
    await testPage.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-plan-mode").click();

    // Wait for navigation to session page
    await expect(testPage).toHaveURL(/\/s\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for clarification overlay to appear (agent calls ask_user_question MCP tool)
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // CRITICAL ASSERTION: body must not have pointer-events: none stuck on it.
    // Radix Dialog sets pointer-events: none on body when modal. If the task
    // create dialog unmounts mid-close (onOpenChange(false) then router.push),
    // Radix never finishes cleanup, leaving the page unclickable.
    const pointerEvents = await testPage.evaluate(() => document.body.style.pointerEvents);
    expect(pointerEvents).not.toBe("none");

    // Verify the UI is actually interactive by clicking a clarification option
    await session.clarificationOption("PostgreSQL").click();

    // Agent receives the answer and completes its turn (plan mode uses different placeholder)
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });
});
