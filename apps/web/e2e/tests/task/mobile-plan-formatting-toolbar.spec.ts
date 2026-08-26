// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const PLAN_TEXT = "Select this mobile plan text";
const FINAL_LINE = "Reachable final mobile plan line";
const PLAN_CONTENT = [
  "## Mobile formatting",
  PLAN_TEXT,
  ...Array.from({ length: 40 }, (_, index) => `Mobile detail ${index + 1}`),
  FINAL_LINE,
].join("\n\n");

async function seedMobilePlan(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Plan formatting toolbar",
    seedData.agentProfileId,
    {
      description: planScript(PLAN_CONTENT),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
  await session.waitForChatIdle({ timeout: 45_000 });
  await session.togglePlanMode();
  await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
  await expect(session.planPanel).toContainText(PLAN_TEXT, { timeout: 15_000 });
  return session;
}

async function simulateKeyboardOpen(testPage: Page, height: number): Promise<void> {
  await testPage.evaluate((px) => {
    const vv = window.visualViewport;
    if (!vv) return;
    Object.defineProperty(vv, "height", { configurable: true, value: window.innerHeight - px });
    vv.dispatchEvent(new Event("resize"));
  }, height);
}

async function simulateViewportScroll(testPage: Page, offsetTop: number): Promise<void> {
  await testPage.evaluate((y) => {
    const vv = window.visualViewport;
    if (!vv) return;
    Object.defineProperty(vv, "offsetTop", { configurable: true, value: y });
    vv.dispatchEvent(new Event("scroll"));
  }, offsetTop);
}

test.describe("mobile: Plan formatting toolbar", () => {
  test("docks above the keyboard and preserves the selection for Bold", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(150_000);
    const session = await seedMobilePlan(testPage, apiClient, seedData);
    const editor = session.planPanel.locator(".ProseMirror:visible");
    await expect(editor).toHaveCount(1);

    await editor.focus();
    const toolbar = testPage.getByTestId("plan-mobile-formatting-toolbar");
    await expect(toolbar).toHaveCount(0);

    await testPage.keyboard.press("Control+A");
    await expect(toolbar).toBeVisible({ timeout: 10_000 });
    if (prCapture.capturing) {
      await prCapture.screenshot("mobile-plan-formatting-toolbar", {
        caption: "Mobile Plan formatting controls docked above the task navigation.",
      });
    }

    const bold = testPage.getByTestId("plan-formatting-action-bold");
    await expect(bold).toBeVisible();
    const actionButtons = toolbar.locator('button[data-testid^="plan-formatting-action-"]');
    await expect(actionButtons).toHaveCount(8);
    const actionSizes = await actionButtons.evaluateAll((elements) =>
      elements.map((element) => {
        const { width, height } = element.getBoundingClientRect();
        return { width, height };
      }),
    );
    for (const { width, height } of actionSizes) {
      expect(width).toBeGreaterThanOrEqual(44);
      expect(height).toBeGreaterThanOrEqual(44);
    }
    const visualActionSizes = await toolbar
      .locator('button[data-testid^="plan-formatting-action-"] > span')
      .evaluateAll((elements) =>
        elements.map((element) => {
          const { width, height } = element.getBoundingClientRect();
          return { width, height };
        }),
      );
    await expect(
      toolbar.locator('button[data-testid^="plan-formatting-action-"] > span'),
    ).toHaveCount(8);
    for (const { width, height } of visualActionSizes) {
      expect(width).toBeLessThanOrEqual(32);
      expect(height).toBeLessThanOrEqual(32);
    }
    await expect
      .poll(() => toolbar.evaluate((element) => Math.round(element.getBoundingClientRect().height)))
      .toBe(48);
    await expect(testPage.getByTestId("plan-formatting-action-comment")).toBeEnabled();
    await expect
      .poll(() => editor.evaluate((element) => getComputedStyle(element, "::after").height))
      .toBe("48px");

    const horizontalOverflow = await toolbar.locator(":scope > div").evaluate((element) => ({
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth,
      scrollLeft: element.scrollLeft,
    }));
    expect(horizontalOverflow.scrollWidth).toBe(horizontalOverflow.clientWidth);
    expect(horizontalOverflow.scrollLeft).toBe(0);

    const narrowOverflow = await toolbar.locator(":scope > div").evaluate((element) => {
      const scroller = element as HTMLElement;
      const originalWidth = scroller.style.width;
      scroller.style.width = "100px";
      const scrollWidth = scroller.scrollWidth;
      const clientWidth = scroller.clientWidth;
      scroller.scrollLeft = 16;
      const canScroll = scroller.scrollLeft > 0;
      scroller.style.width = originalWidth;
      scroller.scrollLeft = 0;
      return { scrollWidth, clientWidth, canScroll };
    });
    expect(narrowOverflow.scrollWidth).toBeGreaterThan(narrowOverflow.clientWidth);
    expect(narrowOverflow.canScroll).toBe(true);

    const documentOverflow = await testPage.evaluate(() => ({
      documentWidth: document.documentElement.scrollWidth,
      documentClientWidth: document.documentElement.clientWidth,
      bodyWidth: document.body.scrollWidth,
      bodyClientWidth: document.body.clientWidth,
    }));
    expect(documentOverflow.documentWidth).toBeLessThanOrEqual(
      documentOverflow.documentClientWidth,
    );
    expect(documentOverflow.bodyWidth).toBeLessThanOrEqual(documentOverflow.bodyClientWidth);

    const keyboardHeight = 300;
    await simulateKeyboardOpen(testPage, keyboardHeight);
    const expectedKeyboardTop = await testPage.evaluate(
      (height) => `${window.innerHeight - height - 48}px`,
      keyboardHeight,
    );
    await expect
      .poll(() => toolbar.evaluate((element) => (element as HTMLElement).style.top))
      .toBe(expectedKeyboardTop);
    await expect
      .poll(() => toolbar.evaluate((element) => (element as HTMLElement).style.bottom))
      .toBe("auto");
    await expect
      .poll(() => editor.evaluate((element) => getComputedStyle(element, "::after").height))
      .toBe("296px");

    const finalLine = editor.getByText(FINAL_LINE, { exact: true });
    const editorScrollContainer = testPage.getByTestId("plan-editor-scroll-container");
    const scrollMetrics = await editorScrollContainer.evaluate((element) => {
      const scrollContainer = element as HTMLElement;
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
      return {
        scrollTop: scrollContainer.scrollTop,
        scrollHeight: scrollContainer.scrollHeight,
        clientHeight: scrollContainer.clientHeight,
      };
    });
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
    expect(scrollMetrics.scrollTop).toBeGreaterThan(0);
    const finalLineRect = await finalLine.evaluate((element) => {
      const { bottom, height } = element.getBoundingClientRect();
      return { bottom, height };
    });
    const toolbarTop = await toolbar.evaluate((element) => element.getBoundingClientRect().top);
    expect(finalLineRect.height).toBeGreaterThan(0);
    expect(finalLineRect.bottom).toBeLessThanOrEqual(toolbarTop + 1);

    await simulateViewportScroll(testPage, 48);
    const expectedScrolledTop = await testPage.evaluate(
      (height) => `${window.innerHeight - height + 48 - 48}px`,
      keyboardHeight,
    );
    await expect
      .poll(() => toolbar.evaluate((element) => (element as HTMLElement).style.top))
      .toBe(expectedScrolledTop);

    await bold.tap();
    await expect(editor.locator("strong", { hasText: PLAN_TEXT })).toContainText(PLAN_TEXT);
    expect(await editor.evaluate((element) => document.activeElement === element)).toBe(true);
  });
});
