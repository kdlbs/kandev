import { expect, it, vi } from "vitest";
import {
  RepositoryDiscoveryCoordinator,
  type RepositoryDiscoveryReadKind,
} from "./use-repository-discovery";
import type { RepositoryDiscoveryResponse } from "@/lib/types/http";

const now = Date.parse("2026-10-03T10:00:00Z");
const kinds: RepositoryDiscoveryReadKind[] = ["load", "refresh"];

function snapshot(root = "/baseline"): RepositoryDiscoveryResponse {
  return {
    roots: [root],
    repositories: [{ path: `${root}/repo`, name: root }],
    total: 1,
    root_states: [{ id: root, path: root, display_path: root, state: "connected" }],
    scan_time: new Date(now).toISOString(),
    desktop_runtime: true,
    cached: false,
    refreshing: false,
    home_confirmation_required: false,
    failed_roots: [],
  };
}

function deferred() {
  let resolve!: (value: RepositoryDiscoveryResponse) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<RepositoryDiscoveryResponse>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function setup() {
  const api = {
    getSnapshot: vi.fn(async (_workspace: string) => snapshot()),
    refresh: vi.fn(async (_workspace: string, _trigger?: string) => snapshot()),
  };
  const coordinator = new RepositoryDiscoveryCoordinator(api, { now: () => now });
  const queue = (kind: RepositoryDiscoveryReadKind, read: ReturnType<typeof deferred>) => {
    if (kind === "load") api.getSnapshot.mockReturnValueOnce(read.promise);
    else api.refresh.mockReturnValueOnce(read.promise);
  };
  return { api, coordinator, queue };
}

const overlaps = kinds.flatMap((kind) =>
  kinds.flatMap((oldKind) =>
    [true, false].flatMap((oldFirst) =>
      [true, false].flatMap((oldSucceeds) =>
        [true, false].map((freshSucceeds) => ({
          kind,
          oldKind,
          oldFirst,
          oldSucceeds,
          freshSucceeds,
        })),
      ),
    ),
  ),
);

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.13
// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
it.each(overlaps)(
  "$kind after $oldKind: oldFirst=$oldFirst oldSuccess=$oldSucceeds freshSuccess=$freshSucceeds",
  async ({ kind, oldKind, oldFirst, oldSucceeds, freshSucceeds }) => {
    const { api, coordinator, queue } = setup();
    await coordinator.load("A");
    const old = deferred();
    const fresh = deferred();
    queue(oldKind, old);
    const oldCall = coordinator[oldKind]("A");
    queue(kind, fresh);
    const freshCall = coordinator.synchronizeAfterRootMutation("A", kind);
    const oldError = new Error("old read failed");
    const freshError = new Error("fresh read failed");
    const settleOld = () => (oldSucceeds ? old.resolve(snapshot("/old")) : old.reject(oldError));
    const settleFresh = () =>
      freshSucceeds ? fresh.resolve(snapshot("/current")) : fresh.reject(freshError);
    try {
      expect(api.getSnapshot.mock.calls.length + api.refresh.mock.calls.length).toBe(3);
      if (oldFirst) {
        settleOld();
        await oldCall;
        expect(coordinator.getSnapshot("A")).toMatchObject({
          response: snapshot(),
          error: null,
          isLoading: kind === "load",
          isRefreshing: kind === "refresh",
        });
        settleFresh();
        await freshCall;
      } else {
        settleFresh();
        await freshCall;
        expect(coordinator.getSnapshot("A")).toMatchObject({
          isLoading: false,
          isRefreshing: false,
        });
        const accepted = coordinator.getSnapshot("A");
        const listener = vi.fn();
        const unsubscribe = coordinator.subscribe("A", listener);
        settleOld();
        await oldCall;
        expect(coordinator.getSnapshot("A")).toBe(accepted);
        expect(listener).not.toHaveBeenCalled();
        unsubscribe();
      }
      const expected = freshSucceeds ? snapshot("/current") : snapshot();
      expect(coordinator.getSnapshot("A")).toEqual({
        response: expected,
        error: freshSucceeds ? null : freshError,
        isLoading: false,
        isRefreshing: false,
      });
      expect(await freshCall).toEqual(expected);
    } finally {
      old.resolve(snapshot("/cleanup"));
      fresh.resolve(snapshot("/cleanup"));
      await Promise.all([oldCall, freshCall]);
      coordinator.dispose();
    }
  },
);

it.each(kinds)("%s detaches both old kinds and accepts authoritative empty roots", async (kind) => {
  const { coordinator, queue } = setup();
  await coordinator.load("A");
  const cached = deferred();
  const scan = deferred();
  const fresh = deferred();
  queue("load", cached);
  queue("refresh", scan);
  const reads = [coordinator.load("A"), coordinator.refresh("A")];
  queue(kind, fresh);
  reads.push(coordinator.synchronizeAfterRootMutation("A", kind));
  const empty = { ...snapshot(), roots: [], repositories: [], root_states: [], total: 0 };
  try {
    fresh.resolve(empty);
    expect(await reads[2]).toEqual(empty);
    expect(coordinator.getSnapshot("A")).toMatchObject({
      response: empty,
      isLoading: false,
      isRefreshing: false,
      error: null,
    });
    cached.resolve({ ...snapshot("/old"), cached: true, refreshing: true });
    scan.reject(new Error("obsolete scan"));
    await Promise.all(reads);
    expect(coordinator.getSnapshot("A")).toMatchObject({ response: empty, error: null });
  } finally {
    for (const pending of [cached, scan, fresh]) pending.resolve(snapshot());
    await Promise.all(reads);
    coordinator.dispose();
  }
});

it.each(kinds)("%s preserves ordinary coalescing and newer cross-kind authority", async (kind) => {
  const { api, coordinator, queue } = setup();
  await coordinator.load("A");
  const old = deferred();
  const fresh = deferred();
  const latest = deferred();
  queue(kind, old);
  const reads = [coordinator[kind]("A"), coordinator[kind]("A")];
  queue(kind, fresh);
  reads.push(coordinator.synchronizeAfterRootMutation("A", kind), coordinator[kind]("A"));
  const opposite = kind === "load" ? "refresh" : "load";
  queue(opposite, latest);
  reads.push(coordinator[opposite]("A"), coordinator[kind]("A"));
  try {
    expect(api.getSnapshot.mock.calls.length + api.refresh.mock.calls.length).toBe(4);
    latest.resolve(snapshot("/latest"));
    await reads[4];
    fresh.resolve(snapshot("/fresh"));
    old.resolve(snapshot("/old"));
    const responses = await Promise.all(reads);
    expect(responses.every((value) => value?.roots[0] === "/latest")).toBe(true);
    expect(coordinator.getSnapshot("A")).toMatchObject({
      response: snapshot("/latest"),
      isLoading: false,
      isRefreshing: false,
    });
  } finally {
    for (const pending of [old, fresh, latest]) pending.resolve(snapshot());
    await Promise.all(reads);
    coordinator.dispose();
  }
});

it("fences the preceding mutation synchronization when another mutation succeeds", async () => {
  const { coordinator, queue } = setup();
  const first = deferred();
  const second = deferred();
  queue("load", first);
  const firstCall = coordinator.synchronizeAfterRootMutation("A", "load");
  queue("load", second);
  const secondCall = coordinator.synchronizeAfterRootMutation("A", "load");
  second.resolve(snapshot("/second"));
  await secondCall;
  first.resolve(snapshot("/first"));
  await firstCall;
  expect(coordinator.getSnapshot("A").response).toEqual(snapshot("/second"));
  coordinator.dispose();
});
class Visibility {
  visibilityState: DocumentVisibilityState = "visible";
  listeners = new Set<EventListenerOrEventListenerObject>();
  addEventListener = (_event: string, listener: EventListenerOrEventListenerObject) => {
    this.listeners.add(listener);
  };
  removeEventListener = (_event: string, listener: EventListenerOrEventListenerObject) => {
    this.listeners.delete(listener);
  };
}

// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.3
// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.4
// @covers AC-WORKSPACES-LOCAL-REPOSITORIES-003.14
it.each(["eligible", "hidden", "released", "failed-root"])(
  "keeps %s follow-up policy and ignores detached snapshots",
  async (policy) => {
    const visibility = new Visibility();
    const old = deferred();
    const fresh = deferred();
    const scan = deferred();
    const api = {
      getSnapshot: vi
        .fn()
        .mockResolvedValueOnce(snapshot())
        .mockReturnValueOnce(old.promise)
        .mockReturnValueOnce(fresh.promise),
      refresh: vi.fn().mockReturnValue(scan.promise),
    };
    const coordinator = new RepositoryDiscoveryCoordinator(api, {
      document: visibility,
      now: () => now,
    });
    const release = coordinator.acquire("A");
    await coordinator.load("A");
    const oldCall = coordinator.load("A");
    const freshCall = coordinator.synchronizeAfterRootMutation("A", "load");
    if (policy === "hidden") visibility.visibilityState = "hidden";
    if (policy === "released") release();
    const stale = { ...snapshot("/new"), scan_time: new Date(now - 3_600_000).toISOString() };
    try {
      old.resolve({ ...snapshot("/old"), refreshing: true });
      await oldCall;
      expect(api.refresh).not.toHaveBeenCalled();
      fresh.resolve(policy === "failed-root" ? { ...stale, failed_roots: ["/failed"] } : stale);
      await freshCall;
      expect(api.refresh).toHaveBeenCalledTimes(policy === "eligible" ? 1 : 0);
      scan.resolve(snapshot("/scanned"));
      if (policy === "eligible") await coordinator.refresh("A");
    } finally {
      old.resolve(snapshot());
      fresh.resolve(snapshot());
      scan.resolve(snapshot());
      await Promise.all([oldCall, freshCall]);
      release();
      coordinator.dispose();
    }
  },
);

it("retains mixed authoritative successful and failed roots without a retry", async () => {
  const { coordinator, api } = setup();
  const release = coordinator.acquire("A");
  await coordinator.load("A");
  const mixed = {
    ...snapshot("/changed"),
    roots: ["/changed", "/failed"],
    repositories: [...snapshot("/changed").repositories, ...snapshot("/failed").repositories],
    total: 2,
    root_states: [
      ...snapshot("/changed").root_states!,
      {
        id: "failed",
        path: "/failed",
        display_path: "/failed",
        state: "reconnect_required" as const,
      },
    ],
    failed_roots: ["/failed"],
    scan_time: new Date(now - 3_600_000).toISOString(),
  };
  api.getSnapshot.mockResolvedValueOnce(mixed);
  await coordinator.synchronizeAfterRootMutation("A", "load");
  expect(coordinator.getSnapshot("A").response).toEqual(mixed);
  expect(api.refresh).not.toHaveBeenCalled();
  release();
  coordinator.dispose();
});

it("joins a reserved fresh handle on notification and preserves response reentry", async () => {
  const { coordinator, api, queue } = setup();
  await coordinator.load("A");
  const old = deferred();
  const fresh = deferred();
  const newer = deferred();
  queue("load", old);
  const oldCall = coordinator.load("A");
  queue("load", fresh);
  queue("refresh", newer);
  const joined: Promise<RepositoryDiscoveryResponse | null>[] = [];
  let didJoin = false;
  let didRefresh = false;
  const unsubscribe = coordinator.subscribe("A", () => {
    if (!didJoin) {
      didJoin = true;
      joined.push(coordinator.load("A"));
    }
    if (!didRefresh && coordinator.getSnapshot("A").response?.roots[0] === "/fresh") {
      didRefresh = true;
      joined.push(coordinator.refresh("A"));
    }
  });
  const freshCall = coordinator.synchronizeAfterRootMutation("A", "load");
  try {
    expect(api.getSnapshot).toHaveBeenCalledTimes(3);
    fresh.resolve(snapshot("/fresh"));
    await freshCall;
    old.resolve(snapshot("/old"));
    await oldCall;
    expect(coordinator.getSnapshot("A").isRefreshing).toBe(true);
    newer.resolve(snapshot("/newer"));
    await Promise.all(joined);
    expect(coordinator.getSnapshot("A")).toMatchObject({
      response: snapshot("/newer"),
      isRefreshing: false,
    });
  } finally {
    unsubscribe();
    old.resolve(snapshot());
    fresh.resolve(snapshot());
    newer.resolve(snapshot());
    await Promise.all([oldCall, freshCall, ...joined]);
    coordinator.dispose();
  }
});

it("isolates every A subscriber from B work and releases without discarding fresh A cache", async () => {
  const { coordinator, queue, api } = setup();
  await coordinator.load("A");
  await coordinator.load("B");
  const release = coordinator.acquire("A");
  const oldA = deferred();
  const oldB = deferred();
  const fresh = deferred();
  queue("load", oldA);
  const a = coordinator.load("A");
  queue("load", oldB);
  const b = coordinator.load("B");
  queue("load", fresh);
  const changed = coordinator.synchronizeAfterRootMutation("A", "load");
  const observed: string[][] = [];
  const observedSibling: string[][] = [];
  const unsubA = coordinator.subscribe("A", () =>
    observed.push(coordinator.getSnapshot("A").response!.roots),
  );
  const unsubSibling = coordinator.subscribe("A", () =>
    observedSibling.push(coordinator.getSnapshot("A").response!.roots),
  );
  release();
  fresh.resolve({ ...snapshot("/A-new"), scan_time: "" });
  await changed;
  oldA.resolve(snapshot("/A-old"));
  oldB.resolve(snapshot("/B-current"));
  await Promise.all([a, b]);
  expect(observed).toEqual([["/A-new"]]);
  expect(observedSibling).toEqual(observed);
  expect(coordinator.getSnapshot("B").response).toEqual(snapshot("/B-current"));
  expect(api.getSnapshot).toHaveBeenCalledTimes(5);
  expect(api.refresh).not.toHaveBeenCalled();
  unsubA();
  unsubSibling();
  coordinator.dispose();
});

it.each([false, true])(
  "fences disposal/recreation with disposeAtNotification=%s",
  async (disposeAtNotification) => {
    const { coordinator, api, queue } = setup();
    const old = deferred();
    const fresh = deferred();
    queue("load", old);
    const oldCall = coordinator.load("A");
    const listener = vi.fn(() => {
      if (disposeAtNotification) coordinator.dispose();
    });
    const unsubscribe = coordinator.subscribe("A", listener);
    queue("load", fresh);
    const changed = coordinator.synchronizeAfterRootMutation("A", "load");
    if (!disposeAtNotification) coordinator.dispose();
    listener.mockClear();
    await coordinator.load("A");
    old.resolve({ ...snapshot("/old"), refreshing: true });
    fresh.resolve(snapshot("/removed-entry"));
    await Promise.all([oldCall, changed]);
    expect(coordinator.getSnapshot("A").response).toEqual(snapshot());
    expect(listener).not.toHaveBeenCalled();
    expect(api.refresh).not.toHaveBeenCalled();
    unsubscribe();
    coordinator.dispose();
  },
);
