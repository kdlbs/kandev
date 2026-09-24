import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionAgentctlReady } from "../../helpers/session-store";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export class CanvasFixtureUnavailable extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CanvasFixtureUnavailable";
  }
}

export type CanvasRecord = {
  id: string;
  title: string;
  workspace_id: string;
  task_id?: string;
  scope_kind?: string;
  data_scope_kind?: string;
  grant_generation?: number;
  status?: string;
  active_release_id?: string;
  active_release_status?: string;
  pending_release?: {
    id: string;
    validation_status?: string;
  };
};

export type CanvasReleaseRecord = {
  id: string;
  validation_status?: string;
  permissions?: {
    reads?: string[];
    writes?: string[];
    events?: string[];
    shared_state?: boolean;
  };
};

export type SeededCanvas = {
  taskId: string;
  taskSessionId: string;
  canvas: CanvasRecord;
  session: SessionPage;
};

export async function seedCanvasWorkspacePreview(apiClient: ApiClient, seedData: SeedData) {
  const previewTask = await apiClient.seedTask(
    seedData.workspaceId,
    "E2E Canvas Workspace Preview Task",
    { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId },
  );
  const foreignWorkspaceName = "E2E Canvas Foreign Workspace";
  try {
    const foreignWorkspace = await apiClient.createWorkspace(foreignWorkspaceName);
    return {
      workspaceTaskId: previewTask.task_id,
      foreignWorkspaceId: foreignWorkspace.id,
      cleanup: async () => {
        await apiClient.deleteTask(previewTask.task_id).catch(() => undefined);
        await apiClient
          .deleteWorkspace(foreignWorkspace.id, foreignWorkspaceName)
          .catch(() => undefined);
      },
    };
  } catch (error) {
    await apiClient.deleteTask(previewTask.task_id).catch(() => undefined);
    throw error;
  }
}

export async function expectCanvasFrameFillsHost(page: Page): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(() => {
          const host = document.querySelector('[data-testid="canvas-host-route"]');
          const header = document.querySelector('[data-testid="canvas-host-header"]');
          const frame = document.querySelector('[data-testid="web-app-frame"]');
          if (!host || !header || !frame) return false;
          const hostRect = host.getBoundingClientRect();
          const headerRect = header.getBoundingClientRect();
          const frameRect = frame.getBoundingClientRect();
          const headerIsInsideHost = host.contains(header);
          const heightMatches =
            Math.abs(
              frameRect.height - (hostRect.height - (headerIsInsideHost ? headerRect.height : 0)),
            ) <= 2;
          const widthMatches = Math.abs(frameRect.width - hostRect.width) <= 2;
          const documentFits = document.documentElement.scrollWidth <= window.innerWidth;
          return heightMatches && widthMatches && documentFits;
        }),
      {
        timeout: 5_000,
        message: "The canvas iframe host did not settle to the available viewport.",
      },
    )
    .toBe(true);
}

export async function readCanvasFeature(apiClient: ApiClient): Promise<boolean | null> {
  const response = await apiClient.rawRequest("GET", "/api/v1/features");
  if (!response.ok) return null;
  const body = (await response.json()) as { canvases?: unknown };
  return typeof body.canvases === "boolean" ? body.canvases : null;
}

export async function enableCanvasFeature(
  backend: BackendContext,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<() => Promise<void>> {
  const release = await backend.useEnv({ KANDEV_FEATURES_CANVASES: "true" });
  const enabled = await readCanvasFeature(apiClient);
  if (enabled !== true) {
    await release();
    throw new CanvasFixtureUnavailable(
      "Canvas fixture skipped: KANDEV_FEATURES_CANVASES could not be enabled.",
    );
  }

  const probe = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/canvases`,
  );
  if (!probe.ok) {
    await release();
    throw new CanvasFixtureUnavailable(
      `Canvas fixture skipped: canvas API is unavailable (${probe.status}).`,
    );
  }
  return release;
}

export async function listTaskCanvases(
  apiClient: ApiClient,
  taskId: string,
): Promise<CanvasRecord[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/tasks/${encodeURIComponent(taskId)}/canvases`,
  );
  if (!response.ok) {
    throw new Error(`Canvas task discovery failed (${response.status}).`);
  }
  const body = (await response.json()) as { canvases?: CanvasRecord[] };
  return body.canvases ?? [];
}

