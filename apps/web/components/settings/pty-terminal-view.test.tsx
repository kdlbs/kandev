import { act, cleanup, render, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const sockets: MockWebSocket[] = [];
  const terminals: MockTerminal[] = [];
  class MockTerminal {
    cols = 80;
    rows = 24;
    loadAddon = vi.fn();
    open = vi.fn();
    write = vi.fn();
    dispose = vi.fn();
    onData = vi.fn(() => ({ dispose: vi.fn() }));
    constructor() {
      terminals.push(this);
    }
  }
  class MockWebSocket {
    static readonly OPEN = 1;
    static readonly CLOSED = 3;
    readyState = MockWebSocket.OPEN;
    binaryType = "";
    onmessage: ((event: MessageEvent) => void) | null = null;
    onclose: (() => void) | null = null;
    onerror: (() => void) | null = null;
    send = vi.fn();
    close = vi.fn(() => {
      this.readyState = MockWebSocket.CLOSED;
      this.onclose?.();
    });
    addEventListener = vi.fn((type: string, listener: EventListener) => {
      if (type === "open") queueMicrotask(() => listener(new Event("open")));
    });

    constructor() {
      sockets.push(this);
    }
  }
  return {
    stopAgentLogin: vi.fn().mockResolvedValue(undefined),
    getAgentLoginStatus: vi.fn(),
    resizeAgentLogin: vi.fn().mockResolvedValue(undefined),
    sockets,
    terminals,
    resizeCallbacks: [] as ResizeObserverCallback[],
    MockTerminal,
    MockWebSocket,
  };
});

vi.mock("@xterm/xterm", () => ({ Terminal: mocks.MockTerminal }));
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit = vi.fn();
  },
}));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class {} }));
vi.mock("@/lib/desktop/external-links", () => ({ openExternalLink: vi.fn() }));
vi.mock("@/lib/api", () => ({
  agentLoginStreamUrl: vi.fn((sessionId: string) => `ws://api.test/${sessionId}`),
  getAgentLoginStatus: mocks.getAgentLoginStatus,
  resizeAgentLogin: mocks.resizeAgentLogin,
  stopAgentLogin: mocks.stopAgentLogin,
}));

import { PtyTerminalView, type StartPtySession } from "./pty-terminal-view";
import { cancelPtyTerminalStart } from "./pty-terminal-lifecycle";

const session = {
  session_id: "session-1",
  agent_id: "_host_shell",
  cmd: ["/bin/bash"],
  running: true,
  started_at: "2026-08-04T00:00:00Z",
};

function startSession(): ReturnType<StartPtySession> {
  return Promise.resolve(session);
}

