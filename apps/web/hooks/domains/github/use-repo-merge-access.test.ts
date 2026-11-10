import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { createElement, startTransition, Suspense, useLayoutEffect, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GitHubStatus, RepoMergeAccess } from "@/lib/types/github";
import { invalidateIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { useRepoMergeAccess } from "./use-repo-merge-access";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  status: null as GitHubStatus | null,
  auth: { user: { id: "user-1" }, mode: "local", authenticated: true },
}));
vi.mock("@/lib/api/domains/github-api", () => ({ getRepoMergeAccess: mocks.request }));
vi.mock("./use-github-status", () => ({ useGitHubStatus: () => ({ status: mocks.status }) }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { auth: typeof mocks.auth }) => unknown) => selector(mocks),
}));

const allowed: RepoMergeAccess = {
  state: "allowed",
  actor: { kind: "human", source: "pat", login: "writer", workspace_id: "ws-1" },
};
const denied: RepoMergeAccess = { ...allowed, state: "denied" };

function pendingAccess() {
  let resolve!: (value: RepoMergeAccess) => void;
  const promise = new Promise<RepoMergeAccess>((done) => (resolve = done));
  return { promise, resolve };
}

beforeEach(() => {
  mocks.request.mockReset();
  mocks.status = null;
  mocks.auth.user.id = "user-1";
});
afterEach(cleanup);