export async function getCanvas(
  apiClient: ApiClient,
  canvasId: string,
): Promise<CanvasRecord | null> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/canvases/${encodeURIComponent(canvasId)}`,
  );
  if (response.status === 404) return null;
  if (!response.ok) {
    throw new Error(`Canvas lookup failed (${response.status}).`);
  }
  return (await response.json()) as CanvasRecord;
}

export async function listCanvasReleases(
  apiClient: ApiClient,
  canvasId: string,
): Promise<CanvasReleaseRecord[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/canvases/${encodeURIComponent(canvasId)}/releases`,
  );
  if (!response.ok) {
    throw new Error(`Canvas release lookup failed (${response.status}).`);
  }
  const body = (await response.json()) as { releases?: CanvasReleaseRecord[] };
  return body.releases ?? [];
}

export async function waitForSessionWorkspace(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  let workspacePath = "";
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((candidate) => candidate.id === sessionId);
        workspacePath =
          session?.workspace_path ??
          session?.worktree_path ??
          session?.worktrees?.find((worktree) => worktree.worktree_path)?.worktree_path ??
          "";
        return workspacePath;
      },
      { timeout: 30_000, message: "The canvas task session did not expose a local workspace." },
    )
    .toMatch(/\S/);
  return path.resolve(workspacePath);
}

export type CanvasSourceOptions = {
  noPermissions?: boolean;
  minimalPermissions?: boolean;
  foreignWorkspaceId?: string;
};

function canvasCapabilities(options?: CanvasSourceOptions): string[] {
  if (options?.noPermissions) return [];
  if (options?.minimalPermissions) return ["  api_read:", "    - tasks", "    - workflows"];
  return [
    "  api_read:",
    "    - tasks",
    "    - workflows",
    "  api_write:",
    "    - tasks",
    "    - messages",
    "  events:",
    "    - task.updated",
    "  state: true",
  ];
}

function canvasManifest(canvas: CanvasRecord, options?: CanvasSourceOptions): string {
  const compactId = canvas.id.replaceAll("-", "").slice(0, 12);
  const capabilities = canvasCapabilities(options);
  return [
    `id: "canvas-${compactId}"`,
    "api_version: 2",
    'version: "1.0.0"',
    'display_name: "E2E Plugin Canvas"',
    'description: "Canvas fixture for Playwright acceptance coverage."',
    'author: "Kandev E2E"',
    'min_kandev_version: "0.94.0"',
    "distribution:",
    "  schema_version: 1",
    '  kind: "canvas"',
    '  license: "MIT"',
    '  source_mode: "static"',
    "ui:",
    "  web_apps:",
    "    - key: main",
    '      title: "E2E Plugin Canvas"',
    "      entry: index.html",
    "      placements:",
    "        - task-canvas",
    "        - workspace-canvas",
    "capabilities:",
    ...capabilities,
    "",
  ].join("\n");
}

