import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { focusManager, onlineManager, QueryClient, useQueryClient } from "@tanstack/react-query";
import { createElement, Fragment, useEffect, useState, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { DatabaseStats } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { useDatabaseStats } from "./use-database-stats";

const config = vi.hoisted(() => ({ apiBaseUrl: "https://backend.example/api///" }));
const API_BASE_URL = "https://backend.example/api";
const PAGE_BOOT_ID = "database-page-boot";
const CURRENT_DATABASE_PATH = "/current-a.db";
const MEASURED_AT = Date.now() - 1_000;

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }),
}));
vi.mock("@/lib/api/domains/system-api");

const AUTH = {
  mode: "enabled" as const,
  authenticated: true,
  user: {
    id: "user-1",
    email: "user@example.com",
    display_name: "User",
    role: "admin" as const,
    status: "active" as const,
  },
};

const READY: DatabaseStats = {
  driver: "sqlite",
  path: "/data/kandev.db",
  backup_directory: "/data/backups",
  size_bytes: 1024,
  wal_size_bytes: 128,
  message_content_bytes: 16,
  message_metadata_bytes: 32,
  message_payload_bytes: 64,
  git_snapshot_bytes: 128,
  logical_stats_state: "ready",
  logical_stats_measured_at: new Date(MEASURED_AT).toISOString(),
  metadata_stale: false,
  metadata_measured_at: new Date(MEASURED_AT).toISOString(),
  schema_version: "1",
  last_backup_at: null,
};

const PENDING: DatabaseStats = {
  ...READY,
  message_content_bytes: null,
  message_metadata_bytes: null,
  message_payload_bytes: null,
  git_snapshot_bytes: null,
  logical_stats_state: "pending",
  logical_stats_measured_at: null,
};

const REFRESHING: DatabaseStats = { ...READY, logical_stats_state: "refreshing" };

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function QueryClientCapture({ onCapture }: { onCapture?: (client: QueryClient) => void }) {
  const client = useQueryClient();
  useEffect(() => onCapture?.(client), [client, onCapture]);
  return null;
}

function StoreCapture({ onCapture }: { onCapture?: (store: StoreApi<AppState>) => void }) {
  const store = useAppStoreApi();
  useEffect(() => onCapture?.(store), [onCapture, store]);
  return null;
}

function TestHarness({
  children,
  apiBaseUrl = API_BASE_URL,
  bootId = PAGE_BOOT_ID,
  onQueryClient,
  onStore,
}: {
  children?: ReactNode;
  apiBaseUrl?: string;
  bootId?: string;
  onQueryClient?: (client: QueryClient) => void;
  onStore?: (store: StoreApi<AppState>) => void;
}) {
  config.apiBaseUrl = apiBaseUrl;
  return createElement(StateProvider, {
    initialState: { auth: AUTH },
    children: createElement(SystemInfoQueryProvider, {
      bootId,
      children: createElement(
        Fragment,
        null,
        onQueryClient ? createElement(QueryClientCapture, { onCapture: onQueryClient }) : null,
        onStore ? createElement(StoreCapture, { onCapture: onStore }) : null,
        children,
      ),
    }),
  });
}

function DatabaseProbe({ id }: { id: string }) {
  const { database, error, isLoading } = useDatabaseStats();
  return createElement(
    "output",
    { "data-testid": id },
    JSON.stringify({ path: database?.path ?? null, error, isLoading }),
  );
}

function StatefulShellProbe() {
  const [value, setValue] = useState("original");
  return createElement("button", { type: "button", onClick: () => setValue("edited") }, value);
}

function renderStatsHook({
  apiBaseUrl = API_BASE_URL,
  bootId = PAGE_BOOT_ID,
}: { apiBaseUrl?: string; bootId?: string } = {}) {
  let queryClient: QueryClient | undefined;
  let store: StoreApi<AppState> | undefined;
  const onQueryClient = (client: QueryClient) => {
    queryClient = client;
  };
  const onStore = (nextStore: StoreApi<AppState>) => {
    store = nextStore;
  };
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(TestHarness, { apiBaseUrl, bootId, onQueryClient, onStore }, children);
  const hook = renderHook(() => useDatabaseStats(), { wrapper });
  return {
    ...hook,
    get queryClient() {
      return queryClient;
    },
    get store() {
      return store;
    },
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  config.apiBaseUrl = `${API_BASE_URL}///`;
  onlineManager.setOnline(true);
  focusManager.setFocused(true);
  vi.mocked(api.fetchDatabaseStats).mockResolvedValue(READY);
  vi.mocked(api.retryDatabaseStats).mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  focusManager.setFocused(true);
  vi.useRealTimers();
});

