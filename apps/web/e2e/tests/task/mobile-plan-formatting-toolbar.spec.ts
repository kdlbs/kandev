// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { planScript } from "../../helpers/seed-session-messages";
import { SessionPage } from "../../pages/session-page";

const PLAN_TEXT = "Select this mobile plan text";

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
      description: planScript(`## Mobile formatting\n\n${PLAN_TEXT}`),
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
    const editor = session.planPanel.locator(".ProseMirror:visible").first();

    await editor.focus();
    await testPage.keyboard.press("Control+A");
    const toolbar = testPage.getByTestId("plan-mobile-formatting-toolbar");
    await expect(toolbar).toBeVisible({ timeout: 10_000 });
    if (prCapture.capturing) {
      await prCapture.screenshot("mobile-plan-formatting-toolbar", {
        caption: "Mobile Plan formatting controls docked above the task navigation.",
      });
    }

    const bold = testPage.getByTestId("plan-formatting-action-bold");
    await expect(bold).toBeVisible();
    expect((await bold.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expect(testPage.getByTestId("plan-formatting-action-comment")).toBeEnabled();
    await expect(testPage.getByTestId("plan-editor-scroll-container")).toHaveCSS(
      "padding-bottom",
      "56px",
    );

    const horizontalOverflow = await toolbar.locator(":scope > div").evaluate((element) => ({
      scrollWidth: element.scrollWidth,
      clientWidth: element.clientWidth,
    }));
    expect(horizontalOverflow.scrollWidth).toBeGreaterThanOrEqual(horizontalOverflow.clientWidth);

    const keyboardHeight = 300;
    await simulateKeyboardOpen(testPage, keyboardHeight);
    const expectedKeyboardTop = await testPage.evaluate(
      (height) => `${window.innerHeight - height - 56}px`,
      keyboardHeight,
    );
    await expect
      .poll(() => toolbar.evaluate((element) => (element as HTMLElement).style.top))
      .toBe(expectedKeyboardTop);
    await expect
      .poll(() => toolbar.evaluate((element) => (element as HTMLElement).style.bottom))
      .toBe("auto");

    await simulateViewportScroll(testPage, 48);
    const expectedScrolledTop = await testPage.evaluate(
      (height) => `${window.innerHeight - height + 48 - 56}px`,
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
