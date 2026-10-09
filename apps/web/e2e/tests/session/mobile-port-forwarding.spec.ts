// Filename starts with "mobile-" so this runs on the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { dwell, waitForHttp } from "../../helpers/causal-waits";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { AppState } from "../../../lib/state/store";
import type { StoreApi } from "zustand";
import { SessionPage } from "../../pages/session-page";
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
    await openPorts(session);
    await expect(session.portForwardDialog).toBeVisible();
    await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
    await expect(session.mobilePanels).toBeFocused();
    await openPorts(session);
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
    await expect(session.mobilePanels).toBeFocused();
  });

  test("opens from Panels with the header shortcut off and adds a manual port", async ({
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
    await expect(session.mobilePortForwardingToggle).toHaveCount(0);
    await testPage.getByRole("dialog", { name: "Tasks", exact: true }).press("Escape");
    await openPorts(session);
    await expect(session.portForwardButton).toBeHidden();
    await expect(session.portForwardDialog).toHaveAttribute("data-presentation", "drawer");
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
    await expect(session.mobilePanels).toBeFocused();
  });
});

async function openPorts(session: SessionPage) {
  await session.mobilePanels.tap();
  await expect(session.mobilePortForwardingOpen).toBeEnabled();
  await session.mobilePortForwardingOpen.tap();
  await expect(session.portForwardDialog).toBeVisible();
}

