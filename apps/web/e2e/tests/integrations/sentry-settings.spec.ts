import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";

// Opens a fresh task and waits for its layout so the top bar (which hosts the
// SentryLinkButton) is mounted. Returns the task id.
async function openTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<string> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  await expect(testPage.getByTestId("dockview-task-layout")).toBeVisible({ timeout: 15_000 });
  return task.id;
}

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

test.describe("Sentry link button — instance selection", () => {
  const linkTrigger = (testPage: Page) => testPage.getByRole("button", { name: /link sentry/i });
  const submitButton = (testPage: Page) =>
    testPage.getByRole("button", { name: "Link", exact: true });

  // No healthy instance → nothing to link against, so the button stays hidden.
  test("hides the button when the workspace has no healthy instance", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockSentryReset();
    await openTask(testPage, apiClient, seedData, "Sentry link — none");
    await expect(linkTrigger(testPage)).toHaveCount(0);
  });

  // Exactly one healthy instance → auto-selected, so the popover shows no
  // instance picker and submit enables as soon as a key is entered.
  test("auto-selects the sole healthy instance", async ({ testPage, apiClient, seedData }) => {
    await apiClient.mockSentryReset();
    const solo = await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Solo",
      secret: TOKEN,
    });
    await apiClient.mockSentrySetAuthHealth({ instanceId: solo.id, ok: true });

    await openTask(testPage, apiClient, seedData, "Sentry link — one");
    await expect(linkTrigger(testPage)).toBeVisible({ timeout: 10_000 });
    await linkTrigger(testPage).click();

    await expect(testPage.getByTestId("sentry-link-instance-select")).toHaveCount(0);
    await testPage.getByPlaceholder(/PROJ-123/).fill("PROJ-1");
    await expect(submitButton(testPage)).toBeEnabled();
  });

  // Several healthy instances → the popover prompts with an instance picker and
  // keeps submit disabled until one is chosen.
  test("prompts for an instance when several are healthy", async ({
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
      secret: TOKEN,
    });
    await apiClient.mockSentrySetAuthHealth({ instanceId: primary.id, ok: true });
    await apiClient.mockSentrySetAuthHealth({ instanceId: secondary.id, ok: true });

    await openTask(testPage, apiClient, seedData, "Sentry link — many");
    await expect(linkTrigger(testPage)).toBeVisible({ timeout: 10_000 });
    await linkTrigger(testPage).click();

    const select = testPage.getByTestId("sentry-link-instance-select");
    await expect(select).toBeVisible();
    await testPage.getByPlaceholder(/PROJ-123/).fill("PROJ-2");
    // No instance chosen yet → submit stays disabled.
    await expect(submitButton(testPage)).toBeDisabled();

    await select.click();
    await testPage.getByRole("option", { name: "Primary" }).click();
    await expect(submitButton(testPage)).toBeEnabled();
  });
});
