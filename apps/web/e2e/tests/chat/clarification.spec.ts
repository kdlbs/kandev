import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { KanbanPage } from "../../pages/kanban-page";

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

  await testPage.goto(`/t/${task.id}`);

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

  test("timeout closes overlay and renders expired entry in history", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
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
    await expect(session.chat).toContainText("timed out", { timeout: 30_000 });

    // Overlay should auto-close once the canceller marks status=expired. The
    // deferred "your response will be sent as a new message" notice must NOT
    // appear — we're not keeping a stale interactive prompt around.
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 10_000 });
    await expect(session.clarificationDeferredNotice()).not.toBeVisible();

    // Chat history should show the question as expired (orange X + label).
    await expect(session.clarificationExpiredNotice()).toBeVisible();

    // Chat input returns to the default idle placeholder — not the clarification
    // one. Confirms no new turn was triggered by the timeout flow.
    await expect(session.idleInput()).toBeVisible({ timeout: 10_000 });
  });

  test("options render label and description on separate rows", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Clarification Layout",
      "clarification",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // The mock scenario uses three options, each with a label and description.
    const labels = session.clarificationOptionLabels();
    const descriptions = session.clarificationOptionDescriptions();
    await expect(labels).toHaveCount(3);
    await expect(descriptions).toHaveCount(3);

    // Label and description must be stacked vertically (description's top
    // edge sits below the label's bottom edge). Regression guard for the
    // old layout that rendered them side-by-side on a single row.
    const labelBox = await labels.first().boundingBox();
    const descriptionBox = await descriptions.first().boundingBox();
    if (!labelBox || !descriptionBox) {
      throw new Error("expected both label and description to have bounding boxes");
    }
    expect(descriptionBox.y).toBeGreaterThanOrEqual(labelBox.y + labelBox.height - 1);
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
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

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
