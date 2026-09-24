import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { ListAvailableAgentsResponse } from "../../../lib/types/http";

type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      setAvailableAgents: (
        agents: ListAvailableAgentsResponse["agents"],
        tools?: ListAvailableAgentsResponse["tools"],
      ) => void;
    };
  };
};

// Same interception as the desktop spec: the default mock-agent is discovered
// as already available, so the catalog would show its "everything installed"
// state. Seed one unavailable agent with an install script so an install card
// renders.
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

async function seedAvailableAgents(page: Page): Promise<void> {
  await page.waitForFunction(() => Boolean((window as E2EStoreWindow).__KANDEV_E2E_STORE__));
  await page.evaluate((response) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    store.getState().setAvailableAgents(response.agents, response.tools);
  }, AVAILABLE_AGENTS);
}

test.describe("Agents browse page on mobile", () => {
  test("renders statically without a collapsible toggle or horizontal overflow", async ({
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
    await seedAvailableAgents(testPage);

    const heading = testPage.getByRole("heading", { name: "Browse available agents" });
    await expect(heading).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("install-card-codex")).toBeVisible({ timeout: 15_000 });

    // Same static-page contract as the desktop spec: the heading is not a
    // button and the settings content region carries no collapse semantics.
    await expect(testPage.getByRole("button", { name: "Browse available agents" })).toHaveCount(0);
    expect(await heading.evaluate((el) => el.closest("button") === null)).toBe(true);

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

    await assertNoDocumentHorizontalOverflow(testPage, "static browse page");
  });
});
