import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  archiveCanvas,
  confirmCanvasPromotion,
  getCanvasRuntime,
  listTaskCanvases,
  listWorkspaceCanvases,
  requestCanvasPromotion,
  type CanvasPromotionPreview,
  type CanvasRuntimeResponse,
} from "./canvas-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("workspace canvas collection", () => {
  it("requests only workspace-scoped canvases and can include archived records", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ canvases: [] }));

    await listWorkspaceCanvases("workspace / one", {
      includeArchived: true,
      baseUrl: API_BASE_URL,
    });

    expect(fetchSpy).toHaveBeenCalledWith(
      `${API_BASE_URL}/api/v1/workspaces/workspace%20%2F%20one/canvases?include_archived=true`,
      expect.objectContaining({ credentials: "include" }),
    );
  });
});

describe("task canvas collection", () => {
  it("keeps the task and workspace scope in the request", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ canvases: [] }));

    await listTaskCanvases("task-1", {
      workspaceId: "workspace-1",
      includeArchived: false,
      baseUrl: API_BASE_URL,
    });

    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${API_BASE_URL}/api/v1/tasks/task-1/canvases?workspace_id=workspace-1`,
    );
  });
});

describe("canvas host and lifecycle endpoints", () => {
  it("preserves runtime binding and release fields from the host contract", async () => {
    const runtimeResponse: CanvasRuntimeResponse = {
      runtime_url: "https://runtime.test/canvas?token=renewed",
      release_id: "release-7",
      web_app_key: "canvas-app",
      placement: "task_panel",
      expires_in_seconds: 900,
      canvas: {
        id: "canvas-1",
        plugin_instance_id: "instance-1",
        plugin_id: "plugin-1",
        workspace_id: "workspace-1",
        task_id: "task-1",
        scope_kind: "task",
        title: "Canvas",
        status: "active",
        active_release_id: "release-7",
        active_release_status: "valid",
      },
      binding: {
        scope_kind: "task",
        grant_generation: 4,
      },
    };
    fetchSpy.mockResolvedValueOnce(jsonResponse(runtimeResponse));

    await expect(getCanvasRuntime("canvas-1", { baseUrl: API_BASE_URL })).resolves.toEqual(
      runtimeResponse,
    );
  });

  it("preserves promotion source, placement, and permission review fields", async () => {
    const preview: CanvasPromotionPreview = {
      canvas_id: "canvas-1",
      title: "Canvas",
      origin_task_id: "task-1",
      source_actor_kind: "task_agent",
      source_user_id: "user-1",
      source_task_id: "task-1",
      source_session_id: "session-1",
      permissions: {
        reads: ["tasks.read"],
        writes: ["tasks.write"],
        events: ["task.updated"],
        shared_state: true,
        external_origins: ["https://example.test"],
      },
      current_scope: "task",
      target_scope: "workspace",
      placement: "workspace_sidebar",
      active_release_id: "release-7",
      permission_digest: "digest-7",
      grant_generation: 4,
    };
    fetchSpy.mockResolvedValueOnce(jsonResponse(preview));

    await expect(requestCanvasPromotion("canvas-1", { baseUrl: API_BASE_URL })).resolves.toEqual(
      preview,
    );
  });

  it("uses the same canvas identity for runtime and promotion actions", async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ runtime_url: "https://runtime.test/canvas" }))
      .mockResolvedValueOnce(jsonResponse({ canvas_id: "canvas-1", permissions: [] }))
      .mockResolvedValueOnce(jsonResponse({ id: "canvas-1", status: "active" }));

    await getCanvasRuntime("canvas-1", { baseUrl: API_BASE_URL });
    await requestCanvasPromotion("canvas-1", { baseUrl: API_BASE_URL });
    await confirmCanvasPromotion(
      "canvas-1",
      {
        expected_release_id: "release-7",
        expected_permission_digest: "digest-7",
        expected_grant_generation: 4,
      },
      { baseUrl: API_BASE_URL },
    );

    expect(fetchSpy.mock.calls.map(([url]) => url)).toEqual([
      `${API_BASE_URL}/api/v1/canvases/canvas-1/runtime`,
      `${API_BASE_URL}/api/v1/canvases/canvas-1/promotion-preview`,
      `${API_BASE_URL}/api/v1/canvases/canvas-1/promotion`,
    ]);
    expect(fetchSpy.mock.calls[2][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        expected_release_id: "release-7",
        expected_permission_digest: "digest-7",
        expected_grant_generation: 4,
      }),
    });
  });

  it("archives with a mutation request and preserves the API response", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: "canvas-1", status: "archived" }));

    await expect(archiveCanvas("canvas-1", { baseUrl: API_BASE_URL })).resolves.toMatchObject({
      status: "archived",
    });

    expect(fetchSpy.mock.calls[0][1]).toMatchObject({ method: "POST" });
  });
});
