import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { waitForLatestSessionDone } from "../../helpers/session";
import {
  SAVED_PROMPT_CONTENT,
  SAVED_PROMPT_DELIVERY_RESPONSE,
} from "../chat/quick-chat-saved-prompt-delivery-helpers";

export const MOVE_INSTRUCTIONS = "Reproduce the checkout failure before editing.";

export async function selectMoveSavedPrompt(
  page: Page,
  apiClient: ApiClient,
  input: "keyboard" | "mouse" | "touch",
) {
  const prompt = await apiClient.createPrompt("move-review-checklist", SAVED_PROMPT_CONTENT);
  try {
    const editor = page.getByRole("textbox", { name: "One-time instructions" });
    const prefix = `${MOVE_INSTRUCTIONS} Follow `;
    await editor.fill(prefix);
    const form = editor.locator(
      'xpath=ancestor::*[@data-slot="context-menu-sub-content" or @data-slot="popover-content"][1]',
    );
    const initialHeight =
      input === "touch" ? null : await form.evaluate((node) => node.offsetHeight);
    await editor.pressSequentially("@");
    if (initialHeight !== null) {
      const dropdown = page.getByTestId("mention-menu");
      await expect(dropdown.getByRole("option").first()).toBeVisible();
      await expect
        .poll(async () => {
          const [menu, field, height] = await Promise.all([
            dropdown.boundingBox(),
            editor.boundingBox(),
            form.evaluate((node) => node.offsetHeight),
          ]);
          return { height, above: Boolean(menu && field && menu.y + menu.height <= field.y) };
        })
        .toEqual({ height: initialHeight, above: true });
    }
    await editor.pressSequentially("move-review");
    const option = page.getByRole("option", { name: prompt.name, exact: true });
    await expect(option).toBeVisible();
    if (input === "touch") {
      expect((await option.boundingBox())?.height).toBeGreaterThanOrEqual(44);
      await option.tap();
    } else if (input === "keyboard") {
      await editor.press("Escape");
      await expect(option).toBeHidden();
      await editor.pressSequentially("-");
      await expect(option).toBeVisible();
      await editor.press("Enter");
    } else {
      await option.click();
    }
    await expect(option).toBeHidden();
    await expect(editor.getByTestId("custom-prompt-mention")).toContainText(`@${prompt.name}`);
    return { id: prompt.id, instructions: `${prefix}@${prompt.name}` };
  } catch (error) {
    await apiClient.deletePrompt(prompt.id).catch(() => undefined);
    throw error;
  }
}

export async function expectMoveSavedPromptDelivered(apiClient: ApiClient, sessionId: string) {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.filter(
          (message) =>
            message.author_type === "agent" &&
            message.content.includes(SAVED_PROMPT_DELIVERY_RESPONSE),
        ).length;
      },
      { timeout: 30_000 },
    )
    .toBe(1);
}

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

/** Creates an idle task with an isolated workflow and one primary session for in-place moves. */
export async function seedMoveOverrideFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  name: string,
  options: { targetStepName?: string; sourcePrompt?: string } = {},
): Promise<MoveOverrideFixture> {
  const workflow = await apiClient.createWorkflow(seedData.workspaceId, `${name} Workflow`);
  const sourceStep = await apiClient.createWorkflowStep(workflow.id, "Spec", 0, {
    is_start_step: true,
  });
  const targetStep = await apiClient.createWorkflowStep(
    workflow.id,
    options.targetStepName ?? "Verify",
    1,
    {
      auto_advance_requires_signal: true,
    },
  );
  await apiClient.updateWorkflowStep(sourceStep.id, {
    prompt: options.sourcePrompt ?? 'e2e:message("spec ready")\n{{task_prompt}}',
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
  await waitForLatestSessionDone(
    apiClient,
    task.id,
    1,
    "Move fixture agent must finish before navigation",
    30_000,
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