describe("useDatabaseStats query identity", () => {
  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3
  it("uses backend, boot, and auth identity and forwards TanStack's signal", async () => {
    const { result, queryClient, store } = renderStatsHook();
    await waitFor(() => expect(result.current.database).toEqual(READY));

    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
    expect(api.fetchDatabaseStats).toHaveBeenCalledWith({
      baseUrl: API_BASE_URL,
      cache: "no-store",
      init: { signal: expect.any(AbortSignal) },
    });
    expect(
      queryClient
        ?.getQueryCache()
        .getAll()
        .map((query) => query.queryKey),
    ).toEqual([
      ["system", "database-stats", API_BASE_URL, PAGE_BOOT_ID, "enabled", true, "user-1"],
    ]);
    expect("database" in store!.getState().system).toBe(false);
  });

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3
  it("deduplicates concurrent observers of the same snapshot", async () => {
    const response = deferred<DatabaseStats>();
    vi.mocked(api.fetchDatabaseStats).mockReturnValue(response.promise);
    render(
      createElement(
        TestHarness,
        null,
        createElement(
          Fragment,
          null,
          createElement(DatabaseProbe, { id: "first" }),
          createElement(DatabaseProbe, { id: "second" }),
        ),
      ),
    );

    await waitFor(() => expect(api.fetchDatabaseStats).toHaveBeenCalledOnce());
    await act(async () => response.resolve(READY));
    await waitFor(() => expect(screen.getByTestId("first").textContent).toContain(READY.path));
    expect(screen.getByTestId("second").textContent).toContain(READY.path);
  });
});

describe("useDatabaseStats request actions", () => {
  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  it("keeps last-good data and resolves reload after a failed status read", async () => {
    const failure = new Error("database status unavailable");
    vi.mocked(api.fetchDatabaseStats).mockResolvedValueOnce(READY).mockRejectedValueOnce(failure);
    const { result } = renderStatsHook();
    await waitFor(() => expect(result.current.database).toEqual(READY));

    await act(async () => {
      await expect(result.current.reload()).resolves.toBeUndefined();
    });

    await waitFor(() => expect(result.current.error).toBe(failure.message));
    expect(result.current.database).toEqual(READY);
  });

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  it("runs the refresh command before the authoritative status read", async () => {
    const stale: DatabaseStats = { ...READY, logical_stats_state: "stale" };
    const requests: string[] = [];
    vi.mocked(api.fetchDatabaseStats).mockImplementation(async () => {
      requests.push("status");
      return requests.length === 1 ? stale : REFRESHING;
    });
    vi.mocked(api.retryDatabaseStats).mockImplementation(async () => {
      requests.push("refresh");
    });
    const { result } = renderStatsHook();
    await waitFor(() => expect(result.current.database?.logical_stats_state).toBe("stale"));

    await act(async () => {
      await expect(result.current.retry()).resolves.toBeUndefined();
    });

    expect(requests).toEqual(["status", "refresh", "status"]);
    expect(result.current.database?.logical_stats_state).toBe("refreshing");
  });

  it("surfaces a failed refresh command without rejecting the UI caller", async () => {
    const failure = new Error("refresh unavailable");
    const stale: DatabaseStats = { ...READY, logical_stats_state: "stale" };
    vi.mocked(api.fetchDatabaseStats).mockResolvedValue(stale);
    vi.mocked(api.retryDatabaseStats).mockRejectedValue(failure);
    const { result } = renderStatsHook();
    await waitFor(() => expect(result.current.database).toEqual(stale));

    await act(async () => {
      await expect(result.current.retry()).resolves.toBeUndefined();
    });

    expect(result.current.error).toBe(failure.message);
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
  });
});

