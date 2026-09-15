import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { OfficeConfigSyncConfig } from "@/lib/types/office-config-sync";

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}));

const mockRouterRefresh = vi.fn();
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ refresh: mockRouterRefresh }),
}));

const mockUseSettingsSaveContributor = vi.fn();
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (contributor: unknown) => mockUseSettingsSaveContributor(contributor),
}));

const getOfficeConfigSyncConfigMock =
  vi.fn<(workspaceId: string) => Promise<OfficeConfigSyncConfig | null>>();
const setOfficeConfigSyncConfigMock =
  vi.fn<(workspaceId: string, payload: unknown) => Promise<unknown>>();
const deleteOfficeConfigSyncConfigMock = vi.fn<(workspaceId: string) => Promise<unknown>>();
const forceOfficeConfigSyncMock = vi.fn<(workspaceId: string) => Promise<unknown>>();

vi.mock("@/lib/api/domains/office-config-sync-api", () => ({
  getOfficeConfigSyncConfig: (workspaceId: string) => getOfficeConfigSyncConfigMock(workspaceId),
  setOfficeConfigSyncConfig: (workspaceId: string, payload: unknown) =>
    setOfficeConfigSyncConfigMock(workspaceId, payload),
  deleteOfficeConfigSyncConfig: (workspaceId: string) =>
    deleteOfficeConfigSyncConfigMock(workspaceId),
  forceOfficeConfigSync: (workspaceId: string) => forceOfficeConfigSyncMock(workspaceId),
}));

import { useOfficeConfigSync } from "./use-office-config-sync";

const REPO_NAME = "kandev-office-config";

function makeConfig(overrides: Partial<OfficeConfigSyncConfig> = {}): OfficeConfigSyncConfig {
  return {
    workspace_id: "ws-1",
    provider: "github",
    repo_owner: "kdlbs",
    repo_name: REPO_NAME,
    project_path: "",
    branch: "main",
    path: "",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  getOfficeConfigSyncConfigMock.mockResolvedValue(null);
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useOfficeConfigSync", () => {
  it("registers the config draft with the shared settings save coordinator", async () => {
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.update("repo_owner", "kdlbs"));
    act(() => result.current.update("repo_name", REPO_NAME));

    await waitFor(() =>
      expect(mockUseSettingsSaveContributor).toHaveBeenLastCalledWith(
        expect.objectContaining({
          id: "office-config-sync",
          isDirty: true,
          canSave: true,
        }),
      ),
    );
  });

  it("loads config and pre-fills the form on mount", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(
      makeConfig({ repo_owner: "acme", branch: "dev" }),
    );
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config?.repo_owner).toBe("acme");
    expect(result.current.form.repo_owner).toBe("acme");
    expect(result.current.form.branch).toBe("dev");
  });

  it("falls back to defaults when unconfigured (204/null)", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config).toBeNull();
    expect(result.current.form.provider).toBe("github");
    expect(result.current.form.branch).toBe("main");
    expect(result.current.form.interval_seconds).toBe(300);
    expect(result.current.form.poll_enabled).toBe(true);
  });

  it("handleSave surfaces a toast on failure and keeps saving false", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    setOfficeConfigSyncConfigMock.mockRejectedValue(new Error("invalid config"));
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSave();
    });

    expect(mockToastError).toHaveBeenCalled();
    expect(result.current.saving).toBe(false);
  });
});

