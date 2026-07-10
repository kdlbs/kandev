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

    const projectsHeader = sidebar.root.getByRole("button", { name: "Projects", exact: true });
    await expect(projectsHeader).toHaveAttribute("aria-expanded", "true");
    await projectsHeader.click();
    await expect(projectsHeader).toHaveAttribute("aria-expanded", "false");
    await expect(sidebar.root.getByRole("button", { name: "Add project" })).toBeVisible();

    const agentsHeader = sidebar.root.getByRole("button", { name: "Agents", exact: true });
    await expect(agentsHeader).toHaveAttribute("aria-expanded", "true");
    await agentsHeader.click();
    await expect(agentsHeader).toHaveAttribute("aria-expanded", "false");
    await expect(sidebar.root.getByRole("link", { name: "Agent topology" })).toBeVisible();
    await expect(sidebar.root.getByRole("button", { name: "Add agent" })).toBeVisible();
  });

  test("scrolls office navigation above the footer on low-height desktop viewports", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.setViewportSize({ width: 1024, height: 360 });
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    const sidebar = new AppSidebarPage(testPage);
    const scrollRegion = sidebar.root.getByTestId("app-sidebar-scroll");
    const fade = sidebar.root.getByTestId("app-sidebar-bottom-fade");
    await expect(scrollRegion).toBeVisible();
    await expect(fade).toBeVisible();

    const scrollMetrics = await scrollRegion.evaluate((node) => {
      const styles = window.getComputedStyle(node);
      return {
        clientHeight: node.clientHeight,
        overflowY: styles.overflowY,
        scrollHeight: node.scrollHeight,
      };
    });
    expect(scrollMetrics.overflowY).toBe("auto");
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);

    const preferencesLink = sidebar.root.getByRole("link", { name: "Preferences" });
    await preferencesLink.scrollIntoViewIfNeeded();
    await expect(preferencesLink).toBeVisible();

    const [preferencesBox, fadeBox] = await Promise.all([
      preferencesLink.boundingBox(),
      fade.boundingBox(),
    ]);
    expect(preferencesBox).not.toBeNull();
    expect(fadeBox).not.toBeNull();
    expect(preferencesBox!.y + preferencesBox!.height).toBeLessThanOrEqual(fadeBox!.y + 1);
  });

  test("shows only user-defined skills in the muted office sidebar badge", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.listSkills(officeSeed.workspaceId);
    await testPage.goto("/office");
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    const sidebar = new AppSidebarPage(testPage);
    const skillsNavLink = sidebar.root.getByRole("link", { name: "Skills", exact: true });
    await expect(skillsNavLink).toBeVisible();
    await expect(skillsNavLink.getByText("13")).toHaveCount(0);

    const skillSlug = `sidebar-count-skill-${Date.now()}`;
    await officeApi.createSkill(officeSeed.workspaceId, {
      name: "Sidebar Count Skill",
      slug: skillSlug,
      content: "# Sidebar Count Skill\n",
    });
    await testPage.reload();
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });

    await expect(skillsNavLink).toBeVisible();
    const badge = skillsNavLink.getByText("1");
    await expect(badge).toBeVisible();
    await expect(badge).toHaveClass(/bg-muted/);
    await expect(badge).toHaveClass(/text-muted-foreground/);
  });
});
