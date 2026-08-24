// Regression guard for docs/specs/current-turn-authority/spec.md: a
// near-simultaneous duplicate task_session_turns row (the synthetic
// "lifecycle" turn Service.createCompletedTurn writes on agent resume) must
// never outrank a session's real open turn when the UI resolves which turn
// is "current" — otherwise a pending clarification on the open turn is
// silently hidden from the chat overlay. Reproduces Scenarios 1 and 3 from
// the spec via the e2e test harness (real turn resolution end to end, not
// just the pure newestDurableTurnId unit test AC-10 already covers).
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

const QUESTION_PROMPT = "Which environment should I deploy to?";
const STAGING_LABEL = "Staging";
const PRODUCTION_LABEL = "Production";

/**
 * Seeds a session with a real open turn carrying a pending clarification,
 * plus a second turn started 0.8s later that shadows it in "latest by
 * started_at" ordering. `markedLifecycle` selects between AC-9's marked
 * `lifecycle_only: true` shape (Scenario 1) and AC-6's unmarked legacy
 * zero-duration shape (Scenario 3) — both must resolve to the open turn.
 */
async function seedShadowedClarification(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  markedLifecycle: boolean,
): Promise<{ taskId: string; sessionId: string }> {
  const task = await apiClient.createTask(seedData.workspaceId, title, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
  });

  const turnAStartedAt = new Date(Date.now() - 120_000).toISOString();
  const turnBStartedAt = new Date(Date.now() - 119_200).toISOString();

  await apiClient.seedSessionMessage(sessionId, {
    type: "clarification_request",
    newTurn: true,
    turnStartedAt: turnAStartedAt,
    metadata: {
      pending_id: "pend-shadow-1",
      session_id: sessionId,
      question_id: "q1",
      question_index: 0,
      question_total: 1,
      status: "pending",
      question: {
        id: "q1",
        title: "Deploy target",
        prompt: QUESTION_PROMPT,
        options: [
          { option_id: "opt-staging", label: STAGING_LABEL, description: "" },
          { option_id: "opt-prod", label: PRODUCTION_LABEL, description: "" },
        ],
      },
    },
  });

  await apiClient.seedSessionMessage(sessionId, {
    type: "script_execution",
    content: "Resuming session.",
    newTurn: true,
    turnStartedAt: turnBStartedAt,
    turnCompletedAt: turnBStartedAt,
    ...(markedLifecycle ? { turnMetadata: { lifecycle_only: true } } : {}),
  });

  return { taskId: task.id, sessionId };
}

test.describe("Duplicate lifecycle turn does not hide a pending clarification", () => {
  test("a marked lifecycle turn written after an open turn does not shadow its clarification (AC-9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId } = await seedShadowedClarification(
      apiClient,
      seedData,
      "Lifecycle shadow - marked",
      true,
    );

    const wsWatcher = watchWs(testPage);
    const messagesLoaded = wsWatcher.waitForResponse("message.list");
    await testPage.goto(`/t/${taskId}`);
    await messagesLoaded;
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.clarificationOverlay()).toBeVisible();
    await expect(session.clarificationOverlay()).toContainText(QUESTION_PROMPT);

    const staging = session.clarificationOption(STAGING_LABEL);
    await expect(staging).toBeVisible();
    await staging.click();
    await expect(staging).toHaveAttribute("data-selected", "true");
  });

  test("an unmarked legacy zero-duration turn over an open turn still resolves to the open turn (AC-6)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { taskId } = await seedShadowedClarification(
      apiClient,
      seedData,
      "Lifecycle shadow - unmarked legacy",
      false,
    );

    const wsWatcher = watchWs(testPage);
    const messagesLoaded = wsWatcher.waitForResponse("message.list");
    await testPage.goto(`/t/${taskId}`);
    await messagesLoaded;
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.clarificationOverlay()).toBeVisible();
    await expect(session.clarificationOverlay()).toContainText(QUESTION_PROMPT);

    const production = session.clarificationOption(PRODUCTION_LABEL);
    await expect(production).toBeVisible();
    await production.click();
    await expect(production).toHaveAttribute("data-selected", "true");
  });
});
