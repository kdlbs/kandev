import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createElement, Fragment, StrictMode, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { SystemJob, DiskUsageResponse } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { registerSystemEventsHandlers } from "@/lib/ws/handlers/system-events";
import { useDiskUsage } from "@/hooks/domains/system/use-disk-usage";
import { createDiskUsageQueryKey } from "@/hooks/domains/system/system-info-query";

const config = vi.hoisted(() => ({ apiBaseUrl: "" }));
const BACKEND_URL = "https://backend.example/api";
const OTHER_BACKEND_URL = "https://backend.example/other";
const BOOT_ID = "disk-bridge-boot";
const QUERY_STATE_TEST_ID = "disk-usage-query-state";
const NOT_LOADING = '"isLoading":false';
const NEW_DISK_JOB_ID = "new-disk-job";
const BACKUP_JOB_KIND = "backup-create";
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }) }));
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

const EMPTY_USAGE: DiskUsageResponse = {
  data: null,
  computing: false,
  home_dir: "/data/kandev",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function diskUsage(computing: boolean): DiskUsageResponse {
  return { ...EMPTY_USAGE, computing };
}

function job(id: string, state: SystemJob["state"], kind = "disk-walk"): SystemJob {
  return { id, kind, state, started_at: "2026-10-07T08:00:00Z" };
}

let store: StoreApi<AppState> | undefined;
let queryClient: QueryClient | undefined;
let currentReload: (() => Promise<void>) | undefined;

function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

function CaptureQueryClient() {
  queryClient = useQueryClient();
  return null;
}

function DiskUsageProbe() {
  const query = useDiskUsage();
  currentReload = query.reload;
  return createElement(
    "output",
    { "data-testid": QUERY_STATE_TEST_ID },
    JSON.stringify({ diskUsage: query.diskUsage, isLoading: query.isLoading, error: query.error }),
  );
}

function TestHarness({
  children,
  initialJobs = {},
  apiBaseUrl = BACKEND_URL,
  bootId = BOOT_ID,
}: {
  children?: ReactNode;
  initialJobs?: AppState["system"]["jobs"];
  apiBaseUrl?: string;
  bootId?: string;
}) {
  config.apiBaseUrl = apiBaseUrl;
  return createElement(StateProvider, {
    initialState: { auth: AUTH, system: { jobs: initialJobs } },
    children: createElement(SystemInfoQueryProvider, {
      bootId,
      children: createElement(
        Fragment,
        null,
        createElement(CaptureStore),
        createElement(CaptureQueryClient),
        children,
      ),
    }),
  });
}

function updateJob(next: SystemJob) {
  if (!store) throw new Error("The app store should be captured");
  const handler = registerSystemEventsHandlers(store)["system.job.update"];
  if (!handler) throw new Error("The system job update handler should be registered");
  handler({
    type: "notification",
    action: "system.job.update",
    payload: next,
  });
}

beforeEach(() => {
  vi.mocked(api.fetchDiskUsage).mockReset();
  store = undefined;
  queryClient = undefined;
  currentReload = undefined;
});

afterEach(() => {
  cleanup();
  queryClient?.clear();
  store = undefined;
  queryClient = undefined;
  currentReload = undefined;
  vi.clearAllMocks();
});

describe("SystemInfoQueryProvider disk job bridge transitions", () => {
  it.each(["succeeded", "failed"] as const)(
    "revalidates the active snapshot when the real job handler receives %s",
    async (terminalState) => {
      vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
      render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
      await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
      await waitFor(() =>
        expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
      );

      act(() => updateJob(job("disk-job", terminalState)));

      await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
      expect(store?.getState().system.jobs["disk-job"]?.state).toBe(terminalState);
    },
  );

  it("baselines retained terminal rows and suppresses out-of-order duplicate updates", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const initialJob = job("retained-disk-job", "succeeded");
    render(
      createElement(
        TestHarness,
        { initialJobs: { [initialJob.id]: initialJob } },
        createElement(DiskUsageProbe),
      ),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    act(() => updateJob(job(initialJob.id, "succeeded")));
    act(() => updateJob(job(NEW_DISK_JOB_ID, "running")));
    act(() => updateJob(job(NEW_DISK_JOB_ID, "succeeded")));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    act(() => {
      for (let index = 0; index < 16; index += 1) {
        updateJob(job(NEW_DISK_JOB_ID, "running"));
        updateJob(job(NEW_DISK_JOB_ID, "succeeded"));
      }
    });
    act(() => updateJob(job("unrelated-job", "succeeded", BACKUP_JOB_KIND)));

    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2);
    expect(store?.getState().system.jobs["unrelated-job"]?.kind).toBe(BACKUP_JOB_KIND);
  });

  it("bounds reads for a burst of distinct terminal disk jobs", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const acceptedEvents = 16;
    act(() => {
      for (let index = 0; index < acceptedEvents; index += 1) {
        updateJob(job(`burst-disk-job-${index}`, "succeeded"));
      }
    });
    await act(async () => new Promise((resolve) => setTimeout(resolve, 0)));

    const requestCount = vi.mocked(api.fetchDiskUsage).mock.calls.length;
    expect(requestCount).toBeGreaterThan(1);
    expect(requestCount).toBeLessThanOrEqual(acceptedEvents + 1);
  });
});

