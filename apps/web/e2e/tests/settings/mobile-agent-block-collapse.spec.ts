import { test, expect } from "../../fixtures/test-base";

const ZERO_PROFILE_AGENT = "zero-profile-agent";

test.describe("Agent block collapse on mobile", () => {
  test("collapses an agent block from a touch-sized control and keeps it collapsed after reload", async ({
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
      await apiClient.createAgentProfile(agent.id, "Mobile Collapse Count", {
        model: "mock-fast",
      });
    }
    const { agents: refreshed } = await apiClient.listAgents();
    const target = refreshed.find((item) => item.name === agent.name) ?? agent;
    const count = target.profiles.length;
    expect(count).toBeGreaterThan(0);
    const countLabel = count === 1 ? "1 profile" : `${count} profiles`;
    const horizontalOverflow = () =>
      testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );

    await testPage.goto("/settings/agents");
    const group = testPage.getByTestId(`agent-group-${agent.name}`);
    await expect(group).toBeVisible({ timeout: 15_000 });
    // Wait for discovery to resolve before touching any toggle: the injected
    // card exists only in the discovery response, so its presence proves the
    // orphan → detected key transition has completed and the card chrome is
    // stable (an earlier flake detached the toggle mid-scroll).
    await expect(testPage.getByTestId(`agent-group-${ZERO_PROFILE_AGENT}`)).toBeVisible({
      timeout: 15_000,
    });

    const toggle = group.getByTestId(`collapse-agent-${agent.name}`);
    await expect(toggle).toBeVisible();
    await toggle.scrollIntoViewIfNeeded();
    const box = await toggle.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    expect(box!.width).toBeGreaterThanOrEqual(44);

    await toggle.tap();
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);
    // The collapsed header (toggle + count, possibly + Setup Profile) must not
    // push the page horizontally on a phone width.
    expect(await horizontalOverflow()).toBeLessThanOrEqual(0);

    // The zero-profile case renders the long "No profiles yet" sentence in the
    // header — the worst-case width. The injected agent has no saved record,
    // so the card also carries the Setup Profile button in the same cluster.
    const zeroGroup = testPage.getByTestId(`agent-group-${ZERO_PROFILE_AGENT}`);
    const zeroToggle = zeroGroup.getByTestId(`collapse-agent-${ZERO_PROFILE_AGENT}`);
    await expect(zeroToggle).toBeVisible();
    await zeroToggle.tap();
    await expect(zeroGroup.getByTestId(`collapsed-count-${ZERO_PROFILE_AGENT}`)).toContainText(
      "No profiles yet",
    );
    expect(await horizontalOverflow()).toBeLessThanOrEqual(0);

    // The choice survives a reload on the phone too.
    await testPage.reload();
    await expect(group).toBeVisible({ timeout: 15_000 });
    await expect(group.getByTestId(`agent-profiles-${agent.name}`)).toBeHidden();
    await expect(group.getByTestId(`collapsed-count-${agent.name}`)).toHaveText(countLabel);
  });
});
