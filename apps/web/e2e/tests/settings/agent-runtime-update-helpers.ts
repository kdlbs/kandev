import { expect, type Page, type Route, type WebSocketRoute } from "@playwright/test";
import { injectLatency } from "../../helpers/causal-waits";

const AGENT_NAME = "claude-acp";
const NOW = "2026-07-26T12:00:00.000Z";

type UpdateStatus = "queued" | "resolving" | "updating" | "refreshing" | "succeeded" | "failed";
type UpdateOperation = "update" | "rollback" | "repair" | "up_to_date" | "use_default";

type UpdateJob = {
  job_id: string;
  agent_name: string;
  status: UpdateStatus;
  operation?: UpdateOperation;
  active_version?: string;
  current_version?: string;
  default_version?: string;
  effective_version?: string;
  target_version?: string;
  output?: string;
  error?: string;
  refresh_error?: string;
  started_at: string;
  finished_at?: string;
};

type UpdatePreview = {
  agent_name: string;
  package: string;
  current_version: string;
  default_version?: string;
  active_version?: string;
  effective_version?: string;
  target_version: string;
  operation?: UpdateOperation;
  available_versions?: Array<{ version: string; latest: boolean }>;
  command: string[];
  command_string: string;
};

export type RuntimeUpdateStatus = {
  agent_name: string;
  package: string;
  default_version: string;
  active_version?: string;
  effective_version: string;
  latest_version?: string;
  checked_at?: string;
  check_state: "update_available" | "up_to_date" | "unknown";
};

function catalogue(
  models: Array<{ id: string; name: string }>,
  displayName = "Claude",
  runtimeVersion = "0.62.0",
) {
  return {
    agents: [
      {
        name: AGENT_NAME,
        display_name: displayName,
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
          default_model: models[0]?.id ?? "",
          current_model_id: models[0]?.id,
          available_models: models.map((model, index) => ({
            ...model,
            description: `${model.name} model`,
            is_default: index === 0,
            source: "dynamic",
          })),
          supports_dynamic_models: true,
          status: "ok",
        },
        runtime_update: {
          supported: true,
          package: "@agentclientprotocol/claude-agent-acp",
          current_version: runtimeVersion,
          default_version: "0.64.0",
          active_version: runtimeVersion,
          effective_version: runtimeVersion,
        },
        updated_at: NOW,
      },
    ],
    tools: [],
    total: 1,
  };
}

function discovery() {
  return {
    agents: [
      {
        name: AGENT_NAME,
        supports_mcp: true,
        installation_paths: ["claude-agent-acp"],
        available: true,
        matched_path: "/usr/local/bin/claude-agent-acp",
      },
    ],
    total: 1,
  };
}

function savedAgents() {
  return {
    agents: [],
    total: 0,
  };
}

function event(action: string, payload: unknown) {
  return JSON.stringify({
    id: `runtime-update-${action}`,
    type: "notification",
    action,
    payload,
  });
}

function compareStableVersions(left: string, right: string) {
  const leftParts = left.split(".").map(Number);
  const rightParts = right.split(".").map(Number);
  for (let index = 0; index < 3; index += 1) {
    if (leftParts[index] !== rightParts[index])
      return leftParts[index] > rightParts[index] ? 1 : -1;
  }
  return 0;
}

function operationForTarget(currentVersion: string, targetVersion: string): UpdateOperation {
  const comparison = compareStableVersions(targetVersion, currentVersion);
  if (comparison === 0) return "up_to_date";
  return comparison > 0 ? "update" : "rollback";
}

function previewForRequest(
  previewResponse: UpdatePreview,
  requestedTarget: string | null,
  useDefault: boolean,
): UpdatePreview {
  if (useDefault) {
    const defaultVersion = previewResponse.default_version ?? "0.64.0";
    return {
      ...previewResponse,
      target_version: defaultVersion,
      operation: "use_default",
    };
  }
  if (!requestedTarget) return previewResponse;
  return {
    ...previewResponse,
    target_version: requestedTarget,
    operation: operationForTarget(previewResponse.current_version, requestedTarget),
  };
}

