import { test, expect } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";
const ORG_ACME = "acme";
const PROJECT_BACKEND = "Backend (backend)";

test.describe("Sentry settings", () => {
  // The org/project selectors gate on `validated = hasSecret && lastOk`, which is
  // only true after the config is SAVED and the async post-save auth-health probe
  // lands. Test connection alone does not flip lastOk on the persisted row, so the
  // order is: enter token -> Save -> wait for validated -> dropdowns appear.
  test("populates org/project dropdowns once validated and persists the selection", async ({
    testPage,
    apiClient,
  }) => {
    await apiClient.mockSentryReset();
    await apiClient.mockSentrySetAuthResult({
      ok: true,
      userId: "u-1",
      displayName: "Sentry Tester",
      email: "tester@example.com",
    });
    await apiClient.mockSentrySetOrganizations([
      { id: "org-acme", slug: "acme", name: "Acme" },
      { id: "org-globex", slug: "globex", name: "Globex" },
    ]);
    await apiClient.mockSentrySetProjects([
      { id: "proj-backend", slug: "backend", name: "Backend", orgSlug: "acme" },
      { id: "proj-frontend", slug: "frontend", name: "Frontend", orgSlug: "acme" },
      { id: "proj-warehouse", slug: "warehouse", name: "Warehouse", orgSlug: "globex" },
    ]);

    const settings = new SentrySettingsPage(testPage);
    await settings.goto();

    // Connection test surfaces inline success ("Connected as <displayName>").
    await settings.secretInput.fill(TOKEN);
    await settings.testButton.click();
    await expect(testPage.getByText(/Connected as Sentry Tester/)).toBeVisible();

    // Save persists the secret and triggers the auth-health probe that flips
    // lastOk -> true, which is what reveals the org/project selectors.
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveText(/Update/i);
    await apiClient.waitForIntegrationAuthHealthy("sentry");

    // The page only re-polls config every INTEGRATION_STATUS_REFRESH_MS (90s), so
    // reload to pick up the freshly-validated config immediately rather than wait
    // out the interval. Once validated the org/project selectors render.
    await settings.goto();
    await expect(settings.orgSelect).toBeVisible();
    await expect(settings.projectSelect).toBeVisible();

    // Org dropdown lists both seeded orgs.
    await settings.orgSelect.click();
    await expect(testPage.getByRole("option", { name: ORG_ACME })).toBeVisible();
    await expect(testPage.getByRole("option", { name: "globex" })).toBeVisible();
    await testPage.getByRole("option", { name: ORG_ACME }).click();
    await expect(settings.orgSelect).toContainText(ORG_ACME);

    // Project dropdown is scoped to the selected org: only acme's projects show.
    await settings.projectSelect.click();
    await expect(testPage.getByRole("option", { name: PROJECT_BACKEND })).toBeVisible();
    await expect(testPage.getByRole("option", { name: "Frontend (frontend)" })).toBeVisible();
    await expect(testPage.getByRole("option", { name: "Warehouse (warehouse)" })).toHaveCount(0);
    await testPage.getByRole("option", { name: PROJECT_BACKEND }).click();
    await expect(settings.projectSelect).toContainText(PROJECT_BACKEND);

    // Persist the org + project selection, then reload and confirm they survive.
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveText(/Update/i);

    await settings.goto();
    await expect(settings.orgSelect).toBeVisible();
    await expect(settings.orgSelect).toContainText(ORG_ACME);
    await expect(settings.projectSelect).toContainText(PROJECT_BACKEND);
  });
});
