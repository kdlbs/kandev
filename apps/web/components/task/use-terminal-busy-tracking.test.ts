import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { Terminal } from "@xterm/xterm";
import type { IDisposable } from "@xterm/xterm";

const registryMocks = vi.hoisted(() => ({
  markTerminalInput: vi.fn(),
  markTerminalOutput: vi.fn(),
  clearTerminalBusy: vi.fn(),
}));

vi.mock("@/lib/terminal/terminal-busy-registry", () => registryMocks);

import { useTerminalBusyTracking } from "./use-terminal-busy-tracking";

function makeDisposable(): IDisposable {
  return { dispose: vi.fn() };
}

function makeTerminal(): Terminal {
  return {
    onData: vi.fn(() => makeDisposable()),
    onWriteParsed: vi.fn(() => makeDisposable()),
  } as unknown as Terminal;
}

describe("useTerminalBusyTracking", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn((cb: FrameRequestCallback) => {
        cb(0);
        return 1;
      }),
    );
    vi.stubGlobal("cancelAnimationFrame", vi.fn());
    registryMocks.markTerminalInput.mockReset();
    registryMocks.markTerminalOutput.mockReset();
    registryMocks.clearTerminalBusy.mockReset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("registers subscriptions when enabled and terminal is ready", () => {
    const terminal = makeTerminal();
    const xtermRef = { current: terminal };

    renderHook(() => useTerminalBusyTracking("term-1", xtermRef, true, true));

    expect(terminal.onData).toHaveBeenCalledOnce();
    expect(terminal.onWriteParsed).toHaveBeenCalledOnce();
  });

  it("is a no-op when disabled", () => {
    const terminal = makeTerminal();
    const xtermRef = { current: terminal };

    renderHook(() => useTerminalBusyTracking("term-1", xtermRef, false, true));

    expect(terminal.onData).not.toHaveBeenCalled();
    expect(registryMocks.clearTerminalBusy).not.toHaveBeenCalled();
  });

  it("retries attach via requestAnimationFrame when xtermRef is initially null", () => {
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn(() => 42),
    );
    const terminal = makeTerminal();
    const xtermRef: { current: Terminal | null } = { current: null };

    renderHook(() => useTerminalBusyTracking("term-1", xtermRef, true, true));
    expect(terminal.onData).not.toHaveBeenCalled();

    xtermRef.current = terminal;
    const rafCb = vi.mocked(requestAnimationFrame).mock.calls[0]?.[0];
    rafCb?.(0);

    expect(terminal.onData).toHaveBeenCalledOnce();
    expect(terminal.onWriteParsed).toHaveBeenCalledOnce();
  });

  it("cleans up subscriptions and clears busy state on unmount", () => {
    const terminal = makeTerminal();
    const inputSub = makeDisposable();
    const outputSub = makeDisposable();
    vi.mocked(terminal.onData).mockReturnValue(inputSub);
    vi.mocked(terminal.onWriteParsed).mockReturnValue(outputSub);
    const xtermRef = { current: terminal };

    const { unmount } = renderHook(() => useTerminalBusyTracking("term-1", xtermRef, true, true));
    unmount();

    expect(inputSub.dispose).toHaveBeenCalledOnce();
    expect(outputSub.dispose).toHaveBeenCalledOnce();
    expect(registryMocks.clearTerminalBusy).toHaveBeenCalledWith("term-1");
  });
});
