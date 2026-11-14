import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ShellTerminal } from "./shell-terminal";
import { PassthroughTerminal } from "./passthrough-terminal";

const ACTION_NAME = "task:jumpToLatestOutput";

const { terminal, listeners, sendInput, searchState, restoration } = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return { dispose: () => listeners.delete(listener) };
  };
  const terminal = {
    buffer: { active: { type: "normal", viewportY: 5, baseY: 20 }, onBufferChange: subscribe },
    loadAddon: vi.fn(),
    open: vi.fn(),
    write: vi.fn(),
    dispose: vi.fn(),
    attachCustomKeyEventHandler: vi.fn(),
    onScroll: subscribe,
    onWriteParsed: subscribe,
    onResize: subscribe,
    scrollToBottom: vi.fn(() => {
      terminal.buffer.active.viewportY = terminal.buffer.active.baseY;
      listeners.forEach((listener) => listener());
    }),
  };
  return {
    terminal,
    listeners,
    sendInput: vi.fn(),
    searchState: { isOpen: false },
    restoration: { status: null as string | null },
  };
});

vi.mock("@xterm/xterm", () => ({
  Terminal: vi.fn(function () {
    return terminal;
  }),
}));
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: vi.fn(function () {
    return { fit: vi.fn() };
  }),
}));
vi.mock("@xterm/addon-web-links", () => ({
  WebLinksAddon: vi.fn(function () {
    return {};
  }),
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeSessionId: null },
      taskSessions: { items: {} },
      userSettings: {},
      shell: { outputs: {} },
      environmentIdBySessionId: {},
    }),
  useAppStoreApi: () => ({ getState: () => ({}) }),
}));
vi.mock("@/components/theme/app-theme", () => ({ useTheme: () => ({ resolvedTheme: "dark" }) }));
vi.mock("@/hooks/domains/session/use-session", () => ({ useSession: () => ({}) }));
vi.mock("@/hooks/domains/session/use-session-agentctl", () => ({ useSessionAgentctl: () => ({}) }));
vi.mock("@/hooks/domains/session/use-workspace-restoration", () => ({
  useWorkspaceRestoration: () => restoration,
}));
vi.mock("@/hooks/use-environment-session-id", () => ({ useEnvironmentSessionId: () => null }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: true }),
}));
vi.mock("@/hooks/use-clarification-escape-guard", () => ({
  useClarificationEscapeGuard: () => {},
}));
vi.mock("./use-terminal-busy-tracking", () => ({ useTerminalBusyTracking: () => {} }));
vi.mock("./use-touch-scroll", () => ({ useTouchScroll: () => {} }));
vi.mock("./use-passthrough-terminal", async () => {
  const { useEffect } = await import("react");
  return {
    MIN_WIDTH: 40,
    MIN_HEIGHT: 40,
    useTerminalInit: ({
      xtermRef,
      onReady,
    }: {
      xtermRef: { current: unknown };
      onReady: () => void;
    }) => {
      useEffect(() => {
        xtermRef.current = terminal;
        onReady();
        return () => {
          xtermRef.current = null;
        };
      }, []);
    },
    useWebSocketConnection: ({
      isTerminalReady,
      onConnected,
    }: {
      isTerminalReady: boolean;
      onConnected: () => void;
    }) => {
      useEffect(() => {
        if (isTerminalReady) onConnected();
      }, [isTerminalReady]);
    },
    useSendResize: () => vi.fn(),
    useSendInput: () => sendInput,
    useFitAndResize: () => vi.fn(),
  };
});
vi.mock("@/hooks/use-terminal-link-handler", () => ({ useTerminalLinkHandler: () => undefined }));
vi.mock("./use-terminal-theme", () => ({ useTerminalTheme: () => {} }));
vi.mock("./use-terminal-search", () => ({
  useTerminalSearch: () => ({ ...searchState, open: vi.fn(), close: vi.fn() }),
}));
vi.mock("./terminal-search-bar", () => ({ TerminalSearchBar: () => null }));
vi.mock("./workspace-unavailable", () => ({ WorkspaceUnavailable: () => null }));
vi.mock("@/hooks/use-panel-search", () => ({ usePanelSearch: () => {} }));
vi.mock("./terminal-buffer-reader", () => ({
  exposeBufferReader: () => {},
  clearBufferReader: () => {},
}));
vi.mock("@/lib/terminal/suppress-ios-keyboard-assists", () => ({
  suppressIOSKeyboardAssists: () => {},
}));
vi.mock("@/lib/terminal/send-shell-input", () => ({ sendShellInput: sendInput }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

describe("terminal output navigation", () => {
  beforeEach(() => {
    terminal.buffer.active = { type: "normal", viewportY: 5, baseY: 20 };
    terminal.scrollToBottom.mockClear();
    sendInput.mockClear();
    searchState.isOpen = false;
    restoration.status = null;
  });
  afterEach(() => cleanup());

  it("shows the action on the existing dev-server host above the bottom", () => {
    render(<ShellTerminal processOutput="earlier output" processId="dev" />);
    expect(screen.getByRole("button", { name: ACTION_NAME })).toBeTruthy();
  });

  it("hides at the bottom and while search is open", () => {
    terminal.buffer.active.viewportY = terminal.buffer.active.baseY;
    const view = render(<ShellTerminal processOutput="output" processId="dev" />);
    const button = () => screen.queryByRole("button", { name: ACTION_NAME });
    expect(button()).toBeNull();
    act(() => {
      terminal.buffer.active.viewportY = 5;
      listeners.forEach((listener) => listener());
    });
    expect(button()).toBeTruthy();
    searchState.isOpen = true;
    view.rerender(<ShellTerminal processOutput="output" processId="dev" />);
    expect(button()).toBeNull();
    searchState.isOpen = false;
    view.rerender(<ShellTerminal processOutput="output" processId="dev" />);
    expect(button()).toBeTruthy();
  });

  it("limits passthrough navigation to an unobstructed connected task shell", () => {
    const view = render(<PassthroughTerminal mode="shell" environmentId="env" terminalId="term" />);
    const button = () => screen.queryByRole("button", { name: ACTION_NAME });
    expect(button()).toBeTruthy();
    restoration.status = "pending";
    view.rerender(<PassthroughTerminal mode="shell" environmentId="env" terminalId="term" />);
    expect(button()).toBeNull();
    view.unmount();
    restoration.status = null;
    render(<PassthroughTerminal mode="agent" sessionId="agent" />);
    expect(button()).toBeNull();
  });

  it("tracks scrolling, output, alternate buffer and activation without sending input", () => {
    const view = render(<ShellTerminal processOutput="earlier output" processId="dev" />);
    const button = () => screen.queryByRole("button", { name: ACTION_NAME });
    expect(button()).toBeTruthy();
    act(() => {
      terminal.buffer.active.baseY = 21;
      listeners.forEach((listener) => listener());
    });
    expect(terminal.buffer.active.viewportY).toBe(5);
    expect(button()).toBeTruthy();
    act(() => {
      terminal.buffer.active.type = "alternate";
      listeners.forEach((listener) => listener());
    });
    expect(button()).toBeNull();
    act(() => {
      terminal.buffer.active.type = "normal";
      listeners.forEach((listener) => listener());
    });
    const other = document.createElement("button");
    document.body.append(other);
    other.focus();
    fireEvent.click(button()!);
    expect(terminal.scrollToBottom).toHaveBeenCalledOnce();
    expect(sendInput).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(other);
    expect(button()).toBeNull();
    view.unmount();
    expect(listeners.size).toBe(0);
    other.remove();
  });
});
