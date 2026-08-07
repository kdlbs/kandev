import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { WorkflowSyncConfig } from "@/lib/types/workflow-sync";

const mockToast = vi.fn();
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const getWorkflowSyncConfigMock = vi.fn<(opts?: unknown) => Promise<WorkflowSyncConfig | null>>();
const setWorkflowSyncConfigMock = vi.fn<(payload: unknown, opts?: unknown) => Promise<unknown>>();
const deleteWorkflowSyncConfigMock = vi.fn<(opts?: unknown) => Promise<unknown>>();
const forceWorkflowSyncMock = vi.fn<(opts?: unknown) => Promise<unknown>>();

vi.mock("@/lib/api/domains/workflow-sync-api", () => ({
  getWorkflowSyncConfig: (opts?: unknown) => getWorkflowSyncConfigMock(opts),
  setWorkflowSyncConfig: (payload: unknown, opts?: unknown) =>
    setWorkflowSyncConfigMock(payload, opts),
  deleteWorkflowSyncConfig: (opts?: unknown) => deleteWorkflowSyncConfigMock(opts),
  forceWorkflowSync: (opts?: unknown) => forceWorkflowSyncMock(opts),
}));

import { useWorkflowSync } from "./use-workflow-sync";

function makeConfig(overrides: Partial<WorkflowSyncConfig> = {}): WorkflowSyncConfig {
  return {
    workspace_id: "ws-1",
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: "kandev",
    project_path: "",
    branch: "main",
    path: WORKFLOWS_DIR,
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("useWorkflowSync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getWorkflowSyncConfigMock.mockResolvedValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("loads config and pre-fills the form on mount", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ repo_owner: "acme", branch: "dev" }));
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config?.repo_owner).toBe("acme");
    expect(result.current.form.repo_owner).toBe("acme");
    expect(result.current.form.branch).toBe("dev");
  });

  it("falls back to defaults when unconfigured (204/null)", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config).toBeNull();
    expect(result.current.form.branch).toBe("main");
    expect(result.current.form.path).toBe(WORKFLOWS_DIR);
  });

  it("handleSave posts the form and updates config from the response", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(null);
    const saved = makeConfig({ repo_owner: "kdlbs", repo_name: "kandev" });
    setWorkflowSyncConfigMock.mockResolvedValue(saved);
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.update("repo_owner", "kdlbs"));
    act(() => result.current.update("repo_name", "kandev"));
    await act(async () => {
      await result.current.handleSave();
    });

    expect(setWorkflowSyncConfigMock).toHaveBeenCalledWith(
      expect.objectContaining({ repo_owner: "kdlbs", repo_name: "kandev" }),
      { workspaceId: "ws-1" },
    );
    expect(result.current.config?.repo_owner).toBe("kdlbs");
  });

  it("handleSyncNow updates config from the force-sync response, including failures", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    forceWorkflowSyncMock.mockResolvedValue({
      config: makeConfig({ last_ok: false, last_error: "clone failed" }),
      error: "clone failed",
    });
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSyncNow();
    });

    expect(forceWorkflowSyncMock).toHaveBeenCalledWith({ workspaceId: "ws-1" });
    expect(result.current.config?.last_ok).toBe(false);
    expect(result.current.config?.last_error).toBe("clone failed");
    expect(result.current.syncing).toBe(false);
  });

  it("handleDelete clears config and resets the form when confirmed", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig());
    deleteWorkflowSyncConfigMock.mockResolvedValue({ deleted: true });
    vi.stubGlobal(
      "confirm",
      vi.fn(() => true),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleDelete();
    });

    expect(deleteWorkflowSyncConfigMock).toHaveBeenCalledWith({ workspaceId: "ws-1" });
    expect(result.current.config).toBeNull();
    expect(result.current.form.repo_owner).toBe("");
  });

  it("polls getWorkflowSyncConfig on the background refresh interval", async () => {
    vi.useFakeTimers();
    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await vi.waitFor(() => expect(result.current.loading).toBe(false));
    expect(getWorkflowSyncConfigMock).toHaveBeenCalledTimes(1);

    getWorkflowSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: false, last_error: "boom" }));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90_000);
    });
    expect(getWorkflowSyncConfigMock.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(result.current.config?.last_error).toBe("boom");
  });
});

