import { test, expect } from "../../fixtures/test-base";

/**
 * Verifies the unified CLI-flags feature on agent profiles:
 *
 * - The profile editor renders a "Agent CLI flags" section.
 * - Users can add a custom flag via the form, save, and reload; the flag
 *   persists in the DB (assert via the PATCH round-trip).
 * - Toggling a flag off persists across save/reload.
 * - Removing a flag persists across save/reload.
 *
 * Full argv-assembly (tokenisation + launch path) is covered by
 * backend unit tests in apps/backend/internal/agent/lifecycle — we don't
 * exercise a full task launch here because that's both slow and redundant.
 */
test.describe("Agent profile — CLI flags", () => {
  test("cli-flags section renders on profile editor", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profileId = agent.profiles[0].id;

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profileId}`);

    // The CLIFlagsField component is always rendered once the profile form
    // loads — regardless of whether the agent has curated suggestions.
    const field = testPage.getByTestId("cli-flags-field");
    await expect(field).toBeVisible({ timeout: 15_000 });

    // Header and helper copy anchoring the section for the user.
    await expect(field).toContainText(/Agent CLI flags/i);
    await expect(field).toContainText(/Flags passed to the agent CLI/i);

    // The Add form inputs and button are always available.
    await expect(testPage.getByTestId("cli-flag-new-flag-input")).toBeVisible();
    await expect(testPage.getByTestId("cli-flag-add-button")).toBeVisible();
  });

  test("adding a custom flag persists across reload", async ({ testPage, apiClient }) => {
    test.setTimeout(90_000);

    // Create a fresh profile so we don't pollute other tests' fixtures.
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "CLI flags test", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(testPage.getByTestId("cli-flags-field")).toBeVisible({ timeout: 15_000 });

      // Add a custom flag via the UI.
      const flagText = "--my-custom-flag=value";
      await testPage.getByTestId("cli-flag-new-flag-input").fill(flagText);
      await testPage.getByTestId("cli-flag-new-desc-input").fill("My custom flag for testing");
      await testPage.getByTestId("cli-flag-add-button").click();

      // The row should appear immediately, enabled by default. The row's
      // positional index depends on how many curated flags the agent seeded
      // at profile creation, so locate the new row by its flag text rather
      // than by position.
      await expect(testPage.getByTestId("cli-flags-list")).toContainText(flagText);

      // Save via the dirty-state save button; wait for unsaved badge to clear.
      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // Reload — the row must still be there.
      await testPage.reload();
      await expect(testPage.getByTestId("cli-flags-field")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("cli-flags-list")).toContainText(flagText);

      // Direct DB-path verification: fetch the profile via the API and assert
      // the cli_flags JSON contains our entry, enabled.
      const stored = await apiClient.getAgentProfile(profile.id);
      const found = stored.cli_flags.find((f) => f.flag === flagText);
      expect(found, `cli_flags should include ${flagText}`).toBeDefined();
      expect(found?.enabled).toBe(true);
      expect(found?.description).toBe("My custom flag for testing");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("toggling a flag off persists across reload", async ({ testPage, apiClient }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    // Seed a profile with one enabled flag via the API so the UI has
    // something to toggle off.
    const profile = await apiClient.createAgentProfile(agent.id, "CLI flags toggle test", {
      model: "mock-fast",
      cli_flags: [{ description: "toggle target", flag: "--toggle-me", enabled: true }],
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(testPage.getByTestId("cli-flags-field")).toBeVisible({ timeout: 15_000 });

      // The first row should be enabled; toggle it off.
      const toggle = testPage.getByTestId("cli-flag-enabled-0");
      await expect(toggle).toHaveAttribute("data-state", "checked");
      await toggle.click();
      await expect(toggle).toHaveAttribute("data-state", "unchecked");

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // Reload and confirm the toggle remained off.
      await testPage.reload();
      await expect(testPage.getByTestId("cli-flag-enabled-0")).toHaveAttribute(
        "data-state",
        "unchecked",
        { timeout: 15_000 },
      );

      const stored = await apiClient.getAgentProfile(profile.id);
      const found = stored.cli_flags.find((f) => f.flag === "--toggle-me");
      expect(found?.enabled).toBe(false);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("removing a flag persists across reload", async ({ testPage, apiClient }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];

    const profile = await apiClient.createAgentProfile(agent.id, "CLI flags remove test", {
      model: "mock-fast",
      cli_flags: [
        { description: "keep", flag: "--keep-me", enabled: true },
        { description: "remove", flag: "--remove-me", enabled: true },
      ],
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(testPage.getByTestId("cli-flags-field")).toBeVisible({ timeout: 15_000 });

      // Two rows present initially.
      await expect(testPage.getByTestId("cli-flag-row-0")).toBeVisible();
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeVisible();

      // Remove the second (--remove-me).
      await testPage.getByTestId("cli-flag-remove-1").click();
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeHidden();

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await testPage.reload();
      await expect(testPage.getByTestId("cli-flag-row-0")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeHidden();

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.cli_flags.map((f) => f.flag)).toEqual(["--keep-me"]);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
