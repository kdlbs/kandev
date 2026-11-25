import type { Locator, Page } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { readDialogRenderingMetrics } from "../../helpers/dialog-webkit-metrics";
import { openQuickChatSetup } from "./quick-chat-helpers";

const QUICK_CHAT_WIDTH_STORAGE_KEY = "kandev.quickChat.dialogWidth";

type ExitAnimationSnapshot = {
  animationName: string;
  transform: string;
  translate: string;
};

async function waitForPreferredWidth(dialog: Locator) {
  await expect
    .poll(() =>
      dialog.evaluate((element) => {
        const preferred = Number.parseFloat(
          getComputedStyle(element).getPropertyValue("--quick-chat-width"),
        );
        return Math.abs(element.getBoundingClientRect().width - preferred);
      }),
    )
    .toBeLessThan(2);
}

async function prepareDesktopPage(testPage: Page) {
  await testPage.setViewportSize({ width: 1200, height: 901 });
  await testPage.addInitScript(
    ({ key, width }) => window.localStorage.setItem(key, JSON.stringify(width)),
    { key: QUICK_CHAT_WIDTH_STORAGE_KEY, width: 901 },
  );
  await testPage.goto("/");
  await testPage.waitForLoadState("networkidle");
}

test.describe("Quick Chat WebKit rendering", () => {
  test("keeps Chromium motion and panel geometry", async ({ testPage, apiClient }) => {
    await apiClient.saveUserSettings({ sidebar_fast_actions_enabled: true });
    await prepareDesktopPage(testPage);
    await expect(testPage.locator("html")).toHaveAttribute("data-rendering-engine", "other");

    const dialog = await openQuickChatSetup(testPage, false);
    const metrics = await readDialogRenderingMetrics(dialog, {
      contentSelector: '[data-slot="dialog-content"]',
    });
    expect(metrics.animationName).toBe("enter");
    expect(metrics.translate).toContain("-50%");
    expect(metrics.width).toBe(901);
    expect(metrics.centerX).toBeCloseTo(600, 0);
    expect(metrics.centerY).toBeCloseTo(450.5, 0);
    expect(metrics.contentOverOverlay).toBe(true);
  });

  // @covers AC-UI-QUICK-CHAT-ELEVATION-001.2 AC-UI-QUICK-CHAT-ELEVATION-001.5 AC-UI-QUICK-CHAT-ELEVATION-001.8
  test("uses transform-free motion and centered geometry for chat and terminal", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    await apiClient.saveUserSettings({ sidebar_fast_actions_enabled: true });
    await prepareDesktopPage(testPage);
    await testPage.locator("html").evaluate((root) => {
      root.setAttribute("data-rendering-engine", "webkit");
    });

    const dialog = await openQuickChatSetup(testPage, false);
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();

    const assertWebkitPanel = async () => {
      const metrics = await readDialogRenderingMetrics(dialog, {
        contentSelector: '[data-slot="dialog-content"]',
      });
      expect(metrics.animationName).toBe("kandev-dialog-webkit-enter");
      expect(metrics.transform).toBe("none");
      expect(metrics.translate).toBe("none");
      expect(metrics.centerX).toBeCloseTo(viewport!.width / 2, 0);
      expect(metrics.centerY).toBeCloseTo(viewport!.height / 2, 0);
      expect(metrics.contentOverOverlay).toBe(true);
      return metrics;
    };

    await waitForPreferredWidth(dialog);
    const initialMetrics = await assertWebkitPanel();
    expect(initialMetrics.width).toBe(901);
    expect(initialMetrics.height).toBeCloseTo(viewport!.height * 0.85, 1);

    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-terminal").click();
    await expect(dialog.getByTestId("quick-terminal-terminal")).toBeVisible({
      timeout: 15_000,
    });
    const terminalMetrics = await assertWebkitPanel();
    expect(terminalMetrics.width).toBe(initialMetrics.width);
    expect(terminalMetrics.height).toBe(initialMetrics.height);

    const rightHandle = dialog.getByTestId("quick-chat-resize-right");
    const handleBox = await rightHandle.boundingBox();
    expect(handleBox).not.toBeNull();
    await testPage.mouse.move(
      handleBox!.x + handleBox!.width / 2,
      handleBox!.y + handleBox!.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(
      handleBox!.x + handleBox!.width / 2 + 24,
      handleBox!.y + handleBox!.height / 2,
    );
    await testPage.mouse.up();
    await waitForPreferredWidth(dialog);
    await expect
      .poll(() =>
        testPage.evaluate((key) => {
          const saved = window.localStorage.getItem(key);
          return saved === null ? 0 : Number(JSON.parse(saved));
        }, QUICK_CHAT_WIDTH_STORAGE_KEY),
      )
      .toBeGreaterThan(901);

    const resizedMetrics = await assertWebkitPanel();
    const savedWidth = await testPage.evaluate((key) => {
      const saved = window.localStorage.getItem(key);
      return saved === null ? 0 : Number(JSON.parse(saved));
    }, QUICK_CHAT_WIDTH_STORAGE_KEY);
    expect(resizedMetrics.width).toBeGreaterThan(terminalMetrics.width);
    expect(resizedMetrics.width).toBe(savedWidth);
    expect(resizedMetrics.centerX).toBeCloseTo(viewport!.width / 2, 0);

    let closingMotion: ExitAnimationSnapshot | null = null;
    await testPage.exposeFunction(
      "reportQuickChatExitAnimation",
      (snapshot: ExitAnimationSnapshot) => {
        closingMotion = snapshot;
      },
    );
    const listenerInstalled = await dialog.evaluate((element) => {
      const report = (
        window as Window & {
          reportQuickChatExitAnimation: (snapshot: ExitAnimationSnapshot) => void;
        }
      ).reportQuickChatExitAnimation;
      const onAnimationStart = (event: AnimationEvent) => {
        if (event.target !== element) return;
        element.removeEventListener("animationstart", onAnimationStart);
        const style = getComputedStyle(element);
        report({
          animationName: event.animationName,
          transform: style.transform,
          translate: style.translate,
        });
      };
      element.addEventListener("animationstart", onAnimationStart);
      return true;
    });
    expect(listenerInstalled).toBe(true);
    await testPage.keyboard.press("Escape");
    await expect
      .poll(() => closingMotion, { timeout: 2_000 })
      .toEqual({
        animationName: "kandev-dialog-webkit-exit",
        transform: "none",
        translate: "none",
      });
    await expect(dialog).not.toBeVisible();
  });
});
