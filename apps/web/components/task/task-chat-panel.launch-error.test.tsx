import type { ReactNode } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import {
  buildGroupedRenderItems,
  insertLastAgentErrorItem,
  type RenderItem,
} from "@/hooks/use-processed-messages";

const launchError = {
  stamp: "task-wide-launch-error",
  scope: "task" as const,
  occurred_at: "2026-08-20T10:00:00Z",
  preview: "The workspace could not be prepared.",
  category: "workspace_checkout_failed",
};
const SESSION_ID = "prior-session";

const messageListRecoveryRevealKeys = vi.hoisted(() => [] as Array<string | null | undefined>);

const priorTranscriptMessage = {
  id: "prior-transcript",
  author_type: "agent",
  content: "The prior session stopped after its checkout failed.",
  type: "status",
  created_at: "2026-08-19T10:00:00Z",
} as unknown as Message;

const appStoreState = {
  connection: { status: "connected" },
  kanbanMulti: { snapshots: {} },
  kanban: { workflowId: null, tasks: [], steps: [] },
  workflows: { items: [] },
  userSettings: {
    showAnchoredPromptBar: false,
    showScrollToLastPrompt: false,
    showScrollToStart: false,
  },
  workspaces: { activeId: null },
  taskSessions: {
    items: {
      [SESSION_ID]: { name: null, agent_profile_id: null, last_read_message_id: null },
    },
  },
  agentProfiles: { items: [] },
  messages: { bySession: { [SESSION_ID]: [priorTranscriptMessage] } },
  launchWarning: { bySessionId: {} },
};

const panelState = {
  resolvedSessionId: SESSION_ID,
  session: {
    state: "FAILED",
    error_message: "The prior session stopped.",
    metadata: null as Record<string, unknown> | null,
  },
  taskId: "task-1",
  isWorking: false,
  messagesLoading: false,
  historyRefreshPending: false,
  isInitialMessagesLoading: false,
  groupedItems: [] as RenderItem[],
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
  useOptionalAppStore: (selector: (state: typeof appStoreState) => unknown, fallback: unknown) =>
    selector(appStoreState) ?? fallback,
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
    items,
    messages,
    recoveryRevealKey,
  }: {
    items: RenderItem[];
    messages: Message[];
    recoveryRevealKey?: string | null;
  }) => {
    messageListRecoveryRevealKeys.push(recoveryRevealKey);
    return (
      <div data-testid="message-list">
        {messages.map((message) => {
          const metadata = message.metadata as Record<string, unknown> | undefined;
          return (
            <div
              key={message.id}
              data-testid={
                metadata?.recovery_actions === true ? "persisted-recovery-message" : undefined
              }
            >
              {message.content}
            </div>
          );
        })}
        {items.map((item) =>
          item.type === "agent_error_notice" ? (
            <div key={item.id} data-testid="agent-error-notice">
              provisional recovery notice
            </div>
          ) : null,
        )}
      </div>
    );
  },
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

afterEach(() => {
  cleanup();
  messageListRecoveryRevealKeys.length = 0;
});

describe("TaskChatPanel launch-error ownership", () => {
  it("leaves the task-wide card to the task shell and preserves session history", () => {
    render(<TaskChatPanel sessionId={SESSION_ID} taskId="task-1" />);

    expect(screen.queryByTestId("task-launch-error-entry")).toBeNull();
    expect(screen.queryByTestId("session-stopped-banner")).toBeNull();
    expect(screen.getByText(priorTranscriptMessage.content)).toBeTruthy();
    expect(messageListRecoveryRevealKeys.at(-1)).toBeUndefined();
  });

  it("does not reveal a persisted session failure through scroll coupling", () => {
    const previousMetadata = panelState.session.metadata;
    panelState.session.metadata = {
      last_agent_error: {
        message: "The selected session could not start.",
        phase: "bootstrap",
        occurred_at: "2026-08-20T11:00:00Z",
        stamp: "persisted-bootstrap-error",
      },
    };

    try {
      render(<TaskChatPanel sessionId={SESSION_ID} taskId="task-1" />);

      expect(messageListRecoveryRevealKeys.at(-1)).toBeUndefined();
    } finally {
      panelState.session.metadata = previousMetadata;
    }
  });

  it("counts one recovery representation across persisted messages and provisional notices", () => {
    const persistedRecoveryMessage = {
      ...priorTranscriptMessage,
      id: "persisted-recovery",
      session_id: SESSION_ID as Message["session_id"],
      content: "Agent encountered an error: saved session failed.",
      metadata: { recovery_actions: true, error_stamp: "session-failure-1" },
      created_at: "2026-08-20T11:00:00Z",
    } as Message;
    const previousMessages = panelState.allMessages;
    const previousItems = panelState.groupedItems;
    panelState.allMessages = [priorTranscriptMessage, persistedRecoveryMessage];
    panelState.groupedItems = insertLastAgentErrorItem(
      buildGroupedRenderItems(panelState.allMessages, SESSION_ID, {
        canAnchorPrepareProgress: false,
      }),
      SESSION_ID,
      {
        message: "saved session failed",
        occurredAt: "2026-08-20T11:00:00Z",
        stamp: "session-failure-1",
      },
      panelState.allMessages,
    );

    try {
      render(<TaskChatPanel sessionId={SESSION_ID} taskId="task-1" />);

      const persisted = screen.queryAllByTestId("persisted-recovery-message");
      const provisional = screen.queryAllByTestId("agent-error-notice");
      expect([...persisted, ...provisional]).toHaveLength(1);
      expect(provisional).toHaveLength(0);
    } finally {
      panelState.allMessages = previousMessages;
      panelState.groupedItems = previousItems;
    }
  });
});
