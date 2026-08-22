import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

/**
 * The phone board and task-list headers render through `PageTopbar` rather
 * than a bespoke mobile bar, so the chrome contract's phone behaviour needs
 * covering at phone width: the title crumb names the page, the brand link is
 * the only home affordance, and the action strip absorbs pressure instead of
 * pushing fixed chrome off-screen.
 */
test.describe("Mobile kanban topbar", () => {
  test("names the page through the title crumb on the board and the task list", async ({
    testPage,
  }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const header = testPage.locator("header").first();
    const titleCrumb = header.locator('[data-slot="breadcrumb-page"]');
    await expect(titleCrumb).toHaveText("Home");

    await testPage.goto("/tasks");
    await expect(titleCrumb).toHaveText("Tasks");
    // The two-line title/workspace stack the bar used to render is gone: the
    // workspace picker owns workspace identity, the crumb owns the page name.
    await expect(header.getByTestId("mobile-topbar-page-context")).toHaveCount(0);
  });

  test("uses the brand link as the only home affordance", async ({ testPage }) => {
    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();

    const header = testPage.locator("header").first();
    const brand = header.getByTestId("mobile-topbar-brand");
    await expect(brand).toBeVisible();
    // `homeAffordance="none"`: a home crumb sitting next to a brand link that
    // already goes home would say the same thing twice.
    await expect(header.getByTestId("topbar-phone-home")).toHaveCount(0);

    // From the board the brand stays a same-page home link; from the task list
    // it must restore the preferred listing view instead of forcing the board
    // — visiting /tasks stores "list", and Home honors that stored preference
    // with the workspace preserved (the same contract
    // chat/mobile-quick-chat-entry.spec.ts pins for the URL). The brand is
    // still the bar's only home affordance either way.
    await testPage.goto("/tasks");
    await expect(header.getByTestId("topbar-phone-home")).toHaveCount(0);

    await brand.tap();
    // The restore lands back on /tasks with the workspace pinned into the URL
    // — the param only appears through the round trip, so it is the proof the
    // tap navigated rather than sat inert.
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/tasks" && url.searchParams.has("workspace"),
    );
    await expect(header.locator('[data-slot="breadcrumb-page"]')).toHaveText("Tasks");
  });

  test("keeps the brand and menu fixed while the action strip absorbs pressure", async ({
    testPage,
    apiClient,
  }) => {
    // Metrics in the topbar are the cheapest way to make the action row wider
    // than a phone, which is the state the strip exists to survive.
    const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      app_status_bar_enabled: false,
      system_metrics_display: { show_in_topbar: true, simplified: false },
    });
    expect(response.ok).toBe(true);

    const mobile = new MobileKanbanPage(testPage);
    await mobile.goto();
    await testPage.reload();

    const header = testPage.locator("header").first();
    const brand = header.getByTestId("mobile-topbar-brand");
    const menu = header.getByTestId("mobile-topbar-menu");
    const strip = header.getByTestId("mobile-topbar-action-strip");
    await expect(brand).toBeVisible();
    await expect(menu).toBeVisible();
    await expect(strip).toBeVisible();

    await assertNoDocumentHorizontalOverflow(testPage, "mobile kanban topbar");

    const headerBox = await requireBox(header, "topbar");
    const brandBox = await requireBox(brand, "brand link");
    const menuBox = await requireBox(menu, "menu button");
    // Both ends of the bar stay inside it: the strip scrolls, the chrome does
    // not slide out from under the viewport.
    expect(brandBox.x).toBeGreaterThanOrEqual(headerBox.x - 1);
    expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(headerBox.x + headerBox.width + 1);
    // The title crumb keeps its floor rather than being crushed to nothing.
    const titleBox = await requireBox(header.locator('[data-slot="breadcrumb-page"]'), "title");
    expect(titleBox.width).toBeGreaterThan(0);
  });

  test.afterEach(async ({ apiClient }) => {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      system_metrics_display: { show_in_topbar: false },
    });
  });
});
