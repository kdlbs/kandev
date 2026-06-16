import { test, expect } from "../../fixtures/office-fixture";
import { AppSidebarPage } from "../../pages/app-sidebar-page";

async function sectionTop(sidebar: AppSidebarPage, label: string): Promise<number> {
  const box = await sidebar.root.getByRole("button", { name: label, exact: true }).boundingBox();
  if (!box) throw new Error(`Missing sidebar section header: ${label}`);
  return box.y;
}

test.describe("Sidebar workspace mode navigation", () => {
  test("routes brand and footer toggle by active workspace type", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const kanbanWorkspace = await apiClient.createWorkspace("Sidebar Mode Kanban Workspace");
    const sidebar = new AppSidebarPage(testPage);

    await testPage.goto("/");
    await testPage.getByTestId("sidebar-workspace-trigger").click();
    await testPage.getByTestId(`sidebar-workspace-item-${kanbanWorkspace.id}`).click();
    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === kanbanWorkspace.id,
      { timeout: 10_000 },
    );

    await expect(sidebar.root.getByRole("link", { name: "Kandev home" })).toHaveAttribute(
      "href",
      `/?workspaceId=${kanbanWorkspace.id}`,
    );
    await expect(sidebar.root.getByRole("button", { name: "Office" })).toBeVisible();
    await sidebar.root.getByRole("button", { name: "Office" }).click();

    await expect(testPage).toHaveURL(
      (url) =>
        url.pathname === "/office" &&
        url.searchParams.get("workspaceId") === officeSeed.workspaceId,
      { timeout: 10_000 },
    );
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
    await expect(sidebar.root.getByRole("link", { name: "Kandev home" })).toHaveAttribute(
      "href",
      `/office?workspaceId=${officeSeed.workspaceId}`,
    );
    await expect(sidebar.root.getByRole("button", { name: "Kanban" })).toBeVisible();
    await sidebar.root.getByRole("button", { name: "Kanban" }).click();

    await expect(testPage).toHaveURL(
      (url) => url.pathname === "/" && url.searchParams.get("workspaceId") === kanbanWorkspace.id,
      { timeout: 10_000 },
    );
    await expect(sidebar.root.getByRole("link", { name: "Kandev home" })).toHaveAttribute(
      "href",
      `/?workspaceId=${kanbanWorkspace.id}`,
    );
  });

  test("orders office groups and keeps collapsed group actions visible", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    const sidebar = new AppSidebarPage(testPage);
    const orderedTops = await Promise.all(
      ["Work", "Projects", "Agents", "Office"].map((label) => sectionTop(sidebar, label)),
    );
    expect(orderedTops).toEqual([...orderedTops].sort((a, b) => a - b));

    await sidebar.root.getByRole("button", { name: "Projects", exact: true }).click();
    await expect(sidebar.root.getByRole("button", { name: "Add project" })).toBeVisible();

    await sidebar.root.getByRole("button", { name: "Agents", exact: true }).click();
    await expect(sidebar.root.getByRole("link", { name: "Agent topology" })).toBeVisible();
    await expect(sidebar.root.getByRole("button", { name: "Add agent" })).toBeVisible();
  });
});
