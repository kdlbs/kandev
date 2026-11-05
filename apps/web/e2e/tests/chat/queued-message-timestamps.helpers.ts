import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { QueueSessionIdentityInput } from "../../helpers/api-client";

export async function seedTimestampQueue(api: ApiClient, seed: SeedData) {
  const task = await api.createTask(seed.workspaceId, "Queue timestamps", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  const { session_id: sessionId } = await api.seedTaskSession(task.id, {
    state: "IDLE",
    agentProfileId: seed.agentProfileId,
  });
  const identity = await api.getQueueSessionIdentity(task.id, sessionId);
  await api.setQueueAutoRun(identity, false);
  await api.setQueueAutoMerge(identity, false);
  return { task, identity };
}

export function queuePayload(identity: QueueSessionIdentityInput) {
  return {
    task_id: identity.taskId,
    session_id: identity.sessionId,
    session_incarnation_id: identity.sessionIncarnationId,
  };
}

export async function timestampEntries(api: ApiClient, identity: QueueSessionIdentityInput) {
  const result = await api.wsRequest<{
    entries: { id: string; content: string; queued_at: string }[];
  }>("message.queue.get", queuePayload(identity));
  return result.entries;
}

export async function openTimestampQueue(page: Page, taskId: string) {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  const chat = session.activeChat();
  await chat.getByTestId("queue-chip").click();
  const panel = chat.getByTestId("queued-ghost-list");
  await expect(panel).toBeVisible();
  return panel;
}
