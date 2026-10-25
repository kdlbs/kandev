import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Terminal } from "@xterm/xterm";
import { AttachAddon } from "@xterm/addon-attach";
import { useWebSocketConnection } from "./use-passthrough-terminal";

vi.mock("@xterm/addon-attach", () => ({
  AttachAddon: class {
    dispose = vi.fn();
  },
}));

class ControlledSocket {
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: ControlledSocket[] = [];
  readyState = 0;
  onopen: (() => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  close = vi.fn(() => {
    this.readyState = ControlledSocket.CLOSED;
  });
  constructor() {
    ControlledSocket.instances.push(this);
  }
  open() {
    this.readyState = ControlledSocket.OPEN;
    this.onopen?.();
  }
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  ControlledSocket.instances = [];
});

it("keeps a replacement terminal attached when the previous socket closes late", () => {
  vi.useFakeTimers();
  vi.stubGlobal("WebSocket", ControlledSocket);
  const attachAddonRef = { current: null as AttachAddon | null };
  const wsRef = { current: null as WebSocket | null };
  const terminal = { reset: vi.fn(), loadAddon: vi.fn(), refresh: vi.fn(), rows: 24 };
  const onConnected = vi.fn();
  const onDisconnected = vi.fn();
  const options = {
    taskId: "task-1",
    sessionId: "session-1",
    environmentId: "env-1",
    canConnect: true,
    isTerminalReady: true,
    fitAndResize: vi.fn(),
    wsBaseUrl: "ws://localhost",
    mode: "shell" as const,
    label: undefined,
    xtermRef: { current: terminal as unknown as Terminal },
    fitAddonRef: { current: {} as never },
    wsRef,
    attachAddonRef,
    onConnected,
    onDisconnected,
  };
  const { rerender, unmount } = renderHook(
    ({ terminalId }) => useWebSocketConnection({ ...options, terminalId }),
    { initialProps: { terminalId: "shell-default" } },
  );
  act(() => {
    vi.advanceTimersByTime(150);
    ControlledSocket.instances[0].open();
  });
  const oldSocket = ControlledSocket.instances[0];
  rerender({ terminalId: "shell-db-backed" });
  act(() => {
    vi.advanceTimersByTime(150);
    ControlledSocket.instances[1].open();
  });
  const replacementAddon = attachAddonRef.current!;
  act(() => {
    oldSocket.open();
  });
  expect(attachAddonRef.current).toBe(replacementAddon);
  expect(onConnected).toHaveBeenCalledTimes(2);
  act(() => {
    oldSocket.onclose?.({ code: 1006 } as CloseEvent);
  });
  expect(replacementAddon.dispose).not.toHaveBeenCalled();
  expect(attachAddonRef.current).toBe(replacementAddon);
  expect(onDisconnected).not.toHaveBeenCalled();
  act(() => {
    ControlledSocket.instances[1].onclose?.({ code: 1006 } as CloseEvent);
  });
  expect(replacementAddon.dispose).toHaveBeenCalledOnce();
  expect(attachAddonRef.current).toBeNull();
  expect(onDisconnected).toHaveBeenCalledOnce();
  unmount();
});
