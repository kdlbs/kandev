import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { onlineManager, QueryClient, useQueryClient } from "@tanstack/react-query";
import { createElement, Fragment, StrictMode, useEffect, useState, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import type { SnapshotInfo } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { BACKUP_LIST_QUERY_KEY_PREFIX } from "./backup-list-query";
import { useBackups } from "./use-backups";
import type { BackupListScope } from "./backup-list-query";

const config = vi.hoisted(() => ({ apiBaseUrl: "https://backend.example/api///" }));
const API_BASE_URL = "https://backend.example/api";
const LOGGED_OUT_TEST_ID = "logged-out";
const BOOT_ID = "backup-page-boot";
const FIRST_BACKEND_URL = "https://backend.example/one";
const SECOND_BACKEND_URL = "https://backend.example/two";
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
const FIRST: SnapshotInfo = {
  name: "manual-20261006-000000.db",
  kind: "manual",
  size_bytes: 1024,
  mtime: "2026-10-06T00:00:00Z",
};
const EXTERNAL: SnapshotInfo = { ...FIRST, name: "kandev-maintenance-20261006-000000.db" };

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }) }));
vi.mock("@/lib/api/domains/system-api");

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (cause: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
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
  bootId = BOOT_ID,
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
    children: createElement(
      Fragment,
      null,
      onStore ? createElement(StoreCapture, { onCapture: onStore }) : null,
      createElement(AuthenticatedQueryBranch, { bootId, onQueryClient, children }),
    ),
  });
}

function AuthenticatedQueryBranch({
  bootId,
  onQueryClient,
  children,
}: {
  bootId: string;
  onQueryClient?: (client: QueryClient) => void;
  children?: ReactNode;
}) {
  const authenticated = useAppStore((state) => state.auth.authenticated);
  return authenticated
    ? createElement(SystemInfoQueryProvider, {
        bootId,
        children: createElement(
          Fragment,
          null,
          onQueryClient ? createElement(QueryClientCapture, { onCapture: onQueryClient }) : null,
          children,
        ),
      })
    : createElement("output", { "data-testid": LOGGED_OUT_TEST_ID });
}

function BackupProbe({ id }: { id: string }) {
  const query = useBackups();
  return createElement(
    "output",
    { "data-testid": id },
    JSON.stringify({ backups: query.backups, loaded: query.loaded, isLoading: query.isLoading }),
  );
}

let currentReload: (() => Promise<SnapshotInfo[]>) | undefined;
let currentReloadAfterWrite: ((scope?: BackupListScope) => Promise<SnapshotInfo[]>) | undefined;
function ReloadProbe() {
  const query = useBackups();
  currentReload = query.reload;
  currentReloadAfterWrite = query.reloadAfterWrite;
  return null;
}

function StatefulShellProbe() {
  const [value, setValue] = useState("original");
  return createElement("button", { type: "button", onClick: () => setValue("edited") }, value);
}

beforeEach(() => {
  vi.resetAllMocks();
  currentReload = undefined;
  currentReloadAfterWrite = undefined;
  config.apiBaseUrl = `${API_BASE_URL}///`;
  onlineManager.setOnline(true);
  vi.mocked(api.fetchBackups).mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  vi.useRealTimers();
});

