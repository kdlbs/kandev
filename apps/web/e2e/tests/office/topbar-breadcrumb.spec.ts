import { test, expect } from "../../fixtures/office-fixture";

test.describe("Topbar breadcrumb", () => {
  test("issue detail shows task title", async ({ testPage, apiClient, officeSeed }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Breadcrumb Test Task", {
      workflow_id: officeSeed.workflowId,
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByRole("heading", { name: "Breadcrumb Test Task" })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("tasks list shows Tasks heading", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/tasks");
    await expect(testPage.getByRole("heading", { name: /Tasks/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  // Regression: the office topbar bottom border must line up with the AppSidebar
  // header's bottom border (the line under the workspace picker) — both are h-10
  // so the two horizontal borders form one continuous seam where the sidebar
  // meets the page content. Previously the topbar was h-12 and sat ~8px lower.
  test("topbar bottom aligns with sidebar header bottom", async ({ testPage, officeSeed: _ }) => {
    // The seam we assert exists only for the EXPANDED sidebar header — its
    // border sits under the workspace picker, and data-testid="app-sidebar-header"
    // lives on the expanded layout. Force the collapse flag off before load so a
    // leftover collapsed state can't turn this into a confusing timeout (the
    // collapsed-rail header carries no testid) rather than a real assertion.
    await testPage.addInitScript(() => {
      window.localStorage.setItem("kandev.appSidebar.collapsed", "false");
    });
    await testPage.setViewportSize({ width: 1280, height: 900 });
    await testPage.goto("/office/inbox");

    const topbar = testPage.getByTestId("office-topbar");
    const sidebarHeader = testPage.getByTestId("app-sidebar-header");
    await expect(topbar).toBeVisible({ timeout: 10_000 });
    await expect(sidebarHeader).toBeVisible({ timeout: 10_000 });

    const topbarBox = await topbar.boundingBox();
    const sidebarBox = await sidebarHeader.boundingBox();
    expect(topbarBox).not.toBeNull();
    expect(sidebarBox).not.toBeNull();

    // Same height (both h-10) and same bottom y-position → flush borders.
    expect(Math.abs(topbarBox!.height - sidebarBox!.height)).toBeLessThanOrEqual(1);
    const topbarBottom = topbarBox!.y + topbarBox!.height;
    const sidebarBottom = sidebarBox!.y + sidebarBox!.height;
    expect(Math.abs(topbarBottom - sidebarBottom)).toBeLessThanOrEqual(1);
  });
});
