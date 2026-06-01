import { test, expect } from "../../fixtures/test-base";
import { LinearSettingsPage } from "../../pages/linear-settings-page";
import { assertWatcherAgentProfileResetsToStepDefault } from "./watcher-profile-default-flow";

test.describe("Linear settings", () => {
  test("empty workspace shows form with disabled save/test until secret is filled", async ({
    testPage,
  }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
    await expect(settings.saveButton).toBeDisabled();
    await expect(settings.testButton).toBeDisabled();

    await settings.secretInput.fill("lin_api_xxx");
    await expect(settings.saveButton).toBeEnabled();
    await expect(settings.testButton).toBeEnabled();
  });

  test("saving the config persists across reload and shows the auth banner", async ({
    testPage,
    apiClient,
  }) => {
    // Seed a single team so the dropdown post-save can populate without an
    // empty-list flash. The default-team field is optional, but the team
    // fetch fires when hasSecret flips true and we want it to succeed.
    await apiClient.mockLinearSetTeams([{ id: "team-1", key: "ENG", name: "Engineering" }]);

    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    await expect(settings.saveButton).toHaveText(/Update/i);
    // Wait for the async post-save probe to write lastOk=true before reloading.
    await apiClient.waitForIntegrationAuthHealthy("linear");

    await testPage.reload();
    await settings.secretInput.waitFor();
    await expect(settings.statusBanner).toHaveAttribute("data-state", "ok");
    // Saved-secret hint indicates the row was loaded, not started fresh.
    await expect(testPage.getByText(/leave blank to keep the current value/i)).toBeVisible();
  });

  test("test connection surfaces inline success and failure", async ({ testPage, apiClient }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();

    await apiClient.mockLinearSetAuthResult({
      ok: true,
      displayName: "Alice from Linear",
      email: "alice@example.com",
      orgName: "Acme",
    });
    await settings.secretInput.fill("lin_api_xxx");
    await settings.testButton.click();
    await expect(testPage.getByText(/Connected as Alice from Linear/i)).toBeVisible();

    await apiClient.mockLinearSetAuthResult({ ok: false, error: "Bad token" });
    await settings.testButton.click();
    await expect(testPage.getByText(/Failed: Bad token/)).toBeVisible();
  });

  test("seeded auth-health failure renders the failed banner on load", async ({
    testPage,
    apiClient,
  }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();
    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    // Wait for the post-save probe to land BEFORE forcing the failure: the
    // probe goroutine could otherwise overwrite our forced lastOk=false back
    // to true a few ms after the mockLinearSetAuthHealth call.
    await apiClient.waitForIntegrationAuthHealthy("linear");

    await apiClient.mockLinearSetAuthHealth({
      ok: false,
      error: "rate limited",
    });
    await testPage.reload();
    await settings.statusBanner.waitFor();
    await expect(settings.statusBanner).toHaveAttribute("data-state", "failed");
    await expect(settings.statusBanner).toContainText(/rate limited/i);
  });

  test("delete clears the saved configuration", async ({ testPage }) => {
    const settings = new LinearSettingsPage(testPage);
    await settings.goto();
    await settings.secretInput.fill("lin_api_xxx");
    await settings.saveButton.click();
    await expect(settings.deleteButton).toBeVisible();

    testPage.once("dialog", (d) => void d.accept());
    await settings.deleteButton.click();
    await expect(settings.deleteButton).toHaveCount(0);
    await expect(settings.secretInput).toHaveValue("");
    await expect(settings.statusBanner).toHaveCount(0);
  });

  // Regression test for #1107: passthrough profiles were silently filtered
  // out of the watcher dialog after #805 — once #923 made auto-start viable,
  // this filter became stale and hid Claude Code / Codex / Copilot CLI from
  // the Agent Profile selector. The dropdown must now list them.
  test("watcher dialog lists CLI-passthrough agent profiles", async ({ testPage, apiClient }) => {
    await apiClient.setLinearConfig({ secret: "lin_api_xxx" });
    await apiClient.waitForIntegrationAuthHealthy("linear");

    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
    const passthroughName = "Watcher CLI Passthrough";
    await apiClient.createAgentProfile(agents[0].id, passthroughName, {
      model: "mock-fast",
      cli_passthrough: true,
    });

    await testPage.goto("/settings/integrations/linear");
    await testPage.getByRole("button", { name: /new watcher/i }).click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();

    // Reach the Agent Profile combobox via its <label> parent so we don't
    // collide with the other comboboxes (workspace / workflow / executor)
    // rendered in the same dialog.
    const trigger = dialog
      .getByText("Agent Profile", { exact: true })
      .locator("xpath=..")
      .getByRole("combobox");
    await trigger.click();

    // Radix portals the listbox to the document root, so search the page (not
    // the dialog) for the option. Substring match on the profile name handles
    // the "<agent> • <profile>" label format.
    await expect(testPage.getByRole("option", { name: new RegExp(passthroughName) })).toBeVisible();
  });

  test("watcher dialog resets the agent profile back to the step default", async ({ testPage }) => {
    await assertWatcherAgentProfileResetsToStepDefault(testPage);
  });
});
