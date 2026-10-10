// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { routePortForwarding } from "./port-forwarding-helpers";
import {
  assertNoDocumentHorizontalOverflow,
  assertLocatorWithinViewportX,
} from "../../helpers/layout-assertions";

test.describe("Port forwarding on mobile", () => {
  // @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.1-.3, .6-.7
  test("shows forwards first with touch actions and a single bounded scroller", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ports = await routePortForwarding(testPage);
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile active-first ports",
    );
    const taskId = new URL(testPage.url()).pathname.split("/").pop()!;
    const { sessions } = await apiClient.listTaskSessions(taskId);
    expect(sessions).toHaveLength(1);
    ports.setSession(sessions[0].id);
    await session.mobileSessionMenu.tap();
    await expect(session.mobilePortForwardingToggle).toBeEnabled();
    await session.mobilePortForwardingToggle.tap();
    await expect(session.portForwardDialog).toBeVisible();
    await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
    await session.portForwardButton.tap();
    const rows = session.portForwardDialog.locator('[data-testid^="port-forward-row-"]');
    await expect(rows.first()).toHaveAttribute("data-testid", "port-forward-row-9000");
    const active = session.portForwardRow(9000);
    await expect(active.getByText("Forwarding", { exact: true })).toBeVisible();
    const tunnelLink = active.getByRole("link").first();
    await expect(tunnelLink).toHaveAttribute("href", /:49152\/$/);
    await expect(tunnelLink).toHaveAttribute("target", "_blank");
    await active.getByRole("button", { name: "Copy URL", exact: true }).first().tap();
    for (const action of [
      session.portForwardTunnelToggle(9000),
      tunnelLink,
      active.getByRole("button", { name: "Copy URL", exact: true }).first(),
    ]) {
      const size = await action.boundingBox();
      expect(size!.width).toBeGreaterThanOrEqual(44);
      expect(size!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "active-first phone ports");
    await assertLocatorWithinViewportX(session.portForwardDialog, "phone port dialog");
    await testPage.screenshot({ path: test.info().outputPath("active-first-phone.png") });
    await session.portForwardTunnelToggle(9000).tap();
    await expect(active).toHaveAttribute("data-forwarded", "false");
    await expect(rows.first()).toHaveAttribute("data-testid", "port-forward-row-3000");
    await expect(session.portForwardDialog.getByTestId("port-forward-active-heading")).toHaveCount(
      0,
    );
    await session.portForwardTunnelToggle(9000).tap();
    await session.portForwardTunnelStart(9000).tap();
    await expect(rows.first()).toHaveAttribute("data-testid", "port-forward-row-9000");
    await session.portForwardInput.scrollIntoViewIfNeeded();
    await expect(session.portForwardInput).toBeVisible();
    const scrollBody = session.portForwardDialog.getByTestId("port-forward-scroll-body");
    expect(
      await scrollBody.evaluate((element) => element.scrollHeight > element.clientHeight),
    ).toBe(true);
    const add = await session.portForwardAddButton.boundingBox();
    expect(add!.y + add!.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
    await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(session.portForwardButton).toBeFocused();
  });

  test("enables from the active-task drawer action and opens a safe dialog", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const ports = await routePortForwarding(testPage, { empty: true });
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Port Forwarding Test",
    );
    const taskId = new URL(testPage.url()).pathname.split("/").pop()!;
    const { sessions } = await apiClient.listTaskSessions(taskId);
    expect(sessions).toHaveLength(1);
    ports.setSession(sessions[0].id);

    await expect(session.portForwardButton).not.toBeVisible();
    await session.mobileSessionMenu.tap();
    await expect(session.mobilePortForwardingToggle).toBeVisible();
    await expect(session.mobilePortForwardingToggle).toBeEnabled();

    await session.mobilePortForwardingToggle.tap();

    await expect(session.mobilePortForwardingToggle).toBeHidden();
    await expect(session.portForwardButton).toBeVisible();
    await expect(session.portForwardDialog).toBeVisible();
    await assertLocatorWithinViewportX(session.portForwardDialog, "port forwarding dialog");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile port forwarding");

    await expect(
      session.portForwardDialog.getByText("No listening ports detected.", { exact: true }),
    ).toBeVisible();
    await expect(
      session.portForwardDialog.getByRole("heading", { name: "Other ports", exact: true }),
    ).toHaveCount(0);
    await session.portForwardInput.fill("3000");
    await session.portForwardAddButton.tap();
    const row = session.portForwardRow(3000);
    await expect(row).toBeVisible();
    await expect(row.locator("a[target='_blank']")).toHaveAttribute(
      "href",
      /\/port-proxy\/[^/]+\/3000\//,
    );
    await expect(session.portForwardOpenBrowser(3000)).toHaveCount(0);

    await session.portForwardDialog.getByRole("button", { name: "Close" }).tap();
  });
});
