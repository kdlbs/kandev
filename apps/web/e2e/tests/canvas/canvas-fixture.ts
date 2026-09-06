import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
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
  status?: string;
  active_release_id?: string;
  active_release_status?: string;
  pending_release?: {
    id: string;
    validation_status?: string;
  };
};

export type SeededCanvas = {
  taskId: string;
  taskSessionId: string;
  canvas: CanvasRecord;
  session: SessionPage;
};

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
          const heightMatches =
            Math.abs(frameRect.height - (hostRect.height - headerRect.height)) <= 2;
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

async function waitForSessionWorkspace(
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

function canvasManifest(canvas: CanvasRecord): string {
  const compactId = canvas.id.replaceAll("-", "").slice(0, 12);
  return [
    `id: "canvas-${compactId}"`,
    "api_version: 2",
    'version: "1.0.0"',
    'display_name: "E2E Plugin Canvas"',
    'description: "Canvas fixture for Playwright acceptance coverage."',
    'author: "Kandev E2E"',
    "ui:",
    "  web_apps:",
    "    - key: main",
    '      title: "E2E Plugin Canvas"',
    "      entry: index.html",
    "      placements:",
    "        - task-canvas",
    "        - workspace-canvas",
    "capabilities:",
    "  api_read:",
    "    - tasks",
    "    - workflows",
    "  api_write:",
    "    - tasks",
    "    - messages",
    "  events:",
    "    - task.updated",
    "  state: true",
    "",
  ].join("\n");
}

function canvasFixtureScript(canvas: CanvasRecord): string {
  const taskID = JSON.stringify(canvas.task_id ?? "");
  return String.raw`(() => {
  const taskId = ${taskID};
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
    const [context, taskPage, workflowPage] = await Promise.all([
      api("./_kandev/v1/context"),
      api("./_kandev/v1/data/tasks?limit=10"),
      api("./_kandev/v1/data/workflows?limit=10"),
    ]);
    const tasks = taskPage.items || [];
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
    text("canvas-fixture-workflow-count", workflowPage.items?.length || 0);
    text("canvas-fixture-step-id", task?.workflow_step_id || "");
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
    const controller = new AbortController();
    streamController = controller;
    text("canvas-fixture-sse-status", "connecting");
    try {
      const response = await fetch("./_kandev/v1/events", {
        headers: cursor ? { "Last-Event-ID": cursor } : {},
        signal: controller.signal,
      });
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

  text("canvas-fixture-script", "inline-ready");
  renderAppearance();
  void loadProjection().then(() => connectEvents());
})();`;
}

export function writeCanvasSource(workspacePath: string, canvas: CanvasRecord): void {
  const sourceDirectory = path.join(workspacePath, ".kandev", "canvases", canvas.id);
  fs.mkdirSync(sourceDirectory, { recursive: true });
  fs.writeFileSync(path.join(sourceDirectory, "manifest.yaml"), canvasManifest(canvas));
  fs.writeFileSync(
    path.join(sourceDirectory, "index.html"),
    [
      "<!doctype html>",
      '<html lang="en">',
      '  <head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>E2E Plugin Canvas</title><link rel="stylesheet" href="./styles.css"><script src="./appearance.js"></script></head>',
      "  <body>",
      '    <main class="canvas-shell" data-testid="canvas-fixture-content">',
      '      <header class="canvas-header"><p class="eyebrow" data-testid="canvas-fixture-eyebrow">E2E Plugin Canvas</p><h1>E2E Plugin Canvas</h1></header>',
      '      <section class="canvas-card">',
      '      <p data-testid="canvas-fixture-script">loading</p>',
      '      <p data-testid="canvas-fixture-context">loading</p>',
      '      <p data-testid="canvas-fixture-task-count">0</p>',
      '      <p data-testid="canvas-fixture-workflow-count">0</p>',
      '      <p data-testid="canvas-fixture-step-id">loading</p>',
      '      <p data-testid="canvas-fixture-message-status">idle</p>',
      '      <p data-testid="canvas-fixture-move-status">idle</p>',
      '      <p data-testid="canvas-fixture-state-status">idle</p>',
      '      <p data-testid="canvas-fixture-sse-status">loading</p>',
      '      <p data-testid="canvas-fixture-sse-events">0</p>',
      '      <p data-testid="canvas-fixture-sse-resync">idle</p>',
      '      <p data-testid="canvas-fixture-appearance-mode">loading</p>',
      '      <p data-testid="canvas-fixture-appearance-background">loading</p>',
      '      <p data-testid="canvas-fixture-appearance-color-scheme">loading</p>',
      '      <button type="button" data-testid="canvas-fixture-continue">Continue</button>',
      '      <button type="button" data-testid="canvas-fixture-move">Move workflow step</button>',
      '      <button type="button" data-testid="canvas-fixture-state">Recover state</button>',
      '      <button type="button" data-testid="canvas-fixture-reconnect">Reconnect events</button>',
      '      <button type="button" data-testid="canvas-fixture-resync">Force resync</button>',
      "      </section>",
      "    </main>",
      `    <script>${canvasFixtureScript(canvas)}</script>`,
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
    apiClient,
    taskId: task.id,
    taskSessionId: task.session_id,
    session,
    canvas,
    useMobileSubmit,
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
  apiClient: ApiClient;
  taskId: string;
  taskSessionId: string;
  session: SessionPage;
  canvas: CanvasRecord;
  useMobileSubmit?: boolean;
};

export async function publishTaskCanvas({
  apiClient,
  taskId,
  taskSessionId,
  session,
  canvas,
  useMobileSubmit = false,
}: PublishTaskCanvasOptions): Promise<CanvasRecord> {
  const workspacePath = await waitForSessionWorkspace(apiClient, taskId, taskSessionId);
  writeCanvasSource(workspacePath, canvas);
  const sourcePath = `.kandev/canvases/${canvas.id}`;
  const publishScript = `e2e:mcp:kandev:publish_canvas_kandev(${JSON.stringify({
    canvas_id: canvas.id,
    source_path: sourcePath,
  })})`;
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
      { timeout: 30_000, message: "The mock agent did not publish the canvas package." },
    )
    .toBe(true);
  if (!publishedCanvas) throw new Error("The canvas publish response was empty.");
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