describe("SystemInfoQueryProvider disk query races", () => {
  it("replaces a no-data GET after completion and ignores its late aborted response", async () => {
    const requests: Array<{
      signal?: AbortSignal;
      pending: ReturnType<typeof deferred<DiskUsageResponse>>;
    }> = [];
    vi.mocked(api.fetchDiskUsage).mockImplementation((options) => {
      const pending = deferred<DiskUsageResponse>();
      requests.push({ signal: options?.init?.signal as AbortSignal | undefined, pending });
      return pending.promise;
    });
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(requests).toHaveLength(1));

    act(() => updateJob(job("disk-job", "failed")));
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[0]?.signal?.aborted).toBe(true);

    requests[0]?.pending.resolve(diskUsage(true));
    await act(async () => Promise.resolve());
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"diskUsage":null');

    requests[1]?.pending.resolve(diskUsage(false));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"computing":false'),
    );
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2);
  });

  it.each([false, true])(
    "replaces an in-flight cached refetch when completion arrives before stale computing=%s",
    async (staleComputing) => {
      const requests: Array<{
        signal?: AbortSignal;
        pending: ReturnType<typeof deferred<DiskUsageResponse>>;
      }> = [];
      vi.mocked(api.fetchDiskUsage).mockImplementation((options) => {
        const pending = deferred<DiskUsageResponse>();
        requests.push({ signal: options?.init?.signal as AbortSignal | undefined, pending });
        return pending.promise;
      });
      render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
      await waitFor(() => expect(requests).toHaveLength(1));
      requests[0]?.pending.resolve(diskUsage(true));
      await waitFor(() =>
        expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"computing":true'),
      );

      let reloadResult!: Promise<void>;
      act(() => {
        reloadResult = currentReload?.() ?? Promise.resolve();
      });
      await waitFor(() => expect(requests).toHaveLength(2));
      act(() => updateJob(job("cached-disk-job", "succeeded")));
      await waitFor(() => expect(requests).toHaveLength(3));
      expect(requests[1]?.signal?.aborted).toBe(true);
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"isLoading":true');

      requests[1]?.pending.resolve(diskUsage(staleComputing));
      await act(async () => Promise.resolve());
      expect(
        queryClient?.getQueryData(
          createDiskUsageQueryKey({
            apiBaseUrl: BACKEND_URL,
            bootId: BOOT_ID,
            authMode: "enabled",
            authenticated: true,
            userId: "user-1",
          }),
        ),
      ).toEqual(diskUsage(true));
      requests[2]?.pending.resolve(diskUsage(false));
      await Promise.all([
        reloadResult,
        waitFor(() =>
          expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(
            '"computing":false',
          ),
        ),
      ]);
      expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
    },
  );
});

