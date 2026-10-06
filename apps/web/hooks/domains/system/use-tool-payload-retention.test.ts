import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { createElement, Fragment, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { AppState } from "@/lib/state/store";
import * as api from "@/lib/api/domains/tool-payload-retention-api";
import * as systemApi from "@/lib/api/domains/system-api";
import { useToolPayloadRetention } from "./use-tool-payload-retention";
import { useBackups } from "./use-backups";
import { createBackupListQueryKey } from "./backup-list-query";
import type {
  ToolPayloadOperation,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";
vi.mock("@/lib/api/domains/tool-payload-retention-api");
vi.mock("@/lib/api/domains/system-api", () => ({ fetchBackups: vi.fn() }));
const config = vi.hoisted(() => ({ apiBaseUrl: "https://backend.example" }));
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }) }));

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
let currentStore: StoreApi<AppState> | undefined;
let currentQueryClient: QueryClient | undefined;
let observeBackups = false;

function StoreCapture() {
  currentStore = useAppStoreApi();
  return null;
}

function QueryCapture() {
  currentQueryClient = useQueryClient();
  return null;
}

function BackupObserver() {
  useBackups();
  return null;
}

function TestHarness({ children }: { children: ReactNode }) {
  return createElement(StateProvider, {
    initialState: { auth: AUTH },
    children: createElement(
      Fragment,
      null,
      createElement(StoreCapture),
      createElement(SystemInfoQueryProvider, {
        bootId: "retention-test-boot",
        children: createElement(
          Fragment,
          null,
          createElement(QueryCapture),
          observeBackups ? createElement(BackupObserver) : null,
          children,
        ),
      }),
    ),
  });
}

function renderRetentionHook() {
  return renderHook(useToolPayloadRetention, { wrapper: TestHarness });
}
const status: ToolPayloadRetentionStatus = {
  supported: true,
  policy: { enabled: false, age: { value: 3, unit: "months" }, revision: 0 },
  preparation: { state: "none", choice: "" },
};
const backupPolicy = {
  ...status.policy,
  enabled: true,
  age: { value: 4, unit: "months" as const },
  backup_choice: "backup" as const,
};
const PUBLISHED_SNAPSHOT = {
  name: "manual-published.db",
  kind: "manual" as const,
  size_bytes: 64,
  mtime: "2026-10-06T00:00:00Z",
};
const TEST_OPERATION_TIME = "2026-01-01T00:00:00.000Z";

function preparationStatus(
  revision: number,
  state: ToolPayloadRetentionStatus["preparation"]["state"],
  choice: ToolPayloadRetentionStatus["preparation"]["choice"] = "backup",
): ToolPayloadRetentionStatus {
  const { backup_choice: _choice, ...policy } = backupPolicy;
  return {
    ...status,
    policy: { ...policy, revision },
    preparation: { state, choice },
  };
}

function backupListKey() {
  return createBackupListQueryKey({
    apiBaseUrl: config.apiBaseUrl,
    bootId: "retention-test-boot",
    authMode: "enabled",
    authenticated: true,
    userId: "user-1",
  });
}
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (cause: unknown) => void;
  const promise = new Promise<T>((r, j) => {
    resolve = r;
    reject = j;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  vi.resetAllMocks();
  currentStore = undefined;
  currentQueryClient = undefined;
  observeBackups = false;
  config.apiBaseUrl = "https://backend.example";
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(status);
  vi.mocked(systemApi.fetchBackups).mockResolvedValue([]);
});
afterEach(cleanup);
it("loads the disabled policy without launching an analysis", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  expect(api.analyzeToolPayloadRetention).not.toHaveBeenCalled();
});
it("does not let a GET started before saving overwrite the PUT response", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const old = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.fetchToolPayloadRetention).mockReturnValueOnce(old.promise);
  let reload!: Promise<void>;
  act(() => {
    reload = result.current.reload();
  });
  const saved = {
    ...status,
    policy: { ...status.policy, revision: 1, age: { value: 4, unit: "months" as const } },
  };
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue(saved);
  await act(async () => {
    await result.current.save(saved.policy);
  });
  await act(async () => {
    old.resolve(status);
    await reload;
  });
  expect(result.current.status).toEqual(saved);
});

