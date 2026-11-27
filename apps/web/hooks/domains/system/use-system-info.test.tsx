import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { onlineManager, QueryClient, useQueryClient } from "@tanstack/react-query";
import { StrictMode, useEffect, useState, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { SystemInfo } from "@/lib/types/system";
import { createSystemInfoQueryKey, normalizeSystemInfoApiBaseUrl } from "./system-info-query";

const BACKEND_ORIGIN = "https://backend.example";
const PAGE_BOOT_ID = "page-boot-1";
const SYSTEM_INFO_ERROR = "system info failed";
const VERSION_1 = '"version":"1.2.3"';
const LOADING = '"isLoading":true';
const QUERY_STATE_TEST_ID = "query-state";
const config = vi.hoisted(() => ({ apiBaseUrl: "https://backend.example" }));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }),
}));

import { useSystemInfo } from "./use-system-info";

const INFO: SystemInfo = {
  version: "1.2.3",
  commit: "abc1234",
  build_time: "2026-01-01T00:00:00Z",
  go_version: "go1.24",
  os: "linux",
  arch: "amd64",
  boot_id: "server-boot",
  started_at: "2026-01-01T00:00:00Z",
};

const AUTH = {
  mode: "enabled" as const,
  authenticated: true,
  user: {
    id: "user-1",
    email: "user@example.com",
    display_name: "User",
    role: "admin",
    status: "active",
  },
};

let currentReload: (() => Promise<void>) | undefined;

function makeResponse(info: SystemInfo, status = 200): Response {
  return new Response(JSON.stringify(status === 200 ? info : { error: SYSTEM_INFO_ERROR }), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function StoreCapture({ onStore }: { onStore?: (store: StoreApi<AppState>) => void }) {
  const store = useAppStoreApi();
  useEffect(() => onStore?.(store), [onStore, store]);
  return null;
}

function AuthenticatedAppBranch({
  bootId,
  onQueryClient,
  children,
}: {
  bootId: string | undefined;
  onQueryClient?: (queryClient: QueryClient) => void;
  children: ReactNode;
}) {
  const authenticated = useAppStore((state) => state.auth.authenticated);
  return authenticated ? (
    <SystemInfoQueryProvider bootId={bootId}>
      <QueryClientCapture onQueryClient={onQueryClient} />
      {children}
    </SystemInfoQueryProvider>
  ) : (
    <output data-testid="logged-out" />
  );
}

function QueryClientCapture({
  onQueryClient,
}: {
  onQueryClient?: (queryClient: QueryClient) => void;
}) {
  const queryClient = useQueryClient();
  useEffect(() => onQueryClient?.(queryClient), [onQueryClient, queryClient]);
  return null;
}

function TestHarness({
  children,
  apiBaseUrl = BACKEND_ORIGIN,
  bootId = PAGE_BOOT_ID,
  onStore,
  onQueryClient,
}: {
  children: ReactNode;
  apiBaseUrl?: string;
  bootId?: string;
  onStore?: (store: StoreApi<AppState>) => void;
  onQueryClient?: (queryClient: QueryClient) => void;
}) {
  config.apiBaseUrl = apiBaseUrl;
  return (
    <StateProvider initialState={{ auth: AUTH }}>
      <StoreCapture onStore={onStore} />
      <AuthenticatedAppBranch bootId={bootId} onQueryClient={onQueryClient}>
        {children}
      </AuthenticatedAppBranch>
    </StateProvider>
  );
}

function InfoProbe({ id }: { id: string }) {
  const { info, isLoading, error } = useSystemInfo();
  return (
    <output data-testid={id}>
      {JSON.stringify({ version: info?.version ?? null, isLoading, error })}
    </output>
  );
}

function CapturingProbe() {
  const query = useSystemInfo();
  currentReload = query.reload;
  return (
    <output data-testid={QUERY_STATE_TEST_ID}>
      {JSON.stringify({
        isLoading: query.isLoading,
        error: query.error,
        version: query.info?.version,
      })}
    </output>
  );
}

function StatefulShellProbe() {
  const [value, setValue] = useState("original");
  return (
    <button type="button" onClick={() => setValue("edited")}>
      {value}
    </button>
  );
}

beforeEach(() => {
  config.apiBaseUrl = BACKEND_ORIGIN;
  currentReload = undefined;
  onlineManager.setOnline(true);
});

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  vi.unstubAllGlobals();
});

