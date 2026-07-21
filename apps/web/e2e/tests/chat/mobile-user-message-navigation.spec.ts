import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  CURRENT_USER_PROMPT,
  OLD_USER_PROMPT,
  expectLegacyRowArrowsAbsent,
  expectMessageAtNavigationPosition,
  openLongHistory,
  seedLongUserMessageHistory,
} from "./user-message-navigation-helpers";

test.describe("User message navigation on mobile", () => {
  test("keeps touch controls visible and navigates across unloaded history", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const { taskId } = await seedLongUserMessageHistory(apiClient, seedData);
    const session = new SessionPage(testPage);
    await openLongHistory(testPage, session, taskId);

    const rail = session.userMessageNavigationRail();
    const previous = session.previousUserMessageButton();
    const next = session.nextUserMessageButton();
    const currentPrompt = session.userMessageContaining(CURRENT_USER_PROMPT);
    await expect(rail).toHaveCSS("opacity", "1");
    await expect(session.loadOlderMessagesButton()).toBeVisible();
    await expect(session.loadOlderMessagesButton()).toBeEnabled();
    await expectLegacyRowArrowsAbsent(currentPrompt);

    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("Expected a mobile viewport");
    const [railBox, previousBox, nextBox, currentTextBox] = await Promise.all([
      rail.boundingBox(),
      previous.boundingBox(),
      next.boundingBox(),
      currentPrompt.getByText(CURRENT_USER_PROMPT, { exact: true }).boundingBox(),
    ]);
    expect(railBox).not.toBeNull();
    expect(previousBox).not.toBeNull();
    expect(nextBox).not.toBeNull();
    expect(currentTextBox).not.toBeNull();
    expect(previousBox!.width).toBeGreaterThanOrEqual(44);
    expect(previousBox!.height).toBeGreaterThanOrEqual(44);
    expect(nextBox!.width).toBeGreaterThanOrEqual(44);
    expect(nextBox!.height).toBeGreaterThanOrEqual(44);
    expect(railBox!.x).toBeGreaterThanOrEqual(0);
    expect(railBox!.y).toBeGreaterThanOrEqual(0);
    expect(railBox!.x + railBox!.width).toBeLessThanOrEqual(viewport.width);
    expect(railBox!.y + railBox!.height).toBeLessThanOrEqual(viewport.height);
    expect(currentTextBox!.x).toBeGreaterThanOrEqual(0);
    expect(currentTextBox!.x + currentTextBox!.width).toBeLessThanOrEqual(railBox!.x);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await previous.click();
    const oldPrompt = session.userMessageContaining(OLD_USER_PROMPT);
    await expect(oldPrompt).toHaveClass(/search-flash/, { timeout: 60_000 });
    await expectMessageAtNavigationPosition(session.messageScrollOwner(), oldPrompt);
    await expect(previous).toBeDisabled();
    await expect(next).toBeEnabled();

    await next.click();
    await expect(currentPrompt).toHaveClass(/search-flash/, { timeout: 15_000 });
    await expectMessageAtNavigationPosition(session.messageScrollOwner(), currentPrompt);
    await expect(next).toBeDisabled();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
  });
});
