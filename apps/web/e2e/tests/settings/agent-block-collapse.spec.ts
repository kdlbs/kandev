import { test, expect } from "../../fixtures/test-base";

const COLLAPSE_STORAGE_KEY = "kandev:agents:collapsedBlocks:v1";
const ZERO_PROFILE_AGENT = "zero-profile-agent";

test.describe("Agent block collapse", () => {
  test("collapses an agent block, shows the count, and persists across reloads", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    // The standard fixture registers only mock-agent (which has profiles), so
    // a zero-profile card can never occur naturally. Inject one via discovery:
    // it doubles as the deterministic "discovery has settled" signal, because
    // it exists in the DOM only after the discovery response lands (before
    // that, cards render from the boot payload as orphans whose keys change
    // when discovery resolves, remounting the card mid-interaction).
    await testPage.route("**/api/v1/agents/discovery", async (route) => {
      const response = await route.fetch();
      const body = await response.json();
      body.agents = [
        ...(body.agents ?? []),
        {
          name: ZERO_PROFILE_AGENT,
          supports_mcp: false,
          mcp_config_path: null,
          installation_paths: [],
          available: true,
          matched_path: null,
        },
      ];
      await route.fulfill({ response, json: body });
    });

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (agent.profiles.length === 0) {
      await apiClient.createAgentProfile(agent.id, "Collapse Count", { model: "mock-fast" });
    }
    const { agents: refreshed } = await apiClient.listAgents();
    const target = refreshed.find((item) => item.name === agent.name) ?? agent;
    const count = target.profiles.length;
    expect(count).toBeGreaterThan(0);
    const countLabel = count === 1 ? "1 profile" : `${count} profiles`;

    await testPage.goto("/settings/agents");
    const group = testPage.getByTestId(`agent-group-${agent.name}`);
    await expect(group).toBeVisible({ timeout: 15_000 });
    // Wait for discovery to resolve before touching any toggle: the injected
    // card exists only in the discovery response, so its presence proves the
    // orphan → detected key transition has completed.
    await expect(testPage.getByTestId(`agent-group-${ZERO_PROFILE_AGENT}`)).toBeVisible({
      timeout: 15_000,
    });
    const body = group.getByTestId(`agent-profiles-${agent.name}`);
    await expect(body).toBeVisible();
    // Default is expanded: no collapse preference exists before the first toggle.
    expect(
      await testPage.evaluate((key) => localStorage.getItem(key), COLLAPSE_STORAGE_KEY),
    ).toBeNull();

    const toggle = group.getByTestId(`collapse-agent-${agent.name}`);
    await toggle.click();
    await expect(body).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);

    // The choice survives a reload.
    await testPage.reload();
    await expect(group).toBeVisible({ timeout: 15_000 });
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);

    // Expanding restores the body and moves the count back out of the header.
    await group.getByTestId(`collapse-agent-${agent.name}`).click();
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeVisible();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveCount(0);
  });
});
