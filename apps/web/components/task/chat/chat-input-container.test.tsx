import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChatInputContainer } from "./chat-input-container";

const { containerState } = vi.hoisted(() => ({
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

vi.mock("./use-chat-input-container", () => ({
  useChatInputContainer: () => containerState,
}));

vi.mock("./session-stopped-banner", () => ({
  SessionStoppedBanner: () => <div data-testid="session-stopped-banner" />,
}));

vi.mock("./chat-input-body", () => ({
  ChatInputBody: () => <div data-testid="chat-input-body" />,
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

vi.mock("@/lib/i18n", () => ({
  t: (key: string) => key,
}));

const baseProps = {
  onSubmit: vi.fn(),
  sessionId: "session-1",
  taskId: "task-1",
  taskDescription: "",
  planModeEnabled: false,
  onPlanModeChange: vi.fn(),
  isAgentBusy: false,
  isStarting: false,
  isSending: false,
  onCancel: vi.fn(),
};

afterEach(() => cleanup());

describe("ChatInputContainer launch-error ownership", () => {
  it("hides the editor when the task launch card owns a failed session", () => {
    render(<ChatInputContainer {...baseProps} isFailed launchErrorOwned />);

    expect(screen.queryByTestId("chat-input-body")).toBeNull();
    expect(screen.queryByTestId("session-stopped-banner")).toBeNull();
  });

  it("renders the stopped banner when the failed session has no launch-card owner", () => {
    render(<ChatInputContainer {...baseProps} isFailed />);

    expect(screen.queryByTestId("chat-input-body")).toBeNull();
    expect(screen.getByTestId("session-stopped-banner")).toBeTruthy();
  });

  it("keeps the editor visible when the owned launch error is not a failed session", () => {
    render(<ChatInputContainer {...baseProps} launchErrorOwned />);

    expect(screen.getByTestId("chat-input-body")).toBeTruthy();
    expect(screen.queryByTestId("session-stopped-banner")).toBeNull();
  });
});
