import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";

const launchError = {
  stamp: "task-wide-launch-error",
  occurred_at: "2026-08-20T10:00:00Z",
  preview: "The workspace could not be prepared.",
  category: "workspace_checkout_failed",
};

const priorTranscriptMessage = {
  id: "prior-transcript",
  author_type: "agent",
  content: "The prior session stopped after its checkout failed.",
  type: "status",
  created_at: "2026-08-19T10:00:00Z",
} as unknown as Message;

const appStoreState = {
  userSettings: {
    showAnchoredPromptBar: false,
    showScrollToLastPrompt: false,
    showScrollToStart: false,
  },
  taskSessions: {
    items: {
      "prior-session": { name: null, agent_profile_id: null, last_read_message_id: null },
    },
  },
  agentProfiles: { items: [] },
  messages: { bySession: { "prior-session": [priorTranscriptMessage] } },
};

const panelState = {
  resolvedSessionId: "prior-session",
  session: { state: "FAILED", error_message: "The prior session stopped." },
  taskId: "task-1",
  isWorking: false,
  messagesLoading: false,
  historyRefreshPending: false,
  isInitialMessagesLoading: false,
  groupedItems: [],
  allMessages: [priorTranscriptMessage],
  footerActionMessages: [],
  permissionsByToolCallId: new Map(),
  childrenByParentToolCallId: new Map(),
  agentMessageCount: 0,
  pendingClarification: null,
  pendingClarificationGroup: null,
};

vi.mock("./panel-primitives", () => ({
  PanelRoot: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  PanelBody: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof appStoreState) => unknown) => selector(appStoreState),
  useAppStoreApi: () => ({
    getState: () => appStoreState,
  }),
}));

vi.mock("@/hooks/domains/settings/use-settings-data", () => ({
  useSettingsData: () => undefined,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

vi.mock("./task-archived-context", () => ({
  useIsTaskArchived: () => false,
}));

vi.mock("./task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({
    taskId: "task-1",
    workspaceId: "workspace-1",
    statusSummary: { active_error: launchError },
    repositories: [],
  }),
}));

vi.mock("@/hooks/domains/task/use-task-status-summary", () => ({
  useTaskStatusSummary: () => ({ active_error: launchError }),
}));

vi.mock("./chat/use-chat-panel-state", () => ({
  useChatPanelState: () => panelState,
}));

vi.mock("./chat/chat-input-area", () => ({
  ChatInputArea: ({
    launchErrorOwned,
    panelState: state,
  }: {
    launchErrorOwned?: boolean;
    panelState: typeof panelState;
  }) => (
    <div data-testid="chat-footer">
      {!launchErrorOwned && state.session?.state === "FAILED" && (
        <div data-testid="session-stopped-banner">prior session recovery</div>
      )}
    </div>
  ),
  useSubmitHandler: () => ({ isSending: false, handleSubmit: vi.fn() }),
  useChatPanelHandlers: () => ({ handleCancelTurn: vi.fn() }),
}));

vi.mock("@/components/task/chat/message-list", () => ({
  MessageList: ({
    messages,
    launchErrorOwned,
  }: {
    messages: Message[];
    launchErrorOwned?: boolean;
  }) => (
    <div data-testid="message-list">
      {!launchErrorOwned &&
        messages.map((message) => <div key={message.id}>{message.content}</div>)}
    </div>
  ),
}));

vi.mock("./simple/components/task-chat-launch-error", () => ({
  TaskChatLaunchError: ({
    statusSummary,
  }: {
    statusSummary?: { active_error?: typeof launchError };
  }) =>
    statusSummary?.active_error ? (
      <div data-testid="task-launch-error-entry">{statusSummary.active_error.preview}</div>
    ) : null,
}));

vi.mock("@/components/shared/task-markdown-file-link-provider", () => ({
  TaskMarkdownFileLinkProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./chat/clarification-panel-section", () => ({
  ClarificationPanelSection: () => null,
}));

vi.mock("./chat/use-composer-agent-start-hint", () => ({
  useComposerAgentStartHint: () => false,
}));

vi.mock("@/hooks/use-panel-search", () => ({
  usePanelSearch: () => undefined,
}));

vi.mock("@/hooks/domains/session/use-session-search", () => ({
  useSessionSearch: () => ({
    isOpen: false,
    query: "",
    hits: [],
    activeHitId: null,
    isSearching: false,
    setQuery: vi.fn(),
    setActiveHit: vi.fn(),
    open: vi.fn(),
    close: vi.fn(),
  }),
}));

vi.mock("@/hooks/use-lazy-load-messages", () => ({
  useLazyLoadMessages: () => ({ loadMoreRaw: vi.fn(), hasMore: false }),
}));

vi.mock("@/components/task/chat/use-drain-older-messages", () => ({
  useDrainOlderMessages: () => undefined,
}));

vi.mock("./chat/use-session-read-tracking", () => ({
  useSessionReadTracking: () => null,
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { scrollTarget: null }) => unknown) =>
    selector({ scrollTarget: null }),
  getState: () => ({ scrollTarget: null }),
}));

vi.mock("@/hooks/domains/session/load-message-window", () => ({
  loadMessageWindowAround: vi.fn(),
}));

vi.mock("@/lib/session-workspace-path", () => ({
  getSessionWorkspacePath: () => undefined,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { TaskChatPanel } from "./task-chat-panel";

afterEach(() => cleanup());

describe("TaskChatPanel launch-error ownership", () => {
  it("keeps a task-wide card and prior failed-session surfaces together", () => {
    render(<TaskChatPanel sessionId="prior-session" taskId="task-1" />);

    expect(screen.getByTestId("task-launch-error-entry").textContent).toContain(
      launchError.preview,
    );
    expect(screen.getByTestId("session-stopped-banner")).toBeTruthy();
    expect(screen.getByText(priorTranscriptMessage.content)).toBeTruthy();
  });
});
