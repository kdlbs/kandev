import { test, expect } from "../../fixtures/office-fixture";

// Regression coverage for the PageShell migration: Office used to render a
// title-only topbar with no menu and no home link, so on a phone the entire
// Office section was unreachable and un-exitable (browser back excepted).
test.describe("Office mobile navigation", () => {
  test("offers office sections and a home row in the shared nav sheet", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");

    const trigger = testPage.getByTestId("app-nav-trigger");
    await expect(trigger).toBeVisible();
    await trigger.click();

    const sheet = testPage.getByTestId("app-nav-sheet");
    await expect(sheet).toBeVisible();

    // Office's own sections render above the global destinations.
    await expect(sheet.getByRole("link", { name: /Routines/ })).toBeVisible();
    await expect(sheet.getByRole("link", { name: /Skills/ })).toBeVisible();
    // Global destinations come from the navigation manifest.
    await expect(sheet.getByRole("link", { name: "Stats" })).toBeVisible();
    await expect(sheet.getByRole("link", { name: "Settings" })).toBeVisible();

    // Home from inside Office lands on the office dashboard, not kanban.
    await sheet.getByRole("link", { name: "Home", exact: true }).click();
    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/office(?:\?.*)?$/);
  });

  test("navigates between office sections from the sheet", async ({ testPage, officeSeed: _ }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");

    await testPage.getByTestId("app-nav-trigger").click();
    const sheet = testPage.getByTestId("app-nav-sheet");
    await sheet.getByRole("link", { name: /Routines/ }).click();

    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(/\/office\/routines$/);
  });

  test("switches workspaces from the Office mobile menu", async ({ testPage, apiClient }) => {
    const kanbanWorkspace = await apiClient.createWorkspace("Mobile Office Kanban Workspace");

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.goto("/office/tasks");
    await testPage.getByTestId("app-nav-trigger").click();

    const sheet = testPage.getByTestId("app-nav-sheet");
    const workspaceTrigger = sheet.getByTestId("mobile-office-workspace-trigger");
    await expect(workspaceTrigger).toContainText("E2E Workspace");
    await workspaceTrigger.click();
    await testPage.getByTestId(`mobile-office-workspace-item-${kanbanWorkspace.id}`).click();

    await expect(sheet).not.toBeVisible();
    await expect(testPage).toHaveURL(
      (url) =>
        url.pathname === "/" &&
        url.searchParams.get("home") === "overview" &&
        url.searchParams.get("workspaceId") === kanbanWorkspace.id,
      { timeout: 10_000 },
    );
  });
});
