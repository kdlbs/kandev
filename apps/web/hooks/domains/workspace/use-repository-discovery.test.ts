import { afterEach, describe, expect, it, vi } from "vitest";
import {
  RepositoryDiscoveryCoordinator,
  type RepositoryDiscoveryClient,
} from "./use-repository-discovery";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";

class FakeVisibilityDocument {
  visibilityState: DocumentVisibilityState = "visible";
  private readonly listeners = new Set<() => void>();

  addEventListener = (_type: string, listener: EventListenerOrEventListenerObject) => {
    this.listeners.add(listener as () => void);
  };

  removeEventListener = (_type: string, listener: EventListenerOrEventListenerObject) => {
    this.listeners.delete(listener as () => void);
  };

  setVisibility(value: DocumentVisibilityState) {
    this.visibilityState = value;
    for (const listener of this.listeners) listener();
  }
}

const oldScan = "2026-08-27T20:00:00.000Z";
const currentTime = Date.parse("2026-08-27T22:00:00.000Z");
const workspaceId = "workspace-1";

function response(scanTime = oldScan): RepositoryDiscoveryResponse {
  return {
    roots: ["/work"],
    repositories: [{ path: "/work/repo", name: "repo" }],
    total: 1,
    root_states: [],
    scan_time: scanTime,
    refreshing: false,
    cached: true,
    home_confirmation_required: false,
    failed_roots: [],
  };
}

function failedResponse(
  scanTime = new Date(currentTime).toISOString(),
): RepositoryDiscoveryResponse {
  return {
    ...response(scanTime),
    failed_roots: ["/missing"],
  };
}

function client(overrides: Partial<RepositoryDiscoveryClient> = {}): RepositoryDiscoveryClient {
  return {
    getSnapshot: vi.fn(async () => response()),
    refresh: vi.fn(async () => response("2026-08-27T21:00:00.000Z")),
    ...overrides,
  };
}

// eslint-disable-next-line max-lines-per-function -- coordinator scenarios share setup and lifecycle assertions
describe("RepositoryDiscoveryCoordinator", () => {
  it("shares one stale refresh between active leases", async () => {
    const api = client();
    const document = new FakeVisibilityDocument();
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      document,
      now: () => currentTime,
    });

    const releaseOne = coordinator.acquire(workspaceId);
    const releaseTwo = coordinator.acquire(workspaceId);
    await vi.waitFor(() => expect(api.refresh).toHaveBeenCalledTimes(1));

    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    expect(coordinator.getSnapshot(workspaceId).response?.scan_time).toBe(
      "2026-08-27T21:00:00.000Z",
    );
    releaseOne();
    releaseTwo();
    coordinator.dispose();
  });

  it("waits for a visible active lease before refreshing", async () => {
    const api = client();
    const document = new FakeVisibilityDocument();
    document.visibilityState = "hidden";
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      document,
      now: () => currentTime,
    });

    const release = coordinator.acquire(workspaceId);
    await coordinator.load(workspaceId);
    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    expect(api.refresh).not.toHaveBeenCalled();

    document.setVisibility("visible");
    await vi.waitFor(() => expect(api.refresh).toHaveBeenCalledTimes(1));
    release();
    document.setVisibility("hidden");
    document.setVisibility("visible");
    expect(api.refresh).toHaveBeenCalledTimes(1);
    coordinator.dispose();
  });

  it("keeps the last response when a refresh fails", async () => {
    const api = client({
      refresh: vi.fn(async () => {
        throw new Error("permission denied");
      }),
    });
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      now: () => currentTime,
    });
    const release = coordinator.acquire(workspaceId);
    await vi.waitFor(() => expect(api.refresh).toHaveBeenCalledTimes(1));

    const state = coordinator.getSnapshot(workspaceId);
    expect(state.response?.repositories).toHaveLength(1);
    expect(state.error?.message).toBe("permission denied");
    expect(state.isRefreshing).toBe(false);
    release();
    coordinator.dispose();
  });

  it("does not auto-refresh a persisted reconnect-required root", async () => {
    const api = client({
      getSnapshot: vi.fn(async () => ({
        ...response(),
        root_states: [
          {
            id: "root-1",
            path: "/work",
            display_path: "~/work",
            state: "reconnect_required",
          },
        ],
      })),
    });
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      now: () => currentTime,
    });

    const release = coordinator.acquire(workspaceId);
    await coordinator.load(workspaceId);
    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    expect(api.refresh).not.toHaveBeenCalled();
    release();
    coordinator.dispose();
  });

  it("does not auto-refresh a snapshot with failed roots", async () => {
    const api = client({
      getSnapshot: vi.fn(async () => failedResponse(oldScan)),
    });
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      now: () => currentTime,
    });

    const release = coordinator.acquire(workspaceId);
    await coordinator.load(workspaceId);
    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    expect(api.refresh).not.toHaveBeenCalled();
    expect(coordinator.getSnapshot(workspaceId).response?.failed_roots).toEqual(["/missing"]);
    release();
    coordinator.dispose();
  });

  it("allows an explicit refresh to recover a failed snapshot", async () => {
    const api = client({
      getSnapshot: vi.fn(async () => failedResponse()),
      refresh: vi.fn(async () => response(new Date(currentTime).toISOString())),
    });
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      now: () => currentTime,
    });

    const release = coordinator.acquire(workspaceId);
    await coordinator.load(workspaceId);
    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    await coordinator.refresh(workspaceId);

    expect(api.refresh).toHaveBeenCalledWith(workspaceId, "manual_refresh");
    expect(coordinator.getSnapshot(workspaceId).response?.failed_roots).toEqual([]);
    release();
    coordinator.dispose();
  });

  it("joins a refresh reported by a fresh cached snapshot", async () => {
    const api = client({
      getSnapshot: vi.fn(async () => ({
        ...response(new Date(currentTime).toISOString()),
        refreshing: true,
      })),
    });
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      now: () => currentTime,
    });

    const release = coordinator.acquire(workspaceId);
    await vi.waitFor(() => expect(api.refresh).toHaveBeenCalledTimes(1));
    expect(api.refresh).toHaveBeenCalledWith(workspaceId, "stale_refresh");
    release();
    coordinator.dispose();
  });

  it("loads the post-action snapshot without starting a refresh", async () => {
    const api = client();
    const coordinator = new RepositoryDiscoveryCoordinator(api);

    await coordinator.load(workspaceId);
    expect(api.getSnapshot).toHaveBeenCalledTimes(1);
    expect(api.refresh).not.toHaveBeenCalled();
    coordinator.dispose();
  });
});