async function handlePreviewRoute({
  route,
  url,
  previewResponse,
  previewFailures,
  previewDelayMs,
}: {
  route: Route;
  url: URL;
  previewResponse: UpdatePreview;
  previewFailures: string[];
  previewDelayMs: number;
}): Promise<void> {
  const requestedTarget = url.searchParams.get("target_version");
  const useDefault = url.searchParams.get("use_default") === "true";
  if (requestedTarget && previewFailures.includes(requestedTarget)) {
    previewFailures.splice(previewFailures.indexOf(requestedTarget), 1);
    await route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify({ error: "preview temporarily unavailable" }),
    });
    return;
  }
  if (requestedTarget && previewDelayMs > 0) {
    await injectLatency(
      previewDelayMs,
      "simulates a slow agent-update preview so the in-flight preview state stays observable",
    );
  }
  const response = previewForRequest(previewResponse, requestedTarget, useDefault);
  await route.fulfill({
    status: 200,
    contentType: "application/json",
    body: JSON.stringify(response),
  });
}

export type RuntimeUpdateFixtureOptions = {
  retainedJobs?: UpdateJob[];
  postResponse?: UpdateJob;
  previewResponse?: UpdatePreview;
  previewFailures?: string[];
  previewDelayMs?: number;
  statusResponse?: RuntimeUpdateStatus[];
};

