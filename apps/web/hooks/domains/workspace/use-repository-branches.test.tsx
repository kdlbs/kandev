import { act, cleanup, render, renderHook, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { BranchPickerList } from "@/components/task/branch-picker-list";
import type { HydrationState } from "@/lib/state/store";
import type { Branch } from "@/lib/types/http";
import { useBranches, type BranchSource, type UseBranchesResult } from "./use-repository-branches";

const { listBranchesMock, refreshMock } = vi.hoisted(() => ({
  listBranchesMock: vi.fn(),
  refreshMock: vi.fn(),
}));
vi.mock("@/lib/api", () => ({
  listBranches: listBranchesMock,
  listRepositoryBranches: refreshMock,
}));

const WORKSPACE_ID = "workspace-1";
const NEW_BRANCH = "new-branch";
const SOURCE: BranchSource = { kind: "id", workspaceId: WORKSPACE_ID, repositoryId: "repo-a" };
const branches = (name: string): Branch[] => [{ name, type: "local" }];
const response = (name: string) => ({ branches: branches(name) });
type Response = { branches: Branch[] };

function deferred() {
  let resolve!: (value: Response) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Response>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function cachedState(): HydrationState {
  return {
    repositoryBranches: {
      itemsByRepositoryId: { "repo-a": branches("cached") },
      loadedByRepositoryId: { "repo-a": true },
      loadingByRepositoryId: {},
      fetchedAtByRepositoryId: {},
      fetchErrorByRepositoryId: {},
    },
  };
}

function provider(initialState?: HydrationState) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <StateProvider initialState={initialState}>{children}</StateProvider>;
  };
}

function mount(
  source: BranchSource | null = SOURCE,
  enabled = true,
  initialState?: HydrationState,
) {
  return renderHook(
    ({ source, enabled }) => ({ value: useBranches(source, enabled), store: useAppStoreApi() }),
    { wrapper: provider(initialState), initialProps: { source, enabled } },
  );
}

function refresh(value: UseBranchesResult) {
  let pending!: Promise<void>;
  act(() => {
    pending = value.refresh!();
  });
  return pending;
}

async function succeed(reply: ReturnType<typeof deferred>, name: string, pending?: Promise<void>) {
  await act(async () => {
    reply.resolve(response(name));
    await (pending ?? reply.promise);
  });
}

async function fail(reply: ReturnType<typeof deferred>, pending: Promise<void>) {
  await act(async () => {
    reply.reject(new Error("network failure"));
    await pending;
  });
}

