import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";

export const FORK_SOURCE_USER = "conversation-fork-source-user-marker";
export const FORK_SOURCE_TOOL = "conversation-fork-source-tool-marker";
export const FORK_SOURCE_ASSISTANT = "conversation-fork-source-assistant-marker";
export const FORK_SOURCE_AFTER_CUTOFF = "conversation-fork-after-cutoff-marker";
export const FORK_NEW_INSTRUCTION = "/e2e:simple-message";

export async function seedConversationForkSource(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
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
  if (!task.session_id) throw new Error("conversation-fork source did not create a session");
  await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for fork source session");

  const { messages } = await apiClient.listSessionMessages(task.session_id);
  const latest = messages.reduce(
    (value, message) => Math.max(value, Date.parse(message.created_at)),
    0,
  );
  const base = latest + 2_000;
  const timestamp = (offset: number) => new Date(base + offset).toISOString();
  const turnStartedAt = timestamp(1_000);
  const turnCompletedAt = timestamp(2_000);
  const firstMessage = await apiClient.seedSessionMessage(task.session_id, {
    type: "message",
    authorType: "user",
    content: FORK_SOURCE_USER,
    createdAt: timestamp(0),
  });
  await apiClient.seedSessionMessage(task.session_id, {
    type: "tool_execute",
    authorType: "agent",
    content: "Recorded command output for the fork preview.",
    metadata: {
      status: "completed",
      normalized: {
        kind: "shell_exec",
        shell_exec: {
          command: "printf conversation-fork-tool-marker",
          output: { stdout: FORK_SOURCE_TOOL, stderr: "", exit_code: 0 },
        },
      },
    },
    createdAt: timestamp(1_000),
  });
  const cutoff = await apiClient.seedSessionMessage(task.session_id, {
    type: "message",
    authorType: "agent",
    content: FORK_SOURCE_ASSISTANT,
    createdAt: turnCompletedAt,
    newTurn: true,
    turnStartedAt,
    turnCompletedAt,
  });
  await apiClient.seedSessionMessage(task.session_id, {
    type: "message",
    authorType: "user",
    content: FORK_SOURCE_AFTER_CUTOFF,
    createdAt: timestamp(3_000),
  });

  return {
    taskId: task.id,
    sessionId: task.session_id,
    startMessageId: firstMessage.messageId,
    cutoffMessageId: cutoff.messageId,
  };
}
