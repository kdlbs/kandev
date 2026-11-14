import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

export async function openContextCompactionScenario(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:compaction",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id ?? task.primary_session_id;
  expect(sessionId).toBeTruthy();
  if (!sessionId) throw new Error("agent task did not return a session id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.chat.getByTestId("context-compaction-message")).toHaveCount(2, {
    timeout: 30_000,
  });
  await session.waitForChatIdle({ timeout: 30_000 });
  return { taskId: task.id, sessionId, session };
}

export async function persistedCompactions(apiClient: ApiClient, sessionId: string) {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.flatMap((message) => {
    if (message.type !== "tool_call" || !message.metadata) return [];
    const normalized = message.metadata.normalized;
    if (!normalized || typeof normalized !== "object") return [];
    const payload = normalized as { kind?: unknown; context_compaction?: unknown };
    if (payload.kind !== "context_compaction" || !payload.context_compaction) return [];
    return [payload.context_compaction as Record<string, unknown>];
  });
}
