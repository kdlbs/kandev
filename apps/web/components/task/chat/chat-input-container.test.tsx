import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";
import { ChatInputContainer } from "./chat-input-container";

const SESSION_ID = "session-1";

const { failureState, bodyProps, containerState, composerRecovery, testIds } = vi.hoisted(() => ({
  failureState: { episode: null as ExecutorFailureEpisode | null },
  testIds: {
    stoppedBanner: "session-stopped-banner",
    recoveryCard: "session-recovery-card",
    chatInputBody: "chat-input-body",
  },
  bodyProps: { current: null as Record<string, unknown> | null },
  composerRecovery: { current: null as { sessionId: string; model: { sessionId: string } } | null },
  containerState: {
    showNewSessionDialog: false,
    setShowNewSessionDialog: vi.fn(),
    contextPopoverOpen: false,
    setContextPopoverOpen: vi.fn(),
    height: "auto",
    containerRef: { current: null },
    resizeHandleProps: { onMouseDown: vi.fn(), onDoubleClick: vi.fn() },
    value: "",
    inputRef: { current: null },
    addFiles: vi.fn().mockResolvedValue(undefined),
    fileInputRef: { current: null },
    handleChange: vi.fn(),
    handleSubmitWithReset: vi.fn(),
    allItems: [],
    hasPendingAttachmentUploads: false,
    isInputFocused: false,
    setIsInputFocused: vi.fn(),
    hasClarification: false,
    hasPendingComments: false,
    hasContextZone: false,
    showFocusHint: false,
    inputPlaceholder: "Ask to make changes",
    isDisabled: false,
    submitDisabled: false,
    submitDisabledReason: undefined,
  },
}));

vi.mock("../task-launch-error-context", () => ({
  useTaskLaunchErrorContext: () => ({
    taskId: "task-1",
    workspaceId: "workspace",
    statusSummary: { executor_failure: failureState.episode },
  }),
}));

vi.mock("./use-chat-input-container", () => ({
  useChatInputContainer: () => containerState,
}));

vi.mock("./session-stopped-banner", () => ({
  SessionStoppedBanner: () => <div data-testid={testIds.stoppedBanner} />,
}));

vi.mock("./session-recovery-context", () => ({
  useSessionComposerRecovery: () => composerRecovery.current,
}));

vi.mock("./session-recovery-card", () => ({
  SessionRecoveryCard: () => <div data-testid={testIds.recoveryCard} />,
}));

vi.mock("@/components/task/new-session-dialog", () => ({
  NewSessionDialog: () => null,
}));

vi.mock("./chat-input-body", () => ({
  ChatInputBody: (props: Record<string, unknown>) => {
    bodyProps.current = props;
    return <div data-testid={testIds.chatInputBody} />;
  },
}));

vi.mock("@/hooks/domains/session/use-session-recovery-actions", () => ({
  useSessionRecoveryActions: () => ({}),
}));

vi.mock("@/hooks/use-is-utility-configured", () => ({
  useIsUtilityConfigured: () => false,
}));

vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({
    enhancePrompt: vi.fn(),
    isEnhancingPrompt: false,
  }),
}));

vi.mock("@/hooks/use-prompt-result-delivery", () => ({
  usePromptResultDelivery: () => ({
    pendingResult: null,
    captureScope: vi.fn(),
    deliver: vi.fn(),
    applyPending: vi.fn(),
    copyPending: vi.fn(),
  }),
}));

vi.mock("@/lib/i18n", async (original) => ({
  ...(await original<typeof import("@/lib/i18n")>()),
  t: (key: string) => key,
}));

const baseProps = {
  onSubmit: vi.fn(),
  sessionId: SESSION_ID,
  taskId: "task-1",
  taskDescription: "",
  planModeEnabled: false,
  onPlanModeChange: vi.fn(),
  isAgentBusy: false,
  isWorking: false,
  isStarting: false,
  isSending: false,
  onCancel: vi.fn(),
};

afterEach(() => {
  cleanup();
  bodyProps.current = null;
  failureState.episode = null;
  composerRecovery.current = null;
});

describe("ChatInputContainer launch-error ownership", () => {
  it("hides the editor when the task launch card owns a failed session", () => {
    render(<ChatInputContainer {...baseProps} isFailed launchErrorOwned />);

    expect(screen.queryByTestId(testIds.chatInputBody)).toBeNull();
    expect(screen.queryByTestId(testIds.stoppedBanner)).toBeNull();
  });

  it("renders the stopped banner when the failed session has no launch-card owner", () => {
    render(<ChatInputContainer {...baseProps} isFailed />);

    expect(screen.queryByTestId(testIds.chatInputBody)).toBeNull();
    expect(screen.getByTestId(testIds.stoppedBanner)).toBeTruthy();
  });

  it("uses the uncertain-delivery controls instead of generic recovery choices", () => {
    composerRecovery.current = { sessionId: SESSION_ID, model: { sessionId: SESSION_ID } };

    render(<ChatInputContainer {...baseProps} uncertainDelivery />);

    expect(screen.getByTestId(testIds.stoppedBanner)).toBeTruthy();
    expect(screen.queryByTestId(testIds.recoveryCard)).toBeNull();
  });

  it("keeps the editor visible when the owned launch error is not a failed session", () => {
    render(<ChatInputContainer {...baseProps} launchErrorOwned />);

    expect(screen.getByTestId(testIds.chatInputBody)).toBeTruthy();
    expect(screen.queryByTestId(testIds.stoppedBanner)).toBeNull();
  });
});

describe("ChatInputContainer cancellation availability", () => {
  it("shows cancel for a working direct-input session", () => {
    render(<ChatInputContainer {...baseProps} isWorking />);

    const editorAreaProps = bodyProps.current?.editorAreaProps as
      | { canCancelAgent?: boolean; isAgentBusy?: boolean }
      | undefined;
    expect(editorAreaProps).toEqual(
      expect.objectContaining({ canCancelAgent: true, isAgentBusy: false }),
    );
  });

  it("can hide cancel when the surface callback only dismisses the composer", () => {
    render(<ChatInputContainer {...baseProps} isWorking showCancelAgent={false} />);

    const editorAreaProps = bodyProps.current?.editorAreaProps as
      | { canCancelAgent?: boolean }
      | undefined;
    expect(editorAreaProps?.canCancelAgent).toBe(false);
  });
});

it("places a shared executor failure at the composer instead of hiding it", () => {
  failureState.episode = {
    id: "episode",
    task_id: "task-1",
    revision: 1,
    state: "active",
    first_observed_at: "2026-10-07T00:00:00Z",
    last_observed_at: "2026-10-07T00:00:00Z",
    observation: {
      outcome: "missing",
      runtime: "k8s",
      observed_at: "2026-10-07T00:00:00Z",
      container_ready: false,
      workspace: "unknown",
    },
  };
  render(<ChatInputContainer {...baseProps} />);
  expect(screen.getByTestId("session-executor-failure-card").textContent).toContain(
    "Executor no longer available",
  );
  expect(screen.queryByTestId(testIds.chatInputBody)).toBeNull();
});
