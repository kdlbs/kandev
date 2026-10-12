import { QueryClient, QueryObserver, useQueryClient } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createElement, Fragment, useEffect, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { DiskUsageResponse } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { createDiskUsageQueryKey } from "./system-info-query";
import { useDiskUsage } from "./use-disk-usage";

const config = vi.hoisted(() => ({ apiBaseUrl: "" }));
const BACKEND_URL = "https://backend.example/api";
const OTHER_BACKEND_URL = "https://backend.example/other";
const BOOT_ID = "disk-page-boot";
const DISK_USAGE_STATE_TEST_ID = "disk-usage-state";
const NOT_LOADING = '"isLoading":false';
const REFRESH_FAILED_MESSAGE = "refresh failed";

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

const QUERY_KEY = ["system", "disk-usage", BACKEND_URL, "boot-1"] as const;

const EMPTY_USAGE: DiskUsageResponse = {
  data: null,
  computing: false,
  home_dir: "/data/kandev",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (cause: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function response(computing: boolean): DiskUsageResponse {
  return { ...EMPTY_USAGE, computing };
}

function createQueryFixture(initialData?: DiskUsageResponse) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity, networkMode: "always" } },
  });
  if (initialData) queryClient.setQueryData(QUERY_KEY, initialData);

  const requests: Array<{
    signal: AbortSignal;
    deferred: ReturnType<typeof deferred<DiskUsageResponse>>;
  }> = [];
  const observer = new QueryObserver(queryClient, {
    queryKey: QUERY_KEY,
    queryFn: ({ signal }) => {
      const pending = deferred<DiskUsageResponse>();
      requests.push({ signal, deferred: pending });
      return pending.promise;
    },
    staleTime: Infinity,
    refetchOnMount: true,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const unsubscribe = observer.subscribe(() => undefined);

  return { queryClient, observer, requests, unsubscribe };
}

async function cancelAndInvalidate(
  queryClient: QueryClient,
  queryKey: readonly unknown[],
): Promise<void> {
  await queryClient.cancelQueries({ queryKey, exact: true });
  await queryClient.invalidateQueries({ queryKey, exact: true, refetchType: "active" });
}

describe("installed TanStack disk-query invalidation behavior", () => {
  const clients: QueryClient[] = [];
  afterEach(() => {
    for (const client of clients.splice(0)) client.clear();
  });

  it.each([false, true])(
    "replaces a no-data initial GET when the aborted response later says computing=%s",
    async (staleComputing) => {
      const fixture = createQueryFixture();
      clients.push(fixture.queryClient);
      const { queryClient, observer, requests, unsubscribe } = fixture;
      expect(requests).toHaveLength(1);

      const transition = cancelAndInvalidate(queryClient, QUERY_KEY);
      await expect.poll(() => requests.length).toBe(2);
      expect(requests[0].signal.aborted).toBe(true);

      requests[0].deferred.resolve(response(staleComputing));
      await Promise.resolve();
      expect(queryClient.getQueryData(QUERY_KEY)).toBeUndefined();

      const authoritative = response(false);
      requests[1].deferred.resolve(authoritative);
      await transition;

      expect(observer.getCurrentResult().data).toEqual(authoritative);
      expect(requests).toHaveLength(2);
      unsubscribe();
    },
  );

  it.each([false, true])(
    "replaces a cached-data refetch when the aborted response later says computing=%s",
    async (staleComputing) => {
      const fixture = createQueryFixture(response(true));
      clients.push(fixture.queryClient);
      const { queryClient, observer, requests, unsubscribe } = fixture;
      expect(requests).toHaveLength(0);

      const oldRefetch = observer.refetch();
      await expect.poll(() => requests.length).toBe(1);
      const transition = cancelAndInvalidate(queryClient, QUERY_KEY);
      await expect.poll(() => requests.length).toBe(2);
      expect(requests[0].signal.aborted).toBe(true);

      requests[0].deferred.resolve(response(staleComputing));
      await Promise.resolve();
      expect(queryClient.getQueryData(QUERY_KEY)).toEqual(response(true));

      const authoritative = response(false);
      requests[1].deferred.resolve(authoritative);
      await Promise.all([oldRefetch, transition]);

      expect(observer.getCurrentResult().data).toEqual(authoritative);
      expect(requests).toHaveLength(2);
      unsubscribe();
    },
  );
});

let currentReload: (() => Promise<void>) | undefined;
let currentRefresh: (() => Promise<void>) | undefined;
let capturedStore: StoreApi<AppState> | undefined;
let capturedQueryClient: QueryClient | undefined;

function StoreCapture({ onStore }: { onStore?: (store: StoreApi<AppState>) => void }) {
  const store = useAppStoreApi();
  useEffect(() => {
    onStore?.(store);
  }, [onStore, store]);
  return null;
}

function QueryClientCapture({ onCapture }: { onCapture?: (client: QueryClient) => void }) {
  const client = useQueryClient();
  useEffect(() => {
    onCapture?.(client);
  }, [client, onCapture]);
  return null;
}

function DiskUsageProbe() {
  const { diskUsage, isLoading, error, reload, refresh } = useDiskUsage();
  currentReload = reload;
  currentRefresh = refresh;
  return createElement(
    "output",
    { "data-testid": DISK_USAGE_STATE_TEST_ID },
    JSON.stringify({ diskUsage, isLoading, error }),
  );
}

function TestHarness({
  children,
  apiBaseUrl = `${BACKEND_URL}///`,
  onStore,
  onQueryClient,
}: {
  children?: ReactNode;
  apiBaseUrl?: string;
  onStore?: (store: StoreApi<AppState>) => void;
  onQueryClient?: (client: QueryClient) => void;
}) {
  config.apiBaseUrl = apiBaseUrl;
  return createElement(StateProvider, {
    initialState: { auth: AUTH },
    children: createElement(
      Fragment,
      null,
      createElement(StoreCapture, { onStore }),
      createElement(SystemInfoQueryProvider, {
        bootId: BOOT_ID,
        children: createElement(
          Fragment,
          null,
          createElement(QueryClientCapture, { onCapture: onQueryClient }),
          children,
        ),
      }),
    ),
  });
}

beforeEach(() => {
  vi.mocked(api.fetchDiskUsage).mockReset();
  vi.mocked(api.refreshDiskUsage).mockReset();
});

afterEach(() => {
  cleanup();
  capturedQueryClient?.clear();
  vi.useRealTimers();
  currentReload = undefined;
  currentRefresh = undefined;
  capturedStore = undefined;
  capturedQueryClient = undefined;
  vi.clearAllMocks();
});

describe("useDiskUsage query ownership", () => {
  it("loads into the exact identity query and exposes fetching for the GET", async () => {
    const pending = deferred<DiskUsageResponse>();
    vi.mocked(api.fetchDiskUsage).mockReturnValueOnce(pending.promise);
    render(
      createElement(
        TestHarness,
        {
          onStore: (store) => {
            capturedStore = store;
          },
          onQueryClient: (client) => {
            capturedQueryClient = client;
          },
        },
        createElement(DiskUsageProbe),
      ),
    );

    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    const requestOptions = vi.mocked(api.fetchDiskUsage).mock.calls[0]?.[0];
    expect(requestOptions?.baseUrl).toBe(BACKEND_URL);
    expect(requestOptions?.cache).toBe("no-store");
    expect(requestOptions?.init?.signal).toBeInstanceOf(AbortSignal);
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain('"isLoading":true');
    expect(capturedStore?.getState().system).not.toHaveProperty("diskUsage");

    const usage = response(false);
    pending.resolve(usage);
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        '"computing":false',
      ),
    );
    expect(
      capturedQueryClient?.getQueryData(
        createDiskUsageQueryKey({
          apiBaseUrl: BACKEND_URL,
          bootId: BOOT_ID,
          authMode: "enabled",
          authenticated: true,
          userId: "user-1",
        }),
      ),
    ).toEqual(usage);
  });

  it("keeps last-good data and resolves reload after a visible GET error", async () => {
    const usage = response(false);
    vi.mocked(api.fetchDiskUsage).mockResolvedValueOnce(usage);
    render(
      createElement(
        TestHarness,
        { onQueryClient: (client) => (capturedQueryClient = client) },
        createElement(DiskUsageProbe),
      ),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        '"computing":false',
      ),
    );

    const failedRead = deferred<DiskUsageResponse>();
    vi.mocked(api.fetchDiskUsage).mockReturnValueOnce(failedRead.promise);
    let reloadResult!: Promise<void>;
    act(() => {
      reloadResult = currentReload?.() ?? Promise.resolve();
    });
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        '"isLoading":true',
      ),
    );
    failedRead.reject(new Error("disk read failed"));
    await expect(reloadResult).resolves.toBeUndefined();

    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        "disk read failed",
      ),
    );
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
      '"diskUsage":{"data":null,"computing":false',
    );
  });
});

