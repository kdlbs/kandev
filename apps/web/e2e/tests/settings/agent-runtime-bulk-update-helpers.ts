import { expect, type Page } from "@playwright/test";
import { installRuntimeUpdateFixture } from "./agent-runtime-update-helpers";

const CLAUDE_PACKAGE = "@agentclientprotocol/claude-agent-acp";
const CODEX_PACKAGE = "@openai/codex";
const NOW = "2026-07-26T12:00:00.000Z";

export async function installBulkRuntimeFixture(page: Page) {
  const runtime = await installRuntimeUpdateFixture(page, {
    statusResponse: [
      {
        update_mode: "pinned",
        agent_name: "claude-acp",
        display_name: "Claude",
        runtime_id: `npm:${CLAUDE_PACKAGE}`,
        owner: "kandev",
        mechanism: "npm_candidate",
        management: "managed",
        source: CLAUDE_PACKAGE,
        available: true,
        enabled: true,
        auto_update_supported: true,
        auto_update: false,
        package: CLAUDE_PACKAGE,
        default_version: "0.64.0",
        active_version: "0.62.0",
        effective_version: "0.62.0",
        current_version: "0.62.0",
        latest_version: "0.64.0",
        checked_at: NOW,
        check_state: "update_available",
      },
      {
        update_mode: "pinned",
        agent_name: "codex-app-server",
        display_name: "Codex",
        runtime_id: `npm:${CODEX_PACKAGE}`,
        owner: "kandev",
        mechanism: "npm_candidate",
        management: "managed",
        source: CODEX_PACKAGE,
        available: true,
        enabled: true,
        auto_update_supported: true,
        auto_update: false,
        package: CODEX_PACKAGE,
        default_version: "2.1.0",
        active_version: "2.0.0",
        effective_version: "2.0.0",
        current_version: "2.0.0",
        latest_version: "2.1.0",
        checked_at: NOW,
        check_state: "update_available",
      },
    ],
  });
  let codexSucceeds = false;
  let claudePosts = 0;
  let codexPosts = 0;
  await page.route("**/api/v1/agents/discovery", (route) =>
    route.fulfill({
      json: {
        agents: [
          {
            name: "claude-acp",
            supports_mcp: true,
            installation_paths: ["claude-agent-acp"],
            available: true,
            matched_path: "/usr/local/bin/claude-agent-acp",
          },
          {
            name: "codex-app-server",
            supports_mcp: true,
            installation_paths: ["codex"],
            available: true,
            matched_path: "/usr/local/bin/codex",
          },
        ],
        total: 2,
      },
    }),
  );
  await page.route("**/api/v1/agents/available", (route) =>
    route.fulfill({
      json: {
        agents: [
          {
            name: "claude-acp",
            display_name: "Claude",
            description: "Claude ACP runtime",
            supports_mcp: true,
            installation_paths: ["claude-agent-acp"],
            available: true,
            matched_path: "/usr/local/bin/claude-agent-acp",
            capabilities: {
              supports_session_resume: true,
              supports_shell: true,
              supports_workspace_only: false,
            },
            model_config: {
              default_model: "claude-sonnet-4-6",
              current_model_id: "claude-sonnet-4-6",
              available_models: [
                {
                  id: "claude-sonnet-4-6",
                  name: "Claude Sonnet 4.6",
                  description: "Claude Sonnet 4.6 model",
                  is_default: true,
                  source: "dynamic",
                },
              ],
              supports_dynamic_models: true,
              status: "ok",
            },
            runtime_update: {
              supported: true,
              update_mode: "pinned",
              package: CLAUDE_PACKAGE,
            },
            updated_at: NOW,
          },
          {
            name: "codex-app-server",
            display_name: "Codex",
            description: "Codex ACP runtime",
            supports_mcp: true,
            installation_paths: ["codex"],
            available: true,
            matched_path: "/usr/local/bin/codex",
            capabilities: {
              supports_session_resume: true,
              supports_shell: true,
              supports_workspace_only: false,
            },
            model_config: {
              default_model: "gpt-5",
              current_model_id: "gpt-5",
              available_models: [
                {
                  id: "gpt-5",
                  name: "GPT-5",
                  description: "GPT-5 model",
                  is_default: true,
                  source: "dynamic",
                },
              ],
              supports_dynamic_models: true,
              status: "ok",
            },
            runtime_update: {
              supported: true,
              update_mode: "pinned",
              package: CODEX_PACKAGE,
            },
            updated_at: NOW,
          },
        ],
        tools: [],
        total: 2,
      },
    }),
  );
  await page.route("**/api/v1/agent-update/claude-acp/preview**", (route) =>
    route.fulfill({
      json: {
        update_mode: "pinned",
        agent_name: "claude-acp",
        package: CLAUDE_PACKAGE,
        current_version: "0.62.0",
        default_version: "0.64.0",
        active_version: "0.62.0",
        effective_version: "0.62.0",
        target_version: "0.64.0",
        operation: "update",
        available_versions: [
          { version: "0.64.0", latest: true },
          { version: "0.62.0", latest: false },
        ],
        command: ["npm", "exec"],
        command_string: "npm exec",
        reviewed_runtime: {
          runtime_id: `npm:${CLAUDE_PACKAGE}`,
          package: CLAUDE_PACKAGE,
          current_version: "0.62.0",
          effective_version: "0.62.0",
          active_version: "0.62.0",
          source: "managed",
        },
      },
    }),
  );
  await page.route("**/api/v1/agent-update/codex-app-server/preview**", (route) =>
    route.fulfill({
      json: {
        update_mode: "pinned",
        agent_name: "codex-app-server",
        package: CODEX_PACKAGE,
        current_version: "2.0.0",
        default_version: "2.1.0",
        active_version: "2.0.0",
        effective_version: "2.0.0",
        target_version: "2.1.0",
        operation: "update",
        available_versions: [
          { version: "2.1.0", latest: true },
          { version: "2.0.0", latest: false },
        ],
        command: ["npm", "exec"],
        command_string: "npm exec",
        reviewed_runtime: {
          runtime_id: `npm:${CODEX_PACKAGE}`,
          package: CODEX_PACKAGE,
          current_version: "2.0.0",
          effective_version: "2.0.0",
          active_version: "2.0.0",
          source: "managed",
        },
      },
    }),
  );
  await page.route("**/api/v1/agent-update/claude-acp", (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    claudePosts += 1;
    return route.fulfill({
      status: 202,
      json: {
        job_id: "bulk-claude-job",
        agent_name: "claude-acp",
        status: "queued",
        operation: "update",
        target_version: "0.64.0",
        started_at: NOW,
      },
    });
  });
  await page.route("**/api/v1/agent-update/codex-app-server", (route) => {
    if (route.request().method() !== "POST") return route.fallback();
    codexPosts += 1;
    if (!codexSucceeds) {
      return route.fulfill({
        status: 409,
        json: { error: "runtime changed after review", code: "runtime_review_changed" },
      });
    }
    return route.fulfill({
      status: 202,
      json: {
        job_id: "bulk-codex-job",
        agent_name: "codex-app-server",
        status: "queued",
        operation: "update",
        target_version: "2.1.0",
        started_at: NOW,
      },
    });
  });
  return {
    ...runtime,
    setCodexSucceeds: (value: boolean) => {
      codexSucceeds = value;
    },
    claudePosts: () => claudePosts,
    codexPosts: () => codexPosts,
    async finishClaude() {
      await runtime.emit("agent.update.finished", {
        job_id: "bulk-claude-job",
        agent_name: "claude-acp",
        status: "succeeded",
        operation: "update",
        target_version: "0.64.0",
        started_at: NOW,
        finished_at: NOW,
      });
    },
    async finishCodex() {
      await runtime.emit("agent.update.finished", {
        job_id: "bulk-codex-job",
        agent_name: "codex-app-server",
        status: "succeeded",
        operation: "update",
        target_version: "2.1.0",
        started_at: NOW,
        finished_at: NOW,
      });
    },
  };
}

export async function expectBulkManager(page: Page, mobile: boolean) {
  const surface = page.getByTestId(mobile ? "runtime-manager-drawer" : "runtime-manager-dialog");
  await expect(surface).toBeVisible();
  await expect(page.getByTestId("runtime-updates-count")).toHaveText("2");
  await expect(page.getByTestId("runtime-bulk-approve")).toBeEnabled();
  return surface;
}