type ReadKind = "load" | "refresh";

function deferred() {
  let resolve!: (value: RepositoryDiscoveryResponse) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<RepositoryDiscoveryResponse>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function rootResponse(root: string, patch: Partial<RepositoryDiscoveryResponse> = {}) {
  return {
    ...response(new Date(currentTime).toISOString()),
    roots: [root],
    repositories: [{ path: `${root}/repo`, name: root }],
    ...patch,
  };
}

const coordinators: RepositoryDiscoveryCoordinator[] = [];

function controlled() {
  const reads = {
    load: [] as ReturnType<typeof deferred>[],
    refresh: [] as ReturnType<typeof deferred>[],
  };
  const read = (kind: ReadKind) => {
    const pending = deferred();
    reads[kind].push(pending);
    return pending.promise;
  };
  const api = client({
    getSnapshot: vi.fn(() => read("load")),
    refresh: vi.fn(() => read("refresh")),
  });
  const document = new FakeVisibilityDocument();
  const coordinator = new RepositoryDiscoveryCoordinator(api, { document, now: () => currentTime });
  coordinators.push(coordinator);
  return { coordinator, api, reads, document };
}

async function seed(
  coordinator: RepositoryDiscoveryCoordinator,
  reads: ReturnType<typeof controlled>["reads"],
) {
  const pending = coordinator.load(workspaceId);
  const baseline = rootResponse("/baseline");
  reads.load.at(-1)!.resolve(baseline);
  await pending;
  return baseline;
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.13
// eslint-disable-next-line max-lines-per-function -- deterministic ordering matrix keeps each scenario explicit
describe.each<ReadKind>(["load", "refresh"])("discovery ordering with older %s", (olderKind) => {
  const newerKind: ReadKind = olderKind === "load" ? "refresh" : "load";
  afterEach(() => {
    for (const coordinator of coordinators.splice(0)) coordinator.dispose();
  });

  it.each([true, false])(
    "accepts only the latest start when newer settles first: %s",
    async (newerFirst) => {
      const { coordinator, reads } = controlled();
      const older = coordinator[olderKind](workspaceId);
      const oldRead = reads[olderKind].at(-1)!;
      const newer = coordinator[newerKind](workspaceId);
      const newRead = reads[newerKind].at(-1)!;
      const winner = rootResponse("/new", { cached: false, desktop_runtime: true });
      let obsoleteResult: RepositoryDiscoveryResponse | null;
      let interim: RepositoryDiscoveryResponse | null;
      if (newerFirst) {
        newRead.resolve(winner);
        await newer;
        interim = coordinator.getSnapshot(workspaceId).response;
        oldRead.resolve(rootResponse("/old"));
        obsoleteResult = await older;
      } else {
        oldRead.resolve(rootResponse("/old"));
        obsoleteResult = await older;
        interim = coordinator.getSnapshot(workspaceId).response;
        newRead.resolve(winner);
        await newer;
      }
      expect(interim).toEqual(newerFirst ? winner : null);
      expect(obsoleteResult).toEqual(newerFirst ? winner : null);
      expect(coordinator.getSnapshot(workspaceId)).toEqual({
        response: winner,
        error: null,
        isLoading: false,
        isRefreshing: false,
      });
    },
  );

  it("keeps accepted baseline and current failure against an older success", async () => {
    const { coordinator, reads } = controlled();
    const baseline = await seed(coordinator, reads);
    const older = coordinator[olderKind](workspaceId);
    const oldRead = reads[olderKind].at(-1)!;
    const newer = coordinator[newerKind](workspaceId);
    const failure = new Error("current read failed");
    reads[newerKind].at(-1)!.reject(failure);
    const currentResult = await newer;
    oldRead.resolve(rootResponse("/obsolete"));
    const obsoleteResult = await older;
    expect(currentResult).toEqual(baseline);
    expect(obsoleteResult).toEqual(baseline);
    expect(coordinator.getSnapshot(workspaceId)).toEqual({
      response: baseline,
      error: failure,
      isLoading: false,
      isRefreshing: false,
    });
  });

  it("ignores obsolete failure after current success", async () => {
    const { coordinator, reads } = controlled();
    await seed(coordinator, reads);
    const older = coordinator[olderKind](workspaceId);
    const oldRead = reads[olderKind].at(-1)!;
    const newer = coordinator[newerKind](workspaceId);
    const winner = rootResponse("/current", { cached: false });
    reads[newerKind].at(-1)!.resolve(winner);
    await newer;
    oldRead.reject(new Error("obsolete failure"));
    await older;
    expect(coordinator.getSnapshot(workspaceId)).toEqual({
      response: winner,
      error: null,
      isLoading: false,
      isRefreshing: false,
    });
  });

  it("accepts an empty newest response without restoring old roots", async () => {
    const { coordinator, reads } = controlled();
    await seed(coordinator, reads);
    const older = coordinator[olderKind](workspaceId);
    const oldRead = reads[olderKind].at(-1)!;
    const newer = coordinator[newerKind](workspaceId);
    const empty = rootResponse("/empty", { roots: [], repositories: [], total: 0 });
    reads[newerKind].at(-1)!.resolve(empty);
    await newer;
    oldRead.resolve(rootResponse("/removed"));
    expect(await older).toEqual(empty);
    expect(coordinator.getSnapshot(workspaceId).response).toEqual(empty);
  });

  it("shares same-kind calls without renewing obsolete authority", async () => {
    const { coordinator, reads } = controlled();
    const older = coordinator[olderKind](workspaceId);
    const firstJoin = coordinator[olderKind](workspaceId);
    const newer = coordinator[newerKind](workspaceId);
    const lastJoin = coordinator[olderKind](workspaceId);
    const winner = rootResponse("/new");
    reads[newerKind].at(-1)!.resolve(winner);
    await newer;
    reads[olderKind][0].resolve(rootResponse("/old"));
    const results = await Promise.all([older, firstJoin, lastJoin]);
    expect(reads[olderKind]).toHaveLength(1);
    expect(results).toEqual([winner, winner, winner]);
    expect(coordinator.getSnapshot(workspaceId).response).toEqual(winner);
  });
});

// eslint-disable-next-line max-lines-per-function -- lifecycle cases exercise actual subscriptions and transport settlement
describe("discovery ordering lifecycle", () => {
  afterEach(() => {
    for (const coordinator of coordinators.splice(0)) coordinator.dispose();
  });

  it.each([false, true])(
    "does not follow up an obsolete stale/refreshing snapshot: %s",
    async (refreshing) => {
      const { coordinator, api, reads } = controlled();
      const release = coordinator.acquire(workspaceId);
      const older = coordinator.load(workspaceId);
      const newer = coordinator.refresh(workspaceId);
      reads.refresh[0].resolve(rootResponse("/new"));
      await newer;
      reads.load[0].resolve(rootResponse("/old", { scan_time: oldScan, refreshing }));
      await older;
      const refreshCount = vi.mocked(api.refresh).mock.calls.length;
      release();
      for (const read of reads.refresh.slice(1)) read.resolve(rootResponse("/unexpected"));
      await Promise.all(reads.refresh.map((read) => read.promise));
      expect(refreshCount).toBe(1);
      expect(coordinator.getSnapshot(workspaceId).response?.roots).toEqual(["/new"]);
    },
  );

  it.each<ReadKind>(["load", "refresh"])("reserves %s before subscriber joins", async (kind) => {
    const { coordinator, reads } = controlled();
    let joined: Promise<RepositoryDiscoveryResponse | null> | undefined;
    let entered = false;
    const unsubscribe = coordinator.subscribe(workspaceId, () => {
      if (entered) return;
      entered = true;
      joined = coordinator[kind](workspaceId);
    });
    const first = coordinator[kind](workspaceId);
    const winner = rootResponse("/shared");
    for (const read of reads[kind]) read.resolve(winner);
    await Promise.all([first, joined]);
    unsubscribe();
    expect(reads[kind]).toHaveLength(1);
    expect(coordinator.getSnapshot(workspaceId).response).toEqual(winner);
  });

  it("lets response subscribers start a new snapshot without old follow-up", async () => {
    const { coordinator, api, reads } = controlled();
    let newer: Promise<RepositoryDiscoveryResponse | null> | undefined;
    let entered = false;
    const unsubscribe = coordinator.subscribe(workspaceId, () => {
      if (entered || coordinator.getSnapshot(workspaceId).response?.roots[0] !== "/old") return;
      entered = true;
      newer = coordinator.load(workspaceId);
    });
    const release = coordinator.acquire(workspaceId);
    const older = coordinator.load(workspaceId);
    reads.load[0].resolve(rootResponse("/old", { scan_time: oldScan }));
    await older;
    const snapshotCount = reads.load.length;
    const refreshCount = vi.mocked(api.refresh).mock.calls.length;
    release();
    for (const read of reads.load.slice(1)) read.resolve(rootResponse("/new"));
    for (const read of reads.refresh) read.resolve(rootResponse("/unexpected"));
    await newer;
    await Promise.all(reads.refresh.map((read) => read.promise));
    unsubscribe();
    expect(snapshotCount).toBe(2);
    expect(refreshCount).toBe(0);
    expect(coordinator.getSnapshot(workspaceId).response?.roots).toEqual(["/new"]);
  });

  it("keeps pending refresh and accepted scan metadata through snapshot cleanup", async () => {
    const { coordinator, reads } = controlled();
    const older = coordinator.load(workspaceId);
    const newer = coordinator.refresh(workspaceId);
    reads.load[0].resolve(rootResponse("/old"));
    await older;
    const interim = coordinator.getSnapshot(workspaceId);
    reads.refresh[0].resolve(rootResponse("/new", { refreshing: true }));
    await newer;
    expect(interim).toMatchObject({ response: null, isLoading: false, isRefreshing: true });
    expect(coordinator.getSnapshot(workspaceId)).toMatchObject({
      isLoading: false,
      isRefreshing: true,
      error: null,
    });
  });

  it("keeps workspace operations isolated", async () => {
    const { coordinator, reads } = controlled();
    const first = coordinator.load("workspace-a");
    const second = coordinator.refresh("workspace-b");
    reads.refresh[0].resolve(rootResponse("/b"));
    await second;
    reads.load[0].resolve(rootResponse("/a"));
    await first;
    expect(coordinator.getSnapshot("workspace-a").response?.roots).toEqual(["/a"]);
    expect(coordinator.getSnapshot("workspace-b").response?.roots).toEqual(["/b"]);
  });

  it("retains cache after release without starting stale follow-up", async () => {
    const { coordinator, api, reads } = controlled();
    const release = coordinator.acquire(workspaceId);
    const pending = coordinator.load(workspaceId);
    release();
    const cached = rootResponse("/released", { scan_time: oldScan });
    reads.load[0].resolve(cached);
    expect(await pending).toEqual(cached);
    expect(coordinator.getSnapshot(workspaceId)).toMatchObject({
      response: cached,
      isLoading: false,
    });
    expect(api.refresh).not.toHaveBeenCalled();
  });

  it.each([false, true])("fences disposed entries, recreated: %s", async (recreate) => {
    const { coordinator, api, reads } = controlled();
    const notified = vi.fn();
    coordinator.subscribe(workspaceId, notified);
    const release = coordinator.acquire(workspaceId);
    const old = coordinator.load(workspaceId);
    coordinator.dispose();
    notified.mockClear();
    let newer: Promise<RepositoryDiscoveryResponse | null> | undefined;
    if (recreate) {
      newer = coordinator.load(workspaceId);
      reads.load[1].resolve(rootResponse("/replacement"));
      await newer;
    }
    reads.load[0].resolve(rootResponse("/disposed", { scan_time: oldScan, refreshing: true }));
    const obsoleteResult = await old;
    const refreshCount = vi.mocked(api.refresh).mock.calls.length;
    release();
    for (const read of reads.refresh) read.resolve(rootResponse("/unexpected"));
    await Promise.all(reads.refresh.map((read) => read.promise));
    expect(obsoleteResult?.roots ?? null).toEqual(recreate ? ["/replacement"] : null);
    expect(notified).not.toHaveBeenCalled();
    expect(refreshCount).toBe(0);
    expect(coordinator.getSnapshot(workspaceId).response?.roots ?? null).toEqual(
      recreate ? ["/replacement"] : null,
    );
  });
});
