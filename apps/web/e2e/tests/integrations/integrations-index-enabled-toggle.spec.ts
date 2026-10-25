import { test, expect } from "../../fixtures/test-base";
import { setSettingsMenuMode, SETTINGS_TAKEOVER_TESTID } from "../../helpers/settings-menu";

// Covers docs/specs/integrations/requirements/enable-disable-toggle.md's index-page slider
// scenarios: every integration row has a working slider, toggling it never
// navigates, and the toggle stays in sync with that integration's own
// settings page (the shared per-workspace `useXEnabled` state).
test.describe("integrations index page enable/disable sliders", () => {
  // @covers AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.9
  test("saved GitHub and GitLab toggles update their Settings navigation badges", async ({
    testPage,
    seedData,
  }) => {
    // Keep credential status connected throughout: only the saved toggle changes.
    for (const slug of ["github", "gitlab"]) {
      await testPage.route(`**/api/v1/${slug}/status?*`, (route) =>
        route.fulfill({
          json: {
            workspace_id: seedData.workspaceId,
            authenticated: true,
            token_configured: true,
            username: "badge-test",
            auth_method: "pat",
            host: slug === "gitlab" ? "gitlab.com" : "github.com",
            required_scopes: [],
          },
        }),
      );
    }
    await setSettingsMenuMode(testPage, "persistent");
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations`);
    const menu = testPage.getByTestId(SETTINGS_TAKEOVER_TESTID);
    const save = testPage.getByTestId("settings-floating-save");

    for (const slug of ["github", "gitlab"]) {
      const row = menu.locator(
        `a[href="/settings/workspaces/${seedData.workspaceId}/integrations/${slug}"]`,
      );
      const toggle = testPage.locator(`#${slug}-enabled`);
      await expect(row).toBeVisible();
      await expect(row).toContainText("Enabled");
      await toggle.click();
      await expect(toggle).toHaveAttribute("aria-checked", "false");
      await expect(row).toContainText("Enabled");
      await save.getByRole("button", { name: "Save changes" }).click();
      await expect(save).not.toBeVisible();
      await expect(row).toBeVisible();
      await expect(row).not.toContainText("Enabled");

      await toggle.click();
      await save.getByRole("button", { name: "Save changes" }).click();
      await expect(save).not.toBeVisible();
      await expect(row).toContainText("Enabled");
    }
  });

  test("every integration row has a slider, toggling it does not navigate, and it stays in sync with the own-page slider", async ({
    testPage,
  }) => {
    // The install-level index redirects to the active workspace's integrations
    // tab — that page is now the integrations index the spec describes.
    await testPage.goto("/settings/integrations");
    await expect(testPage).toHaveURL(/\/settings\/workspaces\/[^/]+\/integrations$/);
    const indexUrl = testPage.url();

    // One slider per integration row.
    const slugs = ["azure-devops", "github", "gitlab", "jira", "linear", "sentry"];
    for (const slug of slugs) {
      await expect(testPage.locator(`#${slug}-enabled`)).toBeVisible();
    }

    const githubSwitch = testPage.locator("#github-enabled");
    await expect(githubSwitch).toHaveAttribute("aria-checked", "true");

    // Toggling the slider must not navigate away from the index page.
    await githubSwitch.click();
    await expect(githubSwitch).toHaveAttribute("aria-checked", "false");
    await expect(testPage).toHaveURL(indexUrl);

    await testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" })
      .click();
    await expect(testPage.getByTestId("settings-floating-save")).not.toBeVisible();

    // GitHub's own settings page reflects the same persisted state.
    await testPage.goto("/settings/integrations/github");
    await expect(testPage.locator("#github-enabled")).toHaveAttribute("aria-checked", "false");

    // The GitHub page uses the same compact section header as the other
    // integration settings pages, with its toggle in that header.
    const githubHeading = testPage.locator("h3[data-testid='github-integration-heading']");
    await expect(githubHeading).toBeVisible();
    await expect(
      testPage.locator("section").filter({ has: githubHeading }).locator("#github-enabled"),
    ).toBeVisible();
  });
});