export async function installRuntimeUpdateFixture(
  page: Page,
  options: RuntimeUpdateFixtureOptions = {},
) {
  let currentModels = [{ id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6" }];
  let persistedRuntimeVersion = "0.62.0";
  let retainedJobs = options.retainedJobs ?? [];
  let postResponse: UpdateJob =
    options.postResponse ??
    ({
      job_id: "runtime-update-job-1",
      agent_name: AGENT_NAME,
      status: "resolving",
      current_version: "0.62.0",
      started_at: NOW,
    } satisfies UpdateJob);
  let postCount = 0;
  let previewCount = 0;
  let previewDefaultCount = 0;
  let statusRequestCount = 0;
  const previewTargets: string[] = [];
  const postTargets: string[] = [];
  const previewFailures = [...(options.previewFailures ?? [])];
  const previewDelayMs = options.previewDelayMs ?? 0;
  let statusResponse = options.statusResponse ?? [
    {
      agent_name: AGENT_NAME,
      package: "@agentclientprotocol/claude-agent-acp",
      default_version: "0.64.0",
      active_version: "0.62.0",
      effective_version: "0.62.0",
      latest_version: "0.64.0",
      checked_at: NOW,
      check_state: "update_available",
    },
  ];
  let previewResponse: UpdatePreview =
    options.previewResponse ??
    ({
      agent_name: AGENT_NAME,
      package: "@agentclientprotocol/claude-agent-acp",
      current_version: "0.62.0",
      target_version: "0.63.0",
      operation: "update",
      default_version: "0.64.0",
      active_version: "0.62.0",
      effective_version: "0.62.0",
      available_versions: [
        { version: "0.64.0", latest: true },
        { version: "0.63.0", latest: false },
        { version: "0.62.0", latest: false },
        { version: "0.61.0", latest: false },
      ],
      command: [
        "npm",
        "exec",
        "--yes",
        "--prefer-online",
        "--package=@agentclientprotocol/claude-agent-acp",
        "--",
        "node",
        "-e",
        "",
      ],
      command_string:
        'npm exec --yes --prefer-online --package=@agentclientprotocol/claude-agent-acp -- node -e ""',
    } satisfies UpdatePreview);
  let socket: WebSocketRoute | undefined;
  let clientReady = false;

  await page.route("**/api/v1/agents/discovery", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(discovery()),
    }),
  );
  await page.route("**/api/v1/agents/available", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(catalogue(currentModels, "Claude", persistedRuntimeVersion)),
    }),
  );
  await page.route("**/api/v1/agents", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(savedAgents()),
    }),
  );
  await page.route("**/api/v1/agent-update/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (
      request.method() === "GET" &&
      url.pathname.endsWith(`/agent-update/${AGENT_NAME}/preview`)
    ) {
      previewCount += 1;
      const requestedTarget = url.searchParams.get("target_version");
      previewTargets.push(requestedTarget ?? "");
      if (url.searchParams.get("use_default") === "true") previewDefaultCount += 1;
      return handlePreviewRoute({
        route,
        url,
        previewResponse,
        previewFailures,
        previewDelayMs,
      });
    }
    if (request.method() === "GET" && url.pathname.endsWith("/agent-update/status")) {
      statusRequestCount += 1;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ statuses: statusResponse }),
      });
    }
    if (request.method() === "GET" && url.pathname.endsWith("/agent-update/jobs")) {
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ jobs: retainedJobs }),
      });
    }
    if (request.method() === "POST" && url.pathname.endsWith(`/agent-update/${AGENT_NAME}`)) {
      postCount += 1;
      const body = request.postDataJSON() as {
        target_version?: string;
        use_default?: boolean;
      } | null;
      postTargets.push(body?.use_default ? "__kandev_default__" : (body?.target_version ?? ""));
      return route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify(postResponse),
      });
    }
    return route.fallback();
  });
  await page.route(`**/api/v1/agent-models/${AGENT_NAME}**`, (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        agent_name: AGENT_NAME,
        status: "ok",
        models: currentModels.map((model, index) => ({
          ...model,
          description: `${model.name} model`,
          is_default: index === 0,
          source: "dynamic",
        })),
        current_model_id: currentModels[0]?.id,
        modes: [],
        commands: [],
        error: null,
      }),
    }),
  );
  await page.routeWebSocket(/\/ws$/, (ws) => {
    socket = ws;
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      clientReady = true;
      server.send(message);
    });
    server.onMessage((message) => ws.send(message));
  });

  return {
    agentName: AGENT_NAME,
    setRetainedJobs(jobs: UpdateJob[]) {
      retainedJobs = jobs;
    },
    setPostResponse(job: UpdateJob) {
      postResponse = job;
    },
    setPreviewResponse(preview: UpdatePreview) {
      previewResponse = preview;
    },
    setPersistedRuntimeVersion(version: string) {
      persistedRuntimeVersion = version;
      previewResponse = {
        ...previewResponse,
        current_version: version,
        active_version: version,
        effective_version: version,
        target_version: version,
        operation: "up_to_date",
      };
      statusResponse = statusResponse.map((status) =>
        status.agent_name === AGENT_NAME
          ? {
              ...status,
              active_version: version,
              effective_version: version,
              check_state: "up_to_date" as const,
            }
          : status,
      );
    },
    setStatusResponse(statuses: RuntimeUpdateStatus[]) {
      statusResponse = statuses;
    },
    postCount: () => postCount,
    previewCount: () => previewCount,
    previewDefaultCount: () => previewDefaultCount,
    statusRequestCount: () => statusRequestCount,
    previewTargets: () => [...previewTargets],
    postTargets: () => [...postTargets],
    async emit(action: string, payload: unknown) {
      await expect.poll(() => Boolean(socket)).toBe(true);
      await expect.poll(() => clientReady).toBe(true);
      socket?.send(event(action, payload));
    },
    async emitUpdate(job: UpdateJob) {
      const action =
        job.status === "succeeded" || job.status === "failed"
          ? "agent.update.finished"
          : "agent.update.started";
      await this.emit(action, job);
    },
    async emitOutput(chunk: string) {
      await this.emit("agent.update.output", {
        job_id: "runtime-update-job-1",
        agent_name: AGENT_NAME,
        chunk,
      });
    },
    async emitCatalogue(models: Array<{ id: string; name: string }>) {
      currentModels = models;
      await this.emit(
        "agent.available.updated",
        catalogue(models, "Claude refreshed", persistedRuntimeVersion),
      );
    },
  };
}

export function updateJob(overrides: Partial<UpdateJob> = {}): UpdateJob {
  return {
    job_id: "runtime-update-job-1",
    agent_name: AGENT_NAME,
    status: "updating",
    current_version: "0.62.0",
    target_version: "0.63.0",
    operation: "update",
    output: "Downloading @agentclientprotocol/claude-agent-acp…\n",
    started_at: NOW,
    ...overrides,
  };
}
