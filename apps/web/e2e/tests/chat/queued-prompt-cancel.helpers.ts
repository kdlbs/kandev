import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { typeWhileBusy, waitForComposerQueueMode } from "../../helpers/type-while-busy";
import { SessionPage } from "../../pages/session-page";

export const QUEUED_CANCEL_PROMPT = "/e2e:cancel-hold";
export const PARKED_PROMPT = "parked message after queued pause";
export const FOLLOW_UP_PROMPT = 'e2e:message("queued-pause-follow-up-response")';
export const FOLLOW_UP_RESPONSE = "queued-pause-follow-up-response";

export async function seedQueuedPauseSession(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<{
  session: SessionPage;
  taskId: string;
  sessionId: string;
  executionId: string;
}> {
  // These tests cover pause and queue ownership, so cancellation must not move
  // the workflow to its next step.
  await apiClient.updateWorkflowStep(seedData.startStepId, {
    cancel_triggers_turn_complete: false,
  });
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
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  const { sessions } = await apiClient.listTaskSessions(task.id);
  const executionId = sessions.find(
    (candidate) => candidate.id === task.session_id,
  )?.agent_execution_id;
  if (!executionId) throw new Error("task session has no accepted agent execution");
  return { session, taskId: task.id, sessionId: task.session_id, executionId };
}

export async function queueFromBusyComposer(
  page: Page,
  session: SessionPage,
  prompt: string,
  mobile: boolean,
): Promise<void> {
  await waitForComposerQueueMode(session.activeChat());
  const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
  await typeWhileBusy(page, editor, prompt);
  if (mobile) {
    await session.tapSubmitWhenReady();
  } else {
    await session.clickSubmitWhenReady();
  }
}

export async function waitForAcceptedQueuedTurn(
  apiClient: ApiClient,
  sessionId: string,
  prompt: string,
  acceptanceMarker: string,
): Promise<{ messageId: string; turnId: string }> {
  let lastObservedState = "not observed";
  const matchingMessage = async () => {
    const { messages } = await apiClient.listSessionMessages(sessionId);
    return [...messages]
      .reverse()
      .find(
        (message) =>
          message.author_type.toLowerCase() === "user" && message.content.trim() === prompt,
      );
  };

  await expect
    .poll(
      async () => {
        const message = await matchingMessage();
        const [{ turns }, { messages }] = await Promise.all([
          apiClient.listSessionTurns(sessionId),
          apiClient.listSessionMessages(sessionId),
        ]);
        const openTurn = message?.turn_id
          ? turns.find((turn) => turn.id === message.turn_id && !turn.completed_at)
          : undefined;
        const matchingAgentMessage = message?.turn_id
          ? messages.find(
              (candidate) =>
                candidate.author_type.toLowerCase() === "agent" &&
                candidate.turn_id === message.turn_id &&
                candidate.content.includes(acceptanceMarker),
            )
          : undefined;
        lastObservedState = JSON.stringify({
          matchingUserMessage: message && {
            authorType: message.author_type,
            content: message.content.slice(-240),
            turnId: message.turn_id,
          },
          matchingUserTurnIsOpen: Boolean(openTurn),
          markerOnSameTurn: Boolean(matchingAgentMessage),
          recentAgentMessages: messages
            .filter((candidate) => candidate.author_type.toLowerCase() === "agent")
            .slice(-6)
            .map((candidate) => ({
              content: candidate.content.slice(-240),
              turnId: candidate.turn_id,
            })),
        });
        return Boolean(openTurn && matchingAgentMessage);
      },
      { timeout: 30_000, message: "provider should accept the queued prompt on its live turn" },
    )
    .toBe(true)
    .catch((error: unknown) => {
      throw new Error(`${String(error)}\nLast persisted acceptance state: ${lastObservedState}`);
    });

  const accepted = await matchingMessage();
  if (!accepted?.turn_id) throw new Error("accepted queued prompt has no turn identity");
  return { messageId: accepted.id, turnId: accepted.turn_id };
}

export async function waitForSessionExecution(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string | undefined> {
  const { sessions } = await apiClient.listTaskSessions(taskId);
  return sessions.find((session) => session.id === sessionId)?.agent_execution_id;
}
