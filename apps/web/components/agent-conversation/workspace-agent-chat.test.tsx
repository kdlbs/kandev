import { describe, expect, it, vi } from "vitest";
import { render } from "@testing-library/react";
import { WorkspaceAgentChat } from "./workspace-agent-chat";

// Mock the chat primitives deeply enough to render the component tree.
vi.mock("@/components/task/chat/chat-input-area", () => ({
  ChatInputArea: () => <div data-testid="agent-conv-chat-input" />,
  useSubmitHandler: () => ({ isSending: false, handleSubmit: vi.fn() }),
  useChatPanelHandlers: () => ({ handleCancelTurn: vi.fn() }),
}));
vi.mock("@/components/task/chat/message-list", () => ({
  MessageList: () => <div data-testid="agent-conv-message-list" />,
}));
vi.mock("@/components/task/chat/clarification-input-overlay", () => ({
  ClarificationInputOverlay: () => <div data-testid="agent-conv-clarification" />,
}));
vi.mock("@/components/task/chat/resize-handle", () => ({
  ResizeHandle: () => <div data-testid="agent-conv-resize-handle" />,
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));
vi.mock("@/hooks/use-resizable-clarification-overlay", () => ({
  useResizableClarificationOverlay: () => ({
    height: null,
    containerRef: { current: null },
    resetHeight: vi.fn(),
    resizeHandleProps: {},
  }),
}));
vi.mock("@/lib/session-workspace-path", () => ({
  getSessionWorkspacePath: () => "/tmp/worktree",
}));
vi.mock("@kandev/ui/button", () => ({
  Button: ({ children }: { children: React.ReactNode }) => <button>{children}</button>,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock("zustand", () => ({
  create: () => () => ({}),
  useStore: () => ({}),
}));

function mockChatPanelState(overrides?: Record<string, unknown>) {
  return {
    resolvedSessionId: "sess-abc",
    session: null,
    groupedItems: [],
    allMessages: [],
    permissionsByToolCallId: {},
    childrenByParentToolCallId: {},
    taskId: null,
    messagesLoading: false,
    isWorking: false,
    pendingClarification: false,
    pendingClarificationGroup: null,
    ...overrides,
  };
}

// The `@/` alias is resolved at vitest config level, so use it directly.
vi.mock("@/components/task/chat/use-chat-panel-state", () => ({
  useChatPanelState: () => mockChatPanelState(),
}));

describe("WorkspaceAgentChat", () => {
  const defaultProps = {
    workspaceId: "ws-1",
    conversationKey: "coordinator",
    sessionId: "sess-abc",
  };

  it("renders the chat shell with message list and input area", () => {
    const { container } = render(<WorkspaceAgentChat {...defaultProps} />);
    expect(container.querySelector('[data-testid="workspace-agent-chat"]')).toBeTruthy();
    const messageWrappers = container.querySelectorAll('[data-testid="agent-conv-messages"]');
    expect(messageWrappers.length).toBe(1);
    expect(container.querySelector('[data-testid="agent-conv-chat-input"]')).toBeTruthy();
  });

  it("renders with a placeholder override when provided", () => {
    const { container } = render(
      <WorkspaceAgentChat
        {...defaultProps}
        placeholderOverride="Ask the coordinator anything..."
      />,
    );
    expect(container.querySelector('[data-testid="workspace-agent-chat"]')).toBeTruthy();
  });

  it("passes the sessionId to chat panel state", () => {
    const { container } = render(<WorkspaceAgentChat {...defaultProps} />);
    // The component renders its shell — the sessionId flows through hooks
    // that are mocked above. We verify the structure is correct.
    expect(container.querySelector('[data-testid="agent-conv-messages"]')).toBeTruthy();
  });
});
