import type { Page } from "@playwright/test";
import type { AgentUpdateStatus } from "../../../lib/api/domains/agent-update-api";
import type {
  Agent,
  AgentDiscovery,
  AvailableAgent,
  ListAvailableAgentsResponse,
} from "../../../lib/types/http";

const OPENCODE_AGENT_NAME = "opencode-acp";
const FIXTURE_TIME = "2026-11-26T00:00:00Z";

const openCodeAvailableAgent: AvailableAgent = {
  name: OPENCODE_AGENT_NAME,
  display_name: "OpenCode",
  install_script:
    'npm --prefix ~/.kandev/managed-npm-runtime exec --yes --prefer-online --package=@opencode/cli@2.3.2 -- node -e ""',
  supports_mcp: true,
  mcp_config_path: null,
  installation_paths: [],
  available: false,
  matched_path: null,
  capabilities: {
    supports_session_resume: true,
    supports_shell: true,
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
  updated_at: "2026-11-26T00:00:00Z",
};

const otherAvailableAgent: AvailableAgent = {
  ...openCodeAvailableAgent,
  name: "codex",
  display_name: "OpenAI Codex CLI",
  install_script: "npm install -g @openai/codex",
};

function discoveryAgent(name: string, available: boolean): AgentDiscovery {
  return {
    name,
    supports_mcp: true,
    mcp_config_path: null,
    installation_paths: [],
    available,
    matched_path: available ? "/usr/local/bin/opencode" : null,
  };
}

function openCodeRuntimeStatus(runtimeSource: "managed" | "native"): AgentUpdateStatus {
  const managed = runtimeSource === "managed";
  return {
    update_mode: "pinned",
    managed_fallback: false,
    display_name: "OpenCode",
    runtime_id: managed ? "npm:@opencode/cli" : "native:opencode",
    owner: managed ? "kandev" : "external",
    mechanism: managed ? "npm_candidate" : "native",
    management: managed ? "managed" : "manual",
    source: runtimeSource,
    runtime_source: runtimeSource,
    available: false,
    enabled: true,
    auto_update_supported: managed,
    auto_update: false,
    agent_name: OPENCODE_AGENT_NAME,
    package: "@opencode/cli",
    default_version: "2.3.2",
    effective_version: "2.3.2",
    check_state: "unknown",
  };
}

function availableAgents(installed: boolean): ListAvailableAgentsResponse {
  const opencode: AvailableAgent = {
    ...openCodeAvailableAgent,
    available: installed,
    ...(installed
      ? {
          runtime_update: {
            supported: true,
            update_mode: "pinned",
            package: "@opencode/cli",
            default_version: "2.3.2",
            effective_version: "2.3.2",
          },
        }
      : {}),
  };
  return {
    agents: [otherAvailableAgent, opencode],
    tools: [],
    total: 2,
  };
}

function discoveryAgents(installed: boolean): AgentDiscovery[] {
  return [discoveryAgent(OPENCODE_AGENT_NAME, installed), discoveryAgent("codex", false)];
}

function savedOpenCodeAgent(): Agent {
  const agentId = "e2e-managed-opencode-agent";
  return {
    id: agentId,
    name: OPENCODE_AGENT_NAME,
    profiles: [
      {
        id: "e2e-managed-opencode-profile",
        name: "Saved OpenCode profile",
        agentId,
        agentDisplayName: "OpenCode",
        model: "fixture-model",
        allowIndexing: false,
        autoApprove: false,
        cliFlags: [],
        cliPassthrough: false,
        createdAt: FIXTURE_TIME,
        updatedAt: FIXTURE_TIME,
      },
    ],
    supports_mcp: true,
    created_at: FIXTURE_TIME,
    updated_at: FIXTURE_TIME,
  };
}

export async function installOpenCodeReinstallFixture(
  page: Page,
  options: { runtimeSource?: "managed" | "native"; failFirstAttempt?: boolean } = {},
) {
  const runtimeSource = options.runtimeSource ?? "managed";
  let installRequests = 0;
  let profileMutationRequests = 0;
  let installed = false;

  page.on("request", (request) => {
    if (!["POST", "PUT", "PATCH", "DELETE"].includes(request.method())) return;
    const { pathname } = new URL(request.url());
    if (
      pathname.startsWith("/api/v1/agent-profiles/") ||
      /^\/api\/v1\/agents\/[^/]+\/profiles(?:\/[^/]+)?$/.test(pathname)
    ) {
      profileMutationRequests += 1;
    }
  });

  await page.route("**/api/v1/agent-update/status**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ statuses: [openCodeRuntimeStatus(runtimeSource)] }),
    }),
  );
  await page.route("**/api/v1/agent-install/jobs**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ jobs: [] }),
    }),
  );
  await page.route("**/api/v1/agents/discovery**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ agents: discoveryAgents(installed), total: 2 }),
    }),
  );
  await page.route("**/api/v1/agents/available**", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(availableAgents(installed)),
    }),
  );
  await page.route("**/api/v1/agents", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ agents: [savedOpenCodeAgent()], total: 1 }),
    }),
  );
  await page.route("**/api/v1/agent-install/opencode-acp**", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    installRequests += 1;
    const failed = options.failFirstAttempt && installRequests === 1;
    installed = !failed;
    const job = {
      job_id: `e2e-opencode-install-${installRequests}`,
      agent_name: OPENCODE_AGENT_NAME,
      status: failed ? "failed" : "succeeded",
      output: failed ? "npm ERR! EAI_AGAIN registry.npmjs.org" : "Installed @opencode/cli@2.3.2",
      ...(failed ? { error: "Temporary registry failure" } : {}),
      started_at: FIXTURE_TIME,
      finished_at: "2026-11-26T00:00:01Z",
    };
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(job),
    });
  });

  return {
    installRequests: () => installRequests,
    profileMutationRequests: () => profileMutationRequests,
  };
}
