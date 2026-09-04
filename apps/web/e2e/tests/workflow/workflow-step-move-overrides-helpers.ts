import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const MOVE_INSTRUCTIONS = "Reproduce the checkout failure before editing.";

/** Durable e2e:message emitted by the target step's configured prompt. Its
 * presence proves the step prompt ran; skip_step_prompt suppresses it. */
export const TARGET_STEP_MESSAGE = "verification started";

export type MoveOverrideFixture = {
  taskId: string;
  workflowId: string;
  sourceStepId: string;
  targetStepId: string;
  /** The task's single primary session. Moves apply in place; this never changes. */
  primarySessionId: string;
  session: SessionPage;
};

export async function seedMoveOverrideFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
): Promise<MoveOverrideFixture> {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} Workflow`);
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Spec", 0, {
    is_start_step: true,
  });
  const targetStep = await apiClient.createWorkflowStep(workflow.id, "Verify", 1, {
    auto_advance_requires_signal: true,
  });
  await apiClient.updateWorkflowStep(sourceStep.id, {
    prompt: 'e2e:message("spec ready")\n{{task_prompt}}',
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  await apiClient.updateWorkflowStep(targetStep.id, {
    prompt: `e2e:message("${TARGET_STEP_MESSAGE}")\n{{task_prompt}}`,
    events: { on_enter: [{ type: "auto_start_agent" }] },
    auto_advance_requires_signal: true,
  });

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    `${name} Task`,
    seedData.agentProfileId,
    {
      workflow_id: workflow.id,
      workflow_step_id: sourceStep.id,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });

  const primarySessionId = (await apiClient.getTask(task.id)).primary_session_id ?? "";
  if (!primarySessionId) throw new Error("move override fixture task has no primary session");

  return {
    taskId: task.id,
    workflowId: workflow.id,
    sourceStepId: sourceStep.id,
    targetStepId: targetStep.id,
    primarySessionId,
    session,
  };
}

/**
 * Toggle the one-shot move-entry controls. Defaults reflect the common
 * reset-context + instructions case; pass explicit flags for the skip cases.
 * Pass an empty `instructions` to leave the field untouched.
 */
export async function fillMoveOverrides(
  page: Page,
  options: { instructions?: string; resetContext?: boolean; skipStepPrompt?: boolean } = {},
): Promise<void> {
  const { instructions = MOVE_INSTRUCTIONS, resetContext = true, skipStepPrompt = false } = options;
  if (resetContext) {
    await page.getByTestId("workflow-move-reset-context").click();
  }
  if (skipStepPrompt) {
    await page.getByTestId("workflow-move-skip-step-prompt").click();
  }
  if (instructions) {
    await page.getByTestId("workflow-move-instructions").fill(instructions);
  }
}

export function waitForMoveRequest(page: Page, taskId: string) {
  return page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      new URL(request.url()).pathname === `/api/v1/tasks/${taskId}/move`,
  );
}

/**
 * Verify the one-shot move instructions reached the session and rendered.
 *
 * Chat collapses the move-instructions block behind a labeled toggle
 * (`workflow-move-instructions-toggle`), so the raw instruction text is hidden
 * until the toggle is expanded. Wait for the toggle on the delivered user
 * message, expand it, and assert the instruction text is then visible.
 *
 * Scope to the visible chat panel via `activeChat()`: dockview keeps background
 * session-chat panels mounted, so the unscoped `session.chat` locator can match
 * a hidden panel's toggle whose click never resolves (visible-but-not-stable).
 */
export async function expectMoveInstructionsDelivered(session: SessionPage): Promise<void> {
  const chat = session.activeChat();
  const toggle = chat.getByTestId("workflow-move-instructions-toggle").first();
  await expect(toggle).toBeVisible({ timeout: 30_000 });
  await toggle.click();
  await expect(chat.getByText(MOVE_INSTRUCTIONS, { exact: false }).first()).toBeVisible();
}
