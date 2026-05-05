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

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await expect(session.clarificationOverlay()).toContainText("Which database");

    // Single-question bundles still expose option click → instant resolve.
    await session.clarificationOption("PostgreSQL").click();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText(/You answered|selected_option/);
  });

  test("skip clarification", async ({ testPage, apiClient, seedData }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Clarification Skip",
      "clarification",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationSkip().click();
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

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("timed out", { timeout: 30_000 });
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 10_000 });
    await expect(session.clarificationDeferredNotice()).not.toBeVisible();
    await expect(session.clarificationExpiredNotice()).toBeVisible();
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
    const labels = session.clarificationOptionLabels();
    const descriptions = session.clarificationOptionDescriptions();
    await expect(labels).toHaveCount(3);
    await expect(descriptions).toHaveCount(3);
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
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    await kanban.createTaskButton.first().click();
    const dialog = testPage.getByTestId("create-task-dialog");
    await expect(dialog).toBeVisible();

    await testPage.getByTestId("task-title-input").fill("Plan Mode Clarification PE");

    const descriptionInput = dialog.getByRole("textbox", {
      name: "Write a prompt for the agent...",
    });
    await descriptionInput.click();
    await descriptionInput.fill("/e2e:clarification");

    await testPage.getByTestId("submit-start-agent-chevron").click();
    await testPage.getByTestId("submit-plan-mode").click();

    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const pointerEvents = await testPage.evaluate(() => document.body.style.pointerEvents);
    expect(pointerEvents).not.toBe("none");

    await session.clarificationOption("PostgreSQL").click();
    await expect(session.planModeInput()).toBeVisible({ timeout: 30_000 });
  });
});

// Multi-question carousel UX. Each scenario uses the mock-agent's
// `clarification-multi` scenario which sends 3 questions in a single MCP call.
test.describe("Multi-question clarification carousel", () => {
  test.describe.configure({ retries: 1 });

  test("renders stepper with 3 steps and shows the first question", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q stepper",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // 3 stepper buttons rendered; the first is active and unanswered.
    await expect(session.clarificationSteps()).toHaveCount(3);
    await expect(session.clarificationStep(0)).toHaveAttribute("data-active", "true");
    await expect(session.clarificationStep(0)).toHaveAttribute("data-answered", "false");
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "false");

    // Group progress + per-question chip both rendered.
    await expect(session.clarificationGroupProgress()).toContainText("0 of 3 answered");
    await expect(session.clarificationOverlay()).toContainText("Question 1 of 3");

    // Only one card is visible at a time (carousel UX, not stacked).
    await expect(session.clarificationQuestionCards()).toHaveCount(1);
    await expect(session.clarificationOverlay()).toContainText("Which database");
  });

  test("answering option auto-advances to next step and marks step as answered", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q advance",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Pick an option on step 1; auto-advances to step 2.
    await session.clarificationOption("PostgreSQL").click();
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "true");
    await expect(session.clarificationStep(0)).toHaveAttribute("data-answered", "true");
    await expect(session.clarificationGroupProgress()).toContainText("1 of 3 answered");
    await expect(session.clarificationOverlay()).toContainText("Question 2 of 3");
    await expect(session.clarificationOverlay()).toContainText("Which language");
  });

  test("Back button restores the previous question and the prior selection", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q back",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await session.clarificationOption("PostgreSQL").click();
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "true");

    await session.clarificationPrev().click();
    await expect(session.clarificationStep(0)).toHaveAttribute("data-active", "true");

    // Previous answer is still selected.
    const selectedOption = session
      .clarificationQuestionCardById("db")
      .locator('[data-testid="clarification-option"][data-selected="true"]');
    await expect(selectedOption).toContainText("PostgreSQL");
  });

  test("clicking a step in the stepper jumps directly to that question", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q jump",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await session.clarificationStep(2).click();
    await expect(session.clarificationStep(2)).toHaveAttribute("data-active", "true");
    await expect(session.clarificationOverlay()).toContainText("Question 3 of 3");
    await expect(session.clarificationOverlay()).toContainText("How should we deploy");
  });

  test("Submit button disabled until every question is answered", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q submit gating",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Jump to last step without answering.
    await session.clarificationStep(2).click();
    const submit = session.clarificationSubmit();
    await expect(submit).toBeVisible();
    await expect(submit).toBeDisabled();
  });

  test("happy path: answer all 3 then Submit unblocks the agent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q happy path",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await session.clarificationOption("PostgreSQL").click();
    await session.clarificationOption("Go").click();
    await session.clarificationOption("Docker").click();

    // We're on the last step; Submit is enabled.
    const submit = session.clarificationSubmit();
    await expect(submit).toBeEnabled();
    await submit.click();

    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("selected_option");
  });

  test("mix custom text + option selections round-trips", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q mixed",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Q1: option click → advances.
    await session.clarificationOption("SQLite").click();

    // Q2: custom text via Enter → advances.
    const langInput = session.clarificationInputForQuestion("language");
    await langInput.click();
    await langInput.fill("Elixir");
    await langInput.press("Enter");
    await expect(session.clarificationGroupProgress()).toContainText("2 of 3 answered");

    // Q3: option click; submit batch.
    await session.clarificationOption("Bare metal").click();
    await session.clarificationSubmit().click();

    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("Elixir");
  });

  test("revising an answer via stepper jump updates the response", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q revise",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await session.clarificationOption("PostgreSQL").click();
    await session.clarificationOption("Go").click();
    await session.clarificationOption("Docker").click();

    // Jump back to Q1 and pick a different option.
    await session.clarificationStep(0).click();
    await session.clarificationOption("MongoDB").click();
    // Auto-advance brings us to Q2; jump to last step to submit.
    await session.clarificationStep(2).click();

    await session.clarificationSubmit().click();
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("MongoDB");
  });

  test("skip rejects the entire bundle from any step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q skip mid",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Answer the first one then skip from step 2.
    await session.clarificationOption("PostgreSQL").click();
    await session.clarificationSkip().click();

    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("rejected");
  });

  test("number key shortcuts pick options on the active step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q kbd",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    // Press "1" → first option of the first question, auto-advance.
    await testPage.keyboard.press("1");
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "true");
    await expect(session.clarificationGroupProgress()).toContainText("1 of 3 answered");

    // Press "2" → second option of Q2.
    await testPage.keyboard.press("2");
    await expect(session.clarificationStep(2)).toHaveAttribute("data-active", "true");
    await expect(session.clarificationGroupProgress()).toContainText("2 of 3 answered");

    // Press "1" → first option of Q3 (last step, no advance).
    await testPage.keyboard.press("1");
    await expect(session.clarificationGroupProgress()).toContainText("3 of 3 answered");

    // ArrowRight on the last step with all answered → submits.
    await testPage.keyboard.press("ArrowRight");
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  });

  test("Esc skips the entire bundle from anywhere in the carousel", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q esc",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    await session.clarificationStep(1).click();
    await testPage.keyboard.press("Escape");

    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("rejected");
  });

  test("Back button is disabled on the first step", async ({ testPage, apiClient, seedData }) => {
    const session = await seedClarificationTask(
      testPage,
      apiClient,
      seedData,
      "Multi-q back disabled",
      "clarification-multi",
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await expect(session.clarificationPrev()).toBeDisabled();
  });
});
