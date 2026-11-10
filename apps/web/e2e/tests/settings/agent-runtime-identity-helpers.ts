import { expect, type Page } from "@playwright/test";
import type { AgentUpdateStatus } from "../../../lib/api/domains/agent-update-api";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

export const LOCAL_PATH = `/opt/homebrew/${"long-installation-directory/".repeat(10)}bin/copilot`;

export async function installIdentityFixture(page: Page, native = false) {
  const status: AgentUpdateStatus = {
    update_mode: "pinned",
    agent_name: "copilot-acp",
    display_name: "Copilot",
    runtime_id: native ? "native:copilot" : "npm:@github/copilot",
    owner: native ? "external" : "kandev",
    mechanism: native ? "native" : "npm_candidate",
    management: native ? "manual" : "managed",
    source: native ? "native" : "managed",
    package: "@github/copilot",
    default_version: "1.0.88",
    effective_version: "1.0.89",
    current_version: "1.0.70",
    available: true,
    enabled: true,
    auto_update: false,
    auto_update_supported: !native,
    check_state: "unknown",
  };
  const runtime = await installRuntimeUpdateFixture(page, {
    agentName: "copilot-acp",
    displayName: "Copilot",
    packageName: "@github/copilot",
    currentVersion: "1.0.88",
    defaultVersion: "1.0.88",
    statusResponse: [status],
  });
  const agent = {
    name: "copilot-acp",
    display_name: "Copilot",
    supports_mcp: true,
    installation_paths: ["copilot"],
    available: true,
    matched_path: LOCAL_PATH,
    cli_version: "1.0.70",
    capabilities: {
      supports_session_resume: true,
      supports_shell: true,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: "gpt-5",
      available_models: [{ id: "gpt-5", name: "GPT-5" }],
      status: "ok",
    },
    runtime_update: {
      supported: true,
      update_mode: "pinned",
      package: "@github/copilot",
      effective_version: "1.0.88",
      managed_fallback: native,
    },
  };
  for (const endpoint of ["discovery", "available"]) {
    await page.route(`**/api/v1/agents/${endpoint}`, (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ agents: [agent], total: 1 }),
      }),
    );
  }
  return runtime;
}

export async function expectIdentity(page: Page, native = false) {
  const card = page.getByTestId("agent-group-copilot-acp");
  const summary = card.getByTestId("session-runtime-copilot-acp");
  await expect(summary).toContainText(
    native ? "Session runtime: Native CLI" : "Session runtime: Managed by Kandev",
  );
  if (!native) await expect(summary).toContainText("@github/copilot · 1.0.89");
  await expect(summary).not.toContainText("1.0.70");
  await expect(summary).toContainText("For new sessions");
  const diagnostics = card.getByTestId("agent-diagnostics-copilot-acp");
  await expect(diagnostics).not.toHaveAttribute("open");
  await expect(card.getByText(`Detected local CLI: ${LOCAL_PATH}`, { exact: true })).toBeHidden();
  return { card, diagnostics };
}

export async function expectExpandedDiagnostics(page: Page) {
  const diagnostics = page.getByTestId("agent-diagnostics-copilot-acp");
  await expect(diagnostics).toHaveAttribute("open");
  await expect(diagnostics).toContainText(LOCAL_PATH);
  await expect(diagnostics.getByTestId("cli-version-copilot-acp")).toHaveText(
    "Local CLI: Copilot 1.0.70",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
