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

const longName =
  "installed marketplace plugin with a long translated name that should wrap on a phone";

test("phone users can select, save, reset, and clear Copilot profile plugins", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;
  const discovery = copilotPluginDiscovery(fixture.agentId, fixture.profileId);
  discovery.plugins[0].name = longName;
  const discoveryRequests: Array<Record<string, unknown>> = [];

  try {
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
      discoveryRequests.push(route.request().postDataJSON() as Record<string, unknown>);
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(discovery),
      });
    });
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });
    const checkbox = selector.getByRole("checkbox", { name: `Select ${longName}` });
    const row = selector.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
    await expect(checkbox).toHaveCount(1);
    await expect(row).toHaveCount(1);
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
    await checkbox.tap();
    await expect(checkbox).toBeChecked();
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
    await testPage.setViewportSize({ width: 767, height: 800 });
    await expect(checkbox).toBeChecked();
    await testPage.setViewportSize({ width: 390, height: 844 });
    await expect(checkbox).toBeChecked();

    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.surface).toHaveCount(1);
    await expect(actions.save).toHaveCount(1);
    const saveBox = await actions.save.boundingBox();
    expect(saveBox).not.toBeNull();
    expect(saveBox!.height).toBeGreaterThanOrEqual(44);
    await actions.save.tap();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([COPILOT_RECOVERY_PLUGIN_ID]);

    await testPage.reload();
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(checkbox).toBeChecked({ timeout: 15_000 });
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);

    const refresh = selector.getByTestId("copilot-plugins-refresh");
    await expect(refresh).toHaveCount(1);
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
    const requestCountBeforeRefresh = discoveryRequests.length;
    await refresh.tap();
    await expect
      .poll(() =>
        discoveryRequests
          .slice(requestCountBeforeRefresh)
          .some((request) => request.refresh === true && request.profile_id === fixture.profileId),
      )
      .toBe(true);

    await checkbox.tap();
    await expect(actions.reset).toHaveCount(1);
    const resetBox = await actions.reset.boundingBox();
    expect(resetBox).not.toBeNull();
    expect(resetBox!.height).toBeGreaterThanOrEqual(44);
    await expect(actions.reset).toBeEnabled();
    await actions.reset.tap();
    await expect(checkbox).toBeChecked();

    await checkbox.tap();
    await expect(actions.save).toHaveCount(1);
    await actions.save.tap();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2, AC-AGENTS-COPILOT-PLUGINS-006.5, AC-AGENTS-COPILOT-PLUGINS-006.6
