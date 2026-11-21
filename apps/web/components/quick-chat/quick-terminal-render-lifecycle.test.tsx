import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => {
  const terminals: MockTerminal[] = [];
  const sockets: MockWebSocket[] = [];

  class MockTerminal {
    cols = 80;
    rows = 24;
    loadAddon = vi.fn();
    open = vi.fn();
    write = vi.fn();
    dispose = vi.fn();
    focus = vi.fn();
    onData = vi.fn(() => ({ dispose: vi.fn() }));
    constructor() {
      terminals.push(this);
    }
  }

  class MockWebSocket {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSED = 3;
    readyState = MockWebSocket.CONNECTING;
    binaryType = "";
    onmessage: ((event: MessageEvent) => void) | null = null;
    onclose: (() => void) | null = null;
    onerror: (() => void) | null = null;
    send = vi.fn();
    close = vi.fn(() => {
      const wasConnecting = this.readyState === MockWebSocket.CONNECTING;
      this.readyState = MockWebSocket.CLOSED;
      this.onclose?.();
      if (wasConnecting) this.onerror?.();
    });
    addEventListener = vi.fn();
    constructor() {
      sockets.push(this);
    }
  }

  return {
    terminals,
    sockets,
    getAgentLoginStatus: vi.fn(),
    resizeAgentLogin: vi.fn().mockResolvedValue(undefined),
    stopAgentLogin: vi.fn().mockResolvedValue(undefined),
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
  startHostShell: vi.fn(),
  createQuickTerminalTab: vi.fn(),
  toQuickTerminalTab: (tab: unknown) => tab,
  agentLoginStreamUrl: (sessionId: string) => `ws://api.test/${sessionId}`,
  getAgentLoginStatus: mocks.getAgentLoginStatus,
  resizeAgentLogin: mocks.resizeAgentLogin,
  stopAgentLogin: mocks.stopAgentLogin,
}));

import { QuickTerminalTabView } from "./quick-terminal-tab-view";

const session = {
  session_id: "session-1",
  agent_id: "_host_shell",
  cmd: ["/bin/bash"],
  running: true,
  started_at: "2026-08-04T00:00:00Z",
};

const tab = {
  tabId: "6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b",
  workspaceId: "workspace-1",
  sessionId: "session-1",
  sequence: 1,
  status: "running" as const,
};

beforeEach(() => {
  mocks.getAgentLoginStatus.mockReset().mockResolvedValue(session);
  mocks.resizeAgentLogin.mockClear();
  mocks.stopAgentLogin.mockClear();
  mocks.terminals.length = 0;
  mocks.sockets.length = 0;
  vi.stubGlobal("WebSocket", mocks.MockWebSocket);
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe = vi.fn();
      disconnect = vi.fn();
      constructor(_callback: ResizeObserverCallback) {}
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Quick Terminal render lifecycle", () => {
  // @covers AC-UI-QUICK-TERMINAL-001.15
  it("retains one terminal and stream across inline callback rerenders", async () => {
    function Harness() {
      const [revision, setRevision] = useState(0);
      return (
        <>
          <button
            type="button"
            data-testid="rerender"
            onClick={() => setRevision((value) => value + 1)}
          >
            {revision}
          </button>
          <QuickTerminalTabView tab={tab} onStateChange={() => undefined} />
        </>
      );
    }

    const view = render(<Harness />);
    await waitFor(() => expect(mocks.sockets).toHaveLength(1));
    for (let index = 0; index < 5; index += 1) {
      fireEvent.click(view.getByTestId("rerender"));
    }

    expect(mocks.getAgentLoginStatus).toHaveBeenCalledTimes(1);
    expect(mocks.terminals).toHaveLength(1);
    expect(mocks.sockets).toHaveLength(1);
    expect(mocks.terminals[0]?.dispose).not.toHaveBeenCalled();

    view.unmount();
    expect(mocks.terminals[0]?.dispose).toHaveBeenCalledTimes(1);
  });

  // @covers AC-UI-QUICK-TERMINAL-001.15, AC-UI-QUICK-TERMINAL-001.16
  it("does not loop when persisted state rerenders the parent and a connecting socket errors", async () => {
    function Harness() {
      const [updates, setUpdates] = useState(0);
      return (
        <QuickTerminalTabView
          tab={tab}
          onStateChange={() => {
            if (updates < 12) {
              setUpdates((value) => value + 1);
            }
          }}
        />
      );
    }

    const view = render(<Harness />);
    await waitFor(() => expect(mocks.sockets.length).toBeGreaterThan(0));

    expect(mocks.getAgentLoginStatus).toHaveBeenCalledTimes(1);
    expect(mocks.terminals).toHaveLength(1);
    expect(mocks.sockets).toHaveLength(1);
    expect(mocks.sockets[0]?.close).not.toHaveBeenCalled();
    view.unmount();
  });
});