async function fetchesLazilyAndDeduplicatesConsumers() {
  const fetchMock = vi.fn().mockResolvedValue(makeResponse(INFO));
  vi.stubGlobal("fetch", fetchMock);

  const idle = render(
    <TestHarness>
      <div>idle</div>
    </TestHarness>,
  );
  expect(fetchMock).not.toHaveBeenCalled();
  idle.unmount();

  const view = render(
    <TestHarness>
      <InfoProbe id="first" />
      <InfoProbe id="second" />
    </TestHarness>,
  );

  await waitFor(() => expect(screen.getByTestId("first").textContent).toContain(VERSION_1));
  expect(screen.getByTestId("second").textContent).toContain(VERSION_1);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock).toHaveBeenCalledWith(`${BACKEND_ORIGIN}/api/v1/system/info`, {
    cache: "no-store",
    credentials: "include",
    headers: expect.any(Headers),
    signal: expect.any(AbortSignal),
  });

  view.rerender(
    <TestHarness>
      <div>away</div>
    </TestHarness>,
  );
  view.rerender(
    <TestHarness>
      <InfoProbe id="returned" />
    </TestHarness>,
  );
  expect(screen.getByTestId("returned").textContent).toContain(VERSION_1);
  expect(fetchMock).toHaveBeenCalledTimes(1);
}

async function resolvesDataAcrossStrictModeObserverReplay() {
  const requests: Array<{ signal: AbortSignal }> = [];
  const fetchMock = vi.fn((_url: string, init: RequestInit) => {
    const signal = init.signal as AbortSignal;
    requests.push({ signal });
    return new Promise<Response>((resolve, reject) => {
      const timeout = setTimeout(() => {
        signal.removeEventListener("abort", onAbort);
        resolve(makeResponse(INFO));
      }, 10);
      const onAbort = () => {
        clearTimeout(timeout);
        reject(new DOMException("Request aborted", "AbortError"));
      };
      signal.addEventListener("abort", onAbort, { once: true });
    });
  });
  vi.stubGlobal("fetch", fetchMock);

  render(
    <StrictMode>
      <TestHarness>
        <InfoProbe id="strict-mode" />
      </TestHarness>
    </StrictMode>,
  );

  await waitFor(() => expect(screen.getByTestId("strict-mode").textContent).toContain(VERSION_1));
  expect(fetchMock).toHaveBeenCalled();
  if (requests[0]?.signal.aborted) {
    expect(requests.length).toBeGreaterThan(1);
  }
}

async function exposesLoadingErrorsAndExplicitRetry() {
  const initial = deferred<Response>();
  const retry = deferred<Response>();
  const refresh = deferred<Response>();
  const fetchMock = vi
    .fn()
    .mockReturnValueOnce(initial.promise)
    .mockReturnValueOnce(retry.promise)
    .mockReturnValueOnce(refresh.promise);
  vi.stubGlobal("fetch", fetchMock);

  render(
    <TestHarness>
      <CapturingProbe />
    </TestHarness>,
  );
  await waitFor(() =>
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(LOADING),
  );

  await act(async () => initial.resolve(makeResponse(INFO, 500)));
  await waitFor(() =>
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(SYSTEM_INFO_ERROR),
  );
  expect(fetchMock).toHaveBeenCalledTimes(1);

  let reloadPromise: Promise<void> | undefined;
  act(() => {
    reloadPromise = currentReload?.();
  });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(LOADING);

  await act(async () => {
    retry.resolve(makeResponse(INFO));
    await reloadPromise;
  });
  await waitFor(() =>
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain(VERSION_1),
  );
  expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"error":null');
  expect(fetchMock).toHaveBeenCalledTimes(2);

  act(() => {
    reloadPromise = currentReload?.();
  });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
  await act(async () => {
    refresh.resolve(makeResponse({ ...INFO, version: "1.2.4" }));
    await reloadPromise;
  });
  await waitFor(() =>
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"version":"1.2.4"'),
  );
}

async function attemptsOnceWhileOfflineAndDoesNotResumeOnReconnect() {
  act(() => onlineManager.setOnline(false));
  const fetchMock = vi.fn().mockRejectedValue(new TypeError("offline"));
  vi.stubGlobal("fetch", fetchMock);

  render(
    <TestHarness>
      <CapturingProbe />
    </TestHarness>,
  );

  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  await waitFor(() =>
    expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"error":"offline"'),
  );

  act(() => onlineManager.setOnline(true));
  expect(screen.getByTestId(QUERY_STATE_TEST_ID).textContent).toContain('"isLoading":false');
  expect(fetchMock).toHaveBeenCalledTimes(1);
}

