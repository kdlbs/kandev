import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { DatabaseStats } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { useDatabaseStats } from "./use-database-stats";

const store = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const state = {
    database: null as DatabaseStats | null,
    setSystemDatabase: vi.fn(),
    notify: () => listeners.forEach((listener) => listener()),
    subscribe: (listener: () => void) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
  state.setSystemDatabase = vi.fn((database: DatabaseStats) => {
    state.database = database;
    state.notify();
  });
  return state;
});

vi.mock("@/components/state-provider", async () => {
  const React = await vi.importActual<typeof import("react")>("react");
  return {
    useAppStore: (selector: (state: unknown) => unknown) => {
      const [database, setDatabase] = React.useState(store.database);
      React.useEffect(() => store.subscribe(() => setDatabase(store.database)), []);
      return selector({
        system: { database },
        setSystemDatabase: store.setSystemDatabase,
      });
    },
  };
});
vi.mock("@/lib/api/domains/system-api");

const measured: DatabaseStats = {
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
  logical_stats_measured_at: "2026-09-27T10:00:00Z",
  metadata_stale: false,
  metadata_measured_at: "2026-09-27T10:00:01Z",
  schema_version: "1",
  last_backup_at: null,
};

beforeEach(() => {
  vi.resetAllMocks();
  store.database = null;
  store.setSystemDatabase.mockImplementation((database: DatabaseStats) => {
    store.database = database;
    store.notify();
  });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

it("keeps the last database response when a refresh fails", async () => {
  store.database = measured;
  const failure = new Error("database status unavailable");
  vi.mocked(api.fetchDatabaseStats).mockRejectedValue(failure);

  const { result } = renderHook(() => useDatabaseStats());
  await waitFor(() => expect(result.current.error).toBe(failure.message));

  expect(result.current.database).toBe(measured);
  expect(store.setSystemDatabase).not.toHaveBeenCalled();
});

it("requests an immediate server retry before reading the new scan state", async () => {
  const stale: DatabaseStats = { ...measured, logical_stats_state: "stale" };
  const refreshing: DatabaseStats = { ...measured, logical_stats_state: "refreshing" };
  store.database = stale;
  const requests: string[] = [];
  vi.mocked(api.fetchDatabaseStats).mockImplementation(async () => {
    requests.push("status");
    return requests.length === 1 ? stale : refreshing;
  });
  vi.mocked(api.retryDatabaseStats).mockImplementation(async () => {
    requests.push("retry");
  });

  const { result } = renderHook(() => useDatabaseStats());
  await waitFor(() => expect(result.current.database?.logical_stats_state).toBe("stale"));
  await act(async () => {
    await result.current.retry();
  });

  expect(requests).toEqual(["status", "retry", "status"]);
  expect(result.current.database?.logical_stats_state).toBe("refreshing");
});

it("retries a failed status read after a bounded delay when ready data is cached", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-27T10:01:00Z"));
  store.database = measured;
  const failure = new Error("database status unavailable");
  const refreshing: DatabaseStats = { ...measured, logical_stats_state: "refreshing" };
  vi.mocked(api.fetchDatabaseStats)
    .mockRejectedValueOnce(failure)
    .mockResolvedValueOnce(refreshing);

  const { result } = renderHook(() => useDatabaseStats());
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.error).toBe(failure.message);
  expect(result.current.database).toBe(measured);
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(1);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(29_999);
  });
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(1);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(result.current.database?.logical_stats_state).toBe("refreshing");
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
});

it("stores a cold pending response and polls to the measured result while mounted", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-27T10:01:00Z"));
  const pending: DatabaseStats = {
    ...measured,
    message_content_bytes: null,
    message_metadata_bytes: null,
    message_payload_bytes: null,
    git_snapshot_bytes: null,
    logical_stats_state: "pending",
    logical_stats_measured_at: null,
  };
  vi.mocked(api.fetchDatabaseStats).mockResolvedValueOnce(pending).mockResolvedValueOnce(measured);
  const { result, unmount } = renderHook(() => useDatabaseStats());

  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.database).toEqual(pending);
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(1);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(2_000);
  });
  expect(result.current.database).toEqual(measured);
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);

  unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(10_000);
  });
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
});

it("revalidates a stored status and polls through the backend retry backoff", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-27T12:00:00Z"));
  const stale: DatabaseStats = {
    ...measured,
    logical_stats_state: "stale",
  };
  const refreshing: DatabaseStats = {
    ...stale,
    logical_stats_state: "refreshing",
  };
  store.database = stale;
  vi.mocked(api.fetchDatabaseStats)
    .mockResolvedValueOnce(stale)
    .mockResolvedValueOnce(refreshing)
    .mockResolvedValueOnce(measured);
  const { result } = renderHook(() => useDatabaseStats());

  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.database?.logical_stats_state).toBe("stale");
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(1);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(29_999);
  });
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(1);
  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(result.current.database?.logical_stats_state).toBe("refreshing");
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(2_000);
  });
  expect(result.current.database?.logical_stats_state).toBe("ready");
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(3);
});

it("refreshes a ready snapshot when its measurement expires", async () => {
  vi.useFakeTimers();
  const expiresAt = new Date(measured.logical_stats_measured_at!).getTime() + 15 * 60_000;
  vi.setSystemTime(expiresAt - 1);
  const refreshing: DatabaseStats = { ...measured, logical_stats_state: "refreshing" };
  store.database = measured;
  vi.mocked(api.fetchDatabaseStats)
    .mockResolvedValueOnce(measured)
    .mockResolvedValueOnce(refreshing);
  const { result } = renderHook(() => useDatabaseStats());

  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(result.current.database?.logical_stats_state).toBe("ready");

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1);
  });
  expect(result.current.database?.logical_stats_state).toBe("refreshing");
  expect(api.fetchDatabaseStats).toHaveBeenCalledTimes(2);
});