it("serializes same-tick commands and keeps pending until the initiating request settles", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const request = deferred<{ operation_id: string }>();
  vi.mocked(api.analyzeToolPayloadRetention).mockReturnValueOnce(request.promise);
  let first!: Promise<unknown>;
  await act(async () => {
    first = result.current.analyze(status.policy.age);
    await expect(result.current.analyze(status.policy.age)).rejects.toMatchObject({ status: 409 });
    await result.current.reload();
  });
  expect(result.current.pending).toBe(true);
  expect(api.analyzeToolPayloadRetention).toHaveBeenCalledTimes(1);
  await act(async () => {
    request.resolve({ operation_id: "scan" });
    await first;
  });
  expect(result.current.pending).toBe(false);
  expect(result.current.active).toBe(true);
});
it("keeps saved policy on failure and releases the mutation lock for retry", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const error = new Error("offline");
  vi.mocked(api.saveToolPayloadRetention)
    .mockRejectedValueOnce(error)
    .mockResolvedValueOnce(status);
  await act(async () => {
    await expect(result.current.save(status.policy)).rejects.toBe(error);
  });
  expect(result.current.status).toEqual(status);
  expect(result.current.pending).toBe(false);
  expect(result.current.error).toBe(error);
  await act(async () => {
    await result.current.save(status.policy);
  });
  expect(result.current.error).toBeNull();
});

it("clears a recovered status error on background polling", async () => {
  vi.useFakeTimers();
  let unmount: (() => void) | undefined;
  try {
    const readError = new Error("status unavailable");
    vi.mocked(api.fetchToolPayloadRetention)
      .mockResolvedValueOnce(status)
      .mockRejectedValueOnce(readError)
      .mockResolvedValueOnce(status);
    const rendered = renderRetentionHook();
    unmount = rendered.unmount;
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(rendered.result.current.error).toBe(readError);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(rendered.result.current.error).toBeNull();
  } finally {
    unmount?.();
    vi.useRealTimers();
  }
});

it("keeps action failures when a status read succeeds", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const actionError = new Error("analysis failed");
  vi.mocked(api.analyzeToolPayloadRetention).mockRejectedValueOnce(actionError);
  await act(async () => {
    await expect(result.current.analyze(status.policy.age)).rejects.toBe(actionError);
  });
  expect(result.current.error).toBe(actionError);
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(status);
  await act(async () => {
    await result.current.reload();
  });
  expect(result.current.error).toBe(actionError);
});

it("clears status-read failures after refresh without clearing action failures", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const actionError = new Error("analysis failed");
  vi.mocked(api.analyzeToolPayloadRetention).mockRejectedValueOnce(actionError);
  await act(async () => {
    await expect(result.current.analyze(status.policy.age)).rejects.toBe(actionError);
  });

  const statusError = new Error("status unavailable");
  vi.mocked(api.fetchToolPayloadRetention).mockRejectedValueOnce(statusError);
  await act(async () => {
    await result.current.reload();
  });
  expect(result.current.statusError).toBe(statusError);
  expect(result.current.actionError).toBe(actionError);
  expect(result.current.error).toBe(actionError);

  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(status);
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.statusError).toBeNull();
  expect(result.current.actionError).toBe(actionError);
  expect(result.current.error).toBe(actionError);
});

it("does not let a stale status failure replace a mutation result", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const old = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.fetchToolPayloadRetention).mockReturnValueOnce(old.promise);
  let reload!: Promise<void>;
  act(() => {
    reload = result.current.reload();
  });
  const saved = {
    ...status,
    policy: { ...status.policy, revision: 1, age: { value: 4, unit: "months" as const } },
  };
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue(saved);
  await act(async () => {
    await result.current.save(saved.policy);
  });
  const staleError = new Error("stale read failed");
  await act(async () => {
    old.reject(staleError);
    await reload;
  });
  expect(result.current.status).toEqual(saved);
  expect(result.current.error).toBeNull();
});
it("cancels the accepted operation and applies the returned status", async () => {
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  vi.mocked(api.runToolPayloadRetention).mockResolvedValue({ operation_id: "cleanup" });
  await act(async () => {
    await result.current.run(0);
  });
  vi.mocked(api.cancelToolPayloadRetention).mockResolvedValue(status);
  await act(async () => {
    await result.current.cancel("cleanup");
  });
  expect(result.current.active).toBe(false);
  expect(result.current.acceptedId).toBeNull();
});

it("invalidates once when a matching save response is already terminal", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  const listKey = backupListKey();
  await waitFor(() => expect(currentQueryClient?.getQueryData(listKey)).toEqual([]));
  vi.mocked(systemApi.fetchBackups).mockClear();
  const ready = preparationStatus(1, "ready");
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(ready);

  await act(async () => {
    await result.current.save(backupPolicy, { backupChoiceAttempt: true });
  });
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());

  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(ready);
  await act(async () => {
    await result.current.reload();
    await result.current.reload();
  });
  expect(systemApi.fetchBackups).toHaveBeenCalledOnce();
});

