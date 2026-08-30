// Filename routes this phone-width overflow regression to mobile-chrome (Pixel 5).
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile: Markdown table wrapping", () => {
  test("ordinary two-column pnpm tables wrap without overflowing the chat or document", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const marker = "dependency lifecycle scripts";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Wrap Pnpm Settings Table",
      seedData.agentProfileId,
      {
        description: `e2e:message("${[
          "| pnpm setting | Effect |",
          "| --- | --- |",
          "| default (pnpm 10+) | dependency lifecycle scripts **off** unless approved |",
          "| `strictDepBuilds: true` | install **fails** if a dep wants a build and isn't allowlisted |",
          "| `allowBuilds.esbuild: true` | only then may esbuild's postinstall run |",
        ].join("\\n")}")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

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
    await expect(markdown.getByTestId(/^markdown-table-resizer-/)).toHaveCount(0);
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
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });

  test("wide thinking tables scroll locally without overflowing the chat or document", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const marker = "Mobile wide thinking table marker";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Scroll Wide Thinking Table",
      seedData.agentProfileId,
      {
        description: `e2e:thinking("${[
          "| Status | Owner | Project | Branch | Review | Build | Deploy | Notes |",
          "| --- | --- | --- | --- | --- | --- | --- | --- |",
          `| Failing checks | Team Alpha | Kandev Web | main | Pending | Passing | Ready | ${marker} |`,
        ].join("\\n")}")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const thinkingRow = session.activeChat().getByText("Thinking", { exact: true });
    await expect(thinkingRow).toBeVisible({ timeout: 30_000 });
    await thinkingRow.click();

    const markdown = session.activeChat().locator(".markdown-body", { hasText: marker }).last();
    const table = markdown.locator("table");
    const tableWrapper = table.locator("xpath=..");

    await expect(table).toBeVisible();
    await expect(markdown.getByTestId(/^markdown-table-resizer-/)).toHaveCount(0);
    expect(
      await tableWrapper.evaluate((element) => element.scrollWidth > element.clientWidth + 1),
    ).toBe(true);
    expect(
      await markdown.evaluate((element) => element.scrollWidth <= element.clientWidth + 1),
    ).toBe(true);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });

  test("thinking preview stays visible and contained before expansion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const firstLine =
      "First meaningful reasoning summary stays visible while this long subject is truncated to the available mobile row width";
    const laterLine = "Later reasoning detail remains available after expansion";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Thinking Message Preview",
      seedData.agentProfileId,
      {
        description: `e2e:thinking("${["", "## ", `**${firstLine}**`, "", laterLine].join("\\n")}")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("preview task did not create a session");

    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) =>
              message.type === "thinking" &&
              String(message.metadata?.thinking ?? message.content ?? "").includes(firstLine),
          );
        },
        { timeout: 30_000, message: "Waiting for thinking preview content to persist" },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    const preview = chat.getByTestId("thinking-message-preview");
    await expect(preview).toBeVisible();
    await expect(preview).toHaveText(firstLine, { exact: true });
    await expect(chat.getByText(laterLine, { exact: true })).toHaveCount(0);

    const previewMetrics = await preview.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
      clientHeight: element.clientHeight,
      lineHeight: Number.parseFloat(getComputedStyle(element).lineHeight),
    }));
    expect(previewMetrics.clientWidth).toBeGreaterThan(0);
    expect(previewMetrics.scrollWidth).toBeGreaterThan(previewMetrics.clientWidth);
    expect(previewMetrics.clientHeight).toBeLessThanOrEqual(previewMetrics.lineHeight + 1);
    expect(await chat.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
      true,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);

    await chat.getByText("Thinking", { exact: true }).tap();
    await expect(chat.getByText(laterLine, { exact: true })).toBeVisible();
  });

  test("compact thinking preview stays contained on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const compactLine =
      "Compact thinking line remains within the available mobile row width for safe rendering";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Compact Thinking Preview",
      seedData.agentProfileId,
      {
        description: `e2e:thinking("${compactLine}")`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("compact preview task did not create a session");

    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          return messages.some(
            (message) =>
              message.type === "thinking" &&
              String(message.metadata?.thinking ?? message.content ?? "").includes(compactLine),
          );
        },
        { timeout: 30_000, message: "Waiting for compact thinking content to persist" },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const chat = session.activeChat();
    const compact = chat.getByText(compactLine, { exact: true });
    await expect(compact).toBeVisible();
    await expect(chat.getByTestId("thinking-message-preview")).toHaveCount(0);

    const compactMetrics = await compact.evaluate((element) => ({
      clientWidth: element.clientWidth,
      scrollWidth: element.scrollWidth,
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      lineHeight: Number.parseFloat(getComputedStyle(element).lineHeight),
      text: element.textContent,
      whiteSpace: getComputedStyle(element).whiteSpace,
    }));
    expect(compactMetrics.clientWidth).toBeGreaterThan(0);
    expect(compactMetrics.scrollWidth).toBeLessThanOrEqual(compactMetrics.clientWidth + 1);
    expect(compactMetrics.clientHeight).toBeGreaterThan(compactMetrics.lineHeight);
    expect(compactMetrics.scrollHeight).toBe(compactMetrics.clientHeight);
    expect(compactMetrics.text).toBe(compactLine);
    expect(compactMetrics.whiteSpace).toBe("normal");
    expect(await chat.evaluate((element) => element.scrollWidth <= element.clientWidth + 1)).toBe(
      true,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1,
      ),
    ).toBe(true);
  });
});