describe("useDiskUsage refresh outcomes", () => {
  it("hides a prior GET error while the refresh POST is pending", async () => {
    const usage = response(false);
    vi.mocked(api.fetchDiskUsage)
      .mockResolvedValueOnce(usage)
      .mockRejectedValueOnce(new Error("old read failed"));
    const post = deferred<{ job_id: string }>();
    vi.mocked(api.refreshDiskUsage).mockReturnValueOnce(post.promise);
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const failedReload = currentReload?.();
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await expect(failedReload).resolves.toBeUndefined();
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain("old read failed"),
    );

    const refreshResult = currentRefresh?.();
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain('"error":null'),
    );
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
      '"diskUsage":{"data":null,"computing":false',
    );

    post.reject(new Error(REFRESH_FAILED_MESSAGE));
    await expect(refreshResult).resolves.toBeUndefined();
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        REFRESH_FAILED_MESSAGE,
      ),
    );
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2);
  });

  it("keeps a refresh POST failure visible and resolves without a follow-up GET", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValueOnce(response(false));
    vi.mocked(api.refreshDiskUsage).mockRejectedValueOnce(new Error(REFRESH_FAILED_MESSAGE));
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    await waitFor(() => expect(currentRefresh).toBeTypeOf("function"));

    const refreshResult = currentRefresh?.();
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());
    await expect(refreshResult).resolves.toBeUndefined();

    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
      REFRESH_FAILED_MESSAGE,
    );
  });

  it("performs an authoritative GET after a successful refresh POST", async () => {
    const first = response(false);
    const refreshed = response(true);
    vi.mocked(api.fetchDiskUsage).mockResolvedValueOnce(first).mockResolvedValueOnce(refreshed);
    vi.mocked(api.refreshDiskUsage).mockResolvedValueOnce({ job_id: "disk-job" });
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const refreshResult = currentRefresh?.();
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await expect(refreshResult).resolves.toBeUndefined();

    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain('"computing":true');
    expect(vi.mocked(api.refreshDiskUsage).mock.calls[0]?.[0]?.baseUrl).toBe(BACKEND_URL);
  });

  it("shows a follow-up GET error while retaining the last-good snapshot after refresh", async () => {
    const usage = response(false);
    vi.mocked(api.fetchDiskUsage)
      .mockResolvedValueOnce(usage)
      .mockRejectedValueOnce(new Error("refresh read failed"));
    vi.mocked(api.refreshDiskUsage).mockResolvedValueOnce({ job_id: "disk-job" });
    render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const refreshResult = currentRefresh?.();
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await expect(refreshResult).resolves.toBeUndefined();

    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        "refresh read failed",
      ),
    );
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
      '"diskUsage":{"data":null,"computing":false',
    );
  });
});