beforeEach(() => {
  mocks.stopAgentLogin.mockClear();
  mocks.getAgentLoginStatus.mockReset();
  mocks.resizeAgentLogin.mockClear();
  mocks.sockets.length = 0;
  mocks.terminals.length = 0;
  mocks.resizeCallbacks.length = 0;
  vi.stubGlobal("WebSocket", mocks.MockWebSocket);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe = vi.fn();
      disconnect = vi.fn();
      constructor(callback: ResizeObserverCallback) {
        mocks.resizeCallbacks.push(callback);
      }
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("PtyTerminalView lifecycle", () => {
  it("stops a standard-dialog session when the view unmounts", async () => {
    const view = render(<PtyTerminalView startSession={startSession} />);

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    view.unmount();

    await waitFor(() => expect(mocks.stopAgentLogin).toHaveBeenCalledWith("session-1"));
  });

  it("detaches a quick-tab session without stopping it", async () => {
    const view = render(
      <PtyTerminalView
        startSession={startSession}
        lifecycle="detach-on-unmount"
        ownerId="tab-1"
        clientId="6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b"
      />,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    view.unmount();
    await act(async () => {});

    expect(mocks.stopAgentLogin).not.toHaveBeenCalled();
    expect(mocks.sockets[0]?.close).toHaveBeenCalledTimes(1);
  });

  it("reports a detached late start so the tab retains its session identity", async () => {
    let resolveStart: ((value: typeof session) => void) | undefined;
    const start = vi.fn(
      () =>
        new Promise<typeof session>((resolve) => {
          resolveStart = resolve;
        }),
    );
    const onStateChange = vi.fn();
    const view = render(
      <PtyTerminalView
        startSession={start}
        lifecycle="detach-on-unmount"
        ownerId="tab-late-start"
        onStateChange={onStateChange}
      />,
    );

    await waitFor(() => expect(start).toHaveBeenCalled());
    view.unmount();
    await act(async () => resolveStart?.(session));

    expect(onStateChange).toHaveBeenLastCalledWith({
      status: "running",
      sessionId: "session-1",
      exitCode: null,
      error: null,
    });
    expect(mocks.stopAgentLogin).not.toHaveBeenCalled();
  });

  it("attaches to an existing session without starting or stopping it on detach", async () => {
    mocks.getAgentLoginStatus.mockResolvedValueOnce(session);
    const start = vi.fn(startSession);
    const view = render(
      <PtyTerminalView
        startSession={start}
        sessionId="session-1"
        lifecycle="detach-on-unmount"
        ownerId="tab-1"
      />,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    expect(start).not.toHaveBeenCalled();
    expect(mocks.getAgentLoginStatus).toHaveBeenCalledWith("session-1");
    view.unmount();
    expect(mocks.stopAgentLogin).not.toHaveBeenCalled();
  });
});

describe("PtyTerminalView lifecycle races", () => {
  it("stops a detached start that is explicitly cancelled after unmount", async () => {
    let resolveStart: ((value: typeof session) => void) | undefined;
    const start = vi.fn(
      () =>
        new Promise<typeof session>((resolve) => {
          resolveStart = resolve;
        }),
    );
    const view = render(
      <PtyTerminalView startSession={start} lifecycle="detach-on-unmount" ownerId="tab-pending" />,
    );
    await waitFor(() => expect(start).toHaveBeenCalled());

    view.unmount();
    cancelPtyTerminalStart("tab-pending");
    await act(async () => resolveStart?.(session));

    await waitFor(() => expect(mocks.stopAgentLogin).toHaveBeenCalledWith("session-1"));
  });

  it("does not claim the first StrictMode mount after a detached replay", async () => {
    const start = vi.fn(startSession);
    const view = render(
      <StrictMode>
        <PtyTerminalView startSession={start} lifecycle="detach-on-unmount" ownerId="tab-1" />
      </StrictMode>,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    expect(start).toHaveBeenCalledTimes(2);
    expect(mocks.stopAgentLogin).not.toHaveBeenCalled();
    view.unmount();
  });

  it("stops a superseded StrictMode start in the standard dialog lifecycle", async () => {
    const resolvers: Array<(value: typeof session) => void> = [];
    const start = vi.fn(
      () =>
        new Promise<typeof session>((resolve) => {
          resolvers.push(resolve);
        }),
    );
    const view = render(
      <StrictMode>
        <PtyTerminalView startSession={start} />
      </StrictMode>,
    );

    await waitFor(() => expect(start).toHaveBeenCalledTimes(2));
    await act(async () => resolvers[0]?.(session));
    await waitFor(() => expect(mocks.stopAgentLogin).toHaveBeenCalledWith("session-1"));

    await act(async () => resolvers[1]?.(session));
    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    view.unmount();
  });

  it("reports a missing attached session without attempting replacement", async () => {
    mocks.getAgentLoginStatus.mockRejectedValueOnce({ status: 404 });
    const onStateChange = vi.fn();
    const start = vi.fn(startSession);
    const view = render(
      <PtyTerminalView
        startSession={start}
        sessionId="missing-session"
        lifecycle="detach-on-unmount"
        onStateChange={onStateChange}
      />,
    );

    await waitFor(() =>
      expect(onStateChange).toHaveBeenLastCalledWith({
        status: "exited",
        sessionId: "missing-session",
        exitCode: null,
        error: "Session is no longer available.",
      }),
    );
    expect(start).not.toHaveBeenCalled();
    expect(mocks.sockets).toHaveLength(0);
    view.unmount();
  });
});

describe("PtyTerminalView resize lifecycle", () => {
  it("disarms resize after a WebSocket exit", async () => {
    const onStateChange = vi.fn();
    const view = render(
      <PtyTerminalView startSession={startSession} onStateChange={onStateChange} />,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    act(() => {
      mocks.sockets[0]?.onmessage?.({
        data: JSON.stringify({ type: "exit", exit_code: 7 }),
      } as MessageEvent);
    });

    expect(onStateChange).toHaveBeenLastCalledWith({
      status: "exited",
      sessionId: "session-1",
      exitCode: 7,
      error: null,
    });

    mocks.resizeAgentLogin.mockClear();
    act(() => mocks.resizeCallbacks[0]?.([], {} as ResizeObserver));
    expect(mocks.resizeAgentLogin).not.toHaveBeenCalled();
    view.unmount();
  });

  it("disarms resize after the WebSocket closes", async () => {
    const view = render(<PtyTerminalView startSession={startSession} />);

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    mocks.sockets[0]?.onclose?.();
    act(() => mocks.resizeCallbacks[0]?.([], {} as ResizeObserver));

    expect(mocks.resizeAgentLogin).not.toHaveBeenCalled();
    view.unmount();
  });
});

describe("PtyTerminalView initial input", () => {
  it("sends initial input only when a new session is started", async () => {
    const view = render(
      <PtyTerminalView startSession={startSession} initialInput={"echo NEW_SESSION\n"} />,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    await waitFor(() =>
      expect(mocks.sockets[0]?.send).toHaveBeenCalledWith(
        new TextEncoder().encode("echo NEW_SESSION\n"),
      ),
    );
    view.unmount();

    mocks.sockets.length = 0;
    mocks.getAgentLoginStatus.mockResolvedValueOnce(session);
    const attached = render(
      <PtyTerminalView
        startSession={startSession}
        sessionId="session-1"
        lifecycle="detach-on-unmount"
        initialInput={"echo SHOULD_NOT_SEND\n"}
      />,
    );

    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    expect(mocks.sockets[0]?.send).not.toHaveBeenCalled();
    attached.unmount();
  });
});
