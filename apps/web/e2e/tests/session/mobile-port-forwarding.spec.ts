// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import {
  assertNoDocumentHorizontalOverflow,
  assertLocatorWithinViewportX,
} from "../../helpers/layout-assertions";

test.describe("Port forwarding on mobile", () => {
  test("enables from the active-task drawer action and opens a safe dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Port Forwarding Test",
    );

    await expect(session.portForwardButton).not.toBeVisible();
    await session.mobileSessionMenu.tap();
    await expect(session.mobilePortForwardingToggle).toBeVisible();
    await expect(session.mobilePortForwardingToggle).toBeEnabled();

    await session.mobilePortForwardingToggle.tap();

    await expect(session.portForwardButton).toBeVisible();
    await expect(session.portForwardDialog).toBeVisible();
    await assertLocatorWithinViewportX(session.portForwardDialog, "port forwarding dialog");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile port forwarding");

    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.tap();
    await expect(session.portForwardRow(3000)).toBeVisible();
    await expect(session.portForwardOpenBrowser(3000)).toHaveCount(0);

    await session.portForwardDialog.getByRole("button", { name: "Close" }).tap();
  });
});
