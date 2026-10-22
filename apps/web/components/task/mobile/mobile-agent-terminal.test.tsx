import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Terminal } from "@xterm/xterm";
import { startTransition, Suspense, useState } from "react";
import { useShellModifiersStore } from "@/lib/terminal/shell-modifiers";
import { AgentTerminalInput } from "./mobile-agent-terminal";

const AGENT_SESSION_ID = "agent-1";
const KEYBAR_TEST_ID = "mobile-terminal-keybar";
const CTRL_TEST_ID = "keybar-key-ctrl";

type TerminalProps = {
  manualInputRouting?: boolean;
  onXtermReady: (terminal: Terminal) => void;
  onWsReady: (socket: WebSocket) => void;
};
const source = vi.hoisted(() => ({
  props: null as TerminalProps | null,
  fallbackSend: vi.fn(),
  quickChat: { isOpen: false },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { quickChat: { isOpen: boolean } }) => unknown) =>
    selector({ quickChat: source.quickChat }),
}));
vi.mock("../passthrough-terminal", () => ({
  PassthroughTerminal: (props: TerminalProps) => {
    source.props = props;
    return null;
  },
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ send: source.fallbackSend }),
}));

function terminalSource() {
  const listeners = new Set<(data: string) => void>();
  const terminal = {
    focus: vi.fn(),
    onData: (listener: (data: string) => void) => {
      listeners.add(listener);
      return { dispose: () => listeners.delete(listener) };
    },
  };
  return {
    terminal: terminal as unknown as Terminal,
    focus: terminal.focus,
    emit: (data: string) => act(() => listeners.forEach((listener) => listener(data))),
    listeners,
  };
}

function socketSource() {
  const socket = Object.assign(new EventTarget(), { readyState: 1, send: vi.fn() });
  return {
    socket: socket as unknown as WebSocket,
    bytes: () => socket.send.mock.calls.map(([data]) => new TextDecoder().decode(data)),
    close: () =>
      act(() => {
        socket.readyState = 3;
        socket.dispatchEvent(new Event("close"));
      }),
  };
}

function connectedTerminal(showControls = true) {
  const view = render(
    <AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls={showControls} />,
  );
  const terminal = terminalSource();
  const socket = socketSource();
  act(() => {
    source.props!.onXtermReady(terminal.terminal);
    source.props!.onWsReady(socket.socket);
  });
  return { ...view, ...terminal, ...socket };
}

beforeEach(() => {
  source.quickChat.isOpen = false;
  source.fallbackSend.mockClear();
  vi.stubGlobal("WebSocket", { OPEN: 1, CLOSED: 3 });
  useShellModifiersStore.getState().reset();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  expect(source.fallbackSend).not.toHaveBeenCalled();
  useShellModifiersStore.getState().reset();
});

describe("phone passthrough input ownership", () => {
  // @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.2, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.3
  it("uses receive-only attachment and transforms each typed chord exactly once", () => {
    const view = connectedTerminal();
    expect(source.props!.manualInputRouting).toBe(true);
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    view.emit("c");
    view.emit("d");
    expect(view.bytes()).toEqual(["\x03", "d"]);
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
    expect(view.listeners.size).toBe(1);
  });

  it("keeps Ctrl sticky for multiple keys and preserves paste chunks", () => {
    const view = connectedTerminal();
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    view.emit("a");
    view.emit("e");
    view.emit("pasted text");
    expect(view.bytes()).toEqual(["\x01", "\x05", "pasted text"]);
    expect(useShellModifiersStore.getState().ctrl.sticky).toBe(true);
  });

  it("sends bar keys directly rather than feeding them back through xterm", () => {
    const view = connectedTerminal();
    for (const key of ["up", "down", "left", "right", "esc", "tab", "ctrl-c", "ctrl-d"]) {
      fireEvent.click(screen.getByTestId(`keybar-key-${key}`));
    }
    fireEvent.click(screen.getByTestId("keybar-key-shift"));
    fireEvent.click(screen.getByTestId("keybar-key-tab"));
    expect(view.bytes()).toEqual([
      "\x1b[A",
      "\x1b[B",
      "\x1b[D",
      "\x1b[C",
      "\x1b",
      "\t",
      "\x03",
      "\x04",
      "\x1b[Z",
    ]);
    expect(view.focus).toHaveBeenCalledTimes(10);
  });

  it("keeps committed input ownership when a transition suspends", () => {
    let hideControls: () => void = () => {};
    const pending = new Promise<void>(() => {});
    function PendingPanel({ open }: { open: boolean }) {
      if (open) throw pending;
      return null;
    }
    function Surface() {
      const [open, setOpen] = useState(false);
      hideControls = () => startTransition(() => setOpen(true));
      return (
        <Suspense fallback={null}>
          <AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls={!open} />
          <PendingPanel open={open} />
        </Suspense>
      );
    }
    render(<Surface />);
    const terminal = terminalSource();
    const socket = socketSource();
    act(() => {
      source.props!.onXtermReady(terminal.terminal);
      source.props!.onWsReady(socket.socket);
    });
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    act(hideControls);
    expect(screen.getByTestId(CTRL_TEST_ID).getAttribute("aria-pressed")).toBe("true");
    terminal.emit("c");
    expect(socket.bytes()).toEqual(["\x03"]);
  });
});

