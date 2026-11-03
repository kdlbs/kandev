import { test, expect } from "../../fixtures/test-base";
import {
  COPILOT_RECOVERY_CONFIG_OPTIONS,
  COPILOT_RECOVERY_MODEL_ID,
  COPILOT_RECOVERY_PLUGIN_ID,
  copilotPluginDiscovery,
  copilotProfileSaveActions,
  createCopilotPluginFixture,
  installCopilotPluginProbeRoutes,
  requireCopilotPluginFixture,
} from "../../helpers/copilot-plugin-fixture";

test("selects, saves, reloads, resets, and clears Copilot profile plugins", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;

  try {
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(copilotPluginDiscovery(fixture.agentId, fixture.profileId)),
      });
    });
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });

    const spark = selector.getByRole("checkbox", { name: "Select spark" });
    const builtin = selector.getByRole("checkbox", { name: "Select browser-use" });
    const sparkRow = selector.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
    await expect(spark).toHaveCount(1);
    await expect(builtin).toHaveCount(1);
    await expect(sparkRow).toHaveCount(1);
    await expect(sparkRow).toContainText("Native status: disabled");
    await expect(spark).not.toBeChecked();
    await expect(builtin).toHaveAttribute("disabled", "");
    const refresh = selector.getByTestId("copilot-plugins-refresh");
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeCloseTo(28, 1);

    await spark.click();
    await expect(spark).toBeChecked();
    await expect(selector).toHaveAttribute("data-settings-dirty", "true");
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);

    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.surface).toHaveCount(1);
    await expect(actions.save).toHaveCount(1);
    const saveBox = await actions.save.boundingBox();
    expect(saveBox).not.toBeNull();
    expect(saveBox!.height).toBeCloseTo(28, 1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([COPILOT_RECOVERY_PLUGIN_ID]);

    await testPage.reload();
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(spark).toBeChecked({ timeout: 15_000 });
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);

    await spark.click();
    await expect(actions.reset).toHaveCount(1);
    await expect(actions.reset).toBeEnabled();
    const resetBox = await actions.reset.boundingBox();
    expect(resetBox).not.toBeNull();
    expect(resetBox!.height).toBeCloseTo(28, 1);
    await actions.reset.click();
    await expect(spark).toBeChecked();

    await spark.click();
    await expect(actions.save).toHaveCount(1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
  } finally {
    await fixture.dispose();
  }
});

test("sends the saved draft context and an explicit cache-busting refresh", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;
  const requests: Array<Record<string, unknown>> = [];

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
      requests.push(JSON.parse(route.request().postData() ?? "{}") as Record<string, unknown>);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(copilotPluginDiscovery(fixture.agentId, fixture.profileId)),
      });
    });
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(selector.getByRole("checkbox", { name: "Select spark" })).toBeChecked();
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
    await expect.poll(() => requests.length).toBeGreaterThan(0);
    const initial = requests[0];
    expect(initial.profile_id).toBe(fixture.profileId);
    expect(initial.launch_settings).toBeDefined();
    expect(initial.refresh).not.toBe(true);

    const refreshButton = selector.getByTestId("copilot-plugins-refresh");
    await expect(refreshButton).toHaveCount(1);
    await refreshButton.click();
    await expect.poll(() => requests.length).toBeGreaterThan(1);
    const refresh = requests[requests.length - 1];
    expect(refresh.profile_id).toBe(fixture.profileId);
    expect(refresh.refresh).toBe(true);
    expect(refresh.launch_settings).toBeDefined();
  } finally {
    await fixture.dispose();
  }
});

test("keeps saved choices removable when plugin refresh fails", async ({ testPage, apiClient }) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;
  const agentId = fixture.agentId;
  const profileId = fixture.profileId;
  let failRefresh = false;

  try {
    await apiClient.updateAgentProfile(profileId, {
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    await testPage.route(`**/api/v1/agents/${agentId}/plugin-discovery`, async (route) => {
      if (failRefresh) {
        await route.fulfill({ status: 500, body: "offline" });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(copilotPluginDiscovery(agentId, profileId)),
      });
    });
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    const spark = selector.getByRole("checkbox", { name: "Select spark" });
    await expect(spark).toBeChecked({ timeout: 15_000 });
    failRefresh = true;
    const refresh = selector.getByTestId("copilot-plugins-refresh");
    await expect(refresh).toHaveCount(1);
    await refresh.click();
    await expect(testPage.getByTestId("copilot-plugins-failed")).toBeVisible();
    await expect(testPage.getByText(/Saved selection, availability not checked/)).toBeVisible();
    await expect(spark).not.toHaveAttribute("disabled", "");
    await spark.click();
    await expect(spark).not.toBeChecked();
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.save).toHaveCount(1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(profileId)).copilotSelectedPlugins)
      .toEqual([]);
  } finally {
    await fixture.dispose();
  }
});

