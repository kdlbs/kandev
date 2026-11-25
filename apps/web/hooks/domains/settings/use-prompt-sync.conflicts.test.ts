import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import type { PromptSyncConfig, PromptSyncConflict } from "@/lib/types/prompt-sync";

const mocks = vi.hoisted(() => ({
  get: vi.fn<() => Promise<PromptSyncConfig | null>>(),
  set: vi.fn<(payload: unknown) => Promise<unknown>>(),
  check: vi.fn<(payload?: unknown) => Promise<unknown>>(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));
vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  getPromptSyncConfig: () => mocks.get(),
  setPromptSyncConfig: (payload: unknown) => mocks.set(payload),
  checkPromptSync: (payload?: unknown) => mocks.check(payload),
  deletePromptSyncConfig: vi.fn(),
  runPromptSync: vi.fn(),
}));

import { usePromptSync } from "./use-prompt-sync";

const PROMPTS_PATH = ".kandev/prompts";
const PROJECT = "acme/prompts";

const config: PromptSyncConfig = {
  workspace_id: "ws-1",
  provider: "gitlab",
  repo_owner: "",
  repo_name: "",
  project_path: PROJECT,
  branch: "main",
  path: PROMPTS_PATH,
  interval_seconds: 300,
  poll_enabled: true,
  last_ok: true,
  created_at: "2026-11-20T00:00:00Z",
  updated_at: "2026-11-20T00:00:00Z",
};

const conflict = (name: string): PromptSyncConflict => ({
  name,
  path: `${PROMPTS_PATH}/${name}.md`,
  local_content: "Local",
  repository_content: "Repo",
});

async function renderReady() {
  const hook = renderHook(() => usePromptSync("ws-1"));
  await waitFor(() => expect(hook.result.current.loading).toBe(false));
  act(() => hook.result.current.setProvider("gitlab"));
  act(() => hook.result.current.setUrlInput(PROJECT));
  return hook;
}

async function save(result: { current: ReturnType<typeof usePromptSync> }) {
  let saved = true;
  await act(async () => {
    saved = await result.current.handleSave();
  });
  return saved;
}

describe("usePromptSync conflict review", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockResolvedValue(null);
    mocks.check.mockResolvedValue({ conflicts: [] });
    mocks.set.mockResolvedValue({ config, result: { conflicts: [] } });
  });

  it("checks the source before saving and saves straight away without conflicts", async () => {
    const { result } = await renderReady();
    expect(await save(result)).toBe(true);
    expect(mocks.check).toHaveBeenCalledWith(expect.objectContaining({ project_path: PROJECT }));
    expect(mocks.set).toHaveBeenCalledWith(expect.objectContaining({ resolutions: {} }));
    expect(mocks.check.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.set.mock.invocationCallOrder[0]!,
    );
  });

  it("stops on the review step when the check finds conflicts, then saves the choices", async () => {
    mocks.check.mockResolvedValueOnce({ conflicts: [conflict("principles"), conflict("notes")] });
    const { result } = await renderReady();
    expect(await save(result)).toBe(false);
    expect(mocks.set).not.toHaveBeenCalled();
    expect(result.current.review.active).toBe(true);
    expect(result.current.review.choices).toEqual({ principles: "local", notes: "local" });

    act(() => result.current.review.setChoice("notes", "repository"));
    expect(await save(result)).toBe(true);
    expect(mocks.check).toHaveBeenCalledTimes(1);
    expect(mocks.set).toHaveBeenCalledWith(
      expect.objectContaining({ resolutions: { principles: "local", notes: "repository" } }),
    );
    expect(result.current.review.active).toBe(false);
  });

  it("saves nothing when the check fails", async () => {
    mocks.check.mockRejectedValueOnce(new Error("Repository not found"));
    const { result } = await renderReady();
    expect(await save(result)).toBe(false);
    expect(mocks.set).not.toHaveBeenCalled();
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("returns to the review step for conflicts the sync found after the check", async () => {
    mocks.set.mockResolvedValueOnce({ config, result: { conflicts: ["late"] } });
    mocks.check
      .mockResolvedValueOnce({ conflicts: [] })
      .mockResolvedValueOnce({ conflicts: [conflict("late")] });
    const { result } = await renderReady();
    expect(await save(result)).toBe(false);
    expect(mocks.check).toHaveBeenLastCalledWith(undefined);
    expect(result.current.config?.project_path).toBe(PROJECT);
    expect(result.current.review.conflicts.map((item) => item.name)).toEqual(["late"]);
  });

  it("goes back to the fields with the draft intact", async () => {
    mocks.check.mockResolvedValueOnce({ conflicts: [conflict("principles")] });
    const { result } = await renderReady();
    await save(result);
    act(() => result.current.backToFields());
    expect(result.current.review.active).toBe(false);
    expect(result.current.form.project_path).toBe(PROJECT);
  });

  it("keeps the saved configuration when the sync after saving fails", async () => {
    mocks.set.mockResolvedValueOnce({
      config: { ...config, last_ok: false },
      error: "Prompt sync failed",
    });
    const { result } = await renderReady();
    expect(await save(result)).toBe(true);
    expect(result.current.config?.last_ok).toBe(false);
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });
});
