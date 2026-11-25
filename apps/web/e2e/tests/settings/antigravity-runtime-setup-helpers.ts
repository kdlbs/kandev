import { expect, type Page } from "@playwright/test";
import type {
  ListAgentDiscoveryResponse,
  ListAvailableAgentsResponse,
} from "../../../lib/types/http";
import { serializeInlineScriptJSON } from "../../helpers/inline-script-json";

const AGENT_NAME = "antigravity-acp";
const LOGIN_COMMAND = {
  cmd: ["/opt/antigravity cli/agy", "acp", "--login"],
  variants: {
    "antigravity-cli": ["/opt/antigravity cli/agy", "acp", "--login"],
  },
};

export async function installAntigravityRuntimeFixture(
  page: Page,
  runtimeVariant: "cli" | "standalone",
) {
  let signedIn = false;
  let modelRefreshes = 0;
  let loginVariant: string | undefined;
  let loginStarts = 0;
  let consentPolls = 0;

  const discovery: ListAgentDiscoveryResponse = {
    agents: [
      {
        name: AGENT_NAME,
        supports_mcp: true,
        mcp_config_path: null,
        installation_paths: ["/opt/antigravity"],
        available: true,
        matched_path: "/opt/antigravity/agy_acp_server.par",
      },
    ],
    total: 1,
  };
  const available = (): ListAvailableAgentsResponse => ({
    agents: [
      {
        name: AGENT_NAME,
        display_name: "Antigravity",
        supports_mcp: true,
        mcp_config_path: null,
        installation_paths: ["/opt/antigravity"],
        available: true,
        matched_path: "/opt/antigravity/agy_acp_server.par",
        capabilities: {
          supports_session_resume: true,
          supports_shell: false,
          supports_workspace_only: false,
        },
        model_config: {
          default_model: "",
          runtime_variant: runtimeVariant,
          available_models: signedIn
            ? [{ id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", is_default: true }]
            : [],
          available_modes: [],
          supports_dynamic_models: false,
          status: signedIn ? "ok" : "auth_required",
          error: "",
        },
        login_command: LOGIN_COMMAND,
        updated_at: "2026-11-24T00:00:00Z",
      },
    ],
    tools: [],
    total: 1,
  });

  await page.route("**/settings/agents", async (route) => {
    const response = await route.fetch();
    const html = await response.text();
    const assignment = "window.__KANDEV_BOOT_PAYLOAD__=";
    const payloadStart = html.indexOf(assignment);
    expect(payloadStart).toBeGreaterThanOrEqual(0);
    const jsonStart = payloadStart + assignment.length;
    const scriptEnd = html.indexOf(";</script>", jsonStart);
    expect(scriptEnd).toBeGreaterThan(jsonStart);
    const payload = JSON.parse(html.slice(jsonStart, scriptEnd)) as {
      initialState?: Record<string, unknown>;
    };
    const seeded = {
      ...payload,
      initialState: {
        ...payload.initialState,
        agentDiscovery: { items: discovery.agents, loading: false, loaded: true },
        availableAgents: { items: available().agents, tools: [], loading: false, loaded: true },
      },
    };
    await route.fulfill({
      response,
      body: `${html.slice(0, jsonStart)}${serializeInlineScriptJSON(seeded)}${html.slice(scriptEnd)}`,
    });
  });

  await page.route("**/api/v1/agents/available**", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(available()),
    }),
  );
  await page.route("**/api/v1/agents/discovery**", (route) =>
    route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(discovery),
    }),
  );
  await page.route(`**/api/v1/agent-login/agents/${AGENT_NAME}/start`, async (route) => {
    loginStarts += 1;
    loginVariant = (route.request().postDataJSON() as { command_variant?: string }).command_variant;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        session_id: "antigravity-e2e-login",
        agent_id: AGENT_NAME,
        cmd: LOGIN_COMMAND.cmd,
        running: true,
        started_at: "2026-11-24T00:00:00Z",
      }),
    });
  });
  await page.route("**/api/v1/agent-login/sessions/antigravity-e2e-login/**", (route) =>
    route.fulfill({ contentType: "application/json", body: JSON.stringify({ ok: true }) }),
  );
  await page.routeWebSocket(
    /\/api\/v1\/agent-login\/sessions\/antigravity-e2e-login\/stream$/,
    (socket) => {
      socket.onMessage((message) => socket.send(message));
    },
  );
  await page.route(`**/api/v1/agent-models/${AGENT_NAME}**`, async (route) => {
    signedIn = true;
    modelRefreshes += 1;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: AGENT_NAME,
        status: "ok",
        models: [{ id: "gemini-2.5-pro", name: "Gemini 2.5 Pro", is_default: true }],
        modes: [],
        commands: [],
      }),
    });
  });
  await page.route(`**/api/v1/antigravity-acp/login`, async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          attempt_id: "antigravity-e2e-consent",
          state: "waiting_for_browser",
          expires_at: "2026-11-25T00:00:00Z",
          authorization_url: "https://accounts.google.com/example",
        }),
      });
      return;
    }
    await route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "no active login" }),
    });
  });
  await page.route(`**/api/v1/antigravity-acp/login/antigravity-e2e-consent`, async (route) => {
    consentPolls += 1;
    if (consentPolls >= 1) signedIn = true;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify({
        attempt_id: "antigravity-e2e-consent",
        state: "succeeded",
        expires_at: "2026-11-25T00:00:00Z",
      }),
    });
  });

  return {
    loginVariant: () => loginVariant,
    loginStarts: () => loginStarts,
    modelRefreshes: () => modelRefreshes,
    consentPolls: () => consentPolls,
  };
}
