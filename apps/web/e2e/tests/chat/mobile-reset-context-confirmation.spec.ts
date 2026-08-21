import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  seedResetContextSession,
  seedStaleContextWindow,
} from "./reset-context-confirmation-helpers";

test("mobile reset context confirms inline without stacking another overlay", async ({
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
  const inlineConfirmation = testPage.getByTestId("reset-context-inline-confirm");
  await expect(inlineConfirmation).toBeVisible();
  await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
  await prCapture.screenshot("mobile-reset-context-confirmation", {
    caption: "Mobile toolbar reset context confirmation",
  });

  const confirmBox = await inlineConfirmation.getByTestId("reset-context-confirm").boundingBox();
  expect(confirmBox).not.toBeNull();
  expect(confirmBox!.height).toBeGreaterThanOrEqual(44);

  await inlineConfirmation.getByRole("button", { name: "Cancel" }).tap();
  await expect(inlineConfirmation).toHaveCount(0);
  await expect(contextRing).toBeVisible();
  await expect(session.contextResetDivider()).toHaveCount(0);

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
