import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const useSubmitHandlerSpy = vi.hoisted(() => vi.fn());
const useQuickChatInitialDraftSpy = vi.hoisted(() => vi.fn());
const panelState = vi.hoisted(
  () =>
    ({
      resolvedSessionId: "session-1",
      taskId: "task-1",
      pendingClarification: null,
      pendingClarificationGroup: [],
      planCommentMigration: { isBlocking: false },
      groupedItems: [],
      allMessages: [],
      permissionsByToolCallId: {},
      childrenByParentToolCallId: {},
      messagesLoading: false,
      isWorking: false,
      session: null,
    }) as Record<string, unknown>,
);

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: vi.fn(),
}));

vi.mock("@/components/task/chat/use-chat-panel-state", () => ({
  useChatPanelState: () => panelState,
}));

vi.mock("@/components/task/chat/chat-input-area", () => ({
  ChatInputArea: () => <div data-testid="chat-input-area" />,
  useSubmitHandler: (
    state: unknown,
    onSend: unknown,
    options: { transformOutgoing?: (message: string) => string },
  ) => {
    useSubmitHandlerSpy(state, onSend, options);
    return { isSending: false, handleSubmit: vi.fn() };
  },
  useChatPanelHandlers: () => ({ handleCancelTurn: vi.fn() }),
}));

vi.mock("@/components/task/chat/clarification-panel-section", () => ({
  ClarificationPanelSection: () => null,
}));

vi.mock("@/components/task/chat/message-list", () => ({
  MessageList: () => <div data-testid="message-list" />,
}));

vi.mock("@/lib/session-workspace-path", () => ({
  getSessionWorkspacePath: () => null,
}));

vi.mock("@/components/task/chat/route-panel-mouse-down", () => ({
  routePanelMouseDown: vi.fn(),
}));

vi.mock("./use-quick-chat-initial-prompt", () => ({
  useQuickChatInitialPrompt: vi.fn(),
}));

vi.mock("./use-quick-chat-initial-draft", () => ({
  useQuickChatInitialDraft: (args: unknown) => useQuickChatInitialDraftSpy(args),
}));

vi.mock("./quick-chat-cancel-commands", () => ({
  QuickChatCancelCommands: () => null,
}));

vi.mock("@/hooks/use-late-clarification-message", () => ({
  useLateClarificationMessage: () => ({ send: vi.fn(), state: "idle" }),
}));

import { QuickChatContent } from "./quick-chat-content";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("QuickChatContent transformOutgoing", () => {
  it("threads transformOutgoing into useSubmitHandler's options", () => {
    const transformOutgoing = (message: string) => `About t-1: ${message}`;

    render(<QuickChatContent sessionId="session-1" transformOutgoing={transformOutgoing} />);

    expect(useSubmitHandlerSpy).toHaveBeenCalledWith(panelState, undefined, {
      transformOutgoing,
    });
  });

  it("passes no transform by default, leaving the submit path unchanged", () => {
    render(<QuickChatContent sessionId="session-1" />);

    expect(useSubmitHandlerSpy).toHaveBeenCalledWith(panelState, undefined, {
      transformOutgoing: undefined,
    });
  });
});

describe("QuickChatContent initialDraft", () => {
  it("passes initialDraft, initialPrompt and the composer ref to useQuickChatInitialDraft", () => {
    render(<QuickChatContent sessionId="session-1" initialDraft="why is KAN-418 here?" />);

    expect(useQuickChatInitialDraftSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        draft: "why is KAN-418 here?",
        initialPrompt: undefined,
        chatInputRef: expect.objectContaining({ current: null }),
      }),
    );
  });
});