describe("useOfficeConfigSync — save/delete", () => {
  it("handleSave posts the form and updates config from the response", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    const saved = makeConfig({ repo_owner: "kdlbs", repo_name: REPO_NAME });
    setOfficeConfigSyncConfigMock.mockResolvedValue(saved);
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.update("repo_owner", "kdlbs"));
    act(() => result.current.update("repo_name", REPO_NAME));
    await act(async () => {
      await result.current.handleSave();
    });

    expect(setOfficeConfigSyncConfigMock).toHaveBeenCalledWith(
      "ws-1",
      expect.objectContaining({ repo_owner: "kdlbs", repo_name: REPO_NAME }),
    );
    expect(result.current.config?.repo_owner).toBe("kdlbs");
    expect(mockToastSuccess).toHaveBeenCalled();
  });

  it("handleSave sends path verbatim, without trimming whitespace", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(null);
    setOfficeConfigSyncConfigMock.mockRejectedValue(new Error("path must not be blank"));
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    act(() => result.current.update("repo_owner", "kdlbs"));
    act(() => result.current.update("repo_name", REPO_NAME));
    act(() => result.current.update("path", "  office/ "));
    await act(async () => {
      await result.current.handleSave();
    });

    expect(setOfficeConfigSyncConfigMock).toHaveBeenCalledWith(
      "ws-1",
      expect.objectContaining({ path: "  office/ " }),
    );
  });

  it("handleDelete clears config, resets the form, and refreshes the router", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig());
    deleteOfficeConfigSyncConfigMock.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleDelete();
    });

    expect(deleteOfficeConfigSyncConfigMock).toHaveBeenCalledWith("ws-1");
    expect(result.current.config).toBeNull();
    expect(result.current.form.repo_owner).toBe("");
    expect(mockRouterRefresh).toHaveBeenCalledTimes(1);
  });
});

describe("useOfficeConfigSync — GitLab form paths", () => {
  it("setProvider clears the other provider's fields", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(
      makeConfig({ repo_owner: "acme", repo_name: "flows" }),
    );
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.form.repo_owner).toBe("acme");

    act(() => result.current.setProvider("gitlab"));

    expect(result.current.form.provider).toBe("gitlab");
    expect(result.current.form.repo_owner).toBe("");
    expect(result.current.form.repo_name).toBe("");
    expect(result.current.form.project_path).toBe("");
  });

  it("handleSave for a GitLab form emits project_path without GitHub identifiers", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(
      makeConfig({
        provider: "gitlab",
        repo_owner: "",
        repo_name: "",
        project_path: "group/project",
      }),
    );
    const saved = makeConfig({
      provider: "gitlab",
      repo_owner: "",
      repo_name: "",
      project_path: "group/project",
    });
    setOfficeConfigSyncConfigMock.mockResolvedValue(saved);
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSave();
    });

    const payload = setOfficeConfigSyncConfigMock.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(payload.provider).toBe("gitlab");
    expect(payload.project_path).toBe("group/project");
    expect(payload).not.toHaveProperty("repo_owner");
    expect(payload).not.toHaveProperty("repo_name");
  });
});

describe("useOfficeConfigSync — sync now and polling", () => {
  it("handleSyncNow refreshes entity lists after a failed sync attempt", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    forceOfficeConfigSyncMock.mockResolvedValue({
      config: makeConfig({ last_ok: false, last_error: "clone failed" }),
      error: "clone failed",
    });
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSyncNow();
    });

    expect(forceOfficeConfigSyncMock).toHaveBeenCalledWith("ws-1");
    expect(result.current.config?.last_ok).toBe(false);
    expect(result.current.config?.last_error).toBe("clone failed");
    expect(result.current.syncing).toBe(false);
    expect(mockToastError).toHaveBeenCalled();
    // A later reconciliation phase can fail after earlier entity writes commit.
    // The UI must refresh even when the failed response omits `result`.
    expect(mockRouterRefresh).toHaveBeenCalledTimes(1);
  });

  it("handleSyncNow refreshes the router when the sync changed something", async () => {
    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    forceOfficeConfigSyncMock.mockResolvedValue({
      config: makeConfig({ last_ok: true }),
      result: { created: ["agent.yaml"], updated: [], deleted: [], warnings: [], unchanged: false },
    });
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.handleSyncNow();
    });

    expect(mockRouterRefresh).toHaveBeenCalledTimes(1);
  });

  it("polls getOfficeConfigSyncConfig on the background refresh interval", async () => {
    vi.useFakeTimers();
    getOfficeConfigSyncConfigMock.mockResolvedValue(makeConfig({ last_ok: true }));
    const { result } = renderHook(() => useOfficeConfigSync("ws-1"));
    await vi.waitFor(() => expect(result.current.loading).toBe(false));
    expect(getOfficeConfigSyncConfigMock).toHaveBeenCalledTimes(1);

    getOfficeConfigSyncConfigMock.mockResolvedValue(
      makeConfig({ last_ok: false, last_error: "boom" }),
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(90_000);
    });
    expect(getOfficeConfigSyncConfigMock.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(result.current.config?.last_error).toBe("boom");
  });
});
