// Filename routes this semantic chat regression to mobile-chrome (Pixel 5).
import { test, expect } from "../../fixtures/test-base";
import {
  assertMarkdownSeparatorRendering,
  MARKDOWN_SEPARATOR_FIXTURE,
  openMarkdownSeparatorTask,
  seedMarkdownSeparatorTask,
  assertNoDocumentHorizontalOverflow,
} from "./markdown-separators-helpers";

test.describe("mobile: Markdown prose separators", () => {
  test("preserves separator semantics and raw content after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { task, sessionId } = await seedMarkdownSeparatorTask(
      apiClient,
      seedData,
      "Markdown separator mobile regression",
    );
    const session = await openMarkdownSeparatorTask(testPage, task.id);
    await assertMarkdownSeparatorRendering(session);
    await assertNoDocumentHorizontalOverflow(testPage);

    await testPage.reload();
    await session.waitForLoad();
    await assertMarkdownSeparatorRendering(session);
    await assertNoDocumentHorizontalOverflow(testPage);

    const { messages } = await apiClient.listSessionMessages(sessionId);
    expect(messages.some((message) => message.content === MARKDOWN_SEPARATOR_FIXTURE)).toBe(true);
  });
});
