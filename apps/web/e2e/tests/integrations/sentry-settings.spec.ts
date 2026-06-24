import { test, expect } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";

test.describe("Sentry settings", () => {
  // The settings page manages a list of named Sentry instances, each with its
  // own auth token, URL, and independent backend auth-health banner.
  test("adds multiple instances and renders independent per-instance health", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);
    await apiClient.mockSentryReset();
    await apiClient.mockSentrySetAuthResult({
      ok: true,
      userId: "u-1",
      displayName: "Sentry Tester",
    });

    const settings = new SentrySettingsPage(testPage);
    await settings.goto();

    // Fresh install: no instances yet.
    await expect(settings.emptyState).toBeVisible();

    // Add a SaaS instance and a self-hosted one with distinct names.
    await settings.addInstance({ name: "Production", url: "https://sentry.io", secret: TOKEN });
    await settings.addInstance({
      name: "Staging",
      url: "https://sentry.acme.example.com",
      secret: TOKEN,
    });

    // Both cards render with their own URL.
    await expect(testPage.getByTestId("sentry-instance-card")).toHaveCount(2);
    await expect(settings.card("Production")).toContainText("https://sentry.io");
    await expect(settings.card("Staging")).toContainText("https://sentry.acme.example.com");

    // The async create probe (mock auth-result ok) flips both to healthy.
    const prod = await apiClient.findSentryInstanceByName("Production");
    const staging = await apiClient.findSentryInstanceByName("Staging");
    expect(prod).not.toBeNull();
    expect(staging).not.toBeNull();
    await apiClient.waitForSentryInstanceHealthy(prod!.id);
    await apiClient.waitForSentryInstanceHealthy(staging!.id);

    // Drive the two to DIFFERENT health so the per-instance banners are
    // demonstrably independent: Staging fails while Production stays healthy.
    await apiClient.mockSentrySetAuthHealth({
      instanceId: staging!.id,
      ok: false,
      error: "invalid token",
    });

    await settings.goto();
    await expect(settings.statusBanner("Production")).toHaveAttribute("data-state", "ok");
    await expect(settings.statusBanner("Staging")).toHaveAttribute("data-state", "failed");
    await expect(settings.statusBanner("Staging")).toContainText(/invalid token/);
  });

  // Deleting an instance that issue watches still reference is refused server-
  // side (409 SENTRY_INSTANCE_IN_USE); the UI surfaces the watch count.
  test("blocks deleting an instance still referenced by an issue watch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await apiClient.mockSentryReset();
    await apiClient.mockSentrySetAuthResult({ ok: true, userId: "u-2", displayName: "Watcher" });

    const settings = new SentrySettingsPage(testPage);
    await settings.goto();
    await settings.addInstance({ name: "Production", secret: TOKEN });

    const prod = await apiClient.findSentryInstanceByName("Production");
    expect(prod).not.toBeNull();

    // Bind an issue watch to the instance so the delete must be refused.
    await apiClient.createSentryIssueWatch({
      workspaceId: seedData.workspaceId,
      instanceId: prod!.id,
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      agentProfileId: seedData.agentProfileId,
    });

    // Confirm the delete prompt, then assert the IN_USE message surfaces and the
    // instance is NOT removed.
    testPage.once("dialog", (d) => void d.accept());
    await settings.deleteButton("Production").click();
    await expect(
      testPage.getByText(/In use by 1 issue watch — reassign or delete those first/),
    ).toBeVisible();
    await expect(settings.card("Production")).toBeVisible();
  });
});
