import { createRef } from "react";
import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useChatInputContainer } from "./use-chat-input-container";
import type { ChatInputContainerHandle } from "./chat-input-container";

function renderInputState(overrides: Partial<Parameters<typeof useChatInputContainer>[0]> = {}) {
  return renderHook(() =>
    useChatInputContainer({
      ref: createRef<ChatInputContainerHandle>(),
      sessionId: "session-1",
      isSending: false,
      isStarting: false,
      isPreparingEnvironment: false,
      isMoving: false,
      isFailed: false,
      needsRecovery: false,
      executorUnavailable: false,
      isAgentBusy: false,
      hasAgentCommands: true,
      placeholder: undefined,
      contextItems: [],
      pendingClarification: null,
      onClarificationResolved: undefined,
      pendingCommentsByFile: undefined,
      hasContextComments: false,
      showRequestChangesTooltip: false,
      onRequestChangesTooltipDismiss: undefined,
      onSubmit: vi.fn(),
      ...overrides,
    }),
  );
}

describe("useChatInputContainer", () => {
  it("disables the editor while the session is still STARTING", () => {
    // The editor must stay uneditable until the agent reaches RUNNING — if
    // the user can press Cmd+Enter mid-startup, the backend rejects with
    // "Failed to send message to agent" because the agent process isn't
    // ready yet. This is the regression from earlier rounds where the e2e
    // quick-chat suite kept failing on race conditions.
    const { result } = renderInputState({ isStarting: true });

    expect(result.current.isDisabled).toBe(true);
    expect(result.current.submitDisabled).toBe(true);
    expect(result.current.submitDisabledReason).toBeUndefined();
  });

  it("surfaces the setup tooltip only while a container/sandbox is preparing", () => {
    const { result } = renderInputState({
      isStarting: true,
      isPreparingEnvironment: true,
    });

    expect(result.current.submitDisabledReason).toBe("The agent is still being set up.");
  });
});