test("reports a missing saved plugin after a successful inventory", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      copilot_selected_plugins: ["installed:missing/plugin"],
    });
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          copilotPluginDiscovery(fixture.agentId, fixture.profileId, { plugins: [] }),
        ),
      });
    });
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(selector.getByText(/Plugin is missing/)).toBeVisible({ timeout: 15_000 });
    await expect(
      selector.getByText(/A new process cannot start with this selection/),
    ).toBeVisible();
    const missing = selector.getByRole("checkbox", { name: "Select plugin" });
    await expect(missing).toHaveCount(1);
    await expect(missing).toBeChecked();
    await expect(missing).not.toHaveAttribute("disabled", "");
    await missing.click();
    await expect(missing).toHaveCount(0);
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.save).toHaveCount(1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.8
// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2
test("probes profile capabilities and model options with a supported plugin selection", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await requireCopilotPluginFixture(apiClient);

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      model: COPILOT_RECOVERY_MODEL_ID,
      config_options: { ...COPILOT_RECOVERY_CONFIG_OPTIONS },
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    const probes = await installCopilotPluginProbeRoutes(testPage, fixture, "supported");

    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const pluginSection = testPage.getByTestId("copilot-plugins-selection");
    const spark = pluginSection.getByRole("checkbox", { name: "Select spark" });
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    const capabilityStatus = testPage.getByTestId("profile-capability-status");
    const modelOptionsStatus = testPage.getByTestId("model-config-resolution-error");

    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).toHaveCount(1);
    await expect(spark).toBeChecked();
    await expect(pluginSection.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
    await expect.poll(() => probes.capabilityRequests.length).toBeGreaterThan(0);
    await expect.poll(() => probes.modelOptionsRequests.length).toBeGreaterThan(0);
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready");
    await expect(modelOptionsStatus).toHaveCount(0);
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).toContainText("High");
    expect(
      probes.capabilityRequests.some((request) => request.profile_id === fixture.profileId),
    ).toBe(true);
    expect(
      probes.modelOptionsRequests.some((request) => request.profile_id === fixture.profileId),
    ).toBe(true);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    await refresh.click();
    await expect
      .poll(() =>
        probes.capabilityRequests.some(
          (request) =>
            request.refresh === true &&
            request.launch_settings?.copilot_selected_plugins?.includes(COPILOT_RECOVERY_PLUGIN_ID),
        ),
      )
      .toBe(true);
    await expect.poll(() => probes.modelOptionsRequests.length).toBeGreaterThan(1);
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready");
    await expect(modelOptionsStatus).toHaveCount(0);
    await expect(pluginSection.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.8
test("recovers a policy-blocked profile capability probe without changing saved model options", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await requireCopilotPluginFixture(apiClient);

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      model: COPILOT_RECOVERY_MODEL_ID,
      config_options: { ...COPILOT_RECOVERY_CONFIG_OPTIONS },
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    const recovery = await installCopilotPluginProbeRoutes(testPage, fixture, "capability");

    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const pluginSection = testPage.getByTestId("copilot-plugins-selection");
    const spark = pluginSection.getByRole("checkbox", { name: "Select spark" });
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    const capabilityStatus = testPage.getByTestId("profile-capability-status");

    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).toBeChecked();
    await expect(capabilityStatus).toHaveAttribute("data-status", "failed");
    await expect(capabilityStatus).toContainText(
      "Model discovery is blocked by this plugin selection.",
    );
    await expect(capabilityStatus).not.toContainText("Probe failed. Check agent logs for details.");
    await expect(testPage.getByText("Synthetic policy preflight diagnostic.")).toHaveCount(0);
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).not.toHaveClass(/text-destructive/);
    await expect(pluginSection.getByTestId("copilot-plugins-start-blocked")).toContainText(
      "Uncheck the selected plugins and Save the profile to start without them.",
    );

    await spark.click();
    await expect(spark).not.toBeChecked();
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.save).toHaveCount(1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
    const savedAfterClear = await apiClient.getAgentProfile(fixture.profileId);
    expect(savedAfterClear.model).toBe(COPILOT_RECOVERY_MODEL_ID);
    expect(savedAfterClear.configOptions).toEqual(COPILOT_RECOVERY_CONFIG_OPTIONS);

    recovery.allowRecovery();
    await testPage.reload();
    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).not.toBeChecked();
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready", { timeout: 20_000 });
    await expect(capabilityStatus).not.toContainText(
      "Model discovery is blocked by this plugin selection.",
    );
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).not.toHaveClass(/text-destructive/);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    await refresh.click();
    await expect
      .poll(() =>
        recovery.capabilityRequests.some(
          (request) =>
            request.refresh === true &&
            request.launch_settings?.copilot_selected_plugins?.length === 0,
        ),
      )
      .toBe(true);
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready");
    const refreshedProfile = await apiClient.getAgentProfile(fixture.profileId);
    expect(refreshedProfile.copilotSelectedPlugins).toEqual([]);
    expect(refreshedProfile.model).toBe(COPILOT_RECOVERY_MODEL_ID);
    expect(refreshedProfile.configOptions).toEqual(COPILOT_RECOVERY_CONFIG_OPTIONS);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.8
