import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ListeningPort, TunnelInfo } from "@/lib/api/domains/port-api";
import { usePortForwardManagement } from "./use-port-forward-management";

const { listPortsMock, listTunnelsMock, startTunnelMock, stopTunnelMock } = vi.hoisted(() => ({
  listPortsMock: vi.fn(),
  listTunnelsMock: vi.fn(),
  startTunnelMock: vi.fn(),
  stopTunnelMock: vi.fn(),
}));

vi.mock("@/lib/api/domains/port-api", () => ({
  listPorts: listPortsMock,
  listTunnels: listTunnelsMock,
  startTunnel: startTunnelMock,
  stopTunnel: stopTunnelMock,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));
vi.mock("@/lib/toast/sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

beforeEach(() => {
  listPortsMock.mockReset().mockResolvedValue([]);
  listTunnelsMock.mockReset().mockResolvedValue([]);
  startTunnelMock.mockReset().mockResolvedValue(49152);
  stopTunnelMock.mockReset().mockResolvedValue(undefined);
});
afterEach(() => cleanup());

describe("port detection request ownership", () => {
  it.each(["older first", "newer first"])(
    "keeps the newest refresh authoritative when requests finish %s",
    async (order) => {
      const older = deferred<ListeningPort[]>();
      const newer = deferred<ListeningPort[]>();
      listPortsMock.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
      const { result } = renderHook(() => usePortForwardManagement("session", false));
      let olderRefresh!: Promise<void>;
      let newerRefresh!: Promise<void>;
      act(() => {
        olderRefresh = result.current.refresh();
        newerRefresh = result.current.refresh();
      });
      if (order === "older first") {
        await act(async () => {
          older.resolve([{ port: 3000, address: "*" }]);
          await olderRefresh;
        });
        expect(result.current.loading).toBe(true);
        expect(result.current.detectedPorts).toEqual([]);
      }
      await act(async () => {
        newer.resolve([{ port: 9000, address: "*" }]);
        await newerRefresh;
      });
      if (order === "newer first") {
        await act(async () => {
          older.resolve([{ port: 3000, address: "*" }]);
          await olderRefresh;
        });
      }
      expect(result.current.detectedPorts).toEqual([{ port: 9000, address: "*" }]);
      expect(result.current.loading).toBe(false);
    },
  );

  it("keeps a replacement session's refresh alive when an old callback runs", async () => {
    const current = deferred<ListeningPort[]>();
    listPortsMock.mockReturnValueOnce(current.promise);
    const { result, rerender } = renderHook(
      ({ sessionId }) => usePortForwardManagement(sessionId, false),
      { initialProps: { sessionId: "first" } },
    );
    const oldRefresh = result.current.refresh;
    rerender({ sessionId: "second" });
    let currentRefresh!: Promise<void>;
    act(() => {
      currentRefresh = result.current.refresh();
    });
    await act(async () => oldRefresh());
    expect(result.current.loading).toBe(true);
    await act(async () => {
      current.resolve([{ port: 9000, address: "*" }]);
      await currentRefresh;
    });
    expect(result.current.detectedPorts).toEqual([{ port: 9000, address: "*" }]);
    expect(result.current.loading).toBe(false);
  });
});

// @covers AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001.7-.8
describe("tunnel snapshot freshness", () => {
  it("retains known active tunnels when a later snapshot is unavailable", async () => {
    const { result, rerender } = renderHook(
      ({ active }) => usePortForwardManagement("session", active),
      { initialProps: { active: true } },
    );
    await act(async () => result.current.handleTunnelStart(9000));
    listTunnelsMock.mockResolvedValueOnce(null);
    rerender({ active: false });
    await act(async () => rerender({ active: true }));
    expect(result.current.activeTunnels.get(9000)).toBe(49152);
  });

  it("accepts an externally stopped tunnel on a later management visit", async () => {
    const { result, rerender } = renderHook(
      ({ active }) => usePortForwardManagement("session", active),
      { initialProps: { active: true } },
    );
    await act(async () => result.current.handleTunnelStart(9000));
    expect(result.current.activeTunnels.get(9000)).toBe(49152);
    rerender({ active: false });
    await act(async () => rerender({ active: true }));
    expect(result.current.activeTunnels.size).toBe(0);
    expect(result.current.manualPorts).toContain(9000);
  });

  it("preserves a start followed by a stop while an older tunnel snapshot is pending", async () => {
    const snapshot = deferred<TunnelInfo[]>();
    listTunnelsMock.mockReturnValueOnce(snapshot.promise);
    const { result } = renderHook(() => usePortForwardManagement("session", true));
    await act(async () => result.current.handleTunnelStart(9000));
    await act(async () => result.current.handleTunnelStop(9000));
    await act(async () => {
      snapshot.resolve([{ port: 9000, tunnel_port: 49152 }]);
      await snapshot.promise;
    });
    expect(result.current.activeTunnels.size).toBe(0);
    expect(result.current.manualPorts).toContain(9000);
  });
});
