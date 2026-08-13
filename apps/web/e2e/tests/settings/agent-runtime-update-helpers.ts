import { expect, type Page, type WebSocketRoute } from "@playwright/test";

const AGENT_NAME = "claude-acp";
const NOW = "2026-07-26T12:00:00.000Z";

type UpdateStatus = "queued" | "resolving" | "updating" | "refreshing" | "succeeded" | "failed";
type UpdateOperation = "update" | "rollback" | "repair" | "up_to_date";

type UpdateJob = {
  job_id: string;
  agent_name: string;
  status: UpdateStatus;
  operation?: UpdateOperation;
  active_version?: string;
  current_version?: string;
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
  active_version?: string;
  target_version: string;
  operation?: UpdateOperation;
  available_versions?: Array<{ version: string; latest: boolean }>;
  command: string[];
  command_string: string;
};

function catalogue(models: Array<{ id: string; name: string }>, displayName = "Claude") {
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
          current_version: "0.62.0",
          active_version: "0.62.0",
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

export type RuntimeUpdateFixtureOptions = {
  retainedJobs?: UpdateJob[];
  postResponse?: UpdateJob;
  previewResponse?: UpdatePreview;
  previewFailures?: string[];
  previewDelayMs?: number;
};

export async function installRuntimeUpdateFixture(
  page: Page,
  options: RuntimeUpdateFixtureOptions = {},
) {
  let currentModels = [{ id: "claude-sonnet-4-6", name: "Claude Sonnet 4.6" }];
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
  const previewTargets: string[] = [];
  const postTargets: string[] = [];
  const previewFailures = [...(options.previewFailures ?? [])];
  const previewDelayMs = options.previewDelayMs ?? 0;
  let previewResponse: UpdatePreview =
    options.previewResponse ??
    ({
      agent_name: AGENT_NAME,
      package: "@agentclientprotocol/claude-agent-acp",
      current_version: "0.62.0",
      target_version: "0.63.0",
      operation: "update",
      active_version: "0.62.0",
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
      body: JSON.stringify(catalogue(currentModels)),
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
      previewTargets.push(url.searchParams.get("target_version") ?? "");
      const requestedTarget = url.searchParams.get("target_version");
      if (requestedTarget && previewFailures.includes(requestedTarget)) {
        previewFailures.splice(previewFailures.indexOf(requestedTarget), 1);
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "preview temporarily unavailable" }),
        });
      }
      if (requestedTarget && previewDelayMs > 0) {
        await new Promise((resolve) => setTimeout(resolve, previewDelayMs));
      }
      const response = requestedTarget
        ? {
            ...previewResponse,
            target_version: requestedTarget,
            operation: operationForTarget(previewResponse.current_version, requestedTarget),
          }
        : previewResponse;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(response),
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
      const body = request.postDataJSON() as { target_version?: string } | null;
      postTargets.push(body?.target_version ?? "");
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
    postCount: () => postCount,
    previewCount: () => previewCount,
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
      await this.emit("agent.available.updated", catalogue(models, "Claude refreshed"));
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