// The link field must redisplay only the repo identity, never branch or
// directory — otherwise it can silently disagree with the separate,
// directly-editable Branch/Directory fields once a user corrects one of them
// without retyping the whole link.
describe("useWorkflowSync — link field redisplay", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("redisplays only the repo identity, not branch or directory", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(
      makeConfig({
        repo_owner: "acme",
        repo_name: "flows",
        branch: "release/1.2",
        path: "custom/dir",
      }),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.url).toBe("https://github.com/acme/flows");
    expect(result.current.form.branch).toBe("release/1.2");
    expect(result.current.form.path).toBe("custom/dir");
  });

  it("redisplays a GitLab config's project_path alone", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(
      makeConfig({
        provider: "gitlab",
        repo_owner: "",
        repo_name: "",
        project_path: "front-end/km-mobile-app",
        branch: "features/kegmil-location-adoption",
        path: WORKFLOWS_DIR,
      }),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.url).toBe("front-end/km-mobile-app");
    expect(result.current.form.branch).toBe("features/kegmil-location-adoption");
  });
});

const HOST = "https://gitlab.com";
const GITLAB_PROJECT = "acme/project";
const WORKFLOWS_DIR = ".kandev/workflows";
const GITLAB_REF = `${HOST}/${GITLAB_PROJECT}/-/tree/main/${WORKFLOWS_DIR}`;

// GitLab form paths: provider switching, GitLab URL parsing into
// project_path, and the save payload that must carry project_path without
// any GitHub identifiers.
describe("useWorkflowSync — GitLab form paths", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("setProvider clears the other provider's fields", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(
      makeConfig({ repo_owner: "acme", repo_name: "flows" }),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.form.repo_owner).toBe("acme");

    act(() => result.current.setProvider("gitlab"));

    expect(result.current.form.provider).toBe("gitlab");
    expect(result.current.form.repo_owner).toBe("");
    expect(result.current.form.repo_name).toBe("");
    expect(result.current.form.project_path).toBe("");
    expect(result.current.url).toBe("");
  });

  it("setUrlInput parses a GitLab URL into project_path, branch, and path", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(
      makeConfig({ provider: "gitlab", project_path: GITLAB_PROJECT }),
    );
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.setUrlInput(GITLAB_REF));

    expect(result.current.form.project_path).toBe(GITLAB_PROJECT);
    expect(result.current.form.branch).toBe("main");
    expect(result.current.form.path).toBe(WORKFLOWS_DIR);
    expect(result.current.urlInvalid).toBe(false);
  });

  it("handleSave for a GitLab form emits project_path without GitHub identifiers", async () => {
    getWorkflowSyncConfigMock.mockResolvedValue(
      makeConfig({ provider: "gitlab", project_path: GITLAB_PROJECT }),
    );
    const saved = makeConfig({
      provider: "gitlab",
      repo_owner: "",
      repo_name: "",
      project_path: GITLAB_PROJECT,
    });
    setWorkflowSyncConfigMock.mockResolvedValue(saved);
    const { result } = renderHook(() => useWorkflowSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSave();
    });

    expect(setWorkflowSyncConfigMock).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: "gitlab",
        project_path: GITLAB_PROJECT,
        branch: "main",
        path: WORKFLOWS_DIR,
      }),
      { workspaceId: "ws-1" },
    );
    const payload = setWorkflowSyncConfigMock.mock.calls[0][0] as Record<string, unknown>;
    expect(payload).not.toHaveProperty("repo_owner");
    expect(payload).not.toHaveProperty("repo_name");
  });
});