beforeEach(() => {
  listBranchesMock.mockReset().mockResolvedValue({ branches: [] });
  refreshMock.mockReset().mockResolvedValue({ branches: [] });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

// @covers AC-WORKSPACES-BRANCH-READS-001.1, AC-WORKSPACES-BRANCH-READS-001.2
describe("shared branch request ordering", () => {
  it("keeps refresh results after an older initial load", async () => {
    const initial = deferred(),
      latest = deferred();
    listBranchesMock.mockReturnValueOnce(initial.promise);
    refreshMock.mockReturnValueOnce(latest.promise);
    const { result } = mount();
    const pending = refresh(result.current.value);
    await succeed(latest, NEW_BRANCH, pending);
    await succeed(initial, "old-branch");
    expect(result.current.value).toMatchObject({
      branches: branches(NEW_BRANCH),
      isLoaded: true,
      isLoading: false,
    });
    expect(
      result.current.store.getState().repositoryBranches.itemsByRepositoryId["repo-a"],
    ).toEqual(branches(NEW_BRANCH));
  });

  it("keeps the newest of two refreshes", async () => {
    const older = deferred(),
      newer = deferred();
    refreshMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    const { result } = mount(SOURCE, false, cachedState());
    const first = refresh(result.current.value),
      second = refresh(result.current.value);
    await succeed(newer, "latest", second);
    await succeed(older, "obsolete", first);
    expect(result.current.value.branches).toEqual(branches("latest"));
  });

  it.each(["success", "failure"])(
    "keeps loading while an older refresh finishes with %s",
    async (outcome) => {
      const older = deferred(),
        newer = deferred();
      refreshMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
      const { result } = mount(SOURCE, false, cachedState());
      const first = refresh(result.current.value),
        second = refresh(result.current.value);
      if (outcome === "success") await succeed(older, "obsolete", first);
      else await fail(older, first);
      const beforeNewest = result.current.value;
      await succeed(newer, "latest", second);
      expect(beforeNewest).toMatchObject({ isLoading: true, branches: branches("cached") });
      expect(result.current.value.isLoading).toBe(false);
    },
  );

  it("shares initial loading and refresh acceptance across sibling consumers", async () => {
    const initial = deferred(),
      latest = deferred();
    listBranchesMock.mockReturnValue(initial.promise);
    refreshMock.mockReturnValueOnce(latest.promise);
    const { result } = renderHook(
      () => ({ first: useBranches(SOURCE), second: useBranches(SOURCE) }),
      { wrapper: provider() },
    );
    const initialCalls = listBranchesMock.mock.calls.length;
    const pending = refresh(result.current.second);
    await succeed(latest, "latest", pending);
    await succeed(initial, "obsolete");
    expect(initialCalls).toBe(1);
    expect(result.current.first).toMatchObject({
      branches: branches("latest"),
      isLoaded: true,
      isLoading: false,
    });
    expect(result.current.second).toMatchObject({
      branches: branches("latest"),
      isLoaded: true,
      isLoading: false,
    });
  });

  it("orders refreshes started by different consumers of one source", async () => {
    const older = deferred(),
      newer = deferred();
    refreshMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    const { result } = renderHook(
      () => ({ first: useBranches(SOURCE), second: useBranches(SOURCE) }),
      { wrapper: provider(cachedState()) },
    );
    const first = refresh(result.current.first),
      second = refresh(result.current.second);
    await succeed(older, "obsolete", first);
    const loading = [result.current.first.isLoading, result.current.second.isLoading];
    await succeed(newer, "latest", second);
    expect(loading).toEqual([true, true]);
    expect(result.current.first.branches).toEqual(branches("latest"));
    expect(result.current.second.branches).toEqual(branches("latest"));
  });
});

describe("branch publication ownership", () => {
  it("does not clear a refresh started synchronously by a store subscriber during publication", async () => {
    const firstReply = deferred(),
      reentrantReply = deferred();
    refreshMock.mockReturnValueOnce(firstReply.promise).mockReturnValueOnce(reentrantReply.promise);
    const { result } = mount(SOURCE, false, cachedState());
    let reentrant: Promise<void> | undefined;
    const unsubscribe = result.current.store.subscribe((state) => {
      if (state.repositoryBranches.itemsByRepositoryId["repo-a"]?.[0]?.name !== "first") return;
      unsubscribe();
      reentrant = result.current.value.refresh!();
    });
    const first = refresh(result.current.value);
    await succeed(firstReply, "first", first);
    const loadingAfterPublication = result.current.value.isLoading;
    expect(reentrant).toBeDefined();
    await succeed(reentrantReply, "reentrant", reentrant);
    expect(loadingAfterPublication).toBe(true);
    expect(result.current.value).toMatchObject({
      branches: branches("reentrant"),
      isLoading: false,
    });
    expect(refreshMock).toHaveBeenCalledTimes(2);
  });
});

// @covers AC-WORKSPACES-BRANCH-READS-001.3, AC-WORKSPACES-BRANCH-READS-001.4
describe("branch read outcomes", () => {
  it.each([true, false])(
    "keeps accepted state after newest failure, with prior cache %s",
    async (cached) => {
      const older = deferred(),
        newer = deferred(),
        retry = deferred();
      refreshMock
        .mockReturnValueOnce(older.promise)
        .mockReturnValueOnce(newer.promise)
        .mockReturnValueOnce(retry.promise);
      const { result } = mount(SOURCE, false, cached ? cachedState() : undefined);
      const first = refresh(result.current.value),
        second = refresh(result.current.value);
      await fail(newer, second);
      const afterFailure = result.current.value;
      await succeed(older, "obsolete", first);
      const afterObsolete = result.current.value;
      const third = refresh(result.current.value);
      await succeed(retry, "retry", third);
      const expected = {
        branches: cached ? branches("cached") : [],
        isLoaded: cached,
        isLoading: false,
      };
      expect(afterFailure).toMatchObject(expected);
      expect(afterObsolete).toMatchObject(expected);
      expect(result.current.value).toMatchObject({
        branches: branches("retry"),
        isLoaded: true,
        isLoading: false,
      });
    },
  );

  it.each(["initial", "refresh"])("accepts an empty %s response as loaded", async (mode) => {
    const reply = deferred();
    if (mode === "initial") listBranchesMock.mockReturnValueOnce(reply.promise);
    else refreshMock.mockReturnValueOnce(reply.promise);
    const { result } = mount(
      SOURCE,
      mode === "initial",
      mode === "refresh" ? cachedState() : undefined,
    );
    const pending = mode === "refresh" ? refresh(result.current.value) : undefined;
    await act(async () => {
      reply.resolve({ branches: [] });
      await (pending ?? reply.promise);
    });
    expect(result.current.value).toMatchObject({ branches: [], isLoaded: true, isLoading: false });
  });

  it("does not repopulate an empty refresh from an obsolete success", async () => {
    const older = deferred(),
      newer = deferred();
    refreshMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
    const { result } = mount(SOURCE, false, cachedState());
    const first = refresh(result.current.value),
      second = refresh(result.current.value);
    await act(async () => {
      newer.resolve({ branches: [] });
      await second;
    });
    await succeed(older, "obsolete", first);
    expect(result.current.value).toMatchObject({ branches: [], isLoaded: true, isLoading: false });
  });

  it("completes ordinary initial loading and a subsequent refresh", async () => {
    const initial = deferred(),
      latest = deferred();
    listBranchesMock.mockReturnValueOnce(initial.promise);
    refreshMock.mockReturnValueOnce(latest.promise);
    const { result } = mount();
    expect(result.current.value.isLoading).toBe(true);
    await succeed(initial, "initial");
    expect(result.current.value).toMatchObject({
      branches: branches("initial"),
      isLoaded: true,
      isLoading: false,
    });
    const pending = refresh(result.current.value);
    await succeed(latest, "refreshed", pending);
    expect(result.current.value).toMatchObject({
      branches: branches("refreshed"),
      isLoaded: true,
      isLoading: false,
    });
    expect(refreshMock).toHaveBeenCalledWith("repo-a", { refresh: true });
  });
});

// @covers AC-WORKSPACES-BRANCH-READS-001.5
describe("branch source isolation", () => {
  const cases: [string, BranchSource, string][] = [
    ["another repository", { ...SOURCE, kind: "id", repositoryId: "repo-b" }, "repo-b"],
    [
      "a path",
      { kind: "path", workspaceId: WORKSPACE_ID, path: "/repo" },
      "path::workspace-1::/repo",
    ],
  ];
  it.each(cases)("loads %s while the prior source remains pending", async (_, other, otherKey) => {
    const prior = deferred(),
      next = deferred();
    listBranchesMock.mockReturnValueOnce(prior.promise).mockReturnValueOnce(next.promise);
    const { result, rerender } = mount();
    rerender({ source: other, enabled: true });
    await succeed(next, "other");
    const cacheWhilePriorPending = result.current.store.getState().repositoryBranches;
    await succeed(prior, "prior");
    expect(cacheWhilePriorPending.loadingByRepositoryId["repo-a"]).toBe(true);
    expect(cacheWhilePriorPending.loadingByRepositoryId[otherKey]).toBe(false);
    expect(result.current.value.branches).toEqual(branches("other"));
    expect(
      result.current.store.getState().repositoryBranches.itemsByRepositoryId["repo-a"],
    ).toEqual(branches("prior"));
    expect(listBranchesMock).toHaveBeenNthCalledWith(
      2,
      other.workspaceId,
      other.kind === "id" ? { repositoryId: other.repositoryId } : { path: other.path },
    );
  });

  it.each([
    ["distinct paths", WORKSPACE_ID, "/another"],
    ["the same path in another workspace", "workspace-2", "/repo"],
  ])("isolates refreshes for %s", async (_, workspaceId, path) => {
    const left: BranchSource = { kind: "path", workspaceId: WORKSPACE_ID, path: "/repo" };
    const right: BranchSource = { kind: "path", workspaceId, path };
    const a = deferred(),
      b = deferred();
    listBranchesMock.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const { result } = renderHook(
      () => ({
        left: useBranches(left, false),
        right: useBranches(right, false),
        store: useAppStoreApi(),
      }),
      { wrapper: provider() },
    );
    const first = refresh(result.current.left),
      second = refresh(result.current.right);
    await succeed(b, "right", second);
    const leftLoading = result.current.left.isLoading;
    await succeed(a, "left", first);
    expect(leftLoading).toBe(true);
    expect(result.current.left.branches).toEqual(branches("left"));
    expect(result.current.right.branches).toEqual(branches("right"));
    expect(
      result.current.store.getState().repositoryBranches.itemsByRepositoryId[
        `path::${workspaceId}::${path}`
      ],
    ).toEqual(branches("right"));
    expect(listBranchesMock).toHaveBeenNthCalledWith(2, workspaceId, { path });
    expect(refreshMock).not.toHaveBeenCalled();
  });

  it("isolates the same source in independent production stores", async () => {
    const a = deferred(),
      b = deferred();
    refreshMock.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const left = mount(SOURCE, false),
      right = mount(SOURCE, false);
    const first = refresh(left.result.current.value),
      second = refresh(right.result.current.value);
    await succeed(b, "right", second);
    const leftLoading = left.result.current.value.isLoading;
    await succeed(a, "left", first);
    expect(left.result.current.store).not.toBe(right.result.current.store);
    expect(leftLoading).toBe(true);
    expect(left.result.current.value.branches).toEqual(branches("left"));
    expect(right.result.current.value.branches).toEqual(branches("right"));
  });
});

// @covers AC-WORKSPACES-BRANCH-READS-001.6
describe("branch load demand and retry compatibility", () => {
  it("does not load a null, disabled, or already cached source", () => {
    const absent = mount(null),
      disabled = mount(SOURCE, false),
      cached = mount(SOURCE, true, cachedState());
    expect(absent.result.current.value.refresh).toBeUndefined();
    expect(disabled.result.current.value).toMatchObject({
      branches: [],
      isLoaded: false,
      isLoading: false,
    });
    expect(cached.result.current.value.branches).toEqual(branches("cached"));
    expect(listBranchesMock).not.toHaveBeenCalled();
  });

  it("loads on enable and retains repository-ID identity across workspace argument changes", async () => {
    const reply = deferred();
    listBranchesMock.mockReturnValueOnce(reply.promise);
    const { result, rerender } = mount(SOURCE, false);
    rerender({ source: SOURCE, enabled: true });
    await succeed(reply, "loaded");
    rerender({ source: { ...SOURCE, workspaceId: "workspace-2" }, enabled: true });
    expect(result.current.value.branches).toEqual(branches("loaded"));
    expect(listBranchesMock).toHaveBeenCalledExactlyOnceWith(WORKSPACE_ID, {
      repositoryId: "repo-a",
    });
  });

  it("retries a transient branch-list failure before accepting the result", async () => {
    vi.useFakeTimers();
    listBranchesMock
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockResolvedValueOnce(response("recovered"));
    const { result } = mount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(99);
    });
    expect(listBranchesMock).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(listBranchesMock).toHaveBeenCalledTimes(2);
    expect(result.current.value).toMatchObject({
      branches: branches("recovered"),
      isLoaded: true,
      isLoading: false,
    });
  });

  it("exhausts the existing initial retries without a continuous failure loop, then retries on demand", async () => {
    vi.useFakeTimers();
    listBranchesMock.mockRejectedValue(new Error("network failure"));
    const { result, rerender } = mount();
    for (const [index, delay] of [100, 250, 500, 1000].entries()) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(delay - 1);
      });
      expect(listBranchesMock).toHaveBeenCalledTimes(index + 1);
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      expect(listBranchesMock).toHaveBeenCalledTimes(index + 2);
    }
    expect(result.current.value).toMatchObject({ branches: [], isLoaded: false, isLoading: false });
    expect(vi.getTimerCount()).toBe(0);
    listBranchesMock.mockResolvedValueOnce(response("retried"));
    rerender({ source: SOURCE, enabled: false });
    await act(async () => {
      rerender({ source: SOURCE, enabled: true });
    });
    expect(result.current.value).toMatchObject({
      branches: branches("retried"),
      isLoaded: true,
      isLoading: false,
    });
    expect(listBranchesMock).toHaveBeenCalledTimes(6);
  });
});

