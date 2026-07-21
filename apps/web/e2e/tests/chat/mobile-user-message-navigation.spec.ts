import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  CURRENT_USER_PROMPT,
  OLD_USER_PROMPT,
  expectMessageAtNavigationPosition,
  expectNavigationOutline,
  openLongHistory,
  seedLongUserMessageHistory,
} from "./user-message-navigation-helpers";

test.describe("User message navigation on mobile", () => {
  test("keeps touch controls visible and navigates across unloaded history", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    const { taskId } = await seedLongUserMessageHistory(apiClient, seedData);
    const session = new SessionPage(testPage);
    await openLongHistory(testPage, session, taskId);

    const currentPrompt = session.userMessageContaining(CURRENT_USER_PROMPT);
    const previous = session.previousUserMessageButton(currentPrompt);
    const next = session.nextUserMessageButton(currentPrompt);
    await expect(currentPrompt.getByTestId("message-actions")).toHaveCSS("opacity", "1");
    await expect(session.activeChat().getByTestId("user-message-navigation-rail")).toHaveCount(0);
    await expect(session.loadOlderMessagesButton()).toBeVisible();
    await expect(session.loadOlderMessagesButton()).toBeEnabled();

    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("Expected a mobile viewport");
    const [previousBox, nextBox, currentTextBox] = await Promise.all([
      previous.boundingBox(),
      next.boundingBox(),
      currentPrompt.getByText(CURRENT_USER_PROMPT, { exact: true }).boundingBox(),
    ]);
    expect(previousBox).not.toBeNull();
    expect(nextBox).not.toBeNull();
    expect(currentTextBox).not.toBeNull();
    expect(previousBox!.width).toBeGreaterThanOrEqual(44);
    expect(previousBox!.height).toBeGreaterThanOrEqual(44);
    expect(nextBox!.width).toBeGreaterThanOrEqual(44);
    expect(nextBox!.height).toBeGreaterThanOrEqual(44);
    expect(previousBox!.x).toBeGreaterThanOrEqual(0);
    expect(previousBox!.x + previousBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(nextBox!.x).toBeGreaterThanOrEqual(0);
    expect(nextBox!.x + nextBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(currentTextBox!.x).toBeGreaterThanOrEqual(0);
    expect(currentTextBox!.x + currentTextBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await previous.click();
    const oldPrompt = session.userMessageContaining(OLD_USER_PROMPT);
    await expect(oldPrompt).toHaveClass(/search-flash/, { timeout: 60_000 });
    await expectNavigationOutline(oldPrompt);
    await expectMessageAtNavigationPosition(session.messageScrollOwner(), oldPrompt);
    const oldPrevious = session.previousUserMessageButton(oldPrompt);
    const oldNext = session.nextUserMessageButton(oldPrompt);
    await expect(oldPrevious).toBeDisabled();
    await expect(oldNext).toBeEnabled();
    await prCapture.screenshot("inline-user-message-navigation-mobile", {
      caption: "Touch-sized navigation actions on a user message",
    });

    await oldNext.click();
    await expect(currentPrompt).toHaveClass(/search-flash/, { timeout: 15_000 });
    await expectNavigationOutline(currentPrompt);
    await expectMessageAtNavigationPosition(session.messageScrollOwner(), currentPrompt);
    await expect(next).toBeDisabled();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