async function isolatesAndCancelsRequestsAcrossIdentityChanges() {
  const requests: Array<{
    url: string;
    signal: AbortSignal;
    pending: ReturnType<typeof deferred<Response>>;
  }> = [];
  const fetchMock = vi.fn((url: string, init: RequestInit) => {
    const pending = deferred<Response>();
    requests.push({ url, signal: init.signal as AbortSignal, pending });
    return pending.promise;
  });
  vi.stubGlobal("fetch", fetchMock);
  let store: StoreApi<AppState> | undefined;
  const onStore = (nextStore: StoreApi<AppState>) => {
    store = nextStore;
  };

  const view = render(
    <TestHarness apiBaseUrl={`${BACKEND_ORIGIN}/one`} bootId={PAGE_BOOT_ID} onStore={onStore}>
      <InfoProbe id="current" />
      <StatefulShellProbe />
    </TestHarness>,
  );
  await waitFor(() => expect(requests).toHaveLength(1));
  expect(requests[0]?.url).toBe(`${BACKEND_ORIGIN}/one/api/v1/system/info`);
  fireEvent.click(screen.getByRole("button", { name: "original" }));

  view.rerender(
    <TestHarness apiBaseUrl={`${BACKEND_ORIGIN}/two`} bootId="page-boot-2" onStore={onStore}>
      <InfoProbe id="current" />
      <StatefulShellProbe />
    </TestHarness>,
  );
  await waitFor(() => expect(requests).toHaveLength(2));
  await waitFor(() => expect(requests[0]?.signal.aborted).toBe(true));
  expect(screen.getByRole("button", { name: "edited" })).toBeTruthy();
  expect(screen.getByTestId("current").textContent).toContain('"version":null');

  act(() => {
    store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
  });
  await waitFor(() => expect(requests).toHaveLength(3));
  await waitFor(() => expect(requests[1]?.signal.aborted).toBe(true));
  expect(screen.getByRole("button", { name: "edited" })).toBeTruthy();
  expect(screen.getByTestId("current").textContent).toContain('"version":null');

  act(() => {
    store?.getState().clearAuthenticated();
  });
  await waitFor(() => expect(screen.getByTestId("logged-out")).toBeTruthy());
  await waitFor(() => expect(requests[2]?.signal.aborted).toBe(true));

  act(() => {
    store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
  });
  await waitFor(() => expect(requests).toHaveLength(4));
  expect(screen.getByTestId("current").textContent).toContain('"version":null');

  await act(async () =>
    requests[3]?.pending.resolve(makeResponse({ ...INFO, version: "current" })),
  );
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"current"'),
  );

  await act(async () => {
    requests[0]?.pending.resolve(makeResponse({ ...INFO, version: "stale-backend" }));
    requests[1]?.pending.resolve(makeResponse({ ...INFO, version: "stale-user" }));
    requests[2]?.pending.resolve(makeResponse({ ...INFO, version: "stale-auth" }));
  });
  expect(screen.getByTestId("current").textContent).toContain('"version":"current"');
}

async function discardsSnapshotsFromPreviousAuthIdentities() {
  let store: StoreApi<AppState> | undefined;
  let snapshot = 0;
  const fetchMock = vi.fn(async () => makeResponse({ ...INFO, version: `snapshot-${++snapshot}` }));
  vi.stubGlobal("fetch", fetchMock);
  const onStore = (nextStore: StoreApi<AppState>) => {
    store = nextStore;
  };

  render(
    <TestHarness onStore={onStore}>
      <InfoProbe id="current" />
    </TestHarness>,
  );
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"snapshot-1"'),
  );

  act(() => {
    store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
  });
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"snapshot-2"'),
  );

  act(() => {
    store?.getState().setAuthState(AUTH);
  });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"snapshot-3"'),
  );
}

async function protectsReturnedIdentityFromDelayedCleanup() {
  const requests: Array<{
    signal: AbortSignal;
    pending: ReturnType<typeof deferred<Response>>;
  }> = [];
  const fetchMock = vi.fn((_url: string, init: RequestInit) => {
    const pending = deferred<Response>();
    requests.push({ signal: init.signal as AbortSignal, pending });
    return pending.promise;
  });
  vi.stubGlobal("fetch", fetchMock);

  let store: StoreApi<AppState> | undefined;
  let queryClient: QueryClient | undefined;
  const onStore = (nextStore: StoreApi<AppState>) => {
    store = nextStore;
  };
  const onQueryClient = (nextClient: QueryClient) => {
    queryClient = nextClient;
  };

  render(
    <TestHarness onStore={onStore} onQueryClient={onQueryClient}>
      <InfoProbe id="current" />
    </TestHarness>,
  );
  await waitFor(() => expect(requests).toHaveLength(1));
  await act(async () => requests[0]?.pending.resolve(makeResponse(INFO)));
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"1.2.3"'),
  );

  const releaseCleanup = deferred<void>();
  const cancelQueries = queryClient?.cancelQueries.bind(queryClient);
  if (!queryClient || !cancelQueries) throw new Error("query client was not provided");
  queryClient.cancelQueries = ((...args: Parameters<QueryClient["cancelQueries"]>) =>
    cancelQueries(...args).then(
      async () => releaseCleanup.promise,
    )) as QueryClient["cancelQueries"];

  try {
    act(() => {
      store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
    });
    await waitFor(() => expect(requests).toHaveLength(2));

    act(() => store?.getState().setAuthState(AUTH));
    await waitFor(() => expect(requests).toHaveLength(3));
    expect(requests[1]?.signal.aborted).toBe(true);
    expect(requests[2]?.signal.aborted).toBe(false);

    const currentInfo = { ...INFO, version: "current-a" };
    await act(async () => requests[2]?.pending.resolve(makeResponse(currentInfo)));
    await waitFor(() =>
      expect(screen.getByTestId("current").textContent).toContain('"version":"current-a"'),
    );

    await act(async () => releaseCleanup.resolve());
    const currentKey = createSystemInfoQueryKey({
      apiBaseUrl: BACKEND_ORIGIN,
      bootId: PAGE_BOOT_ID,
      authMode: AUTH.mode,
      authenticated: AUTH.authenticated,
      userId: AUTH.user.id,
    });
    expect(queryClient.getQueryData(currentKey)).toEqual(currentInfo);
    expect(requests[2]?.signal.aborted).toBe(false);
  } finally {
    releaseCleanup.resolve();
  }
}

