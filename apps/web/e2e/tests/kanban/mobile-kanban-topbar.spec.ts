import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";

test.describe("Shared phone listing topbar", () => {
  test("keeps all three modes compact with native context and menu targets", async ({
    testPage,
  }) => {
    test.setTimeout(120_000);
    for (const width of [360, 393]) {
      await testPage.setViewportSize({ width, height: 851 });
      for (const [route, label] of [
        ["/?home=overview", "Kanban"],
        ["/tasks", "List"],
        ["/threads", "All threads"],
      ]) {
        if (label === "Kanban") {
          await testPage.goto("/tasks");
          await testPage.getByTestId("mobile-topbar-menu").tap();
          await testPage.getByRole("radio", { name: "Kanban", exact: true }).tap();
          await expect(testPage.getByRole("dialog")).toHaveCount(0);
        } else {
          await testPage.goto(route);
        }
        const header = testPage.locator("header").first();
        const context = header.getByTestId(
          label === "All threads" ? "threads-mobile-view-trigger" : "mobile-topbar-page-context",
        );
        const menu = header.getByTestId("mobile-topbar-menu");
        await expect(context).toContainText(label);
        await expect(header.getByTestId("mobile-topbar-brand")).toHaveCount(0);
        await expect(header.getByTestId("mobile-topbar-action-strip")).toHaveCount(0);
        const headerBox = await requireBox(header, "shared phone header");
        expect(headerBox.height).toBeCloseTo(56, 0);
        for (const target of [context, menu]) {
          const box = await requireBox(target, "phone header control");
          expect(box.height).toBeGreaterThanOrEqual(44);
          expect(box.width).toBeGreaterThanOrEqual(44);
          expect(box.x + box.width).toBeLessThanOrEqual(width);
        }
        await assertNoDocumentHorizontalOverflow(testPage, `${label} at ${width}px`);
        await context.tap();
        await expect(testPage.getByRole("dialog")).toBeVisible();
        if (label === "All threads") {
          await expect(testPage.getByTestId("threads-mobile-view-drawer")).toBeVisible();
        } else {
          await expect(testPage.getByRole("radio", { name: label, exact: true })).toHaveAttribute(
            "data-state",
            "on",
          );
        }
        await testPage.keyboard.press("Escape");
        await expect(testPage.getByRole("dialog")).toHaveCount(0);
        await expect(context).toBeFocused();
        await menu.tap();
        await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
        await testPage.keyboard.press("Escape");
        await expect(testPage.getByRole("dialog")).toHaveCount(0);
        await expect(menu).toBeFocused();
      }
    }
  });

  test("keeps workspace-aware Home in the menu and restores the remembered list", async ({
    testPage,
  }) => {
    await testPage.goto("/tasks");
    await testPage.getByTestId("mobile-topbar-menu").tap();
    const dialog = testPage.getByRole("dialog", { name: "Menu", exact: true });
    await dialog.getByRole("link", { name: "Home", exact: true }).tap();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/tasks" && url.searchParams.has("workspace"),
    );
    await expect(testPage.getByTestId("mobile-topbar-page-context")).toContainText("List");
    await expect(dialog).toHaveCount(0);
  });

  test("leaves the coarse-pointer tablet header composition intact", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 820, height: 1180 });
    await testPage.goto("/?home=overview");
    const header = testPage.locator("header").first();
    await expect(header.getByTestId("mobile-topbar-page-context")).toHaveCount(0);
    await expect(header.getByTestId("mobile-topbar-menu")).toHaveCount(0);
    await expect(header).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "tablet listing header");
    const menu = header.getByRole("button", { name: "Open menu", exact: true });
    await expect(menu).toBeVisible();
    // Tablet retains its existing composition and direct 44px tool launchers.
    for (const id of ["tablet-quick-chat-button", "tablet-quick-terminal-button"]) {
      const box = await requireBox(header.getByTestId(id), "tablet launcher");
      expect(box.height).toBeGreaterThanOrEqual(44);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    await menu.tap();
    await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
    await expect(testPage.getByRole("radio", { name: "Pipeline", exact: true })).toBeVisible();
  });

  test("contains a long workspace name without squeezing the phone menu", async ({
    testPage,
    apiClient,
  }) => {
    const name = "Harbor checkout accessibility and international payment reconciliation";
    const workspace = await apiClient.createWorkspace(name);
    try {
      await testPage.setViewportSize({ width: 360, height: 851 });
      await testPage.goto(`/tasks?workspace=${workspace.id}`);
      const context = testPage.getByTestId("mobile-topbar-page-context");
      await expect(context).toContainText(name);
      const label = context.locator("span.truncate").first();
      expect(await label.evaluate((element) => element.scrollWidth > element.clientWidth)).toBe(
        true,
      );
      const menu = testPage.getByTestId("mobile-topbar-menu");
      const box = await requireBox(menu, "menu beside long workspace");
      expect(box.width).toBe(44);
      expect(box.x + box.width).toBeLessThanOrEqual(360);
      await assertNoDocumentHorizontalOverflow(testPage, "long workspace context");
      await context.tap();
      await expect(testPage.getByRole("dialog", { name: "Menu", exact: true })).toBeVisible();
    } finally {
      await apiClient.deleteWorkspace(workspace.id, name);
    }
  });
});
