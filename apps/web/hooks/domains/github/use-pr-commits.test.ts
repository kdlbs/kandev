import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { PRCommitInfo } from "@/lib/types/github";
import { resolvePRCommitsView, usePRCommits, type KeyedPRCommitsState } from "./use-pr-commits";

const requestMock = vi.hoisted(() => vi.fn());
let websocketClient: { request: typeof requestMock } | null = { request: requestMock };
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => websocketClient,
}));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  requestMock.mockReset();
  websocketClient = { request: requestMock };
});

function wrapper({ children }: { children: ReactNode }) {
  const initialState = {
    workspaces: { activeId: "workspace-1" },
  } as unknown as Partial<AppState>;
  return createElement(StateProvider, { initialState, children });
}

function commit(sha: string): PRCommitInfo {
  return {
    sha,
    message: sha,
    author_login: "octocat",
    author_date: "2026-08-04T12:00:00Z",
    additions: 1,
    deletions: 0,
    files_changed: 1,
    stats_available: false,
  };
}

describe("usePRCommits request ownership", () => {
  it("masks state unless the complete workspace and PR source key matches", () => {
    const staleState: KeyedPRCommitsState = {
      sourceKey: "workspace-1/acme/app/1/old-refresh",
      commits: [commit("first-sha")],
      providerHead: "first-sha",
      providerCommitsComplete: true,
      loading: false,
      error: null,
    };

    expect(resolvePRCommitsView(staleState, "workspace-1/acme/other-app/2/new-refresh")).toEqual({
      commits: [],
      providerHead: null,
      providerCommitsComplete: false,
      loading: true,
      error: null,
    });
  });

  it("masks the previous PR while a rapid switch is loading", async () => {
    const first = deferred<{ commits: PRCommitInfo[] }>();
    const second = deferred<{ commits: PRCommitInfo[] }>();
    requestMock.mockImplementation((_action: string, payload: { number: number }) =>
      payload.number === 1 ? first.promise : second.promise,
    );

    const { result, rerender } = renderHook(
      ({ number }) => usePRCommits("acme", "app", number, "synced"),
      { initialProps: { number: 1 }, wrapper },
    );

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(1));
    rerender({ number: 2 });

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    await act(async () => {
      second.resolve({ commits: [commit("second-sha")] });
      await second.promise;
    });
    await waitFor(() => expect(result.current.commits[0]?.sha).toBe("second-sha"));

    await act(async () => {
      first.resolve({ commits: [commit("late-first-sha")] });
      await first.promise;
    });

    expect(result.current.commits[0]?.sha).toBe("second-sha");
  });

  it("exposes the provider head and commit-list completeness", async () => {
    requestMock.mockResolvedValue({
      commits: [commit("provider-head")],
      head_sha: "provider-head",
      complete: true,
    });

    const { result } = renderHook(() => usePRCommits("acme", "app", 1, "synced"), { wrapper });

    await waitFor(() => expect(result.current.providerHead).toBe("provider-head"));
    expect(result.current.providerCommitsComplete).toBe(true);
  });
});
