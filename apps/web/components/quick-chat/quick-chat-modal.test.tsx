import { useCallback, useState } from "react";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useClarificationEscapeGuard } from "@/hooks/use-clarification-escape-guard";

const setQuickChatInitialPrompt = vi.fn();
const quickChatSessionItems: Record<string, { state: string; task_id: string }> = {};
const quickChatPrepareProgress: Record<string, { status: string }> = {};
const handleActivateTerminal = vi.fn();
const handleNewTerminal = vi.fn();
const handleCloseTerminal = vi.fn();
const persistTabOrder = vi.fn();
const handleOpenChange = vi.fn();
const handleNewChat = vi.fn();
const escapePreventDefault = vi.fn();
const CHAT_NAME = "Chat one";
const WORKSPACE_ID = "ws-1";
const TERMINAL_TAB_ID = "terminal-1";
const FIRE_ESCAPE_TESTID = "fire-escape";
const TOGGLE_ESCAPE_GUARD_TESTID = "toggle-escape-guard";
const responsiveMock = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsiveMock,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      setQuickChatInitialPrompt: typeof setQuickChatInitialPrompt;
      taskSessions: { items: typeof quickChatSessionItems };
      prepareProgress: { bySessionId: typeof quickChatPrepareProgress };
    }) => unknown,
  ) =>
    selector({
      setQuickChatInitialPrompt,
      taskSessions: { items: quickChatSessionItems },
      prepareProgress: { bySessionId: quickChatPrepareProgress },
    }),
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
  DialogContent: ({
    children,
    onEscapeKeyDown,
  }: {
    children: React.ReactNode;
    onEscapeKeyDown?: (event: { preventDefault: () => void }) => void;
  }) => (
    <div>
      {children}
      {/* Stands in for Radix's DismissableLayer consulting onEscapeKeyDown
          before deciding whether to close the dialog on a real Escape press. */}
      <button
        type="button"
        data-testid={FIRE_ESCAPE_TESTID}
        onClick={() => onEscapeKeyDown?.({ preventDefault: escapePreventDefault })}
      />
    </div>
  ),
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

function EscapeGuardProbe() {
  const [guarded, setGuarded] = useState(false);
  // A stand-in for a real handled/in-scope/unmodified Escape: the toggle
  // simulates whichever widget-side condition (enabled, target-in-scope, no
  // modifier) is currently true, and the dialog must consult the predicate
  // itself rather than a value it derived some other way. Memoized like a
  // real caller (e.g. clarification-input-overlay.tsx's useEscapeGuardRegistration)
  // must -- an inline closure would be a fresh reference every render, and the
  // hook's effect re-running every render would re-register in a loop.
  const predicate = useCallback(() => guarded, [guarded]);
  useClarificationEscapeGuard(predicate);
  return (
    <div data-testid="quick-chat-session-view">
      <button
        type="button"
        data-testid={TOGGLE_ESCAPE_GUARD_TESTID}
        onClick={() => setGuarded((current) => !current)}
      />
    </div>
  );
}
vi.mock("./quick-chat-session-view", () => ({
  QuickChatSessionView: () => <EscapeGuardProbe />,
}));
vi.mock("./quick-chat-setup", () => ({
  QuickChatSetup: () => <div data-testid="quick-chat-setup" />,
}));
vi.mock("@/components/config-chat/config-chat-setup", () => ({
  ConfigChatSetup: () => <div data-testid="config-chat-setup" />,
}));
const useQuickChatModalMock = vi.fn(() => defaultQuickChatModalState);
vi.mock("./use-quick-chat-modal", () => ({
  useQuickChatModal: () => useQuickChatModalMock(),
}));

const defaultQuickChatModalState = {
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
  tabOrder: ["conversation:chat-1", "terminal:terminal-1"],
  persistTabOrder,
  tabOrderSyncError: null,
};

import { QuickChatModal } from "./quick-chat-modal";

