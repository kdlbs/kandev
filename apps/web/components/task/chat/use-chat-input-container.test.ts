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
  it("keeps the editor enabled while setup is still blocking submit", () => {
    const { result } = renderInputState({ isStarting: true });

    expect(result.current.isDisabled).toBe(false);
  });
});
