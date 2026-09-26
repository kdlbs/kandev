import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { StrictMode, useEffect, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { SystemInfo } from "@/lib/types/system";
import { createSystemInfoQueryKey, normalizeSystemInfoApiBaseUrl } from "./system-info-query";

const BACKEND_ORIGIN = "https://backend.example";
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

function TestHarness({
  children,
  apiBaseUrl = BACKEND_ORIGIN,
  bootId = "page-boot-1",
  onStore,
}: {
  children: ReactNode;
  apiBaseUrl?: string;
  bootId?: string;
  onStore?: (store: StoreApi<AppState>) => void;
}) {
  config.apiBaseUrl = apiBaseUrl;
  return (
    <StateProvider initialState={{ auth: AUTH }}>
      <SystemInfoQueryProvider bootId={bootId}>
        <StoreCapture onStore={onStore} />
        {children}
      </SystemInfoQueryProvider>
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

beforeEach(() => {
  config.apiBaseUrl = BACKEND_ORIGIN;
  currentReload = undefined;
});

afterEach(() => {
  cleanup();
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
    <StrictMode>
      <TestHarness>
        <InfoProbe id="first" />
        <InfoProbe id="second" />
      </TestHarness>
    </StrictMode>,
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
    <StrictMode>
      <TestHarness>
        <div>away</div>
      </TestHarness>
    </StrictMode>,
  );
  view.rerender(
    <StrictMode>
      <TestHarness>
        <InfoProbe id="returned" />
      </TestHarness>
    </StrictMode>,
  );
  expect(screen.getByTestId("returned").textContent).toContain(VERSION_1);
  expect(fetchMock).toHaveBeenCalledTimes(1);
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
    <TestHarness apiBaseUrl={`${BACKEND_ORIGIN}/one`} bootId="page-boot-1" onStore={onStore}>
      <InfoProbe id="current" />
    </TestHarness>,
  );
  await waitFor(() => expect(requests).toHaveLength(1));
  expect(requests[0]?.url).toBe(`${BACKEND_ORIGIN}/one/api/v1/system/info`);

  view.rerender(
    <TestHarness apiBaseUrl={`${BACKEND_ORIGIN}/two`} bootId="page-boot-2" onStore={onStore}>
      <InfoProbe id="current" />
    </TestHarness>,
  );
  await waitFor(() => expect(requests).toHaveLength(2));
  await waitFor(() => expect(requests[0]?.signal.aborted).toBe(true));

  act(() => {
    store?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } });
  });
  await waitFor(() => expect(requests).toHaveLength(3));
  await waitFor(() => expect(requests[1]?.signal.aborted).toBe(true));

  await act(async () =>
    requests[2]?.pending.resolve(makeResponse({ ...INFO, version: "current" })),
  );
  await waitFor(() =>
    expect(screen.getByTestId("current").textContent).toContain('"version":"current"'),
  );

  await act(async () => {
    requests[0]?.pending.resolve(makeResponse({ ...INFO, version: "stale-backend" }));
    requests[1]?.pending.resolve(makeResponse({ ...INFO, version: "stale-user" }));
  });
  expect(screen.getByTestId("current").textContent).toContain('"version":"current"');

  view.unmount();
  await waitFor(() => expect(requests[2]?.signal.aborted).toBe(true));
}

function scopesTheCacheKeyToTheFullIdentity() {
  const identity = {
    apiBaseUrl: `${BACKEND_ORIGIN}/system/`,
    bootId: "page-boot-1",
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
    "page-boot-1",
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
  it(
    "exposes loading and errors, then supports an explicit retry",
    exposesLoadingErrorsAndExplicitRetry,
  );
  it("cancels and isolates old identity requests", isolatesAndCancelsRequestsAcrossIdentityChanges);
  it(
    "includes full backend, boot, and auth identity in its cache key",
    scopesTheCacheKeyToTheFullIdentity,
  );
});
