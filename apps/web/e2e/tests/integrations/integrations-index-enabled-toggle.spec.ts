import { test, expect } from "../../fixtures/test-base";

// Covers docs/specs/integrations/enable-disable-toggle.md's index-page slider
// scenarios: every integration row has a working slider, toggling it never
// navigates, and the toggle stays in sync with that integration's own
// settings page (the shared install-wide `useXEnabled` state).
test.describe("integrations index page enable/disable sliders", () => {
  test("every integration row has a slider, toggling it does not navigate, and it stays in sync with the own-page slider", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/integrations");

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
    await expect(testPage).toHaveURL(/\/settings\/integrations$/);

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