describe("useBackups concurrent consumers", () => {
  it("shares one initial read across concurrent consumers and binds it to the full identity", async () => {
    const response = deferred<SnapshotInfo[]>();
    vi.mocked(api.fetchBackups).mockReturnValue(response.promise);
    let queryClient: QueryClient | undefined;
    const onQueryClient = (client: QueryClient) => {
      queryClient = client;
    };
    render(
      createElement(
        TestHarness,
        { onQueryClient },
        createElement(Fragment, null, <BackupProbe id="first" />, <BackupProbe id="second" />),
      ),
    );

    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledOnce());
    const request = vi.mocked(api.fetchBackups).mock.calls[0]?.[0];
    expect(request).toEqual({
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
      [...BACKUP_LIST_QUERY_KEY_PREFIX, API_BASE_URL, BOOT_ID, "enabled", true, "user-1"],
    ]);
    expect(screen.getByTestId("first").textContent).toContain('"isLoading":true');
    expect(screen.getByTestId("second").textContent).toContain('"isLoading":true');

    await act(async () => response.resolve([FIRST]));
    await waitFor(() => expect(screen.getByTestId("first").textContent).toContain(FIRST.name));
    expect(screen.getByTestId("second").textContent).toContain(FIRST.name);
  });

  it("keeps the shared request alive while one of two consumers remains mounted", async () => {
    const response = deferred<SnapshotInfo[]>();
    let signal: AbortSignal | undefined;
    vi.mocked(api.fetchBackups).mockImplementation((options) => {
      signal = options?.init?.signal as AbortSignal;
      return response.promise;
    });
    function Consumers() {
      const [showSecond, setShowSecond] = useState(true);
      return (
        <>
          <button type="button" onClick={() => setShowSecond(false)}>
            remove second
          </button>
          <BackupProbe id="first" />
          {showSecond ? <BackupProbe id="second" /> : null}
        </>
      );
    }
    render(
      <TestHarness>
        <Consumers />
      </TestHarness>,
    );

    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledOnce());
    fireEvent.click(screen.getByRole("button", { name: "remove second" }));
    expect(signal?.aborted).toBe(false);
    await act(async () => response.resolve([FIRST]));
    await waitFor(() => expect(screen.getByTestId("first").textContent).toContain(FIRST.name));
    expect(api.fetchBackups).toHaveBeenCalledOnce();
  });

  it("recovers from StrictMode effect replay without publishing an aborted response", async () => {
    const requests: Array<{
      signal?: AbortSignal;
      pending: ReturnType<typeof deferred<SnapshotInfo[]>>;
    }> = [];
    vi.mocked(api.fetchBackups).mockImplementation((options) => {
      const pending = deferred<SnapshotInfo[]>();
      requests.push({ signal: options?.init?.signal as AbortSignal | undefined, pending });
      return pending.promise;
    });
    render(
      <StrictMode>
        <TestHarness>
          <BackupProbe id="list" />
        </TestHarness>
      </StrictMode>,
    );

    await waitFor(() => expect(requests.length).toBeGreaterThanOrEqual(2));
    await waitFor(() => expect(requests[0]?.signal?.aborted).toBe(true));
    await act(async () => {
      requests[0]?.pending.resolve([EXTERNAL]);
      requests[1]?.pending.resolve([FIRST]);
    });

    await waitFor(() => expect(screen.getByTestId("list").textContent).toContain(FIRST.name));
    expect(screen.getByTestId("list").textContent).not.toContain(EXTERNAL.name);
    expect(api.fetchBackups).toHaveBeenCalledTimes(2);
  });
});

describe("useBackups read contract", () => {
  it("returns successful empty data and preserves the exact error and last good list on reload", async () => {
    const { result } = renderHook(useBackups, { wrapper: TestHarness });
    await waitFor(() => expect(result.current.loaded).toBe(true));
    expect(result.current.backups).toEqual([]);
    const failure = new Error("backup list unavailable");
    vi.mocked(api.fetchBackups).mockRejectedValueOnce(failure);

    let reload!: Promise<SnapshotInfo[]>;
    await act(async () => {
      reload = result.current.reload();
      await expect(reload).rejects.toBe(failure);
    });
    expect(result.current.backups).toEqual([]);
    expect(result.current.loaded).toBe(true);
    await waitFor(() => expect(result.current.error).toBe(failure.message));

    vi.mocked(api.fetchBackups).mockResolvedValueOnce([FIRST]);
    await act(async () => {
      await expect(result.current.reload()).resolves.toEqual([FIRST]);
    });
    await waitFor(() => expect(result.current.backups).toEqual([FIRST]));
    expect(result.current.error).toBeNull();
  });

  it("does not retry a failed initial read and attempts it even when browser online detection is false", async () => {
    onlineManager.setOnline(false);
    vi.mocked(api.fetchBackups).mockRejectedValue(new Error("offline"));
    const { result } = renderHook(useBackups, { wrapper: TestHarness });

    await waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(api.fetchBackups).toHaveBeenCalledOnce();
    expect(result.current.loaded).toBe(false);
    expect(result.current.error).toBe("offline");
    act(() => onlineManager.setOnline(true));
    expect(api.fetchBackups).toHaveBeenCalledOnce();
  });

  it("cancels a pending list request when its last observer unmounts", async () => {
    const response = deferred<SnapshotInfo[]>();
    let signal: AbortSignal | undefined;
    vi.mocked(api.fetchBackups).mockImplementation((options) => {
      signal = options?.init?.signal as AbortSignal;
      return response.promise;
    });
    const { unmount } = renderHook(useBackups, { wrapper: TestHarness });
    await waitFor(() => expect(signal).toBeDefined());

    unmount();

    expect(signal?.aborted).toBe(true);
    response.resolve([FIRST]);
  });
});

