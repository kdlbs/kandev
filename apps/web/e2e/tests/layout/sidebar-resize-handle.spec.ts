import { test, expect } from "../../fixtures/test-base";

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
    expect(handleCenter).toBeCloseTo(sidebarEdge, 1);

    await testPage.mouse.move(handleCenter, handleBox!.y + handleBox!.height / 2);
    await testPage.mouse.down();
    await testPage.mouse.move(handleCenter + 20, handleBox!.y + handleBox!.height / 2);

    await expect(handle).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await testPage.mouse.up();
  });
});
