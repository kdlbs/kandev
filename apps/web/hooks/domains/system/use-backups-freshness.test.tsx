import { act, cleanup, render } from "@testing-library/react";
import { focusManager, onlineManager, QueryClient, useQueryClient } from "@tanstack/react-query";
import { createElement, Fragment, useEffect, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { SystemInfoQueryProvider } from "@/components/system-info-query-provider";
import type { SnapshotInfo } from "@/lib/types/system";
import * as api from "@/lib/api/domains/system-api";
import { BACKUP_LIST_QUERY_KEY_PREFIX } from "./backup-list-query";
import { useBackups } from "./use-backups";

const config = vi.hoisted(() => ({ apiBaseUrl: "https://backend.example/api" }));
const API_BASE_URL = "https://backend.example/api";
const BOOT_ID = "backup-freshness-boot";
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
const SNAPSHOT: SnapshotInfo = {
  name: "manual-20261006-000000.db",
  kind: "manual",
  size_bytes: 1024,
  mtime: "2026-10-06T00:00:00Z",
};

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: config.apiBaseUrl }) }));
vi.mock("@/lib/api/domains/system-api");

function QueryClientCapture({ onCapture }: { onCapture: (client: QueryClient) => void }) {
  const client = useQueryClient();
  useEffect(() => onCapture(client), [client, onCapture]);
  return null;
}

function BackupProbe() {
  const query = useBackups();
  return createElement("output", { "data-testid": "list" }, JSON.stringify(query.backups));
}

function TestHarness({
  children,
  onQueryClient,
}: {
  children?: ReactNode;
  onQueryClient?: (client: QueryClient) => void;
}) {
  config.apiBaseUrl = API_BASE_URL;
  return createElement(StateProvider, {
    initialState: { auth: AUTH },
    children: createElement(SystemInfoQueryProvider, {
      bootId: BOOT_ID,
      children: createElement(
        Fragment,
        null,
        onQueryClient ? createElement(QueryClientCapture, { onCapture: onQueryClient }) : null,
        children,
      ),
    }),
  });
}

beforeEach(() => {
  vi.resetAllMocks();
  config.apiBaseUrl = API_BASE_URL;
  onlineManager.setOnline(true);
  focusManager.setFocused(true);
  vi.mocked(api.fetchBackups).mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  onlineManager.setOnline(true);
  focusManager.setFocused(true);
  vi.useRealTimers();
});

describe("useBackups freshness and retention", () => {
  it("refetches on focus and reconnect only after the 30-second freshness window", async () => {
    vi.useFakeTimers();
    vi.mocked(api.fetchBackups).mockResolvedValue([SNAPSHOT]);
    render(
      <TestHarness>
        <BackupProbe />
      </TestHarness>,
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchBackups).toHaveBeenCalledOnce();

    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    act(() => {
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
    });
    expect(api.fetchBackups).toHaveBeenCalledOnce();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_001);
    });
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchBackups).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_001);
    });
    act(() => {
      onlineManager.setOnline(false);
      onlineManager.setOnline(true);
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(api.fetchBackups).toHaveBeenCalledTimes(3);
  });

  it("retains an inactive successful entry for five minutes, then collects it", async () => {
    vi.useFakeTimers();
    vi.mocked(api.fetchBackups).mockResolvedValue([SNAPSHOT]);
    let queryClient: QueryClient | undefined;
    const capture = (client: QueryClient) => {
      queryClient = client;
    };
    const view = render(
      <TestHarness onQueryClient={capture}>
        <BackupProbe />
      </TestHarness>,
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    const queryKey = [
      ...BACKUP_LIST_QUERY_KEY_PREFIX,
      API_BASE_URL,
      BOOT_ID,
      "enabled",
      true,
      "user-1",
    ];
    expect(queryClient?.getQueryData(queryKey)).toEqual([SNAPSHOT]);
    view.rerender(
      <TestHarness onQueryClient={capture}>
        <span>away</span>
      </TestHarness>,
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4 * 60_000);
    });
    expect(queryClient?.getQueryData(queryKey)).toEqual([SNAPSHOT]);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_001);
    });
    expect(queryClient?.getQueryData(queryKey)).toBeUndefined();
    view.unmount();
  });
});