describe("useBackups auth and mount lifecycle", () => {
  it("cancels an obsolete auth read and does not publish it after logout", async () => {
    const requests: Array<{
      signal?: AbortSignal;
      pending: ReturnType<typeof deferred<SnapshotInfo[]>>;
    }> = [];
    vi.mocked(api.fetchBackups).mockImplementation((options) => {
      const pending = deferred<SnapshotInfo[]>();
      requests.push({ signal: options?.init?.signal as AbortSignal | undefined, pending });
      return pending.promise;
    });
    let queryClient: QueryClient | undefined;
    let store: StoreApi<AppState> | undefined;
    const view = render(
      <TestHarness
        onQueryClient={(client) => {
          queryClient = client;
        }}
        onStore={(nextStore) => {
          store = nextStore;
        }}
      >
        <BackupProbe id="list" />
      </TestHarness>,
    );
    await waitFor(() => expect(requests).toHaveLength(1));
    const capturedStore = store;
    const capturedClient = queryClient;
    if (!capturedStore || !capturedClient) throw new Error("The app stores should be captured");

    act(() =>
      capturedStore.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
    );
    await waitFor(() => expect(requests).toHaveLength(2));
    await waitFor(() => expect(requests[0]?.signal?.aborted).toBe(true));
    const obsoleteKey = [
      ...BACKUP_LIST_QUERY_KEY_PREFIX,
      API_BASE_URL,
      BOOT_ID,
      "enabled",
      true,
      "user-1",
    ];
    await waitFor(() => expect(capturedClient.getQueryData(obsoleteKey)).toBeUndefined());

    act(() => capturedStore.getState().clearAuthenticated());
    await waitFor(() => expect(screen.getByTestId(LOGGED_OUT_TEST_ID)).toBeTruthy());
    await waitFor(() => expect(requests[1]?.signal?.aborted).toBe(true));
    await act(async () => {
      requests[0]?.pending.resolve([EXTERNAL]);
      requests[1]?.pending.resolve([FIRST]);
    });

    expect(capturedClient.getQueryData(obsoleteKey)).toBeUndefined();
    expect(screen.getByTestId(LOGGED_OUT_TEST_ID)).toBeTruthy();
    view.unmount();
  });

  it("revalidates a fresh cached list when Backups is mounted again for external maintenance", async () => {
    vi.mocked(api.fetchBackups).mockResolvedValueOnce([FIRST]).mockResolvedValueOnce([EXTERNAL]);
    const view = render(
      <TestHarness>
        <BackupProbe id="list" />
      </TestHarness>,
    );
    try {
      await waitFor(() => expect(screen.getByTestId("list").textContent).toContain(FIRST.name));
      expect(screen.getByTestId("list").textContent).toContain(FIRST.name);
      view.rerender(
        <TestHarness>
          <span>away</span>
        </TestHarness>,
      );
      view.rerender(
        <TestHarness>
          <BackupProbe id="list" />
        </TestHarness>,
      );
      await waitFor(() => expect(screen.getByTestId("list").textContent).toContain(EXTERNAL.name));
      expect(api.fetchBackups).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId("list").textContent).toContain(EXTERNAL.name);
    } finally {
      view.unmount();
    }
  });
});

describe("useBackups explicit reads and writer boundaries", () => {
  it("explicit reload reads again while data is fresh and does not start a polling loop", async () => {
    vi.useFakeTimers();
    vi.mocked(api.fetchBackups).mockResolvedValue([FIRST]);
    render(
      <TestHarness>
        <ReloadProbe />
      </TestHarness>,
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchBackups).toHaveBeenCalledOnce();
    await act(async () => currentReload?.());
    expect(api.fetchBackups).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10 * 60_000);
    });
    expect(api.fetchBackups).toHaveBeenCalledTimes(2);
  });

  it("shares concurrent explicit reads and cancels the initial request at a writer boundary", async () => {
    const firstRead = deferred<SnapshotInfo[]>();
    let firstSignal: AbortSignal | undefined;
    vi.mocked(api.fetchBackups).mockImplementationOnce((options) => {
      firstSignal = options?.init?.signal as AbortSignal;
      return firstRead.promise;
    });
    vi.mocked(api.fetchBackups).mockResolvedValueOnce([FIRST]);
    render(
      <TestHarness>
        <ReloadProbe />
      </TestHarness>,
    );
    await waitFor(() => expect(currentReloadAfterWrite).toBeDefined());
    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledOnce());
    let writerRead!: Promise<SnapshotInfo[]>;
    await act(async () => {
      const concurrent = [currentReload?.(), currentReload?.()];
      expect(api.fetchBackups).toHaveBeenCalledOnce();
      const reloadAfterWrite = currentReloadAfterWrite;
      if (!reloadAfterWrite) throw new Error("The writer reload should be available");
      writerRead = reloadAfterWrite();
      await expect(writerRead).resolves.toEqual([FIRST]);
      await expect(Promise.all(concurrent)).rejects.toBeDefined();
    });

    expect(firstSignal?.aborted).toBe(true);
    expect(api.fetchBackups).toHaveBeenCalledTimes(2);
  });
});