describe("useRepoMergeAccess", () => {
  it("does not request access without a workspace or when disabled", () => {
    renderHook(() => useRepoMergeAccess(null, "upstream", "site"));
    renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site", false));
    expect(mocks.request).not.toHaveBeenCalled();
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.17
  it("blocks while loading and lets a read-only retry recover from failure", async () => {
    mocks.request.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(allowed);
    const { result } = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
    expect(result.current.state).toBe("loading");
    await waitFor(() => expect(result.current.state).toBe("unknown"));
    act(() => result.current.refresh());
    expect(result.current.state).toBe("loading");
    await waitFor(() => expect(result.current.state).toBe("allowed"));
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.18
  it.each(["workspace", "repository", "host", "actor", "credential", "user"])(
    "discards permission responses from a previous %s context",
    async (change) => {
      const old = pendingAccess();
      const current = pendingAccess();
      mocks.request.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
      const { result, rerender } = renderHook(
        ({ workspace, owner, host }) => useRepoMergeAccess(workspace, owner, "site", true, host),
        { initialProps: { workspace: "ws-1", owner: "upstream", host: "git.company.example" } },
      );
      const status = { authenticated: true } as GitHubStatus;
      if (change === "actor")
        status.effective_manual_mutation_actor = { ...allowed.actor!, login: "reader" };
      if (change === "credential")
        status.automation = { credential_generation: 2 } as GitHubStatus["automation"];
      if (change === "actor" || change === "credential") mocks.status = status;
      if (change === "user") mocks.auth.user.id = "user-2";
      rerender({
        workspace: change === "workspace" ? "ws-2" : "ws-1",
        owner: change === "repository" ? "another" : "upstream",
        host: change === "host" ? "github.com" : "git.company.example",
      });
      expect(result.current.state).toBe("loading");
      expect(mocks.request.mock.calls[0][3].init.signal.aborted).toBe(true);
      await act(async () => old.resolve(allowed));
      expect(result.current.state).toBe("loading");
      await act(async () =>
        current.resolve({
          ...denied,
          actor: { ...denied.actor!, workspace_id: change === "workspace" ? "ws-2" : "ws-1" },
        }),
      );
      expect(result.current.state).toBe("denied");
    },
  );

  it.each(["focus", "connection"])(
    "revokes a previous grant during a %s refresh",
    async (event) => {
      const next = pendingAccess();
      mocks.request.mockResolvedValueOnce(allowed).mockReturnValueOnce(next.promise);
      const { result } = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
      await waitFor(() => expect(result.current.state).toBe("allowed"));
      act(() => {
        if (event === "focus") window.dispatchEvent(new Event("focus"));
        else invalidateIntegrationAvailability();
      });
      expect(result.current.state).toBe("loading");
      await act(async () => next.resolve(denied));
      expect(result.current.state).toBe("denied");
    },
  );

  it("does not cache grants across mounts or accept responses after unmount", async () => {
    const late = pendingAccess();
    mocks.request.mockReturnValueOnce(late.promise).mockResolvedValueOnce(denied);
    const first = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
    first.unmount();
    await act(async () => late.resolve(allowed));
    const second = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
    expect(second.result.current.state).toBe("loading");
    await waitFor(() => expect(second.result.current.state).toBe("denied"));
    expect(mocks.request).toHaveBeenCalledTimes(2);
  });

  it("clears a previous grant when credentials change", async () => {
    mocks.request.mockResolvedValueOnce(allowed).mockReturnValueOnce(pendingAccess().promise);
    const { result, rerender } = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
    await waitFor(() => expect(result.current.state).toBe("allowed"));
    mocks.status = { authenticated: true, personal: { credential_generation: 2 } } as GitHubStatus;
    rerender();
    expect(result.current.state).toBe("loading");
    expect(result.current.actor).toBeNull();
  });

  it.each([null, { ...allowed.actor!, workspace_id: "wrong-workspace" }])(
    "rejects an unbound actor response",
    async (actor) => {
      mocks.request.mockResolvedValueOnce({ ...allowed, actor });
      const { result } = renderHook(() => useRepoMergeAccess("ws-1", "upstream", "site"));
      await waitFor(() => expect(result.current.state).toBe("unknown"));
    },
  );
});

describe("useRepoMergeAccess committed lifetime", () => {
  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.18
  it("requires fresh access in the first committed render after re-enabling", async () => {
    const next = pendingAccess();
    const committed: ReturnType<typeof useRepoMergeAccess>[] = [];
    mocks.request.mockResolvedValueOnce(allowed).mockReturnValueOnce(next.promise);
    const { result, rerender } = renderHook(
      ({ enabled }) => {
        const access = useRepoMergeAccess("ws-1", "upstream", "site", enabled);
        useLayoutEffect(() => {
          committed.push(access);
        });
        return access;
      },
      { initialProps: { enabled: true } },
    );
    await waitFor(() => expect(result.current.state).toBe("allowed"));
    rerender({ enabled: false });
    expect(result.current.state).toBe("unknown");
    committed.length = 0;
    rerender({ enabled: true });
    expect(committed[0]).toMatchObject({ state: "loading", actor: null });
    await act(async () => next.resolve(denied));
    expect(result.current.state).toBe("denied");
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.18
  it("keeps the committed lookup and retry active during a suspended render", async () => {
    const initial = pendingAccess();
    const retry = pendingAccess();
    const suspension = pendingAccess();
    mocks.request.mockReturnValueOnce(initial.promise).mockReturnValueOnce(retry.promise);
    let select!: (owner: string) => void;
    let current!: ReturnType<typeof useRepoMergeAccess>;
    let suspended = false;
    function Consumer({ owner }: { owner: string }) {
      const access = useRepoMergeAccess("ws-1", owner, "site");
      useLayoutEffect(() => {
        current = access;
      });
      if (owner === "speculative") {
        suspended = true;
        throw suspension.promise;
      }
      return createElement("span", null, owner);
    }
    function Harness() {
      const [owner, setOwner] = useState("upstream");
      useLayoutEffect(() => {
        select = setOwner;
      }, []);
      return createElement(Suspense, { fallback: null }, createElement(Consumer, { owner }));
    }
    const view = render(createElement(Harness));
    act(() => startTransition(() => select("speculative")));
    expect(suspended).toBe(true);
    expect(view.container.textContent).toBe("upstream");
    await act(async () => initial.resolve(allowed));
    expect(current.state).toBe("allowed");
    act(() => current.refresh());
    expect(mocks.request).toHaveBeenCalledTimes(2);
    expect(current.state).toBe("loading");
    await act(async () => retry.resolve(denied));
    expect(current.state).toBe("denied");
    act(() => select("upstream"));
    await act(async () => suspension.resolve(denied));
  });
});