// @covers AC-WORKSPACES-BRANCH-READS-001.7
describe("shared read lifetime", () => {
  it("shares an initial request across StrictMode effect replay", async () => {
    const reply = deferred();
    listBranchesMock.mockReturnValue(reply.promise);
    const wrapper = ({ children }: { children: ReactNode }) => (
      <StrictMode>
        <StateProvider>{children}</StateProvider>
      </StrictMode>
    );
    const { result } = renderHook(() => useBranches(SOURCE), { wrapper });
    await succeed(reply, "loaded");
    expect(listBranchesMock).toHaveBeenCalledTimes(1);
    expect(result.current).toMatchObject({ branches: branches("loaded"), isLoading: false });
  });

  it("finishes the shared cache after initiator unmount and reuses it on remount", async () => {
    const reply = deferred();
    listBranchesMock.mockReturnValue(reply.promise);
    let sibling!: UseBranchesResult;
    function Reader({ report }: { report?: (value: UseBranchesResult) => void }) {
      const value = useBranches(SOURCE);
      report?.(value);
      return null;
    }
    const view = (showInitiator: boolean) => (
      <StateProvider>
        {showInitiator && <Reader key="initiator" />}
        <Reader
          key="sibling"
          report={(value) => {
            sibling = value;
          }}
        />
      </StateProvider>
    );
    const { rerender } = render(view(true));
    rerender(view(false));
    const loadingAfterUnmount = sibling.isLoading;
    rerender(view(true));
    await succeed(reply, "loaded");
    rerender(view(false));
    rerender(view(true));
    expect(loadingAfterUnmount).toBe(true);
    expect(listBranchesMock).toHaveBeenCalledTimes(1);
    expect(sibling).toMatchObject({
      branches: branches("loaded"),
      isLoaded: true,
      isLoading: false,
    });
  });
});