it("refreshes after observed preparation fails, including a failed attempt from an earlier save", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "running"));
  await act(async () => {
    await result.current.save(backupPolicy, { backupChoiceAttempt: true });
  });
  expect(systemApi.fetchBackups).not.toHaveBeenCalled();

  vi.mocked(systemApi.fetchBackups).mockResolvedValueOnce([PUBLISHED_SNAPSHOT]);
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "failed"));
  await act(async () => {
    await result.current.reload();
  });
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(currentQueryClient?.getQueryData(backupListKey())).toEqual([PUBLISHED_SNAPSHOT]),
  );
});

it("tracks a backup preparation first observed as pending and refreshes when it fails", async () => {
  observeBackups = true;
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "pending"));
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(preparationStatus(1, "pending")));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(systemApi.fetchBackups).mockResolvedValueOnce([PUBLISHED_SNAPSHOT]);
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "failed"));

  await act(async () => result.current.reload());

  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(currentQueryClient?.getQueryData(backupListKey())).toEqual([PUBLISHED_SNAPSHOT]),
  );
});

it("settles a preparation that is cancelled to none or replaced by a skip-choice revision", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "pending"));
  await act(async () => {
    await result.current.save(backupPolicy, { backupChoiceAttempt: true });
  });
  vi.mocked(api.cancelToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "none", ""));
  vi.mocked(systemApi.fetchBackups).mockResolvedValueOnce([PUBLISHED_SNAPSHOT]);
  await act(async () => {
    await result.current.cancel("preparation-operation");
  });
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(currentQueryClient?.getQueryData(backupListKey())).toEqual([PUBLISHED_SNAPSHOT]),
  );

  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(preparationStatus(2, "pending"));
  await act(async () => {
    await result.current.save({ ...backupPolicy, revision: 1 }, { backupChoiceAttempt: true });
  });
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(
    preparationStatus(3, "none", "skip"),
  );
  vi.mocked(systemApi.fetchBackups).mockResolvedValueOnce([PUBLISHED_SNAPSHOT]);
  await act(async () => {
    await result.current.save({ ...backupPolicy, revision: 2, backup_choice: "skip" });
  });
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(currentQueryClient?.getQueryData(backupListKey())).toEqual([PUBLISHED_SNAPSHOT]),
  );
});

it("ignores historical ready state, skip choice, and unrelated cleanup, then tracks a later revision", async () => {
  const historical = preparationStatus(4, "ready");
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(historical);
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(historical));
  const listKey = backupListKey();
  currentQueryClient?.setQueryData(listKey, []);
  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(
    preparationStatus(5, "none", "skip"),
  );
  await act(async () => {
    await result.current.save({ ...backupPolicy, revision: 4, backup_choice: "skip" });
  });
  expect(systemApi.fetchBackups).not.toHaveBeenCalled();
  expect(currentQueryClient?.getQueryState(listKey)?.isInvalidated).toBe(false);

  const cleanup: ToolPayloadOperation = {
    id: "cleanup-operation",
    kind: "cleanup",
    state: "running",
    scanned: 0,
    eligible_tasks: 0,
    eligible_messages: 0,
    removed_messages: 0,
    payload_bytes: 0,
    skipped: {},
    cutoff: TEST_OPERATION_TIME,
    started_at: TEST_OPERATION_TIME,
    age: backupPolicy.age,
  };
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce({
    ...preparationStatus(6, "none", ""),
    operation: cleanup,
  });
  await act(async () => {
    await result.current.save({ ...backupPolicy, revision: 5 });
  });
  expect(systemApi.fetchBackups).not.toHaveBeenCalled();
  expect(currentQueryClient?.getQueryState(listKey)?.isInvalidated).toBe(false);

  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(preparationStatus(7, "pending"));
  await act(async () => {
    await result.current.save({ ...backupPolicy, revision: 6 }, { backupChoiceAttempt: true });
  });
  expect(systemApi.fetchBackups).not.toHaveBeenCalled();
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValueOnce(preparationStatus(7, "failed"));
  await act(async () => {
    await result.current.reload();
  });
  await waitFor(() => expect(currentQueryClient?.getQueryState(listKey)?.isInvalidated).toBe(true));
  expect(systemApi.fetchBackups).not.toHaveBeenCalled();
});

