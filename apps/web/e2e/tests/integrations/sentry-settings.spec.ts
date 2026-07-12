import { test, expect } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";

test.describe("Sentry settings — instances", () => {
  // The settings page manages a LIST of named instances per workspace. This
  // covers add → the card appears → the backend probe flips it healthy.
  test("adds a named instance and reports it healthy", async ({ testPage, apiClient }) => {
    await apiClient.mockSentryReset();

    const settings = new SentrySettingsPage(testPage);
    await settings.goto();

    // The add form pre-fills the SaaS URL default.
    await settings.addInstanceButton.click();
    await expect(settings.addUrlInput).toHaveValue("https://sentry.io");

    await settings.addNameInput.fill("Production");
    await settings.addSecretInput.fill(TOKEN);
    await settings.addSaveButton.click();

    // The instance now renders as a card. Its async probe (unseeded mock = OK)
    // flips lastOk true; reload to pick up the health banner deterministically.
    await expect(settings.cardByName("Production")).toBeVisible();
    await apiClient.waitForIntegrationAuthHealthy("sentry");
    await settings.goto();
    await expect(
      settings.cardByName("Production").getByTestId("integration-auth-status-banner"),
    ).toHaveAttribute("data-state", "ok");
  });

  // Two instances coexist in one workspace; a watcher bound to one FK-protects
  // it, so deleting that instance is blocked (409 SENTRY_INSTANCE_IN_USE with a
  // watch count) while the unbound instance deletes cleanly.
  test("blocks deleting an instance a watcher still uses", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockSentryReset();
    const primary = await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Primary",
      secret: TOKEN,
    });
    const secondary = await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Secondary",
      url: "https://sentry.acme.example.com",
      secret: TOKEN,
    });
    await apiClient.mockSentrySetAuthHealth({ instanceId: primary.id, ok: true });
    await apiClient.mockSentrySetAuthHealth({ instanceId: secondary.id, ok: true });

    // Bind a watch to the primary instance only.
    await apiClient.createSentryIssueWatch({
      workspaceId: seedData.workspaceId,
      sentryInstanceId: primary.id,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      agentProfileId: seedData.agentProfileId,
      orgSlug: "acme",
      projectSlug: "web",
    });

    // API contract: delete-in-use rejects 409 and reports the blocking count.
    const res = await apiClient.deleteSentryInstanceRaw(seedData.workspaceId, primary.id);
    expect(res.status).toBe(409);
    const body = (await res.json()) as { code?: string; watchCount?: number };
    expect(body.code).toBe("SENTRY_INSTANCE_IN_USE");
    expect(body.watchCount).toBe(1);

    // UI: the delete is surfaced as an error and the primary card survives; the
    // watcher-free secondary deletes cleanly.
    const settings = new SentrySettingsPage(testPage);
    await settings.goto();
    await expect(settings.cards).toHaveCount(2);
    testPage.on("dialog", (d) => d.accept());

    await settings.cardByName("Primary").getByTestId("sentry-instance-delete-button").click();
    await expect(testPage.getByText(/still bound to it/i)).toBeVisible();
    await expect(settings.cardByName("Primary")).toBeVisible();

    await settings.cardByName("Secondary").getByTestId("sentry-instance-delete-button").click();
    await expect(settings.cardByName("Secondary")).toHaveCount(0);
    await expect(settings.cardByName("Primary")).toBeVisible();
  });
});
