import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const setQuickChatInitialPrompt = vi.fn();
const handleActivateTerminal = vi.fn();
const handleNewTerminal = vi.fn();
const handleCloseTerminal = vi.fn();
const handleOpenChange = vi.fn();
const handleNewChat = vi.fn();
const CHAT_NAME = "Chat one";
const WORKSPACE_ID = "ws-1";
const TERMINAL_TAB_ID = "terminal-1";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: { setQuickChatInitialPrompt: typeof setQuickChatInitialPrompt }) => unknown,
  ) => selector({ setQuickChatInitialPrompt }),
}));

vi.mock("@/lib/routing/client-dynamic", () => ({
  default: () =>
    function MockTerminalView({ tab }: { tab: { tabId: string } }) {
      return <div data-testid="mock-quick-terminal-view" data-tab-id={tab.tabId} />;
    },
}));

vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div data-testid="quick-chat-dialog">{children}</div> : null,
  DialogContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuItem: ({
    children,
    onSelect,
    ...props
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    [key: string]: unknown;
  }) => (
    <button type="button" {...props} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
}));

vi.mock("@/hooks/use-quick-chat-width", () => ({
  useQuickChatWidth: () => ({
    width: 720,
    leftResizeHandleProps: {},
    rightResizeHandleProps: {},
  }),
}));

vi.mock("@/components/config-chat/use-config-chat", () => ({
  useConfigChat: () => ({
    defaultProfileId: null,
    isStarting: false,
    error: null,
    reset: vi.fn(),
    startSession: vi.fn(),
  }),
}));

vi.mock("./quick-chat-delete-dialog", () => ({
  QuickChatDeleteDialog: () => null,
}));
vi.mock("./quick-chat-session-view", () => ({
  QuickChatSessionView: () => <div data-testid="quick-chat-session-view" />,
}));
vi.mock("./quick-chat-setup", () => ({
  QuickChatSetup: () => <div data-testid="quick-chat-setup" />,
}));
vi.mock("@/components/config-chat/config-chat-setup", () => ({
  ConfigChatSetup: () => <div data-testid="config-chat-setup" />,
}));
vi.mock("./use-quick-chat-modal", () => ({
  useQuickChatModal: () => ({
    isOpen: true,
    sessions: [{ sessionId: "chat-1", workspaceId: WORKSPACE_ID, kind: "chat", name: CHAT_NAME }],
    terminalTabs: [
      {
        tabId: TERMINAL_TAB_ID,
        workspaceId: WORKSPACE_ID,
        sessionId: "pty-1",
        sequence: 1,
        status: "running",
      },
    ],
    activeKind: "terminal",
    activeSessionId: "chat-1",
    activeTerminalTabId: TERMINAL_TAB_ID,
    activeTerminalTab: {
      tabId: TERMINAL_TAB_ID,
      workspaceId: WORKSPACE_ID,
      sessionId: "pty-1",
      sequence: 1,
      status: "running",
    },
    activeSession: {
      sessionId: "chat-1",
      workspaceId: WORKSPACE_ID,
      kind: "chat",
      name: CHAT_NAME,
    },
    sessionToClose: null,
    setupKey: 0,
    activeSessionNeedsAgent: true,
    pendingAgentId: null,
    setActiveQuickChatSession: vi.fn(),
    setSessionToClose: vi.fn(),
    handleOpenChange,
    handleNewChat,
    handleNewTerminal,
    handleActivateTerminal,
    handleTerminalStateChange: vi.fn(),
    handleSetupKindChange: vi.fn(),
    handleSelectAgent: vi.fn(),
    handleCloseTab: vi.fn(),
    handleCloseTerminal,
    handleConfirmClose: vi.fn(),
    handleRename: vi.fn(),
  }),
}));

import { QuickChatModal } from "./quick-chat-modal";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("QuickChatModal mixed tabs", () => {
  it("renders terminal content beside conversation tabs in one dialog", () => {
    render(<QuickChatModal workspaceId={WORKSPACE_ID} />);

    expect(screen.getByTestId("quick-chat-dialog")).toBeTruthy();
    expect(screen.queryByTestId("quick-chat-session-view")).toBeNull();
    expect(screen.queryByTestId("quick-chat-setup")).toBeNull();
    expect(screen.getByTestId("quick-chat-tab").className).toContain("text-muted-foreground");
    expect(screen.getByTestId("mock-quick-terminal-view").getAttribute("data-tab-id")).toBe(
      "terminal-1",
    );
    expect(screen.getByTestId("quick-chat-add-menu-trigger")).toBeTruthy();
    expect(screen.getByText("Agents")).toBeTruthy();
    expect(screen.getByText("Terminals")).toBeTruthy();
    expect(screen.queryByTestId("quick-chat-menu-session-chat-1")).toBeNull();
    expect(screen.queryByTestId("quick-chat-menu-terminal-terminal-1")).toBeNull();
  });
});
