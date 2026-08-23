import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { SentrySettingsPage } from "../../pages/sentry-settings-page";

const TOKEN = "sntrys_xxx";

test.describe("Sentry settings — instances", () => {
  test("keeps an existing instance edit local until the floating Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockSentryReset();
    await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Original Sentry",
      secret: TOKEN,
    });

    const settings = new SentrySettingsPage(testPage);
    await settings.goto(seedData.workspaceId);
    const card = settings.cardByName("Original Sentry");
    await expect(card).toBeVisible({ timeout: 15_000 });
    await card.getByTestId("sentry-instance-edit-button").click();
    await testPage.getByTestId("sentry-edit-name-input").fill("Renamed Sentry");

    expect((await apiClient.listSentryInstances(seedData.workspaceId))[0]?.name).toBe(
      "Original Sentry",
    );
    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });
    expect((await apiClient.listSentryInstances(seedData.workspaceId))[0]?.name).toBe(
      "Renamed Sentry",
    );
    await expect(settings.cardByName("Renamed Sentry")).toBeVisible();
  });

  // The settings page manages a LIST of named instances per workspace. This
  // covers add → the card appears → the backend probe flips it healthy.
  test("adds a named instance and reports it healthy", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.mockSentryReset();

    const settings = new SentrySettingsPage(testPage);
    await settings.goto(seedData.workspaceId);

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
    await settings.goto(seedData.workspaceId);
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
      projectSlugs: ["web"],
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
    await settings.goto(seedData.workspaceId);
    await expect(settings.cards).toHaveCount(2);

    await settings.cardByName("Primary").getByTestId("sentry-instance-delete-button").click();
    await testPage.getByRole("button", { name: 'Remove Sentry instance "Primary"?' }).click();
    await expect(testPage.getByText(/still bound to it/i)).toBeVisible();
    await expect(settings.cardByName("Primary")).toBeVisible();

    await settings.cardByName("Secondary").getByTestId("sentry-instance-delete-button").click();
    await testPage.getByRole("button", { name: 'Remove Sentry instance "Secondary"?' }).click();
    await expect(settings.cardByName("Secondary")).toHaveCount(0);
    await expect(settings.cardByName("Primary")).toBeVisible();
  });
});

// Scopes a Radix Select (or the ProjectMultiSelect trigger, which shares the
// same role="combobox" contract) by its field label. The dialog renders each
// field as `<div class="space-y-1.5"><Label>…</Label><Select>…</Select></div>`,
// so the exact label text uniquely identifies the combobox via its parent.
function comboboxByLabel(root: Locator, label: string) {
  return root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
}

test.describe("Sentry settings — issue watchers", () => {
  test("creates a watcher scoped to multiple projects and persists all of them", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.mockSentryReset();
    const instance = await apiClient.createSentryInstance({
      workspaceId: seedData.workspaceId,
      name: "Multi-project Sentry",
      secret: TOKEN,
    });
    await apiClient.mockSentrySetAuthHealth({ instanceId: instance.id, ok: true });
    await apiClient.mockSentrySetOrganizations(instance.id, [
      { id: "acme", slug: "acme", name: "Acme" },
    ]);
    await apiClient.mockSentrySetProjects(instance.id, [
      { id: "frontend", slug: "frontend", name: "Frontend", orgSlug: "acme" },
      { id: "backend", slug: "backend", name: "Backend", orgSlug: "acme" },
    ]);

    const settings = new SentrySettingsPage(testPage);
    await settings.goto(seedData.workspaceId);

    await testPage.getByRole("button", { name: "New watcher" }).click();
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    const pick = async (label: string, option: string | RegExp) => {
      await comboboxByLabel(dialog, label).click();
      await testPage.getByRole("listbox").getByRole("option", { name: option }).click();
    };

    await pick("Sentry instance", "Multi-project Sentry");
    // The sole org auto-selects once the lookup resolves.
    await expect(comboboxByLabel(dialog, "Organization slug")).toContainText("acme");

    await comboboxByLabel(dialog, "Project slug").click();
    const projectListbox = testPage.getByRole("listbox");
    await projectListbox.getByRole("option", { name: "Frontend (frontend)" }).click();
    await projectListbox.getByRole("option", { name: "Backend (backend)" }).click();
    await expect(comboboxByLabel(dialog, "Project slug")).toContainText("2 projects selected");
    await prCapture.screenshot("watcher-project-multiselect-open", {
      caption: "Selecting multiple Sentry projects for one issue watcher",
    });
    await testPage.keyboard.press("Escape");

    await pick("Workflow", "E2E Workflow");
    await comboboxByLabel(dialog, "Workflow Step").click();
    await testPage.getByRole("listbox").getByRole("option").first().click();

    const createButton = dialog.getByRole("button", { name: "Create" });
    await expect(createButton).toBeEnabled();
    await createButton.click();
    await expect(dialog).toBeHidden();

    // The table's filter summary lists every selected project, proving the
    // watch persisted with both — not just the last one picked.
    await expect(testPage.getByText("project:frontend,backend")).toBeVisible();
    await prCapture.screenshot("watcher-table-multi-project-persisted", {
      caption: "Saved watcher polling both selected projects",
    });
  });
});