describe("SystemInfoQueryProvider auth identity lifecycle", () => {
  it("does not replay a retained terminal job across logout identity changes", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const retained = job("retained-job", "failed");
    render(
      createElement(
        TestHarness,
        { initialJobs: { [retained.id]: retained } },
        createElement(DiskUsageProbe),
      ),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    act(() => store?.getState().clearAuthenticated());
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    act(() =>
      store?.getState().setAuthState({ mode: "disabled", authenticated: false, user: null }),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    expect(store?.getState().system.jobs[retained.id]).toEqual(retained);
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
  });
});

describe("SystemInfoQueryProvider backend identity lifecycle", () => {
  it("guards a delayed old-identity cancellation across A-to-B-to-A", async () => {
    const requests: Array<{
      baseUrl?: string;
      pending: ReturnType<typeof deferred<DiskUsageResponse>>;
    }> = [];
    vi.mocked(api.fetchDiskUsage).mockImplementation((options) => {
      const pending = deferred<DiskUsageResponse>();
      requests.push({ baseUrl: options?.baseUrl, pending });
      return pending.promise;
    });
    const view = render(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    requests[0]?.pending.resolve(diskUsage(false));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const cancelGate = deferred<void>();
    const cancelQueries = queryClient?.cancelQueries.bind(queryClient);
    if (!queryClient || !cancelQueries) throw new Error("The query client should be captured");
    const cancelSpy = vi
      .spyOn(queryClient, "cancelQueries")
      .mockImplementation((filters, options) => {
        const canceled = cancelQueries(filters, options);
        if (filters?.exact) return canceled.then(() => cancelGate.promise);
        return canceled;
      });
    act(() => updateJob(job("old-identity-job", "succeeded")));
    await waitFor(() =>
      expect(cancelSpy).toHaveBeenCalledWith(
        expect.objectContaining({ exact: true, queryKey: expect.any(Array) }),
      ),
    );

    view.rerender(
      createElement(TestHarness, { apiBaseUrl: OTHER_BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(
      queryClient.getQueryData(
        createDiskUsageQueryKey({
          apiBaseUrl: BACKEND_URL,
          bootId: BOOT_ID,
          authMode: "enabled",
          authenticated: true,
          userId: "user-1",
        }),
      ),
    ).toBeUndefined();
    requests[1]?.pending.resolve(diskUsage(false));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    view.rerender(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(requests).toHaveLength(3));
    requests[2]?.pending.resolve(diskUsage(false));
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    cancelGate.resolve();
    await act(async () => Promise.resolve());
    expect(requests.map((request) => request.baseUrl)).toEqual([
      BACKEND_URL,
      OTHER_BACKEND_URL,
      BACKEND_URL,
    ]);
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
  });
});

describe("SystemInfoQueryProvider scoped event updates", () => {
  it("does not issue reads for unrelated jobs or other System state changes", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    act(() => updateJob(job("backup-job", "succeeded", BACKUP_JOB_KIND)));
    act(() =>
      store?.getState().setSystemUpdates({
        current: "1.2.3",
        latest: "1.2.4",
        latest_url: "https://example.com/release",
        latest_checked_at: "2026-10-07T08:00:00Z",
        update_available: true,
        channel: "stable",
        channel_editable: true,
        channel_unsupported_reason: "",
      }),
    );

    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
    expect(store?.getState().system.jobs["backup-job"]?.kind).toBe(BACKUP_JOB_KIND);
    expect(store?.getState().system.updates?.latest).toBe("1.2.4");
  });

  it("documents FIFO eviction by allowing a replay after the retained row is removed", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const initialJobs = Object.fromEntries(
      Array.from({ length: 64 }, (_, index) => {
        const initial = job(`retained-${index}`, "succeeded");
        return [initial.id, initial];
      }),
    );
    render(createElement(TestHarness, { initialJobs }, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    act(() => updateJob(job("newest-retained", "failed")));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    act(() => updateJob(job("retained-0", "succeeded")));
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2);
    act(() => updateJob(job("retained-0", "running")));
    act(() => updateJob(job("retained-0", "succeeded")));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3));
    expect(store?.getState().system.jobs["retained-0"]?.state).toBe("succeeded");
  });
});

describe("SystemInfoQueryProvider consumer lifetimes", () => {
  it("treats deep-merged terminal job additions as store updates after subscription", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    act(() =>
      store?.getState().hydrate({
        system: { jobs: { "late-hydrated-job": job("late-hydrated-job", "failed") } },
      }),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    expect(store?.getState().system.jobs["late-hydrated-job"]?.state).toBe("failed");
  });

  it("does not create a Query or read when no disk consumer is mounted", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(createElement(TestHarness, null, null));
    act(() => updateJob(job("no-consumer-job", "succeeded")));
    await act(async () => Promise.resolve());
    expect(api.fetchDiskUsage).not.toHaveBeenCalled();
    expect(queryClient?.getQueryCache().getAll()).toHaveLength(0);
  });

  it("shares one initial and event read across multiple consumers", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(
      createElement(
        TestHarness,
        null,
        createElement(Fragment, null, createElement(DiskUsageProbe), createElement(DiskUsageProbe)),
      ),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getAllByTestId(QUERY_STATE_TEST_ID)[0]?.textContent).toContain(NOT_LOADING),
    );
    act(() => updateJob(job("shared-query-job", "succeeded")));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    expect(queryClient?.getQueryCache().getAll()).toHaveLength(1);
  });
});

