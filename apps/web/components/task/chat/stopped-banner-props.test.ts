import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { buildStoppedBannerProps } from "./chat-input-container";
import { useComposerProps } from "./use-composer-props";

function composerArgs(errorMessage: string) {
  return {
    panelState: {
      resolvedSessionId: "session-1",
      taskId: "task-1",
      session: { error_message: errorMessage },
      task: null,
      taskDescription: "",
      planModeEnabled: false,
      planModeAvailable: true,
      mcpServers: [],
      mcpAttachmentHistory: [],
      handlePlanModeChange: vi.fn(),
      isAgentBusy: false,
      supportsSteering: false,
      isStarting: false,
      isPreparingEnvironment: false,
      pendingClarification: null,
      pendingCommentsByFile: {},
      planComments: [],
      pendingPRFeedback: [],
      walkthroughComments: [],
      messageComments: [],
      chatSubmitKey: "enter",
      agentCommands: [],
      isFailed: true,
      isCompleted: false,
      needsRecovery: false,
      contextItems: [],
      planContextEnabled: false,
      contextFiles: [],
      handleToggleContextFile: vi.fn(),
      handleAddContextFile: vi.fn(),
    } as never,
    composerWorkspaceId: "workspace-1",
    isMoving: false,
    implementPlanHandler: undefined,
    executor: { unavailable: false },
    placeholder: "",
    handleSubmit: vi.fn(),
    handleCancelTurn: vi.fn(),
    isSending: false,
    showRequestChangesTooltip: false,
    onClarificationResolved: vi.fn(),
  };
}

describe("stopped-session error propagation", () => {
  it("threads the sanitized session error from panel state into composer props", () => {
    const errorMessage = "pods is forbidden: RBAC denied the request";
    const { result } = renderHook(() => useComposerProps(composerArgs(errorMessage)));

    expect(result.current.sessionErrorMessage).toBe(errorMessage);
  });

  it("uses the sanitized session error as recoverable banner copy", () => {
    const errorMessage = "pods is forbidden: RBAC denied the request";

    expect(
      buildStoppedBannerProps({
        executorUnavailable: false,
        executorUnavailableReason: undefined,
        sessionErrorMessage: errorMessage,
      }),
    ).toEqual({ message: errorMessage });
  });

  it("keeps the causal session error when its executor is also unavailable", () => {
    const errorMessage = "pod scheduling failed: 0/1 nodes matched";

    expect(
      buildStoppedBannerProps({
        executorUnavailable: true,
        executorUnavailableReason: "failed",
        sessionErrorMessage: errorMessage,
      }),
    ).toEqual({
      message: errorMessage,
      detail: "failed",
      resumeLabel: "Restart",
      resumingLabel: "Restarting...",
    });
  });
});
