import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

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

test.describe("OnboardingDialog with realistic agent data — mobile layout", () => {
  test("no content overflows the dialog on Pixel-7-class width (412)", async ({ testPage }) => {
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
    await expect(dialog).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();
    // Wait for the agent rows to render — listAvailableAgents resolves async.
    await expect(testPage.getByText("Claude Code (Anthropic CLI)", { exact: true })).toBeVisible();

    await assertNoChildOverflowsDialog(dialog);
  });
});

async function assertNoChildOverflowsDialog(dialog: Locator): Promise<void> {
  const dialogBox = await dialog.boundingBox();
  expect(dialogBox).not.toBeNull();
  if (!dialogBox) return;
  const dialogRight = dialogBox.x + dialogBox.width;

  const overflowing = await dialog.evaluate((root, dialogRightArg) => {
    const limit = dialogRightArg as number;
    const results: { tag: string; text: string; right: number }[] = [];
    const skip = new Set(["SVG", "PATH", "CIRCLE", "RECT", "LINE", "G"]);
    const all = root.querySelectorAll("*");
    for (const node of all) {
      if (skip.has(node.tagName)) continue;
      const rect = node.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) continue;
      if (rect.right > limit + 1) {
        results.push({
          tag: node.tagName.toLowerCase(),
          text: (node.textContent ?? "").trim().slice(0, 80),
          right: rect.right,
        });
      }
    }
    return results;
  }, dialogRight);

  expect(
    overflowing,
    `Pixel 7 AI Agents: ${overflowing.length} element(s) overflow the dialog right edge (${dialogRight.toFixed(
      1,
    )}). First few:\n${overflowing
      .slice(0, 8)
      .map((o) => `  <${o.tag}> right=${o.right.toFixed(1)} text="${o.text}"`)
      .join("\n")}`,
  ).toHaveLength(0);
}