describe("useDatabaseStats polling and freshness", () => {
  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3 AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.4
  it.each([
    ["pending", PENDING],
    ["refreshing", REFRESHING],
  ] as const)("polls a %s snapshot after two seconds", async (_state, initial) => {
    vi.useFakeTimers();
    vi.setSystemTime(MEASURED_AT + 1_000);
    vi.mocked(api.fetchDatabaseStats).mockResolvedValueOnce(initial).mockResolvedValueOnce(READY);
    const { result } = renderStatsHook();

    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(result.current.database).toEqual(initial);
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
    await act(async () => vi.advanceTimersByTimeAsync(1_999));
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    await act(async () => vi.advanceTimersByTimeAsync(1));

    expect(result.current.database).toEqual(READY);
    expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
  });

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  it.each(["stale", "unavailable"] as const)(
    "uses the 30-second recovery interval for %s data",
    async (state) => {
      vi.useFakeTimers();
      vi.setSystemTime(MEASURED_AT + 1_000);
      const initial: DatabaseStats = { ...READY, logical_stats_state: state };
      vi.mocked(api.fetchDatabaseStats).mockResolvedValueOnce(initial).mockResolvedValueOnce(READY);
      const { result } = renderStatsHook();

      await act(async () => vi.advanceTimersByTimeAsync(0));
      expect(result.current.database).toEqual(initial);
      await act(async () => vi.advanceTimersByTimeAsync(29_999));
      expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
      await act(async () => vi.advanceTimersByTimeAsync(1));
      await act(async () => vi.advanceTimersByTimeAsync(1));

      expect(result.current.database).toEqual(READY);
      expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
    },
  );

  it("recovers from a status read error after 30 seconds while retaining data", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(MEASURED_AT + 60_000);
    const failure = new Error("temporary status failure");
    vi.mocked(api.fetchDatabaseStats)
      .mockResolvedValueOnce(READY)
      .mockRejectedValueOnce(failure)
      .mockResolvedValueOnce(REFRESHING);
    const { result } = renderStatsHook();
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(result.current.database).toEqual(READY);

    await act(async () => {
      await expect(result.current.reload()).resolves.toBeUndefined();
    });
    expect(result.current.database).toEqual(READY);
    expect(result.current.error).toBe(failure.message);
    await act(async () => vi.advanceTimersByTimeAsync(29_999));
    expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    await act(async () => vi.advanceTimersByTimeAsync(1));

    expect(result.current.database).toEqual(REFRESHING);
    expect(result.current.error).toBeNull();
    expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(3);
  });

  // @covers AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.4
  it("revalidates a ready response at expiry measured by the server timestamp", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(MEASURED_AT + 15 * 60_000 - 1);
    vi.mocked(api.fetchDatabaseStats)
      .mockResolvedValueOnce(READY)
      .mockResolvedValueOnce(REFRESHING);
    const { result } = renderStatsHook();

    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(result.current.database).toEqual(READY);
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
    await act(async () => vi.advanceTimersByTimeAsync(1));
    await act(async () => vi.advanceTimersByTimeAsync(1));

    expect(result.current.database).toEqual(REFRESHING);
    expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
  });
});

describe("useDatabaseStats cache retention", () => {
  it("retains the query entry for the app branch and revalidates after navigation", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(MEASURED_AT + 1_000);
    const response = deferred<DatabaseStats>();
    vi.mocked(api.fetchDatabaseStats)
      .mockResolvedValueOnce(READY)
      .mockReturnValueOnce(response.promise);
    let queryClient: QueryClient | undefined;
    const onQueryClient = (client: QueryClient) => {
      queryClient = client;
    };
    const view = render(
      createElement(
        TestHarness,
        { onQueryClient },
        createElement(DatabaseProbe, { id: "database" }),
      ),
    );
    await act(async () => vi.advanceTimersByTimeAsync(0));
    expect(screen.getByTestId("database").textContent).toContain(READY.path);
    view.rerender(
      createElement(TestHarness, { onQueryClient }, createElement("span", null, "away")),
    );
    await act(async () => vi.advanceTimersByTimeAsync(6 * 60_000));

    const key = ["system", "database-stats", API_BASE_URL, PAGE_BOOT_ID, "enabled", true, "user-1"];
    expect(queryClient?.getQueryData(key)).toEqual(READY);
    view.rerender(
      createElement(
        TestHarness,
        { onQueryClient },
        createElement(DatabaseProbe, { id: "database" }),
      ),
    );
    await act(async () => vi.advanceTimersByTimeAsync(0));

    expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId("database").textContent).toContain(READY.path);
  });
});

