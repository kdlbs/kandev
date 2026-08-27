import { test, expect } from "../../fixtures/test-base";
import type { ListAvailableAgentsResponse } from "../../../lib/types/http";

// The default mock-agent is discovered as already available (it has an
// InstallScript, but the catalog filters on !available && install_script), so
// the catalog would show its "everything installed" state with no install
// cards. Intercept /api/v1/agents/available and return one unavailable agent
// with an install script so an install card renders.
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

test.describe("Agents browse page", () => {
  test("renders the heading and install cards statically, without a collapsible toggle", async ({
    testPage,
  }) => {
    await testPage.route("**/api/v1/agents/available**", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(AVAILABLE_AGENTS),
      }),
    );

    await testPage.goto("/settings/agents/browse");

    const heading = testPage.getByRole("heading", { name: "Browse available agents" });
    await expect(heading).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible();

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
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible();

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
});
