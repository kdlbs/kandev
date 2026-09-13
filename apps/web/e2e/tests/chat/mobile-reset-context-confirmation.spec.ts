import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  seedResetContextSession,
  seedStaleContextWindow,
} from "./reset-context-confirmation-helpers";

test.describe.configure({ timeout: 120_000 });

test("mobile reset context opens a sheet and preserves composer controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const ws = watchWs(testPage);
  const session = await seedResetContextSession(
    testPage,
    apiClient,
    seedData,
    "Mobile Reset Context Confirmation",
  );
  await seedStaleContextWindow(testPage);

  const contextRing = testPage.getByRole("button", { name: "Context window: 95% used" });
  await expect(contextRing).toBeVisible();

  await session.resetContextButton().tap();
  const inlineConfirmation = testPage.getByTestId("mobile-action-confirmation");
  const warning = inlineConfirmation.getByText(
    "This will clear the agent's conversation history and start a fresh context. Your workspace, files, and git state will be preserved.",
    { exact: true },
  );
  await expect(inlineConfirmation).toBeVisible();
  await expect(warning).toBeVisible();
  await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
  await expect(testPage.getByRole("dialog")).toHaveAttribute("data-slot", "drawer-content");
  await expect(session.resetContextButton()).toBeAttached();
  await expect(testPage.getByTestId("submit-message-button")).toBeAttached();
  await prCapture.screenshot("mobile-reset-context-confirmation", {
    caption: "Mobile context reset preserves the composer behind its compact sheet",
  });

  const confirmBox = await inlineConfirmation.getByTestId("reset-context-confirm").boundingBox();
  expect(confirmBox).not.toBeNull();
  expect(confirmBox!.height).toBeGreaterThanOrEqual(44);
  const [warningBox, viewportWidth] = await Promise.all([
    warning.boundingBox(),
    testPage.evaluate(() => window.innerWidth),
  ]);
  expect(warningBox).not.toBeNull();
  expect(warningBox!.x).toBeGreaterThanOrEqual(0);
  expect(warningBox!.x + warningBox!.width).toBeLessThanOrEqual(viewportWidth);
  const warningIsTopmost = await warning.evaluate((element) => {
    const rect = element.getBoundingClientRect();
    return [0.25, 0.5, 0.75].every((ratio) => {
      const hit = document.elementFromPoint(rect.left + rect.width * ratio, rect.top + 8);
      return hit === element || element.contains(hit);
    });
  });
  expect(warningIsTopmost).toBe(true);
  const confirmationBackground = await testPage
    .getByRole("dialog")
    .evaluate((element) => getComputedStyle(element, "::before").backgroundColor);
  expect(confirmationBackground).not.toBe("rgba(0, 0, 0, 0)");

  await inlineConfirmation.getByRole("button", { name: "Cancel" }).tap();
  await expect(inlineConfirmation).toHaveCount(0);
  await expect(session.resetContextButton()).toBeFocused();
  await expect(contextRing).toBeVisible();
  await expect(session.contextResetDivider()).toHaveCount(0);

  await session.resetContextButton().tap();
  await expect(inlineConfirmation).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(inlineConfirmation).toHaveCount(0);
  await expect(session.resetContextButton()).toBeFocused();

  await session.resetContextButton().tap();
  await expect(inlineConfirmation).toBeVisible();
  const resetResponse = ws.waitForResponse("session.reset_context");
  await inlineConfirmation.getByTestId("reset-context-confirm").tap();
  await resetResponse;

  await expect(session.contextResetDivider()).toBeVisible();
  await expect(session.resetContextButton()).toBeEnabled();

  const hasHorizontalOverflow = await testPage.evaluate(() => {
    const root = document.scrollingElement ?? document.documentElement;
    return root.scrollWidth > root.clientWidth + 1;
  });
  expect(hasHorizontalOverflow).toBe(false);
});
