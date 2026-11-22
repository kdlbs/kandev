import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { PromptSyncConfig } from "@/lib/types/prompt-sync";

const mocks = vi.hoisted(() => ({
  host: undefined as string | undefined,
  get: vi.fn<() => Promise<PromptSyncConfig | null>>(),
  set: vi.fn<(payload: unknown) => Promise<{ config: PromptSyncConfig }>>(),
  check: vi.fn<(payload?: unknown) => Promise<unknown>>(),
  del: vi.fn<() => Promise<unknown>>(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({
    status: mocks.host ? { automation: { github_host: mocks.host } } : null,
  }),
}));
vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  getPromptSyncConfig: () => mocks.get(),
  setPromptSyncConfig: (payload: unknown) => mocks.set(payload),
  checkPromptSync: (payload?: unknown) => mocks.check(payload),
  deletePromptSyncConfig: () => mocks.del(),
  runPromptSync: vi.fn(),
}));

import { usePromptSync } from "./use-prompt-sync";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";

function makeConfig(overrides: Partial<PromptSyncConfig> = {}): PromptSyncConfig {
  return {
    workspace_id: "ws-1",
    provider: "github",
    github_host: "github.com",
    repo_owner: "acme",
    repo_name: "eng",
    project_path: "",
    branch: "main",
    path: ".kandev/prompts",
    interval_seconds: 300,
    poll_enabled: true,
    last_ok: true,
    created_at: "2026-11-19T10:00:00Z",
    updated_at: "2026-11-19T10:00:00Z",
    ...overrides,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

describe("usePromptSync review fixes", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.host = undefined;
    mocks.get.mockResolvedValue(null);
    mocks.check.mockResolvedValue({ conflicts: [] });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it("ignores a status refresh that started before Remove", async () => {
    vi.useFakeTimers();
    mocks.get.mockResolvedValueOnce(makeConfig());
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await act(async () => {});
    expect(result.current.config).not.toBeNull();

    const stale = deferred<PromptSyncConfig | null>();
    mocks.get.mockReturnValueOnce(stale.promise);
    await act(async () => {
      vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS);
    });
    mocks.del.mockResolvedValue({ deleted: true });
    await act(async () => {
      await result.current.handleDelete();
    });
    await act(async () => {
      stale.resolve(makeConfig({ branch: "old" }));
    });
    expect(result.current.config).toBeNull();
  });

  it("keeps edits made while Save is in flight", async () => {
    mocks.get.mockResolvedValue(makeConfig());
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.config).not.toBeNull());

    const pending = deferred<{ config: PromptSyncConfig }>();
    mocks.set.mockReturnValueOnce(pending.promise);
    let saving!: Promise<boolean>;
    act(() => {
      saving = result.current.handleSave();
    });
    act(() => result.current.update("branch", "edited-after-save"));
    let accepted = true;
    await act(async () => {
      await waitFor(() => expect(mocks.set).toHaveBeenCalled());
      pending.resolve({ config: makeConfig() });
      accepted = await saving;
    });
    expect(accepted).toBe(false);
    expect(result.current.form.branch).toBe("edited-after-save");
    expect(result.current.config?.branch).toBe("main");
  });

  it("re-parses a pasted Enterprise link once the host is known", async () => {
    const { result, rerender } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setUrlInput("https://git.company.example/acme/eng"));
    expect(result.current.form.repo_owner).toBe("");

    mocks.host = "git.company.example";
    rerender();
    await waitFor(() => expect(result.current.form.repo_owner).toBe("acme"));
    expect(result.current.form.repo_name).toBe("eng");
    expect(result.current.urlInvalid).toBe(false);
  });

  it("clears the repository when the link input is emptied", async () => {
    const { result } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setUrlInput("https://github.com/acme/eng"));
    expect(result.current.form.repo_owner).toBe("acme");
    act(() => result.current.update("branch", "release"));
    act(() => result.current.setUrlInput(""));
    expect(result.current.form.repo_owner).toBe("");
    expect(result.current.form.repo_name).toBe("");
    expect(result.current.form.branch).toBe("release");
    expect(result.current.form.workspace_id).toBe("ws-1");
  });

  it("does not overwrite edits when the host arrives after a parsed link", async () => {
    const { result, rerender } = renderHook(() => usePromptSync("ws-1"));
    await waitFor(() => expect(result.current.loading).toBe(false));
    act(() => result.current.setUrlInput("https://github.com/acme/eng/tree/main/docs"));
    act(() => result.current.update("branch", "edited"));
    mocks.host = "github.com";
    rerender();
    expect(result.current.form.branch).toBe("edited");
  });
});
