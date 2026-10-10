import fs from "node:fs";
import path from "node:path";
import {
  expect,
  type Locator,
  type Page,
  type Request,
  type WebSocketRoute,
} from "@playwright/test";
import type { AgentProfile } from "../../lib/types/http";
import type { DynamicModelsResponse } from "../../lib/types/http";
import type { ProfileRuntimeComponent } from "../../lib/types/http-agents";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import {
  installRuntimeUpdateFixture,
  updateJob,
} from "../tests/settings/agent-runtime-update-helpers";

export type ProfileDiscoveryRequest = {
  url: string;
  body: Record<string, unknown>;
  responseStatus?: number;
  failure?: string;
};

export type ProfileProbeEvidence = {
  wrapper: boolean;
  command?: string;
  env_catalog?: string;
  cli_catalog?: string;
  profile_args?: string[];
};

async function expectFailedUpdateRetry(surface: Locator, agentName: string) {
  const retry = surface.getByTestId(`agent-update-confirm-${agentName}`);
  await expect(retry).toHaveText("Retry update");
  await expect(retry).toBeEnabled();
}

async function emitRuntimeUpdateOutcome(
  runtime: Awaited<ReturnType<typeof installRuntimeUpdateFixture>>,
  outcome: "succeeded" | "failed",
) {
  if (outcome === "succeeded") runtime.setPersistedRuntimeVersion("0.63.0");
  const result =
    outcome === "succeeded"
      ? { output: "Installed runtime 0.63.0\n" }
      : { error: "Candidate ACP probe failed." };
  await runtime.emitUpdate(
    updateJob({
      agent_name: runtime.agentName,
      status: outcome,
      ...result,
      finished_at: "2026-10-04T12:01:00.000Z",
    }),
  );
}

async function expectRuntimeUpdateOutcome(
  surface: Locator,
  agentName: string,
  outcome: "succeeded" | "failed",
) {
  const result = surface.getByTestId(`agent-update-result-${agentName}`);
  const expectedMessage =
    outcome === "succeeded" ? "Runtime updated successfully" : "Candidate ACP probe failed.";
  await expect(result).toContainText(expectedMessage);
  if (outcome === "failed") await expectFailedUpdateRetry(surface, agentName);
}

export async function installProfileRuntimeObservationFixture(
  page: Page,
  options: { nativeBridge?: boolean; unknownManagedFallback?: boolean } = {},
) {
  let observedVersion = "1.11.0";
  let configuredVersion = "1.10.0";
  let activationModel: { id: string; name: string } | null = null;
  let socket: WebSocketRoute | undefined;
  let clientReady = false;

  function bridgeRuntimeComponent(): ProfileRuntimeComponent {
    if (options.nativeBridge) {
      return {
        role: "bridge",
        name: "OpenCode",
        source: "external",
        owner: "external",
        observed_version: observedVersion,
        guidance_url: "https://opencode.ai/docs/cli/",
      };
    }
    if (options.unknownManagedFallback) {
      return {
        role: "bridge",
        name: "Mock ACP bridge",
        package: "@agentclientprotocol/mock-agent-acp",
        source: "unknown",
        owner: "kandev",
        effective_version: configuredVersion,
      };
    }
    return {
      role: "bridge",
      name: "Mock ACP bridge",
      package: "@agentclientprotocol/mock-agent-acp",
      source: "managed",
      owner: "kandev",
      effective_version: configuredVersion,
      observed_version: observedVersion,
    };
  }

  await page.route("**/api/v1/agent-models/mock-agent/probe", async (route) => {
    const models: DynamicModelsResponse["models"] = [
      { id: "mock-fast", name: "Mock Fast", is_default: true, source: "dynamic" },
      { id: "profile-env-runtime-initial", name: "Profile env runtime-initial", source: "dynamic" },
    ];
    if (activationModel && !models.some((model) => model.id === activationModel?.id)) {
      models.push(activationModel);
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      json: {
        agent_name: "mock-agent",
        status: "ok",
        models,
        current_model_id: "mock-fast",
        modes: [],
        commands: [],
        error: null,
        runtime_info: {
          scope: "host",
          observed_at: "2026-10-04T12:00:00.000Z",
          components: [
            bridgeRuntimeComponent(),
            {
              role: "provider",
              name: "Codex CLI",
              package: "@openai/codex",
              source: "external",
              owner: "external",
              observed_version: "0.153.4",
              guidance_url: "https://github.com/openai/codex",
            },
          ],
        },
        context_revision: `profile-runtime:${observedVersion}`,
      },
    });
  });
  await page.route("**/api/v1/agent-models/mock-agent/resolve", async (route) => {
    const request = route.request().postDataJSON() as { model?: string } | null;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      json: {
        agent_name: "mock-agent",
        model: request?.model ?? "",
        status: "ok",
        config_options: [],
        error: null,
        context_revision: `profile-runtime:${observedVersion}`,
      },
    });
  });
  await page.route("**/api/v1/agent-update/status", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        statuses: [
          {
            agent_name: "mock-agent",
            display_name: "Mock agent",
            runtime_id: "managed:mock-agent",
            owner: "kandev",
            mechanism: "npm",
            management: "managed",
            available: true,
            enabled: true,
            auto_update_supported: false,
            auto_update: false,
            package: "@agentclientprotocol/mock-agent-acp",
            default_version: "1.12.0",
            active_version: configuredVersion,
            effective_version: configuredVersion,
            latest_version: statusCheckState() === "up_to_date" ? configuredVersion : "1.12.0",
            checked_at: "2026-10-04T12:00:00.000Z",
            check_state: statusCheckState(),
          },
        ],
      }),
    }),
  );
  await page.routeWebSocket(/\/ws$/, (webSocket) => {
    socket = webSocket;
    const server = webSocket.connectToServer();
    webSocket.onMessage((message) => {
      clientReady = true;
      server.send(message);
    });
    server.onMessage((message) => webSocket.send(message));
  });

  function statusCheckState(): "update_available" | "up_to_date" {
    return configuredVersion === observedVersion ? "up_to_date" : "update_available";
  }

  return {
    activate(version: string, model: { id: string; name: string }) {
      observedVersion = version;
      configuredVersion = version;
      activationModel = model;
    },
    async emit(action: string, payload: unknown) {
      await expect.poll(() => Boolean(socket)).toBe(true);
      await expect.poll(() => clientReady).toBe(true);
      socket?.send(
        JSON.stringify({
          id: `profile-runtime-${action}-${Date.now()}`,
          type: "notification",
          action,
          payload,
        }),
      );
    },
  };
}

