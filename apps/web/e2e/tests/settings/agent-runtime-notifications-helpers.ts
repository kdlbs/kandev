import { expect, type Page } from "@playwright/test";
import type { AgentUpdateStatus } from "../../../lib/api/domains/agent-update-api";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

export async function runtimeAwareness(page: Page, mobile = false, capture?: PrAssetCapture) {
  let statuses: AgentUpdateStatus[] = [
    {
      agent_name: "claude-acp",
      display_name: "Claude",
      runtime_id: "npm:@agentclientprotocol/claude-agent-acp",
      owner: "kandev",
      mechanism: "npm_candidate",
      management: "managed",
      source: "@agentclientprotocol/claude-agent-acp",
      guidance_url: "https://www.npmjs.com/package/@agentclientprotocol/claude-agent-acp",
      current_version: "0.62.0",
      available: true,
      enabled: true,
      auto_update_supported: true,
      auto_update: false,
      package: "@agentclientprotocol/claude-agent-acp",
      default_version: "0.64.0",
      active_version: "0.62.0",
      effective_version: "0.62.0",
      latest_version: "0.64.0",
      check_state: "update_available",
    },
    {
      agent_name: "kimi-acp",
      display_name: "Kimi",
      runtime_id: "kimi-acp",
      owner: "external",
      mechanism: "native",
      management: "manual",
      source: "github:MoonshotAI/kimi-cli",
      guidance_url: "https://www.kimi.com/code/docs/en/kimi-code-cli/",
      current_version: "1.0.0",
      available: true,
      enabled: true,
      auto_update_supported: false,
      auto_update: false,
      package: "",
      default_version: "",
      effective_version: "1.0.0",
      latest_version: "2.0.0",
      check_state: "update_available",
    },
    {
      agent_name: "cursor-acp",
      display_name: "Cursor",
      runtime_id: "cursor-acp",
      owner: "external",
      mechanism: "native",
      management: "manual",
      source: "",
      guidance_url: "https://docs.cursor.com/en/cli/installation",
      current_version: "",
      available: true,
      enabled: true,
      auto_update_supported: false,
      auto_update: false,
      package: "",
      default_version: "",
      effective_version: "",
      check_state: "unknown",
    },
  ];
  const command = [
    "npm",
    "--prefix",
    "~/.kandev/managed-npm-runtime",
    "exec",
    "--yes",
    "--prefer-online",
    "--package=@agentclientprotocol/claude-agent-acp@0.64.0",
    "--",
    "node",
    "-e",
    "",
  ];
  const runtime = await installRuntimeUpdateFixture(page, {
    statusResponse: statuses,
    previewResponse: {
      agent_name: "claude-acp",
      package: "@agentclientprotocol/claude-agent-acp",
      current_version: "0.62.0",
      active_version: "0.62.0",
      effective_version: "0.62.0",
      default_version: "0.64.0",
      target_version: "0.64.0",
      operation: "update",
      available_versions: [
        { version: "0.64.0", latest: true },
        { version: "0.62.0", latest: false },
        { version: "0.61.0", latest: false },
      ],
      command,
      command_string: command.slice(0, -1).join(" ") + ' ""',
    },
  });
  let policyWrites = 0;
  await page.route("**/api/v1/agent-update/claude-acp/automatic", async (route) => {
    const { enabled } = route.request().postDataJSON() as { enabled: boolean };
    policyWrites++;
    statuses = statuses.map((s) =>
      s.agent_name === "claude-acp" ? { ...s, auto_update: enabled } : s,
    );
    runtime.setStatusResponse(statuses);
    await route.fulfill({ json: { enabled } });
  });
  await page.goto("/");
  const indicator = page.getByTestId("agent-runtime-update-indicator");
  await expect(indicator).toContainText("2 agent runtime updates");
  if (mobile) expect((await indicator.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(runtime.postCount()).toBe(0);
  await runtime.emit("system.update_available", {
    agent_name: "kimi-acp",
    runtime_id: "kimi-acp",
    display_name: "Kimi",
    version: "2.0.0",
    previous_version: "1.0.0",
    runtime_update_status: "available",
    occurrence_id: "native-kimi-2",
    title: "server fallback",
    body: "server fallback",
  });
  const toast = page
    .getByTestId("toast-message")
    .filter({ hasText: "Kimi runtime update available" });
  await expect(toast).toContainText("1.0.0");
  await waitForFiniteAnimations(toast);
  const toastBox = (await toast.boundingBox())!;
  expect(toastBox.x).toBeGreaterThanOrEqual(0);
  expect(toastBox.x + toastBox.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  const review = toast.getByRole("link", { name: "Review runtime" });
  if (mobile) expect((await review.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await capture?.screenshot("runtime-notice", {
    caption: "A named native runtime notice links directly to its update guidance.",
  });
  await review.click();
  await expect(page).toHaveURL(/settings\/agents#runtime-update-kimi-acp$/);
  const managed = page.getByTestId("runtime-policy-claude-acp");
  const automatic = managed.getByRole("switch", { name: "Automatic updates" });
  await expect(automatic).not.toBeChecked();
  await automatic.click();
  expect(policyWrites).toBe(0);
  await page.getByTestId("settings-floating-save").getByRole("button", { name: /Save/i }).click();
  await expect.poll(() => policyWrites).toBe(1);
  await expect(automatic).toBeChecked();
  await page.reload();
  await expect(automatic).toBeChecked();
  const native = page.getByTestId("runtime-policy-kimi-acp");
  await expect(native).toContainText("Managed externally");
  await expect(native.getByRole("switch")).toHaveCount(0);
  await expect(native.getByRole("link", { name: "Manual update guidance" })).toHaveAttribute(
    "href",
    /kimi/,
  );
  const unknown = page.getByTestId("runtime-policy-cursor-acp");
  await expect(unknown).toContainText("Latest: Unknown");
  await expect(unknown).not.toContainText("Up to date");
  statuses = statuses.map((status) =>
    status.agent_name === "claude-acp"
      ? {
          ...status,
          last_outcome: {
            id: "failed-validation",
            status: "failed",
            previous_version: "0.62.0",
            target_version: "0.64.0",
          },
        }
      : status,
  );
  runtime.setStatusResponse(statuses);
  await page.reload();
  await expect(managed).toContainText("could not activate");
  await managed.scrollIntoViewIfNeeded();
  await capture?.screenshot("runtime-settings", {
    caption: "Saved consent, version ownership and retained failure recovery in Agents settings.",
  });
  await managed.getByRole("link", { name: "Manage versions" }).click();
  await page.getByTestId("agent-update-trigger-claude-acp").click();
  const versionControl = page.getByTestId(
    `agent-update-${mobile ? "drawer" : "dialog"}-claude-acp`,
  );
  await expect(versionControl).toBeVisible();
  await waitForFiniteAnimations(versionControl);
  await expect(versionControl).toContainText("0.62.0 → 0.64.0");
  await expect(versionControl).toContainText("claude-agent-acp@0.64.0");
  await capture?.screenshot("runtime-version-control", {
    caption: "Managed version selection retains the existing desktop dialog and phone drawer.",
  });
  expect(runtime.postCount()).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
}
