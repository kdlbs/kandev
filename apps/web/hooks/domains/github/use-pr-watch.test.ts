import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { usePRWatches } from "./use-pr-watch";

const mocks = vi.hoisted(() => ({
  listPRWatches: vi.fn(),
  deletePRWatch: vi.fn(),
}));

vi.mock("@/lib/api/domains/github-api", () => mocks);

afterEach(() => {
  mocks.listPRWatches.mockReset();
  mocks.deletePRWatch.mockReset();
});

function wrapper({ children }: { children: ReactNode }) {
  const initialState = {
    workspaces: { activeId: "workspace-1" },
  } as unknown as Partial<AppState>;
  return createElement(StateProvider, { initialState, children });
}

function unresolvedWorkspaceWrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

describe("usePRWatches", () => {
  it("scopes watch list and deletion requests to the active workspace", async () => {
    mocks.listPRWatches.mockResolvedValue({ watches: [] });
    mocks.deletePRWatch.mockResolvedValue({ success: true });

    const { result } = renderHook(() => usePRWatches(), { wrapper });

    await waitFor(() =>
      expect(mocks.listPRWatches).toHaveBeenCalledWith("workspace-1", { cache: "no-store" }),
    );
    await result.current.remove("watch-1");
    expect(mocks.deletePRWatch).toHaveBeenCalledWith("watch-1", "workspace-1");
  });

  it("does not make unscoped requests before a workspace is active", () => {
    renderHook(() => usePRWatches(), { wrapper: unresolvedWorkspaceWrapper });
    expect(mocks.listPRWatches).not.toHaveBeenCalled();
  });
});
