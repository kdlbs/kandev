import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { PromptSyncConflict } from "@/lib/types/prompt-sync";

const mocks = vi.hoisted(() => ({
  check: vi.fn<(payload?: unknown) => Promise<unknown>>(),
  run: vi.fn<(resolutions?: unknown) => Promise<unknown>>(),
  toast: vi.fn(),
  refresh: vi.fn(),
  store: { id: "store" },
}));

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/components/state-provider", () => ({ useAppStoreApi: () => mocks.store }));
vi.mock("@/lib/state/prompts-loader", () => ({
  refreshPrompts: (store: unknown) => mocks.refresh(store),
}));
vi.mock("@/lib/api/domains/prompt-sync-api", () => ({
  checkPromptSync: (payload?: unknown) => mocks.check(payload),
  runPromptSync: (resolutions?: unknown) => mocks.run(resolutions),
}));

import { usePromptConflictResolver } from "./use-prompt-conflict-resolver";

const conflict = (name: string): PromptSyncConflict => ({
  name,
  path: `.kandev/prompts/${name}.md`,
  local_content: "Local",
  repository_content: "Repo",
});
const config = { workspace_id: "ws-1" };

describe("usePromptConflictResolver", () => {
  beforeEach(() => vi.clearAllMocks());

  it("loads the saved configuration's conflicts and applies the choices with a sync", async () => {
    mocks.check.mockResolvedValue({ conflicts: [conflict("a"), conflict("b")] });
    mocks.run.mockResolvedValue({ config, result: { conflicts: [] } });
    const { result } = renderHook(() => usePromptConflictResolver());

    let opened = false;
    await act(async () => {
      opened = await result.current.load();
    });
    expect(opened).toBe(true);
    expect(mocks.check).toHaveBeenCalledWith(undefined);
    act(() => result.current.review.setChoice("b", "repository"));

    let done = false;
    await act(async () => {
      done = await result.current.apply();
    });
    expect(done).toBe(true);
    expect(mocks.run).toHaveBeenCalledWith({ a: "local", b: "repository" });
    expect(result.current.review.active).toBe(false);
  });

  it("stays open with the error when the sync fails", async () => {
    mocks.check.mockResolvedValue({ conflicts: [conflict("a")] });
    mocks.run.mockResolvedValue({ config, error: "Prompt sync failed" });
    const { result } = renderHook(() => usePromptConflictResolver());
    await act(async () => {
      await result.current.load();
    });
    let done = true;
    await act(async () => {
      done = await result.current.apply();
    });
    expect(done).toBe(false);
    expect(result.current.review.active).toBe(true);
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("reports a failed check and opens nothing", async () => {
    mocks.check.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => usePromptConflictResolver());
    let opened = true;
    await act(async () => {
      opened = await result.current.load();
    });
    expect(opened).toBe(false);
    expect(result.current.review.active).toBe(false);
    expect(mocks.toast).toHaveBeenLastCalledWith(expect.objectContaining({ variant: "error" }));
  });

  it("refreshes stale conflict flags when the repository reports none", async () => {
    mocks.check.mockResolvedValue({ conflicts: [] });
    const { result } = renderHook(() => usePromptConflictResolver());
    let opened = true;
    await act(async () => {
      opened = await result.current.load();
    });
    expect(opened).toBe(false);
    expect(mocks.refresh).toHaveBeenCalledWith(mocks.store);
  });
});