export async function approveRuntimeUpdateInOtherTab(
  profilePage: Page,
  profileRuntime: Awaited<ReturnType<typeof installProfileRuntimeObservationFixture>>,
  outcome: "succeeded" | "failed" = "succeeded",
) {
  const updatePage = await profilePage.context().newPage();
  try {
    const runtime = await installRuntimeUpdateFixture(updatePage, {
      agentName: "mock-agent",
      broadcast: async (action, payload) => {
        const job = payload as { status?: string };
        if (action === "agent.update.finished" && job.status === "succeeded") {
          profileRuntime.activate("0.63.0", {
            id: "profile-env-after-update",
            name: "Profile env after update",
          });
        }
        await profileRuntime.emit(action, payload);
      },
    });

    await updatePage.goto("/settings/agents");
    const trigger = updatePage.getByTestId(`agent-update-trigger-${runtime.agentName}`);
    await expect(trigger).toBeVisible();
    if ((updatePage.viewportSize()?.width ?? 768) < 768) await trigger.tap();
    else await trigger.click();

    const surfaceId =
      (updatePage.viewportSize()?.width ?? 768) < 768
        ? `agent-update-drawer-${runtime.agentName}`
        : `agent-update-dialog-${runtime.agentName}`;
    const surface = updatePage.getByTestId(surfaceId);
    await expect(surface).toBeVisible();
    const confirm = surface.getByTestId(`agent-update-confirm-${runtime.agentName}`);
    if ((updatePage.viewportSize()?.width ?? 768) < 768) await confirm.tap();
    else await confirm.click();

    await emitRuntimeUpdateOutcome(runtime, outcome);
    await expectRuntimeUpdateOutcome(surface, runtime.agentName, outcome);
    return { page: updatePage, runtime };
  } catch (error) {
    await updatePage.close();
    throw error;
  }
}

export function observeProfileDiscoveryRequests(page: Page): ProfileDiscoveryRequest[] {
  const requests: ProfileDiscoveryRequest[] = [];
  const observedByRequest = new Map<Request, ProfileDiscoveryRequest>();
  page.on("request", (request) => {
    if (!request.url().includes("/api/v1/agent-models/mock-agent/")) return;
    const postData = request.postData();
    if (!postData) return;
    try {
      const body = JSON.parse(postData) as Record<string, unknown>;
      const observed = { url: request.url(), body };
      requests.push(observed);
      observedByRequest.set(request, observed);
    } catch {
      // Other request payloads do not contribute to the probe evidence.
    }
  });
  page.on("response", (response) => {
    const observed = observedByRequest.get(response.request());
    if (!observed) return;
    observed.responseStatus = response.status();
  });
  page.on("requestfailed", (request) => {
    const observed = observedByRequest.get(request);
    if (observed) observed.failure = request.failure()?.errorText ?? "request failed";
  });
  return requests;
}

export async function mockProfileProbeRequiresAuth(
  page: Page,
  profileId: string,
): Promise<() => boolean> {
  let matched = false;
  await page.route("**/api/v1/agent-models/mock-agent/probe", async (route) => {
    const body = route.request().postDataJSON() as { profile_id?: string } | null;
    if (body?.profile_id !== profileId) {
      await route.continue();
      return;
    }
    matched = true;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: "mock-agent",
        status: "auth_required",
        models: [],
        modes: [],
        commands: [],
        error: "Agent authentication is required.",
      }),
    });
  });
  return () => matched;
}

export async function createProfileWithCatalog(
  apiClient: ApiClient,
  backend: BackendContext,
  agentId: string,
  name: string,
  catalog: string,
): Promise<{ profile: AgentProfile; evidencePath: string }> {
  const evidencePath = path.join(
    backend.tmpDir,
    `profile-discovery-${name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}.jsonl`,
  );
  fs.rmSync(evidencePath, { force: true });
  const profile = await apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    env_vars: [
      { key: "MOCK_AGENT_PROFILE_CATALOG", value: catalog },
      { key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath },
    ],
    cli_flags: [
      {
        description: "Profile catalog fixture",
        flag: `--profile-catalog=${catalog}`,
        enabled: true,
      },
    ],
    command_prefix: "mock-agent --profile-probe-wrapper",
  });
  return { profile, evidencePath };
}

export function readProfileProbeEvidence(pathname: string): ProfileProbeEvidence[] {
  if (!fs.existsSync(pathname)) return [];
  return fs
    .readFileSync(pathname, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ProfileProbeEvidence);
}
