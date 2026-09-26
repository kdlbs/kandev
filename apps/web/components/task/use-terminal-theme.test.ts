import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Terminal } from "@xterm/xterm";
import { useTerminalTheme } from "./use-terminal-theme";

const getTerminalTheme = vi.hoisted(() => vi.fn(() => ({ background: "#updated" })));

vi.mock("@/lib/theme/terminal-theme", () => ({
  getTerminalTheme,
}));

type QueuedFrame = {
  callback: FrameRequestCallback;
  id: number;
};

function createRefs() {
  const container = document.createElement("div");
  const terminal = {
    element: document.createElement("div"),
    options: { theme: { background: "#initial" } },
  } as unknown as Terminal;
  const terminalRef = { current: terminal };
  const containerRef = { current: container };
  return { container, containerRef, terminal, terminalRef };
}

describe("useTerminalTheme", () => {
  let nextFrameId = 1;
  let queuedFrames: QueuedFrame[] = [];
  let cancelAnimationFrameMock: ReturnType<typeof vi.fn>;

  afterEach(() => {
    vi.unstubAllGlobals();
    queuedFrames = [];
    nextFrameId = 1;
    getTerminalTheme.mockClear();
  });

  function stubAnimationFrames() {
    cancelAnimationFrameMock = vi.fn((frameId: number) => {
      queuedFrames = queuedFrames.filter(({ id }) => id !== frameId);
    });
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      const id = nextFrameId++;
      queuedFrames.push({ callback, id });
      return id;
    });
    vi.stubGlobal("cancelAnimationFrame", cancelAnimationFrameMock);
  }

  function runNextFrame() {
    const frame = queuedFrames.shift();
    expect(frame).toBeDefined();
    act(() => frame?.callback(0));
  }

  it("updates the existing terminal after the second animation frame", () => {
    stubAnimationFrames();
    const { containerRef, terminal, terminalRef } = createRefs();

    renderHook(() =>
      useTerminalTheme({
        terminalRef,
        containerRef,
        isTerminalReady: true,
        resolvedTheme: "dark",
      }),
    );

    expect(terminal.options.theme).toMatchObject({ background: "#initial" });
    runNextFrame();
    expect(terminal.options.theme).toMatchObject({ background: "#initial" });

    runNextFrame();

    expect(getTerminalTheme).toHaveBeenCalledWith(containerRef.current, "dark");
    expect(terminal.options.theme).toMatchObject({ background: "#updated" });
  });

  it("cancels the pending second frame during cleanup", () => {
    stubAnimationFrames();
    const { containerRef, terminal, terminalRef } = createRefs();
    const { unmount } = renderHook(() =>
      useTerminalTheme({
        terminalRef,
        containerRef,
        isTerminalReady: true,
        resolvedTheme: "light",
      }),
    );

    runNextFrame();
    const pendingSecondFrame = queuedFrames[0]?.id;
    expect(pendingSecondFrame).toBeDefined();

    unmount();

    expect(cancelAnimationFrameMock).toHaveBeenCalledWith(pendingSecondFrame);
    expect(queuedFrames).toHaveLength(0);
    expect(terminal.options.theme).toMatchObject({ background: "#initial" });
  });
});