// @covers AC-WORKSPACES-BRANCH-READS-001.7
it("lets a remounted consumer retry after terminal initial failure", async () => {
  vi.useFakeTimers();
  listBranchesMock.mockRejectedValue(new Error("network failure"));
  let value!: UseBranchesResult;
  function Reader() {
    value = useBranches(SOURCE);
    return null;
  }
  const view = (mounted: boolean) => <StateProvider>{mounted && <Reader />}</StateProvider>;
  const { rerender } = render(view(true));
  await act(async () => {
    await vi.runAllTimersAsync();
  });
  expect(value).toMatchObject({ isLoaded: false, isLoading: false });
  expect(listBranchesMock).toHaveBeenCalledTimes(5);
  const retry = deferred();
  listBranchesMock.mockReturnValueOnce(retry.promise);
  rerender(view(false));
  rerender(view(true));
  const loadingAfterRemount = value.isLoading;
  await succeed(retry, "remounted");
  expect(loadingAfterRemount).toBe(true);
  expect(value).toMatchObject({
    branches: branches("remounted"),
    isLoaded: true,
    isLoading: false,
  });
  expect(listBranchesMock).toHaveBeenCalledTimes(6);
});

// @covers AC-WORKSPACES-BRANCH-READS-001.8
it("renders newest shared branch options through the real hook and BranchPickerList", async () => {
  const initial = deferred(),
    latest = deferred();
  listBranchesMock.mockReturnValueOnce(initial.promise);
  refreshMock.mockReturnValueOnce(latest.promise);
  let reader!: UseBranchesResult;
  function Picker() {
    reader = useBranches(SOURCE);
    return (
      <BranchPickerList
        branches={reader.branches}
        isLoadingBranches={reader.isLoading}
        currentBase=""
        onSelect={() => {}}
      />
    );
  }
  render(
    <StateProvider>
      <Picker />
    </StateProvider>,
  );
  const initialLoading = screen.queryByText("Loading branches…");
  const pending = refresh(reader);
  await succeed(latest, NEW_BRANCH, pending);
  await succeed(initial, "old-branch");
  expect(initialLoading).toBeTruthy();
  expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([NEW_BRANCH]);
  expect(screen.queryByText("Loading branches…")).toBeNull();
});
