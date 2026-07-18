import { test, expect } from "../../fixtures/test-base";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

/**
 * Mobile parity for the sidebar Integrations section. The global AppSidebar is
 * desktop-only (`hidden md:block`), so on a phone viewport the configured
 * integration links (GitHub/Jira/Linear/GitLab) and any plugin nav items with
 * `section: "integrations"` are only reachable through the kanban hamburger
 * sheet's Integrations section. This proves a first-party link surfaces there
 * and navigates. Plugin-item filtering is covered by the component test
 * (`components/integrations/integrations-menu.test.ts`), which does not need a
 * real plugin subprocess.
 */
test.describe("Integrations section on mobile", () => {
  test("exposes a configured integration link in the hamburger sheet and navigates", async ({
    testPage,
    apiClient,
  }) => {
    // Make GitHub report authenticated so useConfiguredIntegrationLinks yields
    // the GitHub link before the page loads its status.
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    const kanban = new MobileKanbanPage(testPage);
    await kanban.goto();

    await kanban.mobileMenuButton.click();

    // Scope to the open menu sheet so we never match the display:none desktop
    // sidebar that shares the same "GitHub" link accessible name.
    const sheet = testPage.getByRole("dialog");
    await expect(sheet.getByText("Integrations", { exact: true })).toBeVisible();

    const githubLink = sheet.getByRole("link", { name: "GitHub" });
    await expect(githubLink).toBeVisible({ timeout: 10_000 });
    expect(await githubLink.getAttribute("href")).toBe("/github");

    await githubLink.click();

    // Navigating closes the sheet and lands on the GitHub integration page.
    await expect(testPage).toHaveURL(/\/github$/);
    await expect(sheet).toBeHidden();
    await expect(testPage.getByTestId("github-mobile-menu-button")).toBeVisible({
      timeout: 15_000,
    });
  });
});
