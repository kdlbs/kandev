import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const SEPARATOR_BODY_MARKER = "PROSE_SEPARATOR_BODY";
export const FENCED_LITERAL_MARKER = "PROSE_SEPARATOR_FENCED_LITERAL";

export const MARKDOWN_SEPARATOR_FIXTURE = [
  "Skills to change",
  "---",
  `This is ${SEPARATOR_BODY_MARKER}, a long paragraph that must remain body text before the following horizontal separator in chat rendering.`,
  "---",
  "Next section",
  "",
  "~~~text",
  FENCED_LITERAL_MARKER,
  "---",
  "~~~",
].join("\n");

type StoredMessage = Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"][number];

export async function seedMarkdownSeparatorTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: `e2e:message(${JSON.stringify(MARKDOWN_SEPARATOR_FIXTURE)})`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Markdown separator task did not create a session");

  let storedMessage: StoredMessage | undefined;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        storedMessage = messages.find((message) => message.content === MARKDOWN_SEPARATOR_FIXTURE);
        return storedMessage?.content ?? null;
      },
      {
        timeout: 30_000,
        message: "Waiting for the complete raw markdown separator fixture to persist",
      },
    )
    .toBe(MARKDOWN_SEPARATOR_FIXTURE);

  return { task, sessionId: task.session_id, storedMessage: storedMessage! };
}

export async function openMarkdownSeparatorTask(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  return session;
}

export async function assertMarkdownSeparatorRendering(session: SessionPage): Promise<void> {
  const body = session
    .activeChat()
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: SEPARATOR_BODY_MARKER });
  await expect(body).toHaveCount(1, { timeout: 30_000 });

  const markdown = body.locator(".markdown-body");
  await expect(markdown).toBeVisible();
  await expect(markdown.locator("h2")).toHaveCount(1);
  await expect(markdown.locator("h2")).toHaveText("Skills to change");
  await expect(markdown.locator("hr")).toHaveCount(1);
  await expect(markdown.locator("p").filter({ hasText: SEPARATOR_BODY_MARKER })).toHaveCount(1);
  await expect(markdown.getByText("Next section", { exact: true })).toBeVisible();

  const fencedCode = markdown.locator("pre code").filter({ hasText: FENCED_LITERAL_MARKER });
  await expect(fencedCode).toHaveCount(1);
  await expect(fencedCode).toContainText("---");
}

export async function assertNoDocumentHorizontalOverflow(page: Page): Promise<void> {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
    ),
  ).toBe(true);
}
