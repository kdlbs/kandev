import { test, expect } from "../../fixtures/test-base";
import { assertNoDescendantOverflowsRight } from "../../helpers/layout-assertions";

// Reproduces the "agents section content comes out of the modal on the right"
// bug on Pixel-class mobile widths. The default mock-agent has a short
// display_name and no install_script, so the regular spec can't catch the
// overflow scenarios that real installs trigger (long display names, multi-
// agent install_script rows, etc.). This spec intercepts the
// /api/v1/agents/available endpoint and returns a realistic payload.

const FAKE_AVAILABLE_AGENTS = {
  agents: [
    {
      name: "claude-code",
      display_name: "Claude Code (Anthropic CLI)",
      available: true,
      install_script: "",
      info_url: "",
      model_config: {
        default_model: "claude-sonnet-4-5",
        available_models: [{ id: "claude-sonnet-4-5", name: "Claude Sonnet 4.5 Long Model Name" }],
        modes: [],
        current_mode_id: "",
        status: "ok",
        error: "",
      },
      permission_settings: {},
      passthrough_config: null,
    },
    {
      name: "codex",
      display_name: "OpenAI Codex CLI Tool With Long Name",
      available: false,
      install_script: "npm install -g @openai/codex-cli-with-a-very-long-package-name",
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
    {
      name: "opencode",
      display_name: "OpenCode",
      available: false,
      install_script: "curl -fsSL https://opencode.example.com/install/script.sh | sh",
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
  tools: [
    {
      name: "ripgrep",
      display_name: "ripgrep",
      description:
        "Fast recursive search tool used by agents for codebase navigation and file lookups.",
      available: false,
      install_script: "brew install ripgrep",
      info_url: "https://github.com/BurntSushi/ripgrep",
    },
  ],
  total: 3,
};

test.describe("First-run onboarding with realistic agent data — mobile", () => {
  test("opens on a larger viewport after a phone visit and keeps agent data contained", async ({
    testPage,
  }) => {
    await testPage.route("**/api/v1/agents/available**", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(FAKE_AVAILABLE_AGENTS),
      });
    });
    await testPage.addInitScript(() => {
      localStorage.removeItem("kandev.onboarding.completed");
    });
    // Pixel 7 viewport. Playwright's `mobile-chrome` project ships Pixel 5
    // (393x851); we override to Pixel 7's 412x915 here so the spec
    // reproduces the exact width the user reports.
    await testPage.setViewportSize({ width: 412, height: 915 });
    await testPage.goto("/");

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toHaveCount(0);
    await expect(testPage.getByTestId("mobile-kanban-layout")).toBeVisible();
    await expect(testPage.getByTestId("mobile-fab")).toBeEnabled();
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();

    await testPage.setViewportSize({ width: 768, height: 915 });
    await expect(dialog).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();
    await expect(testPage.getByText("Claude Code (Anthropic CLI)", { exact: true })).toBeVisible();

    await assertNoDescendantOverflowsRight(dialog, "larger viewport AI Agents");
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();
  });
});