describe("useDiskUsage refresh identity", () => {
  it("does not carry a handled refresh error into a different backend identity", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(response(false));
    vi.mocked(api.refreshDiskUsage).mockRejectedValueOnce(new Error("old refresh failed"));
    const view = render(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );

    const refreshResult = currentRefresh?.();
    await expect(refreshResult).resolves.toBeUndefined();
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(
        "old refresh failed",
      ),
    );

    view.rerender(
      createElement(TestHarness, { apiBaseUrl: OTHER_BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain('"error":null');
  });
});

describe("useDiskUsage obsolete POST continuations", () => {
  it("does not publish an obsolete refresh POST error after backend identity changes", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(response(false));
    const post = deferred<{ job_id: string }>();
    vi.mocked(api.refreshDiskUsage).mockReturnValueOnce(post.promise);
    const view = render(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    await waitFor(() => expect(currentRefresh).toBeTypeOf("function"));
    const obsoleteRefresh = currentRefresh;
    let refreshResult!: Promise<void>;
    act(() => {
      refreshResult = obsoleteRefresh?.() ?? Promise.resolve();
    });
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());
    const obsoletePostOptions = vi.mocked(api.refreshDiskUsage).mock.calls[0]?.[0];

    view.rerender(
      createElement(TestHarness, { apiBaseUrl: OTHER_BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    post.reject(new Error("obsolete refresh failed"));
    await expect(refreshResult).resolves.toBeUndefined();

    expect(obsoletePostOptions?.baseUrl).toBe(BACKEND_URL);
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).not.toContain(
      "obsolete refresh failed",
    );
    expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain('"error":null');
  });

  it("does not refetch after a successful POST settles across A-to-B-to-A", async () => {
    vi.mocked(api.fetchDiskUsage).mockResolvedValue(response(false));
    const post = deferred<{ job_id: string }>();
    vi.mocked(api.refreshDiskUsage).mockReturnValueOnce(post.promise);
    const view = render(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(screen.getByTestId(DISK_USAGE_STATE_TEST_ID).textContent).toContain(NOT_LOADING),
    );
    const oldRefresh = currentRefresh;
    let refreshResult!: Promise<void>;
    act(() => {
      refreshResult = oldRefresh?.() ?? Promise.resolve();
    });
    await waitFor(() => expect(api.refreshDiskUsage).toHaveBeenCalledOnce());

    view.rerender(
      createElement(TestHarness, { apiBaseUrl: OTHER_BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2));
    view.rerender(
      createElement(TestHarness, { apiBaseUrl: BACKEND_URL }, createElement(DiskUsageProbe)),
    );
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3));

    post.resolve({ job_id: "old-disk-job" });
    await expect(refreshResult).resolves.toBeUndefined();
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
  });
});

describe("useDiskUsage cancellation and recovery", () => {
  it("aborts an in-flight disk GET when the hook unmounts", async () => {
    const pending = deferred<DiskUsageResponse>();
    vi.mocked(api.fetchDiskUsage).mockReturnValueOnce(pending.promise);
    const view = render(createElement(TestHarness, null, createElement(DiskUsageProbe)));
    await waitFor(() => expect(api.fetchDiskUsage).toHaveBeenCalledOnce());
    const signal = vi.mocked(api.fetchDiskUsage).mock.calls[0]?.[0]?.init?.signal as
      | AbortSignal
      | undefined;

    view.unmount();

    expect(signal?.aborted).toBe(true);
    pending.resolve(response(false));
  });

  it("polls only while the retained snapshot says computing, including after a read error", async () => {
    vi.useFakeTimers();
    vi.mocked(api.fetchDiskUsage)
      .mockResolvedValueOnce(response(true))
      .mockRejectedValueOnce(new Error("poll failed"))
      .mockResolvedValueOnce(response(false));
    render(
      createElement(
        TestHarness,
        { onQueryClient: (client) => (capturedQueryClient = client) },
        createElement(DiskUsageProbe),
      ),
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1499);
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledOnce();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(2);
    expect(
      capturedQueryClient?.getQueryState(
        createDiskUsageQueryKey({
          apiBaseUrl: BACKEND_URL,
          bootId: BOOT_ID,
          authMode: "enabled",
          authenticated: true,
          userId: "user-1",
        }),
      )?.error,
    ).toBeInstanceOf(Error);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1500);
      await Promise.resolve();
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
    expect(
      capturedQueryClient?.getQueryData(
        createDiskUsageQueryKey({
          apiBaseUrl: BACKEND_URL,
          bootId: BOOT_ID,
          authMode: "enabled",
          authenticated: true,
          userId: "user-1",
        }),
      ),
    ).toEqual(response(false));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(api.fetchDiskUsage).toHaveBeenCalledTimes(3);
  });
});
