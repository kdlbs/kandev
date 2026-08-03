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
});
