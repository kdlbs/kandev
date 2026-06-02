import { test, expect } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";

test.describe("Sentry settings", () => {
  // The settings page configures only the auth token; org/project are chosen
  // per-watcher and per-browse, not stored install-wide. This covers the
  // connect → save → validated flow and persistence of the saved token.
  test("connects, saves, and reports a healthy token", async ({ testPage, apiClient }) => {
    await apiClient.mockSentryReset();
    await apiClient.mockSentrySetAuthResult({
      ok: true,
      userId: "u-1",
      displayName: "Sentry Tester",
      email: "tester@example.com",
    });

    const settings = new SentrySettingsPage(testPage);
    await settings.goto();

    // Connection test surfaces inline success ("Connected as <displayName>").
    await settings.secretInput.fill(TOKEN);
    await settings.testButton.click();
    await expect(testPage.getByText(/Connected as Sentry Tester/)).toBeVisible();

    // Save persists the secret and triggers the auth-health probe that flips
    // lastOk -> true.
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveText(/Update/i);
    await apiClient.waitForIntegrationAuthHealthy("sentry");

    // Reload: the saved token survives (placeholder shows the masked state and
    // the action becomes "Update"/"Remove configuration").
    await settings.goto();
    await expect(settings.saveButton).toHaveText(/Update/i);
    await expect(settings.deleteButton).toBeVisible();
  });
});
