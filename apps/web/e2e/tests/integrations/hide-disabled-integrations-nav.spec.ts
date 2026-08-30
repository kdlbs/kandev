import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SETTINGS_TAKEOVER_TESTID, setSettingsMenuMode } from "../../helpers/settings-menu";

function sidebarGitHubRow(testPage: Page) {
  const sidebar = testPage.getByTestId("app-sidebar");
  return sidebar.locator('a[href="/github"]:not([data-testid="integration-header-shortcut"])');
}

async function expandIntegrationsSection(testPage: Page) {
  const sidebar = testPage.getByTestId("app-sidebar");
  const integrationsToggle = sidebar.getByRole("button", {
    name: "Integrations",
    exact: true,
  });
  await expect(integrationsToggle).toHaveCount(1, { timeout: 10_000 });
  if ((await integrationsToggle.getAttribute("aria-expanded")) !== "true") {
    await integrationsToggle.click();
  }
  await expect(integrationsToggle).toHaveAttribute("aria-expanded", "true");
}

// Covers docs/specs/integrations/requirements/enable-disable-toggle.md's nav-visibility
// scenarios: with "Hide disabled integrations from left panel navigation"
// off (the default), a disabled-but-configured integration still shows in
// the sidebar; turning the setting on hides it; turning it back off (or
// re-enabling the integration) reveals it again — all without a reload.
test.describe("hide disabled integrations from left panel navigation", () => {
  test("off by default keeps a disabled integration visible; on hides it; re-enabling reveals it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Make GitHub configured/healthy so it is eligible to appear in the nav
    // regardless of its enabled state.
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await testPage.goto("/settings/integrations");
    const hideDisabledSwitch = testPage.locator("#hide-disabled-integrations-in-nav");
    if ((await hideDisabledSwitch.getAttribute("aria-checked")) === "true") {
      await hideDisabledSwitch.click();
    }
    await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "false");

    const githubSwitch = testPage.locator("#github-enabled");
    if ((await githubSwitch.getAttribute("aria-checked")) === "false") {
      await githubSwitch.click();
    }
    await expect(githubSwitch).toHaveAttribute("aria-checked", "true");
    await githubSwitch.click();
    await expect(githubSwitch).toHaveAttribute("aria-checked", "false");

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    // Leave Settings — the sidebar's Integrations section only renders
    // outside the Settings takeover.
    await testPage.goto("/tasks");
    // The header also exposes an always-visible GitHub shortcut. Scope this
    // assertion to the actual sidebar row so it tests the setting's contract
    // instead of a separate shortcut surface.
    await expandIntegrationsSection(testPage);
    const githubNavLink = sidebarGitHubRow(testPage);
    await expect(githubNavLink).toBeVisible({ timeout: 10_000 });

    // The Settings left panel's per-workspace Integrations tree is also part
    // of left-panel navigation: with "hide disabled" off (default) the
    // disabled-but-configured GitHub entry must stay listed there too.
    // The tree is opt-in — `flat`, the default menu mode, renders no branches
    // at all — so choose a tree mode before asserting on its rows.
    await setSettingsMenuMode(testPage, "accordion");
    // The setting above is scoped to the worker's seeded workspace. Do not
    // use the first workspace returned by the API: another test can create a
    // workspace earlier in the worker, and its GitHub state is independent.
    const workspaceId = seedData.workspaceId;
    const integrationsPath = `/settings/workspaces/${workspaceId}/integrations`;
    await testPage.goto(integrationsPath);
    const settingsTree = testPage.getByTestId(SETTINGS_TAKEOVER_TESTID);
    // By href, not by name: a workspace another spec created can itself be
    // called "GitHub", and the row's own name gains a badge once the
    // integration reports connected — neither of which changes its route.
    const githubTreeRow = settingsTree.locator(`a[href="${integrationsPath}/github"]`);
    const azureTreeRow = settingsTree.locator(`a[href="${integrationsPath}/azure-devops"]`);
    await expect(githubTreeRow).toBeVisible({ timeout: 10_000 });

    // Turn "hide disabled" on.
    await testPage.goto("/settings/integrations");
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "false");
    await testPage.locator("#hide-disabled-integrations-in-nav").click();
    await expect(testPage.locator("#hide-disabled-integrations-in-nav")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    await testPage.goto("/tasks");
    await expect(sidebarGitHubRow(testPage)).toHaveCount(0);

    // The Settings left-panel Integrations tree hides it as well.
    await testPage.goto(integrationsPath);
    await expect(azureTreeRow).toBeVisible({ timeout: 10_000 });
    await expect(githubTreeRow).toHaveCount(0);

    // Re-enabling GitHub reveals it again even with "hide disabled" still on.
    await testPage.goto("/settings/integrations");
    await testPage.locator("#github-enabled").click();
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "true");
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    await testPage.goto("/tasks");
    await expandIntegrationsSection(testPage);
    await expect(sidebarGitHubRow(testPage)).toBeVisible({ timeout: 10_000 });
  });
});
