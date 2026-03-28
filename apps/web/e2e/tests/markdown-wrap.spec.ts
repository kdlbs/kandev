import { test, expect } from "../fixtures/test-base";
import { KanbanPage } from "../pages/kanban-page";
import { SessionPage } from "../pages/session-page";

const LONG_WORD = "abcdefghij".repeat(30); // 300 chars, no spaces

/** Create a task with a mock agent message containing the given text. */
async function createTaskWithMessage(
  testPage: import("@playwright/test").Page,
  apiClient: import("../helpers/api-client").ApiClient,
  seedData: import("../fixtures/test-base").SeedData,
  title: string,
  messageText: string,
): Promise<SessionPage> {
  await apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: `e2e:message("${messageText}")`,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });

  const kanban = new KanbanPage(testPage);
  await kanban.goto();

  const card = kanban.taskCardByTitle(title);
  await expect(card).toBeVisible({ timeout: 30_000 });
  await card.click();
  await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  return session;
}

/** Assert no .markdown-body element overflows horizontally. */
async function expectNoMarkdownOverflow(testPage: import("@playwright/test").Page) {
  const overflows = await testPage.evaluate(() => {
    const els = document.querySelectorAll(".markdown-body");
    return Array.from(els).some((el) => el.scrollWidth > el.clientWidth + 1);
  });
  expect(overflows).toBe(false);
}

test.describe("Markdown text wrapping", () => {
  test("long plain text wraps within the chat message container", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await createTaskWithMessage(
      testPage,
      apiClient,
      seedData,
      "Wrap Plain Text",
      `Here is a finding: ${LONG_WORD} - end of finding`,
    );

    await expect(session.chat.getByText("end of finding").last()).toBeVisible({
      timeout: 30_000,
    });

    await expectNoMarkdownOverflow(testPage);
  });

  test("long inline code wraps within the chat message container", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await createTaskWithMessage(
      testPage,
      apiClient,
      seedData,
      "Wrap Inline Code",
      `Check this path: \`${LONG_WORD}\` for details`,
    );

    await expect(session.chat.getByText("for details").last()).toBeVisible({
      timeout: 30_000,
    });

    await expectNoMarkdownOverflow(testPage);
  });

  test("long lines in code blocks do not overflow the message container", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Fenced code block with a long line — should be contained via
    // horizontal scroll (shiki) or line wrapping (codemirror)
    const session = await createTaskWithMessage(
      testPage,
      apiClient,
      seedData,
      "Wrap Code Block",
      `Code block:\\n\`\`\`\\nconst x = "${LONG_WORD}";\\n\`\`\`\\nAfter code block`,
    );

    await expect(session.chat.getByText("After code block").last()).toBeVisible({
      timeout: 30_000,
    });

    await expectNoMarkdownOverflow(testPage);
  });
});