it("ignores a deferred save response from an obsolete auth identity", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  const saveResponse = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.saveToolPayloadRetention).mockReturnValueOnce(saveResponse.promise);
  let save!: Promise<ToolPayloadRetentionStatus>;
  act(() => {
    save = result.current.save(backupPolicy, { backupChoiceAttempt: true });
  });

  act(() =>
    currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
  );
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() => expect(api.fetchToolPayloadRetention).toHaveBeenCalledTimes(2));

  await act(async () => {
    saveResponse.resolve(preparationStatus(1, "ready"));
    await expect(save).resolves.toEqual(preparationStatus(1, "ready"));
  });
  expect(result.current.status).toEqual(status);
  expect(result.current.pending).toBe(false);
  expect(systemApi.fetchBackups).toHaveBeenCalledOnce();
});

it("ignores a deferred terminal status read after an auth identity change", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  const oldStatus = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.fetchToolPayloadRetention).mockReturnValueOnce(oldStatus.promise);
  let reload!: Promise<void>;
  act(() => {
    reload = result.current.reload();
  });

  act(() =>
    currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
  );
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() => expect(api.fetchToolPayloadRetention).toHaveBeenCalledTimes(3));
  await act(async () => {
    oldStatus.resolve(preparationStatus(1, "failed"));
    await reload;
  });

  expect(result.current.status).toEqual(status);
  expect(systemApi.fetchBackups).toHaveBeenCalledOnce();
});

it("ignores a deferred cancellation response after an auth identity change", async () => {
  observeBackups = true;
  const { result } = renderRetentionHook();
  await waitFor(() => expect(result.current.status).toEqual(status));
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  vi.mocked(systemApi.fetchBackups).mockClear();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValueOnce(preparationStatus(1, "pending"));
  await act(async () => {
    await result.current.save(backupPolicy, { backupChoiceAttempt: true });
  });
  const cancelResponse = deferred<ToolPayloadRetentionStatus>();
  vi.mocked(api.cancelToolPayloadRetention).mockReturnValueOnce(cancelResponse.promise);
  let cancel!: Promise<ToolPayloadRetentionStatus>;
  act(() => {
    cancel = result.current.cancel("preparation-operation");
  });

  act(() =>
    currentStore?.getState().setAuthState({ ...AUTH, user: { ...AUTH.user, id: "user-2" } }),
  );
  await waitFor(() => expect(systemApi.fetchBackups).toHaveBeenCalledOnce());
  await waitFor(() => expect(api.fetchToolPayloadRetention).toHaveBeenCalledTimes(2));
  await act(async () => {
    cancelResponse.resolve(preparationStatus(1, "none", ""));
    await expect(cancel).resolves.toEqual(preparationStatus(1, "none", ""));
  });

  expect(result.current.status).toEqual(status);
  expect(result.current.pending).toBe(false);
  expect(systemApi.fetchBackups).toHaveBeenCalledOnce();
});

it("polls an accepted command and clears pending even when bounded history replaced its id", async () => {
  vi.useFakeTimers();
  try {
    const { result, unmount } = renderRetentionHook();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const operation: ToolPayloadOperation = {
      id: "already-finished",
      kind: "analysis",
      state: "running",
      age: status.policy.age,
      scanned: 0,
      eligible_tasks: 0,
      eligible_messages: 0,
      removed_messages: 0,
      payload_bytes: 0,
      skipped: {},
      cutoff: TEST_OPERATION_TIME,
      started_at: TEST_OPERATION_TIME,
    };
    const replacement: ToolPayloadOperation = {
      ...operation,
      id: "replacement",
      state: "succeeded",
      finished_at: "2026-01-01T00:01:00.000Z",
    };
    vi.mocked(api.fetchToolPayloadRetention)
      .mockResolvedValueOnce({ ...status, operation })
      .mockResolvedValue({ ...status, last_analysis: replacement });
    vi.mocked(api.analyzeToolPayloadRetention).mockResolvedValue({
      operation_id: "already-finished",
    });
    await act(async () => {
      await result.current.analyze(status.policy.age);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.active).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(result.current.active).toBe(false);
    expect(result.current.acceptedId).toBeNull();
    const calls = vi.mocked(api.fetchToolPayloadRetention).mock.calls.length;
    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(api.fetchToolPayloadRetention).toHaveBeenCalledTimes(calls);
  } finally {
    vi.useRealTimers();
  }
});
