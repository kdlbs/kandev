import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useTunnelActions } from "./use-tunnel-actions";

const { startTunnelMock } = vi.hoisted(() => ({ startTunnelMock: vi.fn() }));
vi.mock("@/lib/api/domains/port-api", () => ({
  startTunnel: startTunnelMock,
  stopTunnel: vi.fn(),
}));
vi.mock("@/lib/toast/sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/lib/i18n", () => ({ t: (key: string) => key }));

// @covers AC-UI-PORT-FORWARDING-DISCOVERY-001.13
describe("session-scoped tunnel actions", () => {
  it("clears pending state and ignores an old session's start result", async () => {
    let resolveStart!: (port: number) => void;
    startTunnelMock.mockReturnValueOnce(
      new Promise<number>((resolve) => {
        resolveStart = resolve;
      }),
    );
    let tunnels = new Map<number, number>();
    const setTunnels = (update: (previous: Map<number, number>) => Map<number, number>) => {
      tunnels = update(tunnels);
    };
    const { result, rerender } = renderHook(
      ({ sessionId }) => useTunnelActions(sessionId, setTunnels),
      { initialProps: { sessionId: "first" } },
    );
    let pending!: Promise<void>;
    act(() => {
      pending = result.current.handleTunnelStart(9000);
    });
    expect(result.current.pendingTunnels.has(9000)).toBe(true);
    rerender({ sessionId: "second" });
    expect(result.current.pendingTunnels.size).toBe(0);
    await act(async () => {
      resolveStart(49152);
      await pending;
    });
    expect(tunnels.size).toBe(0);
    startTunnelMock.mockResolvedValueOnce(49153);
    await act(async () => result.current.handleTunnelStart(3000));
    expect([...tunnels]).toEqual([[3000, 49153]]);
  });
});
