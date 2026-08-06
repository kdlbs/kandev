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

  test("ordinary two-column tables wrap instead of scrolling at the 255px chat width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 303, height: 800 });

    const marker = "dependency lifecycle scripts";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Wrap Pnpm Settings Table",
      kind: "message",
      text: [
        "| pnpm setting | Effect |",
        "| --- | --- |",
        "| default (pnpm 10+) | dependency lifecycle scripts **off** unless approved |",
        "| `strictDepBuilds: true` | install **fails** if a dep wants a build and isn't allowlisted |",
        "| `allowBuilds.esbuild: true` | only then may esbuild's postinstall run |",
      ].join("\\n"),
    });

    const table = session
      .activeChat()
      .locator(".markdown-body", { hasText: marker })
      .locator("table");
    const markdown = table.locator(
      "xpath=ancestor::div[contains(concat(' ', normalize-space(@class), ' '), ' markdown-body ')]",
    );
    const tableWrapper = table.locator("xpath=..");
    const firstColumnCode = table.locator("tbody tr").nth(1).locator("td").first().locator("code");

    await expect(table).toBeVisible({ timeout: 30_000 });
    expect(await tableWrapper.evaluate((element) => element.clientWidth)).toBe(255);
    expect(
      await firstColumnCode.evaluate((code) => {
        const range = document.createRange();
        range.selectNodeContents(code);
        return range.getClientRects().length;
      }),
    ).toBeGreaterThan(1);
    expect(
      await tableWrapper.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    expect(await table.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
      true,
    );
    expect(
      await markdown.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    expect(
      await session
        .activeChat()
        .evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    await expectNoMarkdownOverflow(testPage);
    await expectNoDocumentOverflow(testPage);
  });

  test("desktop users resize adjacent table columns from the full-height boundary", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 1100, height: 800 });

    const marker = "Resizable table marker";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Resize Markdown Table Columns",
      kind: "message",
      text: [
        "| Setting | Effect | Notes |",
        "| --- | --- | --- |",
        `| strictDepBuilds | Install fails for unapproved lifecycle scripts | ${marker} |`,
        "| allowBuilds | Approved packages may run build scripts | Ephemeral adjustment |",
      ].join("\\n"),
    });

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker });
    const table = markdown.locator("table");
    const cells = table.locator("tbody tr").first().locator("td");
    const separator = markdown.getByTestId("markdown-table-resizer-0");

    await expect(table).toBeVisible({ timeout: 30_000 });
    await expect(separator).toBeVisible();

    const initialWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    await expect(separator).toHaveAttribute("role", "separator");
    await expect(separator).toHaveAccessibleName("Resize table columns 1 and 2");
    await expect(separator).toHaveAttribute("aria-orientation", "vertical");
    await expect(separator).toHaveAttribute("aria-valuemin", "64");
    await expect(separator).toHaveAttribute("aria-valuenow", String(Math.round(initialWidths[0])));
    expect(Number(await separator.getAttribute("aria-valuemax"))).toBeGreaterThan(initialWidths[0]);
    const initialTableWidth = await table.evaluate(
      (element) => element.getBoundingClientRect().width,
    );
    const [separatorBox, bodyCellBox, tableBox] = await Promise.all([
      separator.boundingBox(),
      cells.first().boundingBox(),
      table.boundingBox(),
    ]);
    expect(separatorBox).not.toBeNull();
    expect(bodyCellBox).not.toBeNull();
    expect(tableBox).not.toBeNull();
    expect(separatorBox!.height).toBeCloseTo(tableBox!.height, 1);

    const boundaryX = separatorBox!.x + separatorBox!.width / 2;
    const bodyRowY = bodyCellBox!.y + bodyCellBox!.height / 2;
    await testPage.mouse.move(boundaryX, bodyRowY);
    await testPage.mouse.down();
    await testPage.mouse.move(boundaryX + 60, bodyRowY);
    await testPage.mouse.up();

    const resizedWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    expect(resizedWidths[0] - initialWidths[0]).toBeCloseTo(60, 0);
    expect(initialWidths[1] - resizedWidths[1]).toBeCloseTo(60, 0);
    expect(resizedWidths[2]).toBeCloseTo(initialWidths[2], 0);
    expect(await table.evaluate((element) => element.getBoundingClientRect().width)).toBeCloseTo(
      initialTableWidth,
      0,
    );

    const movedSeparatorBox = await separator.boundingBox();
    const movedBodyCellBox = await cells.first().boundingBox();
    expect(movedSeparatorBox).not.toBeNull();
    expect(movedBodyCellBox).not.toBeNull();
    const movedBoundaryX = movedSeparatorBox!.x + movedSeparatorBox!.width / 2;
    const movedBodyRowY = movedBodyCellBox!.y + movedBodyCellBox!.height / 2;
    await testPage.mouse.move(movedBoundaryX, movedBodyRowY);
    await testPage.mouse.down();
    await testPage.mouse.move(movedBoundaryX + 1000, movedBodyRowY);
    await testPage.mouse.up();
    const clampedWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    expect(clampedWidths[1]).toBeCloseTo(64, 0);
    expect(clampedWidths[2]).toBeCloseTo(initialWidths[2], 0);

    await separator.dblclick();
    const resetWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    resetWidths.forEach((width, index) => expect(width).toBeCloseTo(initialWidths[index], 0));

    await separator.focus();
    await testPage.keyboard.press("ArrowRight");
    const keyboardWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    expect(keyboardWidths[0] - initialWidths[0]).toBeCloseTo(8, 0);
    expect(initialWidths[1] - keyboardWidths[1]).toBeCloseTo(8, 0);
    await testPage.keyboard.press("Enter");
    const keyboardResetWidths = await cells.evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().width),
    );
    keyboardResetWidths.forEach((width, index) =>
      expect(width).toBeCloseTo(initialWidths[index], 0),
    );

    await testPage.keyboard.press("ArrowRight");
    await expect(table.locator("colgroup")).toHaveCount(1);
    await testPage.setViewportSize({ width: 600, height: 800 });
    await expect(markdown.getByTestId(/^markdown-table-resizer-/)).toHaveCount(0);
    await expect(table.locator("colgroup")).toHaveCount(0);
    await expectNoDocumentOverflow(testPage);
  });

  test("wide desktop table separators stay aligned inside local scrolling", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 900, height: 800 });

    const marker = "Scrollable resizer marker";
    const session = await openTaskWithMarkdown(testPage, apiClient, seedData, {
      title: "Scroll Resizable Markdown Table",
      kind: "message",
      text: [
        "| Status | Owner | Project | Branch | Review | Build | Deploy | Notes |",
        "| --- | --- | --- | --- | --- | --- | --- | --- |",
        `| Failing checks | Team Alpha | Kandev Web | main | Pending | Passing | Ready | ${marker} |`,
      ].join("\\n"),
    });

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker });
    const table = markdown.locator("table");
    const wrapper = table.locator("xpath=..");
    const separator = markdown.getByTestId("markdown-table-resizer-2");
    const thirdHeader = table.locator("thead th").nth(2);

    await expect(separator).toBeVisible({ timeout: 30_000 });
    expect(await wrapper.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
      true,
    );
    await wrapper.evaluate((element) => {
      element.scrollLeft = 160;
    });
    const [separatorBox, headerBox] = await Promise.all([
      separator.boundingBox(),
      thirdHeader.boundingBox(),
    ]);
    expect(separatorBox).not.toBeNull();
    expect(headerBox).not.toBeNull();
    expect(separatorBox!.x + separatorBox!.width / 2).toBeCloseTo(
      headerBox!.x + headerBox!.width,
      0,
    );
    await expectNoMarkdownOverflow(testPage);
    await expectNoDocumentOverflow(testPage);
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
