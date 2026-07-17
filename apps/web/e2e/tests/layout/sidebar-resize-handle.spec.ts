import { test, expect } from "../../fixtures/test-base";
import { openWideTask } from "../../helpers/dockview-resize";

test.describe("App sidebar resize handle", () => {
  test("straddles the sidebar edge and highlights its full width while dragging", async ({
    testPage,
  }) => {
    await testPage.goto("/");

    const sidebar = testPage.getByTestId("app-sidebar");
    const handle = sidebar.getByRole("button", { name: "Resize sidebar" });
    await expect(handle).toBeVisible();

    const [sidebarBox, handleBox] = await Promise.all([
      sidebar.boundingBox(),
      handle.boundingBox(),
    ]);
    expect(sidebarBox).not.toBeNull();
    expect(handleBox).not.toBeNull();

    const sidebarEdge = sidebarBox!.x + sidebarBox!.width;
    const handleCenter = handleBox!.x + handleBox!.width / 2;
    expect(handleCenter).toBeCloseTo(sidebarEdge, 0);

    await testPage.mouse.move(handleCenter, handleBox!.y + handleBox!.height / 2);
    await testPage.mouse.down();

    await expect(handle).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await testPage.mouse.up();
  });

  test("matches the Dockview sash hover width", async ({ testPage, apiClient, seedData }) => {
    await openWideTask(testPage, apiClient, seedData, "Sidebar sash width");

    const handle = testPage
      .getByTestId("app-sidebar")
      .getByRole("button", { name: "Resize sidebar" });
    const handleBox = await handle.boundingBox();
    const sashWidth = await testPage.locator(".dv-sash").evaluateAll((sashes) => {
      const verticalSash = sashes
        .map((sash) => sash.getBoundingClientRect())
        .find((box) => box.height > box.width);
      return verticalSash?.width ?? 0;
    });

    expect(handleBox).not.toBeNull();
    expect(sashWidth).toBeGreaterThan(0);
    expect(handleBox!.width).toBe(sashWidth);
  });
});
