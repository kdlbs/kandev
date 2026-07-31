import { test, expect, type SeedData } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

const LONG_WORD = "abcdefghij".repeat(30); // 300 chars, no spaces

/** Create and open a task with one mock-agent Markdown update. */
async function openTaskWithMarkdown(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  { title, kind, text }: { title: string; kind: "message" | "thinking"; text: string },
): Promise<SessionPage> {
  await apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: `e2e:${kind}("${text}")`,
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
async function expectNoMarkdownOverflow(testPage: Page) {
  const overflows = await testPage.evaluate(() => {
    const els = document.querySelectorAll(".markdown-body");
    return Array.from(els).some((el) => el.scrollWidth > el.clientWidth + 1);
  });
  expect(overflows).toBe(false);
}

async function expectNoDocumentOverflow(testPage: Page) {
  const overflows = await testPage.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
  expect(overflows).toBe(false);
}

test.describe("Markdown text wrapping", () => {
  test("long plain text wraps within the chat message container", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Plain Text",
      kind: "message",
      text: `Here is a finding: ${LONG_WORD} - end of finding`,
    });

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

    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Inline Code",
      kind: "message",
      text: `Check this path: \`${LONG_WORD}\` for details`,
    });

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
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Code Block",
      kind: "message",
      text: `Code block:\\n\`\`\`\\nconst x = "${LONG_WORD}";\\n\`\`\`\\nAfter code block`,
    });

    await expect(session.chat.getByText("After code block").last()).toBeVisible({
      timeout: 30_000,
    });

    await expectNoMarkdownOverflow(testPage);
  });

  test("long value cells wrap inside a readable two-column table at 320px", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 320, height: 800 });

    const marker = "Long table value marker";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Long Table Value",
      kind: "message",
      text: [
        "| Field | Value |",
        "| --- | --- |",
        `| Failing checks | ${LONG_WORD} ${marker} |`,
      ].join("\\n"),
    });

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker });
    const table = markdown.locator("table");
    const tableWrapper = table.locator("xpath=..");
    const valueCell = table.locator("tbody td").nth(1);

    await expect(table).toBeVisible({ timeout: 30_000 });
    await expect(valueCell).toContainText(marker);
    expect(
      await valueCell.evaluate((cell) => {
        const range = document.createRange();
        range.selectNodeContents(cell);
        return range.getClientRects().length;
      }),
    ).toBeGreaterThan(1);
    expect(
      await table.evaluate(
        (element) => element.scrollWidth <= element.parentElement!.clientWidth + 1,
      ),
    ).toBe(true);
    await expectNoMarkdownOverflow(testPage);
    await expectNoDocumentOverflow(testPage);
    await expect(tableWrapper).toBeVisible();
  });

  test("wide tables keep readable columns and scroll internally at 320px", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 320, height: 800 });

    const marker = "Wide first cell marker";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Wide Table First Cell",
      kind: "message",
      text: [
        "| Status | Owner | Project | Branch | Review | Build | Deploy | Notes |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
        `| \`${LONG_WORD}\` ${marker} | Team Alpha | Kandev Web | main | Pending | Passing | Ready | Notes |`,
      ].join("\\n"),
    });

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker });
    const table = markdown.locator("table");
    const tableWrapper = table.locator("xpath=..");
    const firstCell = table.locator("tbody td").first();

    await expect(table).toBeVisible({ timeout: 30_000 });
    await expect(firstCell).toContainText(marker);
    expect(
      await firstCell.evaluate((cell) => cell.getBoundingClientRect().width),
    ).toBeGreaterThanOrEqual(96);
    expect(
      await firstCell.evaluate((cell) => {
        const range = document.createRange();
        range.selectNodeContents(cell);
        return range.getClientRects().length;
      }),
    ).toBeGreaterThan(1);
    expect(await firstCell.evaluate((cell) => cell.scrollWidth <= cell.clientWidth + 1)).toBe(true);
    expect(
      await tableWrapper.evaluate((element) => element.scrollWidth > element.clientWidth + 1),
    ).toBe(true);
    await expectNoMarkdownOverflow(testPage);
    await expectNoDocumentOverflow(testPage);
  });

  test("wide thinking tables scroll locally without overflowing chat at 320px", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 320, height: 800 });

    const marker = "Wide thinking table marker";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Scroll Wide Thinking Table",
      kind: "thinking",
      text: [
        "| Status | Owner | Project | Branch | Review | Build | Deploy | Notes |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
        `| Failing checks | Team Alpha | Kandev Web | main | Pending | Passing | Ready | ${marker} |`,
      ].join("\\n"),
    });

    const thinkingRow = session.activeChat().getByText("Thinking", { exact: true });
    await expect(thinkingRow).toBeVisible({ timeout: 30_000 });
    await thinkingRow.click();

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker });
    const table = markdown.locator("table");
    const tableWrapper = table.locator("xpath=..");

    await expect(table).toBeVisible();
    expect(
      await tableWrapper.evaluate((element) => element.scrollWidth > element.clientWidth + 1),
    ).toBe(true);
    await expectNoMarkdownOverflow(testPage);
    await expectNoDocumentOverflow(testPage);
  });
});