function canvasFixtureScript(canvas: CanvasRecord, options?: CanvasSourceOptions): string {
  const taskID = JSON.stringify(canvas.task_id ?? "");
  const foreignWorkspaceID = JSON.stringify(options?.foreignWorkspaceId ?? "");
  return String.raw`(() => {
  const taskId = ${taskID};
  const foreignWorkspaceId = ${foreignWorkspaceID};
  const text = (testId, value) => {
    const element = document.querySelector('[data-testid="' + testId + '"]');
    if (element) element.textContent = String(value);
  };
  const api = async (path, options = {}) => {
    const response = await fetch(path, {
      ...options,
      headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = new Error(body.error || ("HTTP " + response.status));
      error.status = response.status;
      error.body = body;
      throw error;
    }
    return body;
  };
  let task;
  let steps = [];
  let stateRevision = 0;
  let lastEventId = "";
  let streamController;
  let streamReader;
  let streamCompletion = Promise.resolve();

  const renderTasks = (tasks) => {
    const list = document.querySelector('[data-testid="canvas-fixture-task-list"]');
    if (!list) return;
    list.replaceChildren();
    tasks.forEach((candidate) => {
      const item = document.createElement("article");
      item.className = "task-item";
      const title = document.createElement("h2");
      title.textContent = candidate.title || "Workspace task";
      const status = document.createElement("span");
      status.className = "task-status";
      status.textContent = candidate.status || "Open";
      item.append(title, status);
      list.append(item);
    });
  };

  const renderAppearance = () => {
    const root = document.documentElement;
    const styles = getComputedStyle(root);
    text("canvas-fixture-appearance-mode", root.dataset.kandevAppearance || "loading");
    text(
      "canvas-fixture-appearance-background",
      styles.getPropertyValue("--background").trim() || "loading",
    );
    text("canvas-fixture-appearance-color-scheme", root.style.colorScheme || "loading");
  };
  window.addEventListener("message", (event) => {
    if (event.source === window.parent) renderAppearance();
  });

  const loadProjection = async () => {
    const [context, tasks, workflowPage] = await Promise.all([
      api("./_kandev/v1/context"),
      (async () => {
        const items = [];
        let cursor = "";
        do {
          const query = new URLSearchParams({ limit: "1" });
          if (cursor) query.set("cursor", cursor);
          const page = await api("./_kandev/v1/data/tasks?" + query.toString());
          items.push(...(page.items || []));
          cursor = page.page_info?.next_cursor || "";
        } while (cursor);
        return items;
      })(),
      api("./_kandev/v1/data/workflows?limit=10"),
    ]);
    task = tasks.find((candidate) => candidate.id === taskId) || tasks[0];
    const workflow = (workflowPage.items || []).find(
      (candidate) => candidate.id === task?.workflow_id,
    );
    if (workflow) {
      const stepPage = await api(
        "./_kandev/v1/data/workflows/" + encodeURIComponent(workflow.id) + "/steps",
      );
      steps = stepPage.items || [];
    }
    text("canvas-fixture-context", context.task_id || "workspace");
    text("canvas-fixture-task-count", tasks.length);
    text("canvas-fixture-task-ids", tasks.map((candidate) => candidate.id).join(","));
    renderTasks(tasks);
    text("canvas-fixture-workflow-count", workflowPage.items?.length || 0);
    text("canvas-fixture-step-id", task?.workflow_step_id || "");
    text("canvas-fixture-refresh-status", "refreshed");
    if (foreignWorkspaceId) {
      try {
        await api("./_kandev/v1/data/tasks?workspace_id=" + encodeURIComponent(foreignWorkspaceId));
        text("canvas-fixture-foreign-workspace-status", "unexpected-allowed");
      } catch (error) {
        text("canvas-fixture-foreign-workspace-status", "denied:" + error.status);
      }
    }
  };

  const parseEvent = (block) => {
    if (block.trim().startsWith(":")) return;
    let eventType = "message";
    let eventId = "";
    let data = "";
    block.split("\n").forEach((line) => {
      if (line.startsWith("event: ")) eventType = line.slice(7);
      if (line.startsWith("id: ")) eventId = line.slice(4);
      if (line.startsWith("data: ")) data += line.slice(6);
    });
    if (eventId) lastEventId = eventId;
    if (eventType === "runtime.resync_required") {
      text("canvas-fixture-sse-resync", "received");
      void loadProjection();
      return;
    }
    const current = Number(document.querySelector('[data-testid="canvas-fixture-sse-events"]')?.textContent || 0);
    text("canvas-fixture-sse-events", current + 1);
    void loadProjection();
  };

  const connectEvents = async (cursor = "") => {
    if (streamController) streamController.abort();
    await streamCompletion;
    const controller = new AbortController();
    streamController = controller;
    text("canvas-fixture-sse-status", "connecting");
    const connection = (async () => {
      try {
        let response;
        for (let attempt = 0; ; attempt += 1) {
          response = await fetch("./_kandev/v1/events", {
            headers: cursor ? { "Last-Event-ID": cursor } : {},
            signal: controller.signal,
          });
          if (response.status !== 429 || attempt >= 31) break;
          await response.text();
          await new Promise((resolve) => setTimeout(resolve, Math.min(250, 50 * (attempt + 1))));
        }
        if (!response.ok || !response.body) throw new Error("event stream unavailable");
        text("canvas-fixture-sse-status", "connected");
        streamReader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = "";
        for (;;) {
          const result = await streamReader.read();
          if (result.done) break;
          buffer += decoder.decode(result.value, { stream: true });
          let boundary = buffer.indexOf("\n\n");
          while (boundary >= 0) {
            const block = buffer.slice(0, boundary);
            buffer = buffer.slice(boundary + 2);
            if (block.trim()) parseEvent(block);
            boundary = buffer.indexOf("\n\n");
          }
        }
      } catch (error) {
        if (!controller.signal.aborted) text("canvas-fixture-sse-status", "disconnected");
      }
    })();
    streamCompletion = connection;
    await connection;
  };

  document.querySelector('[data-testid="canvas-fixture-continue"]')?.addEventListener("click", async () => {
    text("canvas-fixture-message-status", "sending");
    try {
      await api("./_kandev/v1/data/tasks/" + encodeURIComponent(taskId) + "/messages", {
        method: "POST",
        body: JSON.stringify({ text: "continue" }),
      });
      text("canvas-fixture-message-status", "accepted");
    } catch (error) {
      text("canvas-fixture-message-status", "error:" + error.status);
    }
  });

  document.querySelector('[data-testid="canvas-fixture-move"]')?.addEventListener("click", async () => {
    const next = steps.find((step) => step.id !== task?.workflow_step_id);
    if (!next) {
      text("canvas-fixture-move-status", "no-next-step");
      return;
    }
    try {
      task = await api("./_kandev/v1/data/tasks/" + encodeURIComponent(taskId), {
        method: "PATCH",
        body: JSON.stringify({ workflow_step_id: next.id }),
      });
      text("canvas-fixture-move-status", "moved:" + next.id);
      text("canvas-fixture-step-id", task.workflow_step_id);
    } catch (error) {
      text("canvas-fixture-move-status", "error:" + error.status);
    }
  });

  document.querySelector('[data-testid="canvas-fixture-state"]')?.addEventListener("click", async () => {
    try {
      const first = await api("./_kandev/v1/state/board", {
        method: "PUT",
        headers: { "If-Match": '"0"' },
        body: JSON.stringify({ selected: true }),
      });
      stateRevision = first.revision;
      try {
        await api("./_kandev/v1/state/board", {
          method: "PUT",
          headers: { "If-Match": '"0"' },
          body: JSON.stringify({ selected: false }),
        });
      } catch (error) {
        if (error.status !== 409) throw error;
        const recovered = await api("./_kandev/v1/state/board");
        stateRevision = recovered.revision;
        text("canvas-fixture-state-status", "conflict-recovered:" + stateRevision);
        return;
      }
      text("canvas-fixture-state-status", "unexpected-no-conflict");
    } catch (error) {
      text("canvas-fixture-state-status", "error:" + error.status);
    }
  });

  document.querySelector('[data-testid="canvas-fixture-reconnect"]')?.addEventListener("click", () => {
    void connectEvents(lastEventId);
  });
  document.querySelector('[data-testid="canvas-fixture-resync"]')?.addEventListener("click", () => {
    void connectEvents("old-generation:1");
  });
  document.querySelector('[data-testid="canvas-fixture-refresh"]')?.addEventListener("click", () => {
    text("canvas-fixture-refresh-status", "refreshing");
    void loadProjection().catch((error) => {
      text("canvas-fixture-refresh-status", "error:" + error.status);
    });
  });

  text("canvas-fixture-script", "inline-ready");
  renderAppearance();
  void loadProjection().then(() => connectEvents());
})();`;
}

