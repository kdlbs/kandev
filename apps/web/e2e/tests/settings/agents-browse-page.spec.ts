import { test, expect } from "../../fixtures/test-base";
import type { ListAvailableAgentsResponse } from "../../../lib/types/http";

// The default mock-agent is discovered as already available (it has an
// InstallScript, but the catalog filters on !available && install_script), so
// the catalog would show its "everything installed" state with no install
// cards. Seed one unavailable agent with an install script after navigation.
// Settings pages hydrate available agents in the server-rendered boot payload,
// so a route-only mock can be skipped by the loaded-state guard.
const AVAILABLE_AGENTS = {
  agents: [
    {
      name: "codex",
      display_name: "OpenAI Codex CLI",
      install_script: "npm install -g @openai/codex",
      supports_mcp: false,
      mcp_config_path: null,
      installation_paths: [],
      available: false,
      matched_path: null,
      capabilities: {
        supports_session_resume: false,
        supports_shell: false,
        supports_workspace_only: false,
      },
      model_config: {
        default_model: "",
        available_models: [],
        available_modes: [],
        current_mode_id: "",
        supports_dynamic_models: false,
        status: "not_installed",
        error: "",
      },
      permission_settings: {},
      updated_at: "2026-08-12T00:00:00Z",
    },
  ],
  tools: [],
  total: 1,
} satisfies ListAvailableAgentsResponse;

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      availableAgents: {
        items: ListAvailableAgentsResponse["agents"];
        tools: ListAvailableAgentsResponse["tools"];
        loading: boolean;
        loaded: boolean;
      };
    };
    setState: (state: {
      availableAgents: {
        items: ListAvailableAgentsResponse["agents"];
        tools: ListAvailableAgentsResponse["tools"];
        loading: boolean;
        loaded: boolean;
      };
    }) => void;
  };
};

test.describe("Agents browse page", () => {
  test("renders the heading and install cards statically, without a collapsible toggle", async ({
    testPage,
  }) => {
    const availableAgents = AVAILABLE_AGENTS.agents.map((agent) => ({
      ...agent,
      updated_at: new Date(Date.now() + 60_000).toISOString(),
    }));
    await testPage.route("**/api/v1/agents/available**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ...AVAILABLE_AGENTS, agents: availableAgents }),
      }),
    );
    await testPage.goto("/settings/agents/browse");

    const heading = testPage.getByRole("heading", { name: "Browse available agents" });
    await expect(heading).toBeVisible({ timeout: 15_000 });
    // The SSR payload marks this resource as loaded before the client hook
    // runs. Replace that hydrated snapshot directly so the assertion does not
    // depend on whether a second fetch happens after the page mounts.
    await testPage.evaluate((agents) => {
      const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      // Use the test store's partial-state bridge instead of the production
      // action. The action rejects snapshots older than the SSR timestamp,
      // while this fixture intentionally owns the catalog contents.
      const current = store.getState().availableAgents;
      store.setState({
        availableAgents: {
          ...current,
          items: agents,
          tools: [],
          loading: false,
          loaded: true,
        },
      });
    }, availableAgents);
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible({ timeout: 15_000 });

    // PR #2544 wrapped the section in a collapsible whose heading row was a
    // toggle button. Reverted, the heading must be a plain heading: no button
    // with the heading's accessible name and no button ancestor. Assert the
    // semantic shape rather than the old implementation's test ID, so any
    // future collapsible reintroduction fails even with different test IDs.
    await expect(testPage.getByRole("button", { name: "Browse available agents" })).toHaveCount(0);
    expect(await heading.evaluate((el) => el.closest("button") === null)).toBe(true);

    // A role-less clickable wrapper (e.g. <div onClick>) would not surface as
    // a button; clicking the heading must not hide the install cards.
    await heading.click();
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible({ timeout: 15_000 });

    // A separately-triggered collapsible (e.g. a toggle button elsewhere in
    // the content) would not be caught by the heading assertions. The page
    // content must carry no collapse semantics: interactive toggles
    // (aria-expanded/aria-controls) or Radix collapse states (data-state
    // open/closed). Other data-state values (e.g. a future streaming status)
    // are not collapse behavior and must not fail the scan. Scoped to the
    // settings content region so the sidebar and topbar chrome (which use
    // Radix data-state/aria-expanded legitimately) do not false-positive.
    const collapseSemantics = await testPage.evaluate(() => {
      const content = document.querySelector('[data-testid="settings-scroll-container"]');
      if (!content) return ["<missing settings-scroll-container>"];
      return [...content.querySelectorAll("[aria-expanded], [aria-controls], [data-state]")]
        .filter(
          (el) =>
            el.hasAttribute("aria-expanded") ||
            el.hasAttribute("aria-controls") ||
            el.getAttribute("data-state") === "open" ||
            el.getAttribute("data-state") === "closed",
        )
        .map(
          (el) =>
            `${el.tagName.toLowerCase()}[data-testid="${el.getAttribute("data-testid") ?? ""}"]`,
        );
    });
    expect(collapseSemantics).toEqual([]);

    // Compatibility guard: the exact test ID PR #2544 introduced is gone too.
    await expect(testPage.getByTestId("available-to-install-trigger")).toHaveCount(0);
  });

  test("renders the saved fallback summary after the model badge", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    if (!agent || agent.profiles.length === 0) {
      throw new Error("The E2E fixture must provide a configured agent profile");
    }

    const fallbackModel = "saved-explicit-model";
    const profileName = "Desktop fallback summary";
    let profileId: string | undefined;

    try {
      await testPage.goto("/settings/agents");
      const seededRow = testPage
        .getByTestId("agent-profile-row")
        .filter({ hasText: agent.profiles[0].name });
      await expect(seededRow).toBeVisible({ timeout: 15_000 });

      const profile = await apiClient.createAgentProfile(agent.id, profileName, {
        model: agent.profiles[0].model,
        fallback_model: fallbackModel,
      });
      profileId = profile.id;
      await testPage.reload();

      const row = testPage.getByTestId("agent-profile-row").filter({ hasText: profile.name });
      await expect(row).toBeVisible({ timeout: 15_000 });
      await expect(row.locator('[data-slot="badge"]')).toHaveText([
        profile.model,
        `fallback: ${fallbackModel}`,
      ]);
    } finally {
      if (profileId) {
        await apiClient.deleteAgentProfile(profileId, true);
      }
    }
  });
});
