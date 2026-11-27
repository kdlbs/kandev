import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export const PROVIDER_DIAGNOSTIC_CHUNKS = [
  "934 `dial tcp",
  " <ip>:6379:",
  " i/o timeout` err",
  "ors, meaning the TCP connect failed. A later occurrence is `dial tcp",
  " <ip>:6379:",
  " i/o timeout",
  "` on the next request.",
] as const;

export const PROVIDER_DIAGNOSTIC_RESPONSE = PROVIDER_DIAGNOSTIC_CHUNKS.join("");
export const PROVIDER_DIAGNOSTIC_RENDERED_TEXT = PROVIDER_DIAGNOSTIC_RESPONSE.replaceAll("`", "");
export const PROVIDER_DIAGNOSTIC_CODE = "dial tcp <ip>:6379: i/o timeout";

export function providerDiagnosticScript(): string {
  return PROVIDER_DIAGNOSTIC_CHUNKS.flatMap((chunk) => [
    `e2e:message(${JSON.stringify(chunk)})`,
    "e2e:delay(100)",
  ]).join("\n");
}

export async function seedProviderDiagnosticTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: providerDiagnosticScript(),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Provider diagnostic task did not create a session");
  return { task, sessionId: task.session_id };
}

export async function openProviderDiagnosticTask(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

export async function waitForProviderDiagnosticTurn(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
  session: SessionPage,
): Promise<void> {
  await waitForSessionDone(
    apiClient,
    taskId,
    sessionId,
    "provider diagnostic response should finish",
  );
  await session.waitForChatIdle({ timeout: 30_000 });
}

export async function assertProviderDiagnosticStorage(
  apiClient: ApiClient,
  sessionId: string,
): Promise<string> {
  let relevantRows: Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"] = [];
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        relevantRows = messages.filter(
          (message) =>
            message.author_type === "agent" &&
            /934|dial tcp|i\/o timeout|meaning the TCP connect failed/.test(message.content),
        );
        return {
          count: relevantRows.length,
          content: relevantRows.length === 1 ? relevantRows[0].content : null,
        };
      },
      { timeout: 30_000, message: "one complete provider diagnostic response should persist" },
    )
    .toEqual({ count: 1, content: PROVIDER_DIAGNOSTIC_RESPONSE });

  expect(relevantRows[0].content).toBe(PROVIDER_DIAGNOSTIC_RESPONSE);
  expect(relevantRows[0].id).toBeTruthy();

  const { messages } = await apiClient.listSessionMessages(sessionId);
  expect(
    messages.filter((message) => message.type === "status" && message.metadata?.retrying === true),
  ).toHaveLength(0);
  return relevantRows[0].id;
}

export async function assertProviderDiagnosticRendering(
  session: SessionPage,
  messageId: string,
): Promise<void> {
  const message = session
    .activeChat()
    .locator(`[data-agent-message-body][data-message-id="${messageId}"]`)
    .filter({ hasText: "934 dial tcp <ip>:6379:" });
  await expect(message).toHaveCount(1, { timeout: 30_000 });
  await expect(message).toHaveAttribute("data-message-id", messageId);
  await expect(message).toContainText(PROVIDER_DIAGNOSTIC_RENDERED_TEXT);

  const codeSpans = message.locator(".markdown-body code");
  await expect(codeSpans).toHaveCount(2);
  await expect(codeSpans).toHaveText([PROVIDER_DIAGNOSTIC_CODE, PROVIDER_DIAGNOSTIC_CODE]);
  await expect(message).toContainText("errors, meaning the TCP connect failed.");
  await expect(session.transientRetryCard()).toHaveCount(0);
}