async function seedPorts(page: Page, apiClient: ApiClient, seedData: SeedData, title: string) {
  const ports = await routePortForwarding(page);
  const session = await seedIdleSession(page, apiClient, seedData, title);
  const taskId = new URL(page.url()).pathname.split("/").pop()!;
  const { sessions } = await apiClient.listTaskSessions(taskId);
  ports.setSession(sessions[0].id);
  return { session, ports, taskId, sessionId: sessions[0].id };
}

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.5, .7-.10, .12
test("repeated opening leaves preference unchanged; its switch persists without dismissing", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session, taskId } = await seedPorts(
    testPage,
    apiClient,
    seedData,
    "Phone shortcut independence",
  );
  let writes = 0;
  testPage.on("request", (request) => {
    if (request.method() === "PATCH" && request.url().endsWith(`/tasks/${taskId}/port-forwarding`))
      writes += 1;
  });
  await openPorts(session);
  await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
  await openPorts(session);
  await dwell(
    testPage,
    150,
    "negative-assertion",
    "Opening management must not mutate the header preference.",
  );
  expect(writes).toBe(0);
  await expect(session.portForwardHeaderShortcut).not.toBeChecked();
  await session.portForwardInput.fill("5432");
  const enabled = waitForHttp(testPage, "PATCH", /\/tasks\/[^/]+\/port-forwarding$/);
  await session.portForwardHeaderShortcut.tap();
  await enabled;
  await expect(session.portForwardHeaderShortcut).toBeChecked();
  await expect(session.portForwardDialog).toBeVisible();
  await expect(session.portForwardInput).toHaveValue("5432");
  const disabled = waitForHttp(testPage, "PATCH", /\/tasks\/[^/]+\/port-forwarding$/);
  await session.portForwardHeaderShortcut.tap();
  await disabled;
  await expect(session.portForwardHeaderShortcut).not.toBeChecked();
  await expect(session.portForwardDialog).toBeVisible();
  await expect(session.portForwardInput).toHaveValue("5432");
  await expect(session.portForwardRow(9000)).toHaveAttribute("data-forwarded", "true");
  await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.portForwardDialog).toHaveCount(0);
  await expect(session.portForwardButton).toBeHidden();
  await openPorts(session);
  await expect(session.portForwardHeaderShortcut).not.toBeChecked();
  const enableAgain = waitForHttp(testPage, "PATCH", /\/tasks\/[^/]+\/port-forwarding$/);
  await session.portForwardHeaderShortcut.tap();
  await enableAgain;
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.portForwardButton).toBeVisible();
  await expect(session.portForwardDialog).toBeHidden();
  await session.portForwardButton.tap();
  await expect(session.portForwardDialog).toBeVisible();
  await session.portForwardDialog.getByRole("button", { name: "Close", exact: true }).tap();
  await expect(session.portForwardButton).toBeFocused();
});

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.8
test("failed shortcut persistence keeps management and manual draft usable", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session } = await seedPorts(testPage, apiClient, seedData, "Phone preference rollback");
  await openPorts(session);
  await session.portForwardInput.fill("5432");
  await testPage.route("**/tasks/*/port-forwarding", (route) =>
    route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ error: "Save failed" }),
    }),
  );
  const failed = waitForHttp(testPage, "PATCH", /\/tasks\/[^/]+\/port-forwarding$/);
  await session.portForwardHeaderShortcut.tap();
  await failed;
  await expect(session.portForwardHeaderShortcut).not.toBeChecked();
  await expect(session.portForwardDialog).toBeVisible();
  await expect(session.portForwardInput).toHaveValue("5432");
  await session.portForwardAddButton.tap();
  await expect(session.portForwardRow(5432)).toBeVisible();
});

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.3, .13
test("readiness loss dismisses management and leaves an explained disabled Panels action", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session, sessionId } = await seedPorts(
    testPage,
    apiClient,
    seedData,
    "Phone runtime unavailable",
  );
  await openPorts(session);
  await testPage.evaluate((id) => {
    const store = (window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> })
      .__KANDEV_E2E_STORE__;
    store.getState().setSessionAgentctlStatus(id, {
      status: "error",
      errorMessage: "Runtime unavailable",
    });
  }, sessionId);
  await expect(session.portForwardDialog).toBeHidden();
  await session.mobilePanels.tap();
  await expect(session.mobilePortForwardingOpen).toBeDisabled();
  await expect(
    testPage.getByText(
      "Port forwarding requires an active, unarchived task session with a ready runtime.",
    ),
  ).toBeVisible();
});

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.11
for (const width of [320, 767]) {
  test(`phone port management stays bounded at ${width}px and with a shortened keyboard viewport`, async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width, height: 851 });
    const { session } = await seedPorts(testPage, apiClient, seedData, `Phone geometry ${width}`);
    await session.mobilePanels.scrollIntoViewIfNeeded();
    await session.mobilePanels.tap();
    const rowBox = await session.mobilePortForwardingOpen.boundingBox();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    await session.mobilePortForwardingOpen.tap();
    await expect(session.portForwardDialog).toHaveAttribute("data-presentation", "drawer");
    await assertNoDocumentHorizontalOverflow(testPage, "phone ports and dock");
    await assertLocatorWithinViewportX(session.portForwardDialog, "phone port drawer");
    await session.portForwardInput.scrollIntoViewIfNeeded();
    await session.portForwardInput.fill("5432");
    await testPage.setViewportSize({ width, height: 420 });
    await session.portForwardAddButton.scrollIntoViewIfNeeded();
    const addBox = await session.portForwardAddButton.boundingBox();
    expect(addBox!.height).toBeGreaterThanOrEqual(44);
    expect(addBox!.y + addBox!.height).toBeLessThanOrEqual(420);
    await session.portForwardAddButton.tap();
    await expect(session.portForwardRow(5432)).toBeVisible();
    await testPage.screenshot({ path: test.info().outputPath(`phone-port-drawer-${width}.png`) });
  });
}

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.4, .14
test("768px coarse-pointer tablet retains its checked launcher and dialog", async ({
  tabletTestPage,
  apiClient,
  seedData,
}) => {
  await tabletTestPage.setViewportSize({ width: 768, height: 900 });
  expect(await tabletTestPage.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
  const { session } = await seedPorts(tabletTestPage, apiClient, seedData, "Tablet port launcher");
  await expect(tabletTestPage.getByTestId("tablet-task-layout")).toBeVisible();
  await expect(session.mobilePanels).toHaveCount(0);
  await tabletTestPage.evaluate(() => {
    const store = (window as Window & { __KANDEV_E2E_STORE__: StoreApi<AppState> })
      .__KANDEV_E2E_STORE__;
    store.getState().setMobileSessionTaskSwitcherOpen(true);
  });
  await expect(session.mobilePortForwardingToggle).toBeVisible();
  await session.mobilePortForwardingToggle.tap();
  await expect(session.portForwardDialog).toHaveAttribute("data-presentation", "dialog");
  await expect(session.portForwardButton).toBeVisible();
  await expect(session.portForwardHeaderShortcut).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(tabletTestPage, "tablet ports");
  await tabletTestPage.screenshot({ path: test.info().outputPath("tablet-port-dialog.png") });
});

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.11-.12
test("expanded localized phone labels stay contained and the shortcut has a 44px hit area", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  await testPage.setViewportSize({ width: 320, height: 851 });
  const { session } = await seedPorts(testPage, apiClient, seedData, "Localized phone ports");
  await testPage.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await testPage.reload();
  await session.waitForLoad();
  await session.mobilePanels.tap();
  await expect(session.mobilePortForwardingOpen).toHaveText(/[À-ɏ]/);
  await assertNoDocumentHorizontalOverflow(testPage, "localized Panels");
  await testPage.screenshot({
    path: test.info().outputPath("localized-phone-panels.png"),
    animations: "disabled",
  });
  await session.mobilePortForwardingOpen.tap();
  await session.portForwardHeaderShortcut.scrollIntoViewIfNeeded();
  await expect(testPage.locator('label[for="port-forward-header-shortcut"]')).toHaveText(/[À-ɏ]/);
  await assertNoDocumentHorizontalOverflow(testPage, "localized phone ports");
  const hitArea = await session.portForwardHeaderShortcut.evaluate((element) => {
    const box = element.getBoundingClientRect();
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    const after = getComputedStyle(element, "::after");
    return {
      hits: [-21.5, 21.5].map((offset) => {
        const hit = document.elementFromPoint(x, y + offset);
        return { matches: element.contains(hit), tag: hit?.tagName, id: hit?.id };
      }),
      top: after.top,
      bottom: after.bottom,
      height: after.height,
      content: after.content,
    };
  });
  expect(
    hitArea.hits.every((hit) => hit.matches),
    JSON.stringify(hitArea),
  ).toBe(true);
  await testPage.screenshot({
    path: test.info().outputPath("localized-phone-preference.png"),
    animations: "disabled",
  });
});

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.3, .13
test("archiving the task closes management and disables its Panels command", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { session, taskId } = await seedPorts(
    testPage,
    apiClient,
    seedData,
    "Archived phone ports",
  );
  await openPorts(session);
  await testPage.screenshot({
    path: test.info().outputPath("phone-port-manager.png"),
    animations: "disabled",
  });
  await apiClient.archiveTask(taskId);
  await expect(session.portForwardDialog).toBeHidden();
  await expect(testPage).not.toHaveURL(new RegExp(`/t/${taskId}$`));
  await testPage.goto(`/t/${taskId}`);
  await session.mobilePanels.tap();
  await expect(session.mobilePortForwardingOpen).toBeDisabled();
  await expect(session.mobilePortForwardingOpen).toHaveAccessibleDescription(
    "Port forwarding requires an active, unarchived task session with a ready runtime.",
  );
});
