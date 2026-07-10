import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office workspace picker", () => {
  test("labels workspace types and keeps the current workspace selection as a no-op", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const kanbanWorkspace = await apiClient.createWorkspace("Picker Kanban Workspace");

    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await expect(testPage).toHaveURL(/\/office$/);

    await testPage.getByTestId("sidebar-workspace-trigger").click();

    const kanbanWorkspaceItem = testPage.getByTestId(
      `sidebar-workspace-item-${kanbanWorkspace.id}`,
    );
    const officeWorkspace = testPage.getByTestId(
      `sidebar-workspace-item-${officeSeed.workspaceId}`,
    );
    await expect(kanbanWorkspaceItem).toContainText("Kanban");
    await expect(officeWorkspace).toContainText("Office");

    await officeWorkspace.click();

    await expect(testPage).toHaveURL(/\/office$/);
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    const reopenedKanbanWorkspaceItem = testPage.getByTestId(
      `sidebar-workspace-item-${kanbanWorkspace.id}`,
    );
    await expect(reopenedKanbanWorkspaceItem).toBeVisible();
    await reopenedKanbanWorkspaceItem.click();
    await expect(testPage).toHaveURL(
      (url) => {
        return url.pathname === "/" && url.searchParams.get("workspaceId") === kanbanWorkspace.id;
      },
      { timeout: 10_000 },
    );
  });

  test("offers separate kanban and office workspace creation actions", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByRole("menuitem", { name: "New office workspace" }).click();
    await expect(testPage).toHaveURL(/\/office\/setup\?mode=new/, { timeout: 10_000 });
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible({ timeout: 10_000 });

    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByRole("menuitem", { name: "New kanban workspace" }).click();
    await expect(testPage).toHaveURL(/\/settings\/workspace$/, { timeout: 10_000 });
  });
});
