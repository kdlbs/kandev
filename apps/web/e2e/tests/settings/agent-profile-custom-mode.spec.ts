import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * Verifies repository-local session mode support in the profile editor:
 *
 * - The mode picker offers a custom entry when the typed filter matches no
 *   advertised mode, and the custom ID persists across reload.
 * - A profile that already stores a mode the agent never advertised (e.g. a
 *   repository-local OpenCode agent) renders it as a selectable custom row
 *   instead of breaking the editor.
 * - Model option resolution tolerates the unadvertised mode: the backend fix
 *   leaves it unapplied during host-side probing instead of failing, so the
 *   resolution error banner never appears and capabilities stay refreshable.
 *   The mode is still applied (and validated) when a real task session starts.
 *
 * Rendered-state assertions wait for the profile, capability, resolve, or save
 * response that causes them (arm before act), then use the default timeout.
 */

const CUSTOM_MODE = "reviewer";

test.describe("Agent profile — repository-local session modes", () => {
  test("mode picker offers and persists a custom mode ID", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is not registered");

    const profile = await apiClient.createAgentProfile(agent.id, "Custom Mode Entry Profile", {
      model: "mock-fast",
    });

    try {
      const capabilitiesLoaded = waitForHttp(
        testPage,
        "POST",
        /^\/api\/v1\/agent-models\/mock-agent\/probe$/,
      );
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await capabilitiesLoaded;
      await expect(testPage.getByTestId("profile-name-input")).toBeVisible();

      const trigger = testPage.getByTestId("profile-mode-select");
      await expect(trigger).toBeVisible();
      await trigger.click();

      await testPage.getByPlaceholder("Search modes...").fill(CUSTOM_MODE);
      const customEntry = testPage.getByTestId("profile-mode-custom-entry");
      await expect(customEntry).toBeVisible();
      await expect(customEntry).toHaveText(`Use "${CUSTOM_MODE}" as mode ID`);

      // Selecting the mode re-resolves model options with the new mode: arm
      // before the click, then assert the resolution outcome it causes.
      const resolvedWithCustomMode = waitForHttp(
        testPage,
        "POST",
        /\/agent-models\/mock-agent\/resolve$/,
        {
          predicate: (response) => {
            const request = response.request().postDataJSON() as { mode?: string };
            return response.status() === 200 && request.mode === CUSTOM_MODE;
          },
        },
      );
      await customEntry.click();
      await resolvedWithCustomMode;

      // The trigger surfaces the raw custom ID; no advertised display name.
      await expect(trigger).toHaveText(new RegExp(CUSTOM_MODE));

      // Resolution must tolerate the unadvertised mode instead of failing.
      await expect(testPage.getByTestId("model-config-resolution-error")).toHaveCount(0);

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await expect(saveButton).toBeEnabled();
      const profileSaved = waitForHttp(
        testPage,
        "PATCH",
        new RegExp(`/api/v1/agent-profiles/${profile.id}$`),
        { predicate: (response) => response.status() === 200 },
      );
      await saveButton.click();
      await profileSaved;
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden();
      expect((await apiClient.getAgentProfile(profile.id)).mode).toBe(CUSTOM_MODE);

      const capabilitiesReloaded = waitForHttp(
        testPage,
        "POST",
        /^\/api\/v1\/agent-models\/mock-agent\/probe$/,
      );
      await testPage.reload();
      await capabilitiesReloaded;
      await expect(testPage.getByTestId("profile-name-input")).toBeVisible();
      await expect(testPage.getByTestId("profile-mode-select")).toHaveText(new RegExp(CUSTOM_MODE));
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("stored unadvertised mode renders as a selectable custom row", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is not registered");

    // The mock agent advertises exactly "default" and "plan-mock", so the
    // stored mode below can never appear in the host-discovered list.
    const profile = await apiClient.createAgentProfile(agent.id, "Stored Custom Mode Profile", {
      model: "mock-fast",
      mode: CUSTOM_MODE,
    });

    try {
      // The editor auto-resolves model options for the saved profile on load;
      // arm before navigation so the load-time resolve is caught.
      const capabilitiesLoaded = waitForHttp(
        testPage,
        "POST",
        /^\/api\/v1\/agent-models\/mock-agent\/probe$/,
      );
      const resolvedOnLoad = waitForHttp(testPage, "POST", /\/agent-models\/mock-agent\/resolve$/, {
        predicate: (response) => {
          const request = response.request().postDataJSON() as { mode?: string };
          return response.status() === 200 && request.mode === CUSTOM_MODE;
        },
      });
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await Promise.all([capabilitiesLoaded, resolvedOnLoad]);
      await expect(testPage.getByTestId("profile-name-input")).toBeVisible();

      // Resolution succeeds despite the unadvertised mode (backend tolerance):
      // no error banner, capabilities remain refreshable.
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      await expect(testPage.getByTestId("model-config-resolution-error")).toHaveCount(0);
      await expect(testPage.getByTestId("profile-refresh-capabilities")).toBeEnabled();

      const trigger = testPage.getByTestId("profile-mode-select");
      await expect(trigger).toHaveText(new RegExp(CUSTOM_MODE));

      await trigger.click();
      const customRow = testPage.getByTestId("profile-mode-custom-row");
      await expect(customRow).toBeVisible();
      await expect(customRow).toHaveAttribute("data-checked", "true");
      await customRow.click();
      await expect(trigger).toHaveText(new RegExp(CUSTOM_MODE));
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
