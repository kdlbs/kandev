import { test, expect } from "../../fixtures/test-base";

// Covers docs/specs/integrations/requirements/enable-disable-toggle.md's per-workspace
// scoping: the enable toggle belongs to the workspace whose settings page it
// sits on, so disabling an integration in one workspace leaves every other
// workspace's toggle alone. It shipped install-wide first, which silenced an
// integration everywhere from a single workspace's slider.
test.describe("integration enable toggles are scoped to their workspace", () => {
  test("disabling GitHub in one workspace leaves the other workspace enabled", async ({
    testPage,
    apiClient,
    prCapture,
  }) => {
    const first = await apiClient.createWorkspace("Toggle Scope A");
    const second = await apiClient.createWorkspace("Toggle Scope B");

    const firstIntegrations = `/settings/workspaces/${first.id}/integrations`;
    const secondIntegrations = `/settings/workspaces/${second.id}/integrations`;

    await testPage.goto(firstIntegrations);
    const githubSwitch = testPage.locator("#github-enabled");
    await expect(githubSwitch).toHaveAttribute("aria-checked", "true");
    await githubSwitch.click();
    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    await prCapture.screenshot("workspace-a-github-disabled", {
      caption: "Workspace A: GitHub disabled",
    });

    // The other workspace never asked for GitHub to be off.
    await testPage.goto(secondIntegrations);
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "true");
    await prCapture.screenshot("workspace-b-github-still-enabled", {
      caption: "Workspace B: GitHub still enabled",
    });

    // ...and GitHub's own settings page under that workspace agrees.
    await testPage.goto(`${secondIntegrations}/github`);
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "true");
    await prCapture.screenshot("workspace-b-github-page-enabled", {
      caption: "Workspace B, GitHub settings page: still enabled",
    });

    // The workspace that disabled it still reads disabled on both surfaces.
    await testPage.goto(firstIntegrations);
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "false");
    await testPage.goto(`${firstIntegrations}/github`);
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "false");
  });
});
