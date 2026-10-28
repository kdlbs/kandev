import { test, expect } from "../../fixtures/test-base";
import {
  copilotPluginDiscovery,
  createCopilotPluginFixture,
} from "../../helpers/copilot-plugin-fixture";

test("selects, saves, reloads, resets, and clears Copilot profile plugins", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(60_000);
  const fixture = await createCopilotPluginFixture(apiClient);
  test.skip(!fixture, "Copilot agent is not registered in this E2E runtime");
  if (!fixture) return;
  await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(copilotPluginDiscovery(fixture.agentId, fixture.profileId)),
    });
  });

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    const selector = testPage.getByTestId("copilot-plugins-selection");
    await expect(selector).toBeVisible({ timeout: 15_000 });

    const spark = testPage.getByRole("checkbox", { name: "Select spark" });
    const builtin = testPage.getByRole("checkbox", { name: "Select browser-use" });
    await expect(spark).not.toBeChecked();
    await expect(builtin).toHaveAttribute("disabled", "");
    await spark.click();
    await expect(spark).toBeChecked();
    await expect(selector).toHaveAttribute("data-settings-dirty", "true");
    await expect(testPage.getByTestId("copilot-plugins-start-blocked")).toContainText(
      "A new process cannot start with this selection.",
    );

    const save = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
    await save.click();
    await expect
      .poll(async () => (await apiClient.getAgentProfile(fixture.profileId)).copilotSelectedPlugins)
      .toEqual(["installed:copilot-plugins/spark"]);

    await testPage.reload();
    await expect(spark).toBeChecked({ timeout: 15_000 });

    await spark.click();
    const reset = testPage.getByRole("button", { name: /^Reset$/i }).first();
    await expect(reset).toBeEnabled();
    await reset.click();
    await expect(spark).toBeChecked();

    await spark.click();
    await save.click();
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
  await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
    requests.push(JSON.parse(route.request().postData() ?? "{}") as Record<string, unknown>);
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(copilotPluginDiscovery(fixture.agentId, fixture.profileId)),
    });
  });

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    await expect(testPage.getByTestId("copilot-plugins-selection")).toBeVisible({
      timeout: 15_000,
    });
    await expect.poll(() => requests.length).toBeGreaterThan(0);
    const initial = requests[0];
    expect(initial.profile_id).toBe(fixture.profileId);
    expect(initial.launch_settings).toBeDefined();
    expect(initial.refresh).not.toBe(true);

    await testPage.getByTestId("copilot-plugins-refresh").click();
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
  await apiClient.updateAgentProfile(profileId, {
    copilot_selected_plugins: ["installed:copilot-plugins/spark"],
  });
  let failRefresh = false;
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

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${profileId}`);
    const spark = testPage.getByRole("checkbox", { name: "Select spark" });
    await expect(spark).toBeChecked({ timeout: 15_000 });
    failRefresh = true;
    await testPage.getByTestId("copilot-plugins-refresh").click();
    await expect(testPage.getByTestId("copilot-plugins-failed")).toBeVisible();
    await expect(testPage.getByText(/Saved selection, availability not checked/)).toBeVisible();
    await expect(spark).not.toHaveAttribute("disabled", "");
    await spark.click();
    await expect(spark).not.toBeChecked();
    await testPage
      .getByRole("button", { name: /^Save( changes)?$/i })
      .first()
      .click();
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
  await apiClient.updateAgentProfile(fixture.profileId, {
    copilot_selected_plugins: ["installed:missing/plugin"],
  });
  await testPage.route(`**/api/v1/agents/${fixture.agentId}/plugin-discovery`, async (route) => {
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_id: fixture.agentId,
        profile_id: fixture.profileId,
        provider_id: "copilot-acp",
        status: "ready",
        plugins: [],
      }),
    });
  });

  try {
    await testPage.goto(`/settings/agents/${fixture.agentName}/profiles/${fixture.profileId}`);
    await expect(testPage.getByText(/Plugin is missing/)).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByText(/A new process cannot start with this selection/),
    ).toBeVisible();
    const missing = testPage.getByRole("checkbox", { name: "Select plugin" });
    await expect(missing).toBeChecked();
    await expect(missing).not.toHaveAttribute("disabled", "");
  } finally {
    await fixture.dispose();
  }
});
