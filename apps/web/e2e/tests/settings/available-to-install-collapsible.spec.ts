import { test, expect } from "../../fixtures/test-base";

// The default mock-agent is discovered as installed with no install_script, so
// the "Available to Install" section would not render. Intercept
// /api/v1/agents/available and return one discoverable-but-not-installed agent
// so the section has an install card to collapse.
const AVAILABLE_AGENTS = {
  agents: [
    {
      name: "codex",
      display_name: "OpenAI Codex CLI",
      available: false,
      install_script: "npm install -g @openai/codex",
      info_url: "",
      model_config: {
        default_model: "",
        available_models: [],
        modes: [],
        current_mode_id: "",
        status: "not_installed",
        error: "",
      },
      permission_settings: {},
      passthrough_config: null,
    },
  ],
  tools: [],
  total: 1,
};

test.describe("Available to Install collapsible section", () => {
  test("collapses and re-expands the install card grid from the heading row", async ({
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

    const trigger = testPage.getByTestId("available-to-install-trigger");
    const installCard = testPage.getByTestId("install-card-codex");
    await expect(trigger).toBeVisible({ timeout: 15_000 });

    // The section renders expanded by default.
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(installCard).toBeVisible();

    // Clicking the heading row collapses the card grid.
    await trigger.click();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await expect(installCard).toBeHidden();

    // Clicking again restores it.
    await trigger.click();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(installCard).toBeVisible();
  });
});
