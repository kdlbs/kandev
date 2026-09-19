import { expect, test } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../helpers/api-client";
import { SessionPage } from "../pages/session-page";

async function createToolbarTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  return session;
}

async function expectTouchHeaders(page: Page, surface: string) {
  await expect
    .poll(
      () =>
        page.locator("[data-panel-header]:visible").evaluateAll((headers) =>
          headers.map((header) => {
            const rect = header.getBoundingClientRect();
            return {
              height: rect.height,
              bottom: rect.bottom,
              nextTop: header.nextElementSibling?.getBoundingClientRect().top ?? null,
              wrap: window.getComputedStyle(header).flexWrap,
            };
          }),
        ),
      { timeout: 15_000, message: `Waiting for a touch header in ${surface}` },
    )
    .not.toEqual([]);

  const headers = await page.locator("[data-panel-header]:visible").evaluateAll((items) =>
    items.map((header) => {
      const rect = header.getBoundingClientRect();
      return {
        height: rect.height,
        bottom: rect.bottom,
        nextTop: header.nextElementSibling?.getBoundingClientRect().top ?? null,
        wrap: window.getComputedStyle(header).flexWrap,
      };
    }),
  );
  expect(headers.length, `expected touch header in ${surface}`).toBeGreaterThan(0);
  for (const header of headers) {
    expect(Math.abs(header.height - 48), `${surface} header height`).toBeLessThanOrEqual(1);
    expect(header.wrap).toBe("nowrap");
    if (header.nextTop !== null) {
      expect(
        Math.abs(header.nextTop - header.bottom),
        `${surface} content edge`,
      ).toBeLessThanOrEqual(1);
    }
  }
}

async function expectNoOverflow(page: Page, surface: string) {
  await expect
    .poll(
      () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
      { timeout: 5_000, message: `${surface} has horizontal overflow` },
    )
    .toBe(true);
}

test.describe("touch panel toolbars", () => {
  test("keeps the 390px phone panels contained and preserves task navigation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    expect(await testPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);

    const session = await createToolbarTask(testPage, apiClient, seedData, "Panel toolbar phone");
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(testPage, "390px Files");

    const searchButton = testPage.getByRole("button", { name: "Search files", exact: true });
    await expect(searchButton).toBeVisible();
    expect((await searchButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expectNoOverflow(testPage, "390px Files");

    await testPage.getByRole("button", { name: /Changes/ }).tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    await expectTouchHeaders(testPage, "390px Changes");
    await expectNoOverflow(testPage, "390px Changes");

    await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
    await expect(session.activeChat()).toBeVisible();
  });

  test("keeps 767px and 900px coarse headers at touch geometry", async ({
    testPage,
    tabletTestPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 767, height: 900 });
    await createToolbarTask(testPage, apiClient, seedData, "Panel toolbar coarse boundaries");
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await testPage.getByRole("button", { name: "Files", exact: true }).tap();
    await expect(testPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(testPage, "767px Files");
    await expectNoOverflow(testPage, "767px Files");

    const taskHref = await testPage.url();
    await tabletTestPage.goto(taskHref);
    const tabletSession = new SessionPage(tabletTestPage);
    await tabletSession.waitForLoad();
    await tabletSession.waitForChatIdle({ timeout: 45_000 });
    expect(await tabletTestPage.evaluate(() => window.innerWidth)).toBe(900);
    expect(await tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
    await expect(tabletTestPage.getByTestId("file-tree-scroll")).toBeVisible();
    await expectTouchHeaders(tabletTestPage, "900px Files");
    await expectNoOverflow(tabletTestPage, "900px Files");
  });
});
