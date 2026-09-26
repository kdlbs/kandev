import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import {
  seedResetContextSession,
  seedStaleContextWindow,
} from "./reset-context-confirmation-helpers";

test("desktop reset context confirms at its toolbar action and preserves cancellation", async ({
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
    "Desktop Reset Context Confirmation",
  );
  await seedStaleContextWindow(testPage);

  const contextRing = testPage.getByRole("button", { name: "Context window: 95% used" });
  await expect(contextRing).toBeVisible();

  await session.resetContextButton().click();
  const confirmation = testPage.getByTestId("reset-context-confirm-popover");
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toHaveAttribute("data-slot", "popover-content");
  await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
  await prCapture.screenshot("desktop-reset-context-confirmation", {
    caption: "Desktop toolbar reset context confirmation",
  });

  await confirmation.getByRole("button", { name: "Cancel" }).click();
  await expect(confirmation).toHaveCount(0);
  await expect(contextRing).toBeVisible();
  await expect(session.contextResetDivider()).toHaveCount(0);

  await session.resetContextButton().click();
  await expect(confirmation).toBeVisible();
  const resetResponse = ws.waitForResponse("session.reset_context");
  await confirmation.getByTestId("reset-context-confirm").click();
  await resetResponse;

  await expect(session.contextResetDivider()).toBeVisible();
  await expect(session.resetContextButton()).toBeEnabled();
});