test("phone users can probe model capabilities and options with a supported plugin selection", async ({
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
    const pluginRow = pluginSection.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
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
    const rowBox = await pluginRow.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);
    expect(
      probes.capabilityRequests.some((request) => request.profile_id === fixture.profileId),
    ).toBe(true);
    expect(
      probes.modelOptionsRequests.some((request) => request.profile_id === fixture.profileId),
    ).toBe(true);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
    await refresh.tap();
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
    const savedProfile = await apiClient.getAgentProfile(fixture.profileId);
    expect(savedProfile.copilotSelectedPlugins).toEqual([COPILOT_RECOVERY_PLUGIN_ID]);
    expect(savedProfile.model).toBe(COPILOT_RECOVERY_MODEL_ID);
    expect(savedProfile.configOptions).toEqual(COPILOT_RECOVERY_CONFIG_OPTIONS);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-005.8, AC-AGENTS-COPILOT-PLUGINS-006.2, AC-AGENTS-COPILOT-PLUGINS-006.3, AC-AGENTS-COPILOT-PLUGINS-006.5
test("phone users can remove a saved selection when the runtime is unsupported", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await requireCopilotPluginFixture(apiClient);

  try {
    await apiClient.updateAgentProfile(fixture.profileId, {
      copilot_selected_plugins: [COPILOT_RECOVERY_PLUGIN_ID],
    });
    await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          copilotPluginDiscovery(fixture.agentId, fixture.profileId, {
            status: "unsupported",
            runtime_version: "unsupported-fixture",
            mount_permitted: false,
            plugins: [],
          }),
        ),
      });
    });

    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    const spark = selector.getByRole("checkbox", { name: "Select spark" });
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("copilot-plugins-unsupported")).toBeVisible();
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toBeVisible();
    await expect(spark).toHaveCount(1);
    await expect(spark).toBeChecked();
    await expect(spark).not.toHaveAttribute("disabled", "");
    const row = selector.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
    await expect(row).toHaveCount(1);
    const rowBox = await row.boundingBox();
    expect(rowBox).not.toBeNull();
    expect(rowBox!.height).toBeGreaterThanOrEqual(44);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await spark.tap();
    await expect(spark).toHaveCount(0);
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.surface).toHaveCount(1);
    await expect(actions.save).toHaveCount(1);
    const saveBox = await actions.save.boundingBox();
    expect(saveBox).not.toBeNull();
    expect(saveBox!.height).toBeGreaterThanOrEqual(44);
    await actions.save.tap();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual([]);

    await testPage.reload();
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("copilot-plugins-unsupported")).toBeVisible();
    await expect(selector.getByRole("checkbox", { name: "Select plugin" })).toHaveCount(0);
    await expect(selector.getByTestId("copilot-plugins-start-blocked")).toHaveCount(0);
  } finally {
    await fixture.dispose();
  }
});

// @covers AC-AGENTS-COPILOT-PLUGINS-006.8
test("phone users can recover a policy-blocked profile capability probe", async ({
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
    const pluginRow = testPage.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    const capabilityStatus = testPage.getByTestId("profile-capability-status");

    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).toHaveCount(1);
    await expect(spark).toBeChecked();
    const initialRowBox = await pluginRow.boundingBox();
    expect(initialRowBox).not.toBeNull();
    expect(initialRowBox!.height).toBeGreaterThanOrEqual(44);
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
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await spark.tap();
    await expect(spark).not.toBeChecked();
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.surface).toHaveCount(1);
    await expect(actions.save).toHaveCount(1);
    await actions.save.tap();
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
    await expect(modelPicker).toContainText(COPILOT_RECOVERY_MODEL_ID);
    await expect(modelPicker).not.toHaveClass(/text-destructive/);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
    await refresh.tap();
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
test("phone users see model-option policy recovery after capability discovery succeeds", async ({
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
    const pluginRow = testPage.locator(
      `[data-testid="copilot-plugin-row"][data-plugin-id="${COPILOT_RECOVERY_PLUGIN_ID}"]`,
    );
    const modelPicker = testPage.getByRole("button", { name: "Profile start model settings" });
    const capabilityStatus = testPage.getByTestId("profile-capability-status");
    const modelOptionsStatus = testPage.getByTestId("model-config-resolution-error");

    await expect(pluginSection).toBeVisible({ timeout: 15_000 });
    await expect(spark).toHaveCount(1);
    await expect(spark).toBeChecked();
    const initialRowBox = await pluginRow.boundingBox();
    expect(initialRowBox).not.toBeNull();
    expect(initialRowBox!.height).toBeGreaterThanOrEqual(44);
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
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    await spark.tap();
    await expect(spark).not.toBeChecked();
    const actions = copilotProfileSaveActions(testPage);
    await expect(actions.surface).toHaveCount(1);
    await expect(actions.save).toHaveCount(1);
    await actions.save.tap();
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
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBe(true);

    const refresh = testPage.getByTestId("profile-refresh-capabilities");
    await expect(refresh).toHaveCount(1);
    const refreshBox = await refresh.boundingBox();
    expect(refreshBox).not.toBeNull();
    expect(refreshBox!.height).toBeGreaterThanOrEqual(44);
    await refresh.tap();
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