afterEach(() => {
  cleanup();
  for (const key of Object.keys(quickChatSessionItems)) delete quickChatSessionItems[key];
  for (const key of Object.keys(quickChatPrepareProgress)) delete quickChatPrepareProgress[key];
  vi.clearAllMocks();
  useQuickChatModalMock.mockReset();
  useQuickChatModalMock.mockReturnValue(defaultQuickChatModalState);
  responsiveMock.isFinePointer = true;
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

  it("shows a spinner only for working conversation tabs", () => {
    quickChatSessionItems["chat-1"] = { state: "RUNNING", task_id: "task-1" };
    quickChatSessionItems["quick-chat-setup:ws-1:chat"] = {
      state: "RUNNING",
      task_id: "task-setup",
    };
    useQuickChatModalMock.mockReturnValue({
      ...defaultQuickChatModalState,
      activeKind: "conversation",
      activeSessionNeedsAgent: false,
      terminalTabs: [],
      sessions: [
        ...defaultQuickChatModalState.sessions,
        { sessionId: "quick-chat-setup:ws-1:chat", workspaceId: WORKSPACE_ID, kind: "chat" },
      ],
    });

    render(<QuickChatModal workspaceId={WORKSPACE_ID} />);

    const tabs = screen.getAllByTestId("quick-chat-tab");
    expect(within(tabs[0]).getByRole("status", { name: "Loading" })).toBeTruthy();
    expect(tabs[1].querySelector('[role="status"]')).toBeNull();
  });

  it("uses the same persisted order callback for coarse-pointer moves", () => {
    responsiveMock.isFinePointer = false;
    render(<QuickChatModal workspaceId={WORKSPACE_ID} />);

    fireEvent.click(screen.getByRole("button", { name: "Actions for Terminal 1" }));
    fireEvent.click(screen.getByRole("button", { name: "Move Terminal 1 left" }));

    expect(persistTabOrder).toHaveBeenCalledWith(["terminal:terminal-1", "conversation:chat-1"]);
  });

  it("gives coarse-pointer close controls a 44px target", () => {
    responsiveMock.isFinePointer = false;
    render(<QuickChatModal workspaceId={WORKSPACE_ID} />);

    expect(screen.getByRole("button", { name: `Actions for ${CHAT_NAME}` }).className).toContain(
      "h-11 w-11",
    );
    expect(screen.getByRole("button", { name: "Actions for Terminal 1" }).className).toContain(
      "h-11 w-11",
    );
  });
});

describe("QuickChatModal — Escape with a pending clarification", () => {
  function renderWithConversationTab() {
    // mockReturnValue (not -Once): QuickChatModal's own escape-guard state
    // lives in this component and re-renders it on every guard toggle, which
    // re-invokes useQuickChatModal() more than the one time a *Once value
    // would cover.
    useQuickChatModalMock.mockReturnValue({
      ...defaultQuickChatModalState,
      activeKind: "conversation",
      activeSessionNeedsAgent: false,
    });
    render(<QuickChatModal workspaceId={WORKSPACE_ID} />);
  }

  it("lets Escape close the modal when no clarification is guarding it", () => {
    renderWithConversationTab();

    fireEvent.click(screen.getByTestId(FIRE_ESCAPE_TESTID));

    expect(escapePreventDefault).not.toHaveBeenCalled();
  });

  it("swallows the first Escape while a clarification is pending and expanded, keeping the modal open", () => {
    renderWithConversationTab();

    fireEvent.click(screen.getByTestId(TOGGLE_ESCAPE_GUARD_TESTID));
    fireEvent.click(screen.getByTestId(FIRE_ESCAPE_TESTID));

    expect(escapePreventDefault).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("quick-chat-dialog")).toBeTruthy();
  });

  it("lets the second Escape close the modal once the clarification has collapsed", () => {
    renderWithConversationTab();

    fireEvent.click(screen.getByTestId(TOGGLE_ESCAPE_GUARD_TESTID)); // guard on: pending + expanded
    fireEvent.click(screen.getByTestId(FIRE_ESCAPE_TESTID)); // 1st Escape: collapses, swallowed
    fireEvent.click(screen.getByTestId(TOGGLE_ESCAPE_GUARD_TESTID)); // simulates the resulting collapse
    fireEvent.click(screen.getByTestId(FIRE_ESCAPE_TESTID)); // 2nd Escape: unguarded, would close

    expect(escapePreventDefault).toHaveBeenCalledTimes(1);
  });
});