describe("useBackups identity cleanup", () => {
  it("cleans obsolete identity entries without clearing unrelated Query data or remounting shell state", async () => {
    let apiBaseUrl = "https://backend.example/one";
    let bootId = "boot-a";
    let queryClient: QueryClient | undefined;
    let store: StoreApi<AppState> | undefined;
    const onQueryClient = (client: QueryClient) => {
      queryClient = client;
    };
    const onStore = (nextStore: StoreApi<AppState>) => {
      store = nextStore;
    };
    const view = render(
      <TestHarness
        apiBaseUrl={apiBaseUrl}
        bootId={bootId}
        onQueryClient={onQueryClient}
        onStore={onStore}
      >
        <BackupProbe id="list" />
        <StatefulShellProbe />
      </TestHarness>,
    );
    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledOnce());
    const client = queryClient;
    if (!client || !store) throw new Error("The Query client and app store must be captured");
    const capturedStore = store;
    const unrelatedKey = ["sessions", "active-task"] as const;
    client.setQueryData(unrelatedKey, { keep: true });
    client.setQueryData(["system", "storage", "retained"], { keep: true });
    fireEvent.click(screen.getByRole("button", { name: "original" }));

    apiBaseUrl = "https://backend.example/two";
    bootId = "boot-b";
    view.rerender(
      <TestHarness
        apiBaseUrl={apiBaseUrl}
        bootId={bootId}
        onQueryClient={onQueryClient}
        onStore={onStore}
      >
        <BackupProbe id="list" />
        <StatefulShellProbe />
      </TestHarness>,
    );
    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        client.getQueryData([
          ...BACKUP_LIST_QUERY_KEY_PREFIX,
          "https://backend.example/one",
          "boot-a",
          "enabled",
          true,
          "user-1",
        ]),
      ).toBeUndefined(),
    );
    expect(client.getQueryData(unrelatedKey)).toEqual({ keep: true });
    expect(client.getQueryData(["system", "storage", "retained"])).toEqual({ keep: true });
    expect(screen.getByRole("button", { name: "edited" })).toBeTruthy();

    act(() =>
      capturedStore.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
    );
    await waitFor(() => expect(api.fetchBackups).toHaveBeenCalledTimes(3));
    act(() => capturedStore.getState().clearAuthenticated());
    expect(screen.getByTestId(LOGGED_OUT_TEST_ID)).toBeTruthy();
    view.unmount();
  });
});

describe("useBackups rapid identity return", () => {
  it("isolates late A responses after an A-to-B-to-A process transition", async () => {
    const requests: Array<{
      baseUrl?: string;
      signal?: AbortSignal;
      pending: ReturnType<typeof deferred<SnapshotInfo[]>>;
    }> = [];
    vi.mocked(api.fetchBackups).mockImplementation((options) => {
      const pending = deferred<SnapshotInfo[]>();
      requests.push({
        baseUrl: options?.baseUrl,
        signal: options?.init?.signal as AbortSignal | undefined,
        pending,
      });
      return pending.promise;
    });
    let queryClient: QueryClient | undefined;
    const onQueryClient = (client: QueryClient) => {
      queryClient = client;
    };
    let apiBaseUrl = FIRST_BACKEND_URL;
    let bootId = "boot-a";
    const page = () => (
      <TestHarness apiBaseUrl={apiBaseUrl} bootId={bootId} onQueryClient={onQueryClient}>
        <BackupProbe id="list" />
      </TestHarness>
    );
    const view = render(page());
    await waitFor(() => expect(requests).toHaveLength(1));
    apiBaseUrl = SECOND_BACKEND_URL;
    bootId = "boot-b";
    view.rerender(page());
    await waitFor(() => expect(requests).toHaveLength(2));
    expect(requests[0]?.signal?.aborted).toBe(true);
    apiBaseUrl = FIRST_BACKEND_URL;
    bootId = "boot-a";
    view.rerender(page());
    await waitFor(() => expect(requests).toHaveLength(3));
    expect(requests[1]?.signal?.aborted).toBe(true);

    await act(async () => requests[2]?.pending.resolve([FIRST]));
    await waitFor(() => expect(screen.getByTestId("list").textContent).toContain(FIRST.name));
    await act(async () => {
      requests[0]?.pending.resolve([EXTERNAL]);
      requests[1]?.pending.resolve([EXTERNAL]);
    });
    expect(screen.getByTestId("list").textContent).toContain(FIRST.name);
    expect(screen.getByTestId("list").textContent).not.toContain(EXTERNAL.name);
    const key = [
      ...BACKUP_LIST_QUERY_KEY_PREFIX,
      FIRST_BACKEND_URL,
      "boot-a",
      "enabled",
      true,
      "user-1",
    ];
    expect(queryClient?.getQueryData(key)).toEqual([FIRST]);
    view.unmount();
  });
});