export function writeCanvasSource(
  workspacePath: string,
  canvas: CanvasRecord,
  options?: CanvasSourceOptions,
): void {
  const sourceDirectory = path.join(workspacePath, ".kandev", "canvases", canvas.id);
  fs.mkdirSync(sourceDirectory, { recursive: true });
  fs.writeFileSync(path.join(sourceDirectory, "manifest.yaml"), canvasManifest(canvas, options));
  fs.writeFileSync(
    path.join(sourceDirectory, "index.html"),
    [
      "<!doctype html>",
      '<html lang="en">',
      '  <head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>E2E Plugin Canvas</title><script src="./appearance.js"></script>',
      "    <style>",
      "      :root { font-family: Inter, ui-sans-serif, system-ui, sans-serif; color: #172033; background: #f7f8fc; font-synthesis: none; }",
      "      * { box-sizing: border-box; } body { margin: 0; padding: 32px; } .canvas-shell { max-width: 920px; margin: 0 auto; }",
      "      .canvas-header { margin-bottom: 24px; } .eyebrow { margin: 0 0 8px; color: #6257df; font-size: 12px; font-weight: 700; letter-spacing: .1em; text-transform: uppercase; }",
      "      h1 { margin: 0; font-size: clamp(24px, 4vw, 34px); line-height: 1.2; letter-spacing: -.03em; } .summary { margin: 8px 0 0; color: #687187; font-size: 14px; }",
      "      .canvas-card { padding: 20px; border: 1px solid #e2e6ef; border-radius: 16px; background: white; box-shadow: 0 10px 32px #27345b0a; }",
      "      .list-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; } .list-heading h2 { margin: 0; font-size: 15px; }",
      "      .task-count { color: #687187; font-size: 12px; } .task-list { display: grid; gap: 10px; }",
      "      .task-item { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 58px; padding: 12px 14px; border: 1px solid #e8ebf2; border-radius: 10px; background: #fff; }",
      "      .task-item h2 { margin: 0; font-size: 14px; font-weight: 600; } .task-status { flex: none; padding: 4px 8px; border-radius: 999px; background: #f0efff; color: #5046c7; font-size: 11px; font-weight: 600; text-transform: capitalize; }",
      '      .canvas-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 16px; } button { min-height: 34px; padding: 7px 11px; border: 1px solid #dfe3ec; border-radius: 8px; background: white; color: #384158; font: inherit; font-size: 12px; cursor: pointer; -webkit-tap-highlight-color: transparent; } button:hover { background: #f6f7fb; } button[data-testid="canvas-fixture-refresh"] { border-color: #5548e8; background: #5548e8; color: white; }',
      "      .diagnostics { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; clip-path: inset(50%); }",
      '      @media (max-width: 600px) { body { padding: 20px 16px; } .canvas-card { padding: 16px; } .task-item { align-items: flex-start; flex-direction: column; gap: 8px; } .canvas-actions button:not([data-testid="canvas-fixture-refresh"]) { display: none; } }',
      "    </style></head>",
      "  <body>",
      '    <main class="canvas-shell" data-testid="canvas-fixture-content">',
      '      <header class="canvas-header"><p class="eyebrow" data-testid="canvas-fixture-eyebrow">Workspace preview</p><h1>Your workspace tasks</h1><p class="summary">A live view of work across this workspace.</p></header>',
      '      <section class="canvas-card">',
      '        <div class="list-heading"><h2>All workspace tasks</h2><span class="task-count"><span data-testid="canvas-fixture-task-count">0</span> tasks</span></div>',
      '        <div class="task-list" data-testid="canvas-fixture-task-list" aria-label="Workspace tasks"></div>',
      '        <div class="canvas-actions"><button type="button" data-testid="canvas-fixture-refresh">Refresh list</button></div>',
      '        <div class="diagnostics" aria-hidden="true">',
      '          <p data-testid="canvas-fixture-script">loading</p><p data-testid="canvas-fixture-context">loading</p><p data-testid="canvas-fixture-task-ids"></p>',
      '          <p data-testid="canvas-fixture-refresh-status">idle</p><p data-testid="canvas-fixture-foreign-workspace-status">not-tested</p><p data-testid="canvas-fixture-workflow-count">0</p><p data-testid="canvas-fixture-step-id">loading</p>',
      '          <p data-testid="canvas-fixture-message-status">idle</p><p data-testid="canvas-fixture-move-status">idle</p><p data-testid="canvas-fixture-state-status">idle</p><p data-testid="canvas-fixture-sse-status">loading</p>',
      '          <p data-testid="canvas-fixture-sse-events">0</p><p data-testid="canvas-fixture-sse-resync">idle</p><p data-testid="canvas-fixture-appearance-mode">loading</p><p data-testid="canvas-fixture-appearance-background">loading</p><p data-testid="canvas-fixture-appearance-color-scheme">loading</p>',
      '          <button type="button" data-testid="canvas-fixture-continue">Continue</button><button type="button" data-testid="canvas-fixture-move">Move workflow step</button><button type="button" data-testid="canvas-fixture-state">Recover state</button>',
      '          <button type="button" data-testid="canvas-fixture-reconnect">Reconnect events</button><button type="button" data-testid="canvas-fixture-resync">Force resync</button>',
      "        </div>",
      "      </section>",
      "    </main>",
      `    <script>${canvasFixtureScript(canvas, options)}</script>`,
      '    <script src="./script.js"></script>',
      "  </body>",
      "</html>",
      "",
    ].join("\n"),
  );
}