describe("phone passthrough readiness", () => {
  // @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.1, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.6
  it.each(["terminal-first", "socket-first"])(
    "waits for both readiness boundaries (%s)",
    (order) => {
      render(<AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls />);
      const terminal = terminalSource();
      const socket = socketSource();
      const ready = [
        () => source.props!.onXtermReady(terminal.terminal),
        () => source.props!.onWsReady(socket.socket),
      ];
      if (order === "socket-first") ready.reverse();
      act(ready[0]);
      expect(screen.queryByTestId(KEYBAR_TEST_ID)).toBeNull();
      act(ready[1]);
      expect(screen.getByTestId(KEYBAR_TEST_ID)).toBeTruthy();
    },
  );

  it("does not clear another terminal's modifiers before acquiring input ownership", () => {
    useShellModifiersStore.getState().toggleCtrl();
    const view = render(
      <AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls />,
    );
    view.unmount();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });

  it("drops disconnected input and targets only the replacement socket", () => {
    const view = connectedTerminal();
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    view.close();
    expect(screen.queryByTestId(KEYBAR_TEST_ID)).toBeNull();
    view.emit("c");
    expect(view.bytes()).toEqual([]);
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
    const replacement = socketSource();
    act(() => source.props!.onWsReady(replacement.socket));
    fireEvent.click(screen.getByTestId("keybar-key-up"));
    expect(replacement.bytes()).toEqual(["\x1b[A"]);
    expect(view.bytes()).toEqual([]);
  });

  it("ignores an old socket close after replacement and retains stable readiness callbacks", () => {
    const view = connectedTerminal();
    const originalCallbacks = source.props;
    const replacement = socketSource();
    act(() => source.props!.onWsReady(replacement.socket));
    view.close();
    view.emit("x");
    expect(replacement.bytes()).toEqual(["x"]);
    expect(source.props!.onWsReady).toBe(originalCallbacks!.onWsReady);
    expect(source.props!.onXtermReady).toBe(originalCallbacks!.onXtermReady);
  });
});

describe("phone passthrough lifecycle", () => {
  // @covers AC-CLI-MOBILE-PASSTHROUGH-NAV-001.5, AC-CLI-MOBILE-PASSTHROUGH-NAV-001.7
  it("hides controls and clears modifiers without reconnecting while the composer is open", () => {
    const view = connectedTerminal();
    const callback = source.props!.onWsReady;
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    view.rerender(
      <AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls={false} />,
    );
    expect(screen.queryByTestId(KEYBAR_TEST_ID)).toBeNull();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
    view.emit("c");
    expect(view.bytes()).toEqual(["c"]);
    view.rerender(<AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} showControls />);
    expect(screen.getByTestId(KEYBAR_TEST_ID)).toBeTruthy();
    expect(source.props!.onWsReady).toBe(callback);
    expect(view.listeners.size).toBe(1);
  });

  it("releases the old terminal listener and modifiers on session change", () => {
    const view = connectedTerminal();
    fireEvent.click(screen.getByTestId("keybar-key-shift"));
    view.rerender(<AgentTerminalInput mobile touch sessionId="agent-2" showControls />);
    view.emit("old");
    expect(view.listeners.size).toBe(0);
    expect(view.bytes()).toEqual([]);
    expect(useShellModifiersStore.getState().shift.latched).toBe(false);
    expect(screen.queryByTestId(KEYBAR_TEST_ID)).toBeNull();
  });

  it("releases subscriptions and modifiers when leaving the surface", () => {
    const view = connectedTerminal();
    act(() => useShellModifiersStore.getState().toggleCtrl());
    view.unmount();
    view.emit("old");
    expect(view.listeners.size).toBe(0);
    expect(view.bytes()).toEqual([]);
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(false);
  });

  it("preserves Quick Terminal modifiers across background reconnect and unmount", () => {
    const view = connectedTerminal();
    fireEvent.click(screen.getByTestId(CTRL_TEST_ID));
    source.quickChat.isOpen = true;
    view.rerender(<AgentTerminalInput mobile touch sessionId={AGENT_SESSION_ID} />);
    // Quick Terminal acquires the shared controls after its overlay opens.
    act(() => {
      useShellModifiersStore.getState().reset();
      useShellModifiersStore.getState().toggleCtrl();
    });
    view.close();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
    const replacement = socketSource();
    act(() => source.props!.onWsReady(replacement.socket));
    view.emit("c");
    expect(replacement.bytes()).toEqual(["c"]);
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
    expect(screen.queryByTestId(KEYBAR_TEST_ID)).toBeNull();
    view.unmount();
    expect(useShellModifiersStore.getState().ctrl.latched).toBe(true);
  });
});
