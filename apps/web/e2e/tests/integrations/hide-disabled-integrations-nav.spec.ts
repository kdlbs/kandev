import { test, expect } from "../../fixtures/test-base";

// Covers docs/specs/integrations/enable-disable-toggle.md's nav-visibility
// scenarios: with "Hide disabled integrations from left panel navigation"
// off (the default), a disabled-but-configured integration still shows in
// the sidebar; turning the setting on hides it; turning it back off (or
// re-enabling the integration) reveals it again — all without a reload.
test.describe("hide disabled integrations from left panel navigation", () => {
  test("off by default keeps a disabled integration visible; on hides it; re-enabling reveals it", async ({
    testPage,
    apiClient,
  }) => {
    // Make GitHub configured/healthy so it is eligible to appear in the nav
    // regardless of its enabled state.
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    await testPage.goto("/settings/integrations");

    const githubSwitch = testPage.locator("#github-enabled");
    await expect(githubSwitch).toHaveAttribute("aria-checked", "true");
    await githubSwitch.click();
    await expect(githubSwitch).toHaveAttribute("aria-checked", "false");

    const hideDisabledSwitch = testPage.locator("#hide-disabled-integrations-in-nav");
    await expect(hideDisabledSwitch).toHaveAttribute("aria-checked", "false");

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    // Leave Settings — the sidebar's Integrations section only renders
    // outside the Settings takeover.
    await testPage.goto("/tasks");
    const githubNavLink = testPage.getByRole("link", { name: "GitHub" });
    await expect(githubNavLink).toBeVisible({ timeout: 10_000 });

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
    await expect(testPage.getByRole("link", { name: "GitHub" })).not.toBeVisible();

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
    await expect(testPage.getByRole("link", { name: "GitHub" })).toBeVisible({ timeout: 10_000 });
  });
});
