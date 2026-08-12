import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

// Same interception as the desktop spec: the default mock-agent is discovered
// as installed with no install_script, so seed one discoverable-but-not-
// installed agent to give the section an install card.
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

test.describe("Available to Install collapsible section on mobile", () => {
  test("collapses and re-expands by tap without horizontal overflow", async ({ testPage }) => {
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

    // The heading row is the touch target for the toggle.
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox).not.toBeNull();
    expect(triggerBox!.height).toBeGreaterThanOrEqual(44);
    expect(triggerBox!.width).toBeGreaterThanOrEqual(44);

    // Expanded by default; tap collapses the card grid.
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(installCard).toBeVisible();
    await trigger.tap();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await expect(installCard).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage, "collapsed section");

    // Tapping again restores it.
    await trigger.tap();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(installCard).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "expanded section");
  });
});