describe("SystemInfoQueryProvider inactive query lifecycle", () => {
  it("marks an inactive query invalid and reads it only when the consumer returns", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const view = render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    view.rerender(createElement(TestHarness, null, null));
    act(() => updateJob(job("inactive-query-job", "succeeded")));
    await act(async () => Promise.resolve());
    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
    const key = createDiskUsageQueryKey({
      apiBaseUrl: BACKEND_URL,
      bootId: BOOT_ID,
      authMode: "enabled",
      authenticated: true,
      userId: "user-1",
    });
    expect(queryClient?.getQueryState(key)?.isInvalidated).toBe(true);

    view.rerender(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
  });

  it("cleans up the subscription when the provider unmounts", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const view = render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    const currentStore = store;

    view.unmount();
    act(() => currentStore?.getState().upsertSystemJob(job("after-unmount-job", "succeeded")));
    await act(async () => Promise.resolve());
    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
  });
});

describe("SystemInfoQueryProvider cancellation lifecycle", () => {
  it("retains an inactive query invalidation when navigation leaves before cancellation resolves", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    const view = render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const cancelGate = deferred<void>();
    if (!queryClient) throw new Error("The query client should be captured");
    const cancelQueries = queryClient.cancelQueries.bind(queryClient);
    vi.spyOn(queryClient, "cancelQueries").mockImplementation((filters, options) => {
      const canceled = cancelQueries(filters, options);
      if (filters?.exact) return canceled.then(() => cancelGate.promise);
      return canceled;
    });
    act(() => updateJob(job("route-departure-job", "succeeded")));
    await waitFor(() =>
      expect(queryClient?.cancelQueries).toHaveBeenCalledWith(
        expect.objectContaining({ exact: true, queryKey: expect.any(Array) }),
      ),
    );

    view.rerender(createElement(TestHarness, null, null));
    cancelGate.resolve();
    await act(async () => Promise.resolve());
    const key = createDiskUsageQueryKey({
      apiBaseUrl: BACKEND_URL,
      bootId: BOOT_ID,
      authMode: "enabled",
      authenticated: true,
      userId: "user-1",
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
    expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);

    view.rerender(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
  });

  it("keeps one active disk observer after StrictMode subscription replay", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(diskUsage(false));
    render(
      createElement(
        StrictMode,
        null,
        createElement(TestHarness, null, createElement(DiskUsageProbe)),
      ),
    );
    await waitFor(() =>
      expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    const settledReads = vi.mocked(api.fetchDiskUsage).mock.calls.length;
    expect(settledReads).toBeGreaterThanOrEqual(1);

    act(() => updateJob(job("strict-mode-job", "failed")));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(settledReads + 1));
    expect(queryClient?.getQueryCache().getAll()).toHaveLength(1);
  });
});
