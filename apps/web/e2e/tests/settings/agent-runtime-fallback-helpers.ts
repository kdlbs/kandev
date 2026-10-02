import { expect, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

export async function managedFallbackAwareness(
  page: Page,
  mobile: boolean,
  capture?: PrAssetCapture,
) {
  await installRuntimeUpdateFixture(page);
  const agent = {
    name: "opencode-acp",
    display_name: "OpenCode",
    description: "OpenCode ACP",
    supports_mcp: true,
    installation_paths: ["opencode"],
    available: true,
    matched_path: "/usr/local/bin/opencode",
    capabilities: {
      supports_session_resume: true,
      supports_shell: true,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: "opencode/test",
      available_models: [],
      supports_dynamic_models: false,
      status: "ok",
    },
    runtime_update: {
      supported: true,
      managed_fallback: true,
      package: "opencode-ai",
      current_version: "",
      default_version: "8.0.0",
      active_version: "8.0.0",
      effective_version: "8.0.0",
    },
  };
  await page.route("**/api/v1/agents/available", (route) =>
    route.fulfill({ json: { agents: [agent], tools: [], total: 1 } }),
  );
  await page.route("**/api/v1/agents/discovery", (route) =>
    route.fulfill({ json: { agents: [agent], total: 1 } }),
  );
  await page.route("**/api/v1/agent-update/status", (route) =>
    route.fulfill({
      json: {
        statuses: [
          {
            agent_name: "opencode-acp",
            display_name: "OpenCode",
            runtime_id: "native:opencode",
            owner: "external",
            mechanism: "native",
            management: "manual",
            managed_fallback: true,
            source: "opencode-ai",
            guidance_url: "https://opencode.ai/docs/cli/",
            current_version: "1.0.0",
            available: true,
            enabled: true,
            auto_update_supported: false,
            auto_update: false,
            package: "opencode-ai",
            default_version: "",
            effective_version: "1.0.0",
            latest_version: "9.0.0",
            check_state: "update_available",
          },
        ],
      },
    }),
  );
  await page.route("**/api/v1/agent-update/opencode-acp/preview**", (route) =>
    route.fulfill({
      json: {
        agent_name: "opencode-acp",
        managed_fallback: true,
        package: "opencode-ai",
        current_version: "",
        default_version: "8.0.0",
        active_version: "8.0.0",
        effective_version: "8.0.0",
        target_version: "9.0.0",
        operation: "update",
        available_versions: [
          { version: "9.0.0", latest: true },
          { version: "8.0.0", latest: false },
        ],
        command: ["npm", "exec", "--package=opencode-ai@9.0.0", "--", "node", "-e", ""],
        command_string: 'npm exec --package=opencode-ai@9.0.0 -- node -e ""',
      },
    }),
  );
  await page.goto("/settings/agents#runtime-update-opencode-acp");
  const native = page.getByTestId("runtime-policy-opencode-acp");
  await expect(native).toContainText("Observed: 1.0.0");
  await expect(native.getByRole("switch")).toHaveCount(0);
  await expect(native.getByRole("link", { name: "Manual update guidance" })).toHaveAttribute(
    "href",
    "https://opencode.ai/docs/cli/",
  );
  const selection = native.getByRole("link", { name: "Manage fallback versions" });
  await selection.scrollIntoViewIfNeeded();
  if (mobile) expect((await selection.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(
    await selection.evaluate((element) => {
      const box = element.getBoundingClientRect();
      return element.contains(
        document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
      );
    }),
  ).toBe(true);
  await selection.click();
  await page.getByTestId("agent-update-trigger-opencode-acp").click();
  const surface = page.getByTestId(`agent-update-${mobile ? "drawer" : "dialog"}-opencode-acp`);
  await expect(surface).toContainText("Unknown → 9.0.0");
  await expect(surface).toContainText("Active version: 8.0.0");
  await expect(surface).toContainText("Effective version: 8.0.0");
  await expect(surface).toContainText("The native host installation stays unchanged");
  await expect(surface).toContainText("--package=opencode-ai@9.0.0");
  await waitForFiniteAnimations(surface);
  await capture?.screenshot("runtime-native-fallback", {
    caption: "Native host guidance stays separate from managed fallback version selection.",
  });
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
}