export async function seedTaskCanvas(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  useMobileSubmit = false,
  sourceOptions?: CanvasSourceOptions,
): Promise<SeededCanvas> {
  const title = "E2E Plugin Canvas";
  const script = [
    `e2e:mcp:kandev:create_canvas_kandev(${JSON.stringify({
      title,
      summary: "Canvas fixture for Playwright acceptance coverage.",
    })})`,
    'e2e:message("Canvas created.")',
  ].join("\n");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "E2E Plugin Canvas Task",
    seedData.agentProfileId,
    {
      description: script,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) {
    throw new CanvasFixtureUnavailable("Canvas fixture skipped: task session was not created.");
  }

  await page.goto(`/t/${encodeURIComponent(task.id)}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });

  const canvas = await waitForTaskCanvas(apiClient, task.id, title);

  const publishedCanvas = await publishTaskCanvas({
    page,
    apiClient,
    taskId: task.id,
    taskSessionId: task.session_id,
    session,
    canvas,
    useMobileSubmit,
    sourceOptions,
  });

  return {
    taskId: task.id,
    taskSessionId: task.session_id,
    canvas: publishedCanvas,
    session,
  };
}

export async function waitForTaskCanvas(
  apiClient: ApiClient,
  taskId: string,
  title: string,
): Promise<CanvasRecord> {
  let canvas: CanvasRecord | null = null;
  await expect
    .poll(
      async () => {
        const canvases = await listTaskCanvases(apiClient, taskId);
        canvas =
          canvases.find(
            (candidate) =>
              candidate.title === title &&
              candidate.scope_kind === "task" &&
              candidate.task_id === taskId,
          ) ?? null;
        return canvas?.id ?? null;
      },
      { timeout: 30_000, message: "The mock agent did not create a task canvas." },
    )
    .not.toBeNull();
  if (!canvas) throw new Error("The canvas fixture returned no canvas record.");
  return canvas;
}

type PublishTaskCanvasOptions = {
  page: Page;
  apiClient: ApiClient;
  taskId: string;
  taskSessionId: string;
  session: SessionPage;
  canvas: CanvasRecord;
  useMobileSubmit?: boolean;
  sourceOptions?: CanvasSourceOptions;
};

export async function publishTaskCanvas({
  page,
  apiClient,
  taskId,
  taskSessionId,
  session,
  canvas,
  useMobileSubmit = false,
  sourceOptions,
}: PublishTaskCanvasOptions): Promise<CanvasRecord> {
  const workspacePath = await waitForSessionWorkspace(apiClient, taskId, taskSessionId);
  writeCanvasSource(workspacePath, canvas, sourceOptions);
  const sourcePath = `.kandev/canvases/${canvas.id}`;
  const publishScript = `e2e:mcp:kandev:publish_canvas_kandev(${JSON.stringify({
    canvas_id: canvas.id,
    source_path: sourcePath,
  })})`;
  await waitForSessionAgentctlReady(page, taskSessionId);
  if (useMobileSubmit) {
    await session.sendMessageViaButton(publishScript);
  } else {
    await session.sendMessage(publishScript);
  }
  // The release poll below is the authoritative completion signal. Waiting
  // for the chat composer is an unrelated UI settle condition and can time
  // out while the publish request is still being processed.
  let publishedCanvas: CanvasRecord | null = null;
  await expect
    .poll(
      async () => {
        publishedCanvas = await getCanvas(apiClient, canvas.id);
        return Boolean(
          publishedCanvas?.pending_release ||
          publishedCanvas?.active_release_id ||
          publishedCanvas?.active_release_status,
        );
      },
      { timeout: 60_000, message: "The mock agent did not publish the canvas package." },
    )
    .toBe(true);
  if (!publishedCanvas) throw new Error("The canvas publish response was empty.");
  await waitForSessionDone(
    apiClient,
    taskId,
    taskSessionId,
    "The canvas publishing session did not finish before interaction coverage.",
    45_000,
  );
  return publishedCanvas;
}

export async function approvePendingCanvas(
  apiClient: ApiClient,
  canvas: CanvasRecord,
): Promise<CanvasRecord> {
  const pendingRelease = canvas.pending_release;
  if (!pendingRelease) throw new Error("The canvas has no pending release to approve.");
  const response = await apiClient.rawRequest(
    "POST",
    `/api/v1/canvases/${encodeURIComponent(canvas.id)}/releases/${encodeURIComponent(pendingRelease.id)}/approve`,
  );
  if (!response.ok) {
    throw new Error(`Canvas release approval failed (${response.status}).`);
  }

  let approvedCanvas: CanvasRecord | null = null;
  await expect
    .poll(
      async () => {
        approvedCanvas = await getCanvas(apiClient, canvas.id);
        return approvedCanvas?.active_release_status === "valid";
      },
      { timeout: 30_000, message: "The canvas release did not become active." },
    )
    .toBe(true);
  if (!approvedCanvas) throw new Error("The approved canvas record was empty.");
  return approvedCanvas;
}

export async function promoteCanvas(
  apiClient: ApiClient,
  canvas: CanvasRecord,
): Promise<CanvasRecord> {
  const previewResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/canvases/${encodeURIComponent(canvas.id)}/promotion-preview`,
  );
  if (!previewResponse.ok) {
    throw new Error(`Canvas promotion preview failed (${previewResponse.status}).`);
  }
  const preview = (await previewResponse.json()) as {
    active_release_id?: string;
    permission_digest?: string;
    grant_generation?: number;
  };
  if (
    !preview.active_release_id ||
    !preview.permission_digest ||
    preview.grant_generation === undefined
  ) {
    throw new Error("Canvas promotion preview was incomplete.");
  }
  const response = await apiClient.rawRequest(
    "POST",
    `/api/v1/canvases/${encodeURIComponent(canvas.id)}/promotion`,
    {
      expected_release_id: preview.active_release_id,
      expected_permission_digest: preview.permission_digest,
      expected_grant_generation: preview.grant_generation,
    },
  );
  if (!response.ok) {
    throw new Error(`Canvas promotion failed (${response.status}).`);
  }
  let promotedCanvas: CanvasRecord | null = null;
  await expect
    .poll(
      async () => {
        promotedCanvas = await getCanvas(apiClient, canvas.id);
        return promotedCanvas?.scope_kind === "workspace";
      },
      { timeout: 30_000, message: "The canvas did not become workspace-scoped." },
    )
    .toBe(true);
  if (!promotedCanvas) throw new Error("The promoted canvas record was empty.");
  return promotedCanvas;
}

export async function removeCanvas(apiClient: ApiClient, canvasId: string): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/v1/canvases/${encodeURIComponent(canvasId)}`);
}

export function canvasHref(canvasId: string): string {
  return `/canvases/${encodeURIComponent(canvasId)}`;
}