describe("useDatabaseStats request cancellation", () => {
  it("attempts once while offline and does not refetch automatically on reconnect", async () => {
    onlineManager.setOnline(false);
    const failure = new TypeError("offline");
    vi.mocked(api.fetchDatabaseStats).mockRejectedValue(failure);
    const { result } = renderStatsHook();

    await waitFor(() => expect(result.current.error).toBe(failure.message));
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
    act(() => onlineManager.setOnline(true));
    expect(api.fetchDatabaseStats).toHaveBeenCalledOnce();
  });

  it("cancels a pending GET when its last observer unmounts", async () => {
    const pending = deferred<DatabaseStats>();
    let signal: AbortSignal | undefined;
    vi.mocked(api.fetchDatabaseStats).mockImplementation((options) => {
      signal = options?.init?.signal as AbortSignal;
      return pending.promise;
    });
    const { unmount } = renderStatsHook();
    await waitFor(() => expect(signal).toBeDefined());

    unmount();

    expect(signal?.aborted).toBe(true);
    pending.resolve(READY);
  });
});

describe("useDatabaseStats identity changes", () => {
  it("survives rapid A-to-B-to-A identity changes and ignores late old responses", async () => {
    const requests: Array<{
      signal?: AbortSignal;
      pending: ReturnType<typeof deferred<DatabaseStats>>;
    }> = [];
    vi.mocked(api.fetchDatabaseStats).mockImplementation((options) => {
      const request = {
        signal: options?.init?.signal as AbortSignal | undefined,
        pending: deferred<DatabaseStats>(),
      };
      requests.push(request);
      return request.pending.promise;
    });
    let queryClient: QueryClient | undefined;
    let store: StoreApi<AppState> | undefined;
    const onQueryClient = (client: QueryClient) => {
      queryClient = client;
    };
    const onStore = (nextStore: StoreApi<AppState>) => {
      store = nextStore;
    };
    const page = (apiBaseUrl: string, bootId: string) =>
      createElement(
        TestHarness,
        { apiBaseUrl, bootId, onQueryClient, onStore },
        createElement(
          Fragment,
          null,
          createElement(DatabaseProbe, { id: "current" }),
          createElement(StatefulShellProbe),
        ),
      );
    const view = render(page("https://backend.example/one", "boot-a"));
    await waitFor(() => expect(requests).toHaveLength(1));
    fireEvent.click(screen.getByRole("button", { name: "original" }));

    const client = queryClient;
    if (!client) throw new Error("QueryClient was not captured");
    const releaseCancellation = deferred<void>();
    const cancelQueries = client.cancelQueries.bind(client);
    client.cancelQueries = ((...args: Parameters<QueryClient["cancelQueries"]>) =>
      cancelQueries(...args).then(
        () => releaseCancellation.promise,
      )) as QueryClient["cancelQueries"];

    try {
      view.rerender(page("https://backend.example/two", "boot-b"));
      await waitFor(() => expect(requests).toHaveLength(2));
      expect(requests[0]?.signal?.aborted).toBe(true);
      expect(screen.getByRole("button", { name: "edited" })).toBeTruthy();
      view.rerender(page("https://backend.example/one", "boot-a"));
      await waitFor(() => expect(requests).toHaveLength(3));
      expect(requests[1]?.signal?.aborted).toBe(true);

      await act(async () =>
        requests[2]?.pending.resolve({ ...READY, path: CURRENT_DATABASE_PATH }),
      );
      await waitFor(() =>
        expect(screen.getByTestId("current").textContent).toContain(CURRENT_DATABASE_PATH),
      );
      releaseCancellation.resolve();
      await act(async () => {
        requests[0]?.pending.resolve({ ...READY, path: "/late-a.db" });
        requests[1]?.pending.resolve({ ...READY, path: "/late-b.db" });
      });

      expect(screen.getByTestId("current").textContent).toContain(CURRENT_DATABASE_PATH);
      expect(screen.getByTestId("current").textContent).not.toContain("late-");
      expect(
        client.getQueryData([
          "system",
          "database-stats",
          "https://backend.example/one",
          "boot-a",
          "enabled",
          true,
          "user-1",
        ]),
      ).toEqual({ ...READY, path: CURRENT_DATABASE_PATH });
      expect(store?.getState().system).not.toHaveProperty("database");
    } finally {
      releaseCancellation.resolve();
      for (const request of requests) request.pending.resolve(READY);
    }
  });
});