async function removesObsoleteDatabaseStatsQueriesOnly() {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => Promise.resolve(makeResponse(INFO))),
  );
  let store: StoreApi<AppState> | undefined;
  let queryClient: QueryClient | undefined;

  render(
    <TestHarness
      onStore={(nextStore) => {
        store = nextStore;
      }}
      onQueryClient={(nextClient) => {
        queryClient = nextClient;
      }}
    >
      <InfoProbe id="current" />
    </TestHarness>,
  );
  await waitFor(() => expect(queryClient).toBeDefined());

  const oldDatabaseKey = [
    "system",
    "database-stats",
    BACKEND_ORIGIN,
    PAGE_BOOT_ID,
    AUTH.mode,
    AUTH.authenticated,
    AUTH.user.id,
  ] as const;
  const unrelatedKey = ["settings", "draft", "unsaved-form"] as const;
  const oldDatabaseStats = { backup_directory: "/old/backups" };
  const unrelatedDraft = { value: "keep me" };
  queryClient?.setQueryData(oldDatabaseKey, oldDatabaseStats);
  queryClient?.setQueryData(unrelatedKey, unrelatedDraft);

  act(() => {
    store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
  });
  await waitFor(() => expect(queryClient?.getQueryData(oldDatabaseKey)).toBeUndefined());
  expect(queryClient?.getQueryData(unrelatedKey)).toEqual(unrelatedDraft);
}

function scopesTheCacheKeyToTheFullIdentity() {
  const identity = {
    apiBaseUrl: `${BACKEND_ORIGIN}/system/`,
    bootId: PAGE_BOOT_ID,
    authMode: "enabled" as const,
    authenticated: true,
    userId: "user-1",
  };
  const key = createSystemInfoQueryKey(identity);
  const variations = [
    { apiBaseUrl: `${BACKEND_ORIGIN}/other` },
    { bootId: "page-boot-2" },
    { authMode: "disabled" as const },
    { authenticated: false },
    { userId: "user-2" },
  ];

  expect(normalizeSystemInfoApiBaseUrl(`${BACKEND_ORIGIN}/system///`)).toBe(
    `${BACKEND_ORIGIN}/system`,
  );
  expect(key).toEqual([
    "system",
    "info",
    `${BACKEND_ORIGIN}/system`,
    PAGE_BOOT_ID,
    "enabled",
    true,
    "user-1",
  ]);
  for (const variation of variations) {
    expect(createSystemInfoQueryKey({ ...identity, ...variation })).not.toEqual(key);
  }
}

describe("useSystemInfo Query cache", () => {
  it(
    "fetches lazily, deduplicates concurrent consumers, and retains fresh data",
    fetchesLazilyAndDeduplicatesConsumers,
  );
  it("resolves data across StrictMode observer replay", resolvesDataAcrossStrictModeObserverReplay);
  it(
    "exposes loading and errors, then supports an explicit retry",
    exposesLoadingErrorsAndExplicitRetry,
  );
  it(
    "attempts once while offline and does not resume on reconnect",
    attemptsOnceWhileOfflineAndDoesNotResumeOnReconnect,
  );
  it("cancels and isolates old identity requests", isolatesAndCancelsRequestsAcrossIdentityChanges);
  it(
    "discards snapshots from previous auth identities",
    discardsSnapshotsFromPreviousAuthIdentities,
  );
  it(
    "does not let delayed identity cleanup remove a newly active query",
    protectsReturnedIdentityFromDelayedCleanup,
  );
  it(
    "removes obsolete DatabaseStats queries without clearing unrelated cache data",
    removesObsoleteDatabaseStatsQueriesOnly,
  );
  it(
    "includes full backend, boot, and auth identity in its cache key",
    scopesTheCacheKeyToTheFullIdentity,
  );
});