test("shows policy recovery for model-option failure after capability discovery succeeds", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await requireCopilotPluginFixture(apiClient);

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      model: COPILOT_RECOVERY_MODEL_ID,
      config_options: { ...COPILOT_RECOVERY_CONFIG_OPTIONS },
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    const recovery = await installCopilotPluginProbeRoutes(testPage, fixture, "model-options");

    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const pluginSection = testPage.getByTestId("copilot-plugins-selection");
    const spark = pluginSection.getByRole("checkbox", { name: "Select spark" });
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    const capabilityStatus = testPage.getByTestId("profile-capability-status");
    const modelOptionsStatus = testPage.getByTestId("model-config-resolution-error");

    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).toBeChecked();
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready", { timeout: 20_000 });
    await expect(modelOptionsStatus).toBeVisible({ timeout: 20_000 });
    await expect(modelOptionsStatus).toContainText(
      "Model options are blocked by this plugin selection.",
    );
    await expect(modelOptionsStatus).not.toContainText("Model options could not be loaded.");
    await expect(modelOptionsStatus).not.toContainText("Synthetic policy preflight diagnostic.");
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).not.toHaveClass(/text-destructive/);
    await expect.poll(() => recovery.modelOptionsRequests.length).toBeGreaterThan(0);

    await spark.click();
    await expect(spark).not.toBeChecked();
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.save).toHaveCount(1);
    await actions.save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
    const savedAfterClear = await apiClient.getAgentProfile(fixture.profileId);
    expect(savedAfterClear.model).toBe(COPILOT_RECOVERY_MODEL_ID);
    expect(savedAfterClear.configOptions).toEqual(COPILOT_RECOVERY_CONFIG_OPTIONS);

    recovery.allowRecovery();
    await testPage.reload();
    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).not.toBeChecked();
    await expect(capabilityStatus).toHaveAttribute("data-status", "ready", { timeout: 20_000 });
    await expect(modelOptionsStatus).toHaveCount(0, { timeout: 20_000 });
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).toContainText("High");
    await expect(modelPicker).not.toHaveClass(/text-destructive/);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    await refresh.click();
    await expect
      .poll(() =>
        recovery.capabilityRequests.some(
          (request) =>
            request.refresh === true &&
            request.launch_settings?.copilot_selected_plugins?.length === 0,
        ),
      )
      .toBe(true);
    await expect.poll(() => recovery.modelOptionsRequests.length).toBeGreaterThan(1);
    const refreshedProfile = await apiClient.getAgentProfile(fixture.profileId);
    expect(refreshedProfile.copilotSelectedPlugins).toEqual([]);
    expect(refreshedProfile.model).toBe(COPILOT_RECOVERY_MODEL_ID);
    expect(refreshedProfile.configOptions).toEqual(COPILOT_RECOVERY_CONFIG_OPTIONS);
  } finally {
    await fixture.dispose();
  }
});
