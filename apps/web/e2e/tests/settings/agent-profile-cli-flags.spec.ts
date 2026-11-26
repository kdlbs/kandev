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

    // The Agent CLI flags card is always rendered on the profile editor.
    const field = testPage.getByTestId("custom-cli-flags-card");
    await expect(field).toBeVisible({ timeout: 15_000 });

    // Header and helper copy anchoring the section for the user.
    await expect(field).toContainText(/Agent CLI flags/i);
    await expect(field).toContainText(/Flags passed to the agent CLI/i);

    // The Add form inputs and button are always available.
    await expect(testPage.getByTestId("cli-flag-new-flag-input")).toBeVisible();
    await expect(testPage.getByTestId("cli-flag-add-button")).toBeVisible();
  });

  test("Antigravity CLI permission flag defaults off and persists both choices", async ({
    backend,
    testPage,
    apiClient,
    prCapture,
  }) => {
    test.setTimeout(90_000);
    const restoreBackend = await backend.useEnv({ KANDEV_E2E_MOCK_ANTIGRAVITY: "true" });
    let profileId: string | undefined;
    const flagSwitch = testPage.getByTestId(
      "cli-flag-curated-enabled-antigravity_skip_permissions",
    );

    try {
      const { agents } = await apiClient.listAgents();
      const agent = agents.find((candidate) => candidate.name === "antigravity-acp");
      if (!agent) throw new Error("Antigravity agent is not available in the E2E registry");
      const profile = await apiClient.createAgentProfile(agent.id, "Antigravity CLI flag test", {
        model: "",
        cli_flags: [],
      });
      profileId = profile.id;
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(flagSwitch).toBeVisible({ timeout: 15_000 });
      await expect(
        testPage.getByText("Skip Antigravity CLI tool prompts", { exact: true }),
      ).toBeVisible();
      await expect(
        testPage.getByText(
          "Antigravity automatically approves all tool requests when this flag applies to a supported CLI launch.",
          { exact: true },
        ),
      ).toBeVisible();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked");

      await flagSwitch.click();
      await expect(flagSwitch).toHaveAttribute("data-state", "checked");
      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          cliFlags: [
            expect.objectContaining({ flag: "--dangerously-skip-permissions", enabled: true }),
          ],
        });

      await testPage.reload();
      await expect(flagSwitch).toHaveAttribute("data-state", "checked", { timeout: 15_000 });
      await flagSwitch.evaluate((element) =>
        element.scrollIntoView({ block: "center", inline: "nearest" }),
      );
      await prCapture.screenshot("antigravity-cli-flag-desktop", {
        caption: "Antigravity CLI permission switch on the desktop profile page",
      });
      await flagSwitch.click();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked");
      await expect(saveButton).toHaveCount(1);
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await testPage.reload();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked", { timeout: 15_000 });
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId).catch(() => {});
      await restoreBackend();
    }
  });

  test("Antigravity switch turns off a saved enabled duplicate", async ({
    backend,
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    const restoreBackend = await backend.useEnv({ KANDEV_E2E_MOCK_ANTIGRAVITY: "true" });
    let profileId: string | undefined;

    try {
      const { agents } = await apiClient.listAgents();
      const agent = agents.find((candidate) => candidate.name === "antigravity-acp");
      if (!agent) throw new Error("Antigravity agent is not available in the E2E registry");
      const profile = await apiClient.createAgentProfile(agent.id, "Antigravity duplicate flag", {
        model: "",
        cli_flags: [
          { flag: "--dangerously-skip-permissions", enabled: false },
          { flag: "--profile-flag", enabled: true },
          { flag: "--dangerously-skip-permissions", enabled: true },
        ],
      });
      profileId = profile.id;
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const flagSwitch = testPage.getByTestId(
        "cli-flag-curated-enabled-antigravity_skip_permissions",
      );
      await expect(flagSwitch).toHaveAttribute("data-state", "checked", { timeout: 15_000 });
      await flagSwitch.click();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked");

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await testPage.reload();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked", { timeout: 15_000 });

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.cliFlags).toEqual([
        expect.objectContaining({ flag: "--profile-flag", enabled: true }),
        expect.objectContaining({ flag: "--dangerously-skip-permissions", enabled: false }),
      ]);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId).catch(() => {});
      await restoreBackend();
    }
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
      await expect(testPage.getByTestId("custom-cli-flags-card")).toBeVisible({ timeout: 15_000 });

      // Add a custom flag via the UI.
      const flagText = "--my-custom-flag=value";
      await testPage.getByTestId("cli-flag-new-flag-input").fill(flagText);
      await testPage.getByTestId("cli-flag-add-button").click();

      // The row should appear immediately, enabled by default. Flag text lives
      // in an <input> element; use toHaveValue on the last flag-input because
      // the newly added flag is always appended last.
      await expect(testPage.locator('[data-testid^="cli-flag-flag-"]').last()).toHaveValue(
        flagText,
      );

      // Save via the dirty-state save button; wait for unsaved badge to clear.
      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await expect(saveButton).toHaveCount(1);
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // Reload — the row must still be there.
      await testPage.reload();
      await expect(testPage.getByTestId("custom-cli-flags-card")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.locator('[data-testid^="cli-flag-flag-"]').last()).toHaveValue(
        flagText,
      );

      // Direct DB-path verification: fetch the profile via the API and assert
      // the normalized CLI flags contain our entry, enabled.
      const stored = await apiClient.getAgentProfile(profile.id);
      const found = stored.cliFlags.find((f) => f.flag === flagText);
      expect(found, `cliFlags should include ${flagText}`).toBeDefined();
      expect(found?.enabled).toBe(true);
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
      await expect(testPage.getByTestId("custom-cli-flags-card")).toBeVisible({ timeout: 15_000 });

      // The first row should be enabled; toggle it off.
      const toggle = testPage.getByTestId("cli-flag-enabled-0");
      await expect(toggle).toHaveAttribute("data-state", "checked");
      await toggle.click();
      await expect(toggle).toHaveAttribute("data-state", "unchecked");

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await expect(saveButton).toHaveCount(1);
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
      const found = stored.cliFlags.find((f) => f.flag === "--toggle-me");
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
      await expect(testPage.getByTestId("custom-cli-flags-card")).toBeVisible({ timeout: 15_000 });

      // Two rows present initially.
      await expect(testPage.getByTestId("cli-flag-row-0")).toBeVisible();
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeVisible();

      // Remove the second (--remove-me).
      await testPage.getByTestId("cli-flag-remove-1").click();
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeHidden();

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await expect(saveButton).toHaveCount(1);
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await testPage.reload();
      await expect(testPage.getByTestId("cli-flag-row-0")).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("cli-flag-row-1")).toBeHidden();

      const stored = await apiClient.getAgentProfile(profile.id);
      expect(stored.cliFlags.map((f) => f.flag)).toEqual(["--keep-me"]);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
