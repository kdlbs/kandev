import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { PromptSyncConfig } from "@/lib/types/prompt-sync";

const mockToast = vi.fn();
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const getMock = vi.fn<() => Promise<PromptSyncConfig | null>>();
const setMock = vi.fn<(payload: unknown) => Promise<{ config: PromptSyncConfig }>>();
const checkMock = vi.fn<(payload?: unknown) => Promise<unknown>>();
const deleteMock = vi.fn<() => Promise<unknown>>();
const runMock = vi.fn<() => Promise<unknown>>();

vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  getPromptSyncConfig: () => getMock(),
  setPromptSyncConfig: (payload: unknown) => setMock(payload),
  checkPromptSync: (payload?: unknown) => checkMock(payload),
  deletePromptSyncConfig: () => deleteMock(),
  runPromptSync: () => runMock(),
}));

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));

import { usePromptSync } from "./use-prompt-sync";

const PROMPTS_DIR = ".kandev/prompts";

function makeConfig(overrides: Partial<PromptSyncConfig> = {}): PromptSyncConfig {
  return {
    workspace_id: "ws-1",
    provider: "gitlab",
    repo_owner: "",
    repo_name: "",
    project_path: "acme/eng",
    branch: "main",
    path: PROMPTS_DIR,
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("usePromptSync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getMock.mockResolvedValue(null);
    checkMock.mockResolvedValue({ conflicts: [] });
  });

  it("defaults the form to the prompts directory and the given workspace", async () => {
    const { result } = renderHook(() => usePromptSync("ws-default"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.config).toBeNull();
    expect(result.current.form.path).toBe(PROMPTS_DIR);
    expect(result.current.form.workspace_id).toBe("ws-default");
  });

  it("loads the saved config into the form", async () => {
    getMock.mockResolvedValue(makeConfig({ workspace_id: "ws-9" }));
    const { result } = renderHook(() => usePromptSync("ws-default"));
    await waitFor(() => expect(result.current.config).not.toBeNull());
    expect(result.current.form.workspace_id).toBe("ws-9");
    expect(result.current.url).toBe("acme/eng");
  });

  it("saves the workspace with the repository payload", async () => {
    setMock.mockResolvedValue({ config: makeConfig() });
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setProvider("gitlab"));
    act(() => result.current.setUrlInput("acme/eng"));
    let saved = false;
    await act(async () => {
      saved = await result.current.handleSave();
    });
    expect(saved).toBe(true);
    expect(setMock).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_id: "ws-1",
        provider: "gitlab",
        project_path: "acme/eng",
      }),
    );
    expect(result.current.config?.project_path).toBe("acme/eng");
  });

  it("runs a sync and surfaces warnings and errors", async () => {
    getMock.mockResolvedValue(makeConfig());
    runMock.mockResolvedValueOnce({
      config: makeConfig({ last_warnings: ["w"] }),
      result: { warnings: ["w"] },
    });
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.config).not.toBeNull());
    await act(async () => {
      await result.current.handleSyncNow();
    });
    expect(result.current.config?.last_warnings).toEqual(["w"]);
    expect(mockToast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "default" }));

    runMock.mockResolvedValueOnce({
      config: makeConfig({ last_ok: false }),
      error: "Prompt sync failed",
    });
    await act(async () => {
      await result.current.handleSyncNow();
    });
    expect(mockToast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("removes the config and resets the form", async () => {
    getMock.mockResolvedValue(makeConfig());
    deleteMock.mockResolvedValue({ deleted: true });
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.config).not.toBeNull());
    let removed = false;
    await act(async () => {
      removed = await result.current.handleDelete();
    });
    expect(removed).toBe(true);
    expect(result.current.config).toBeNull();
    expect(result.current.form.project_path).toBe("");
  });
});
