import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useComposerProps } from "./use-composer-props";

function composerArgs() {
  return {
    panelState: {
      resolvedSessionId: "session-1",
      taskId: "task-1",
      task: null,
      taskDescription: "",
      planModeEnabled: false,
      planModeAvailable: true,
      mcpServers: [],
      mcpAttachmentHistory: [],
      handlePlanModeChange: vi.fn(),
      isAgentBusy: false,
      isWorking: true,
      supportsSteering: false,
      isStarting: false,
      isPreparingEnvironment: false,
      inputMode: "direct",
      isQueueReady: true,
      pendingClarification: null,
      pendingCommentsByFile: {},
      planComments: [],
      pendingPRFeedback: [],
      walkthroughComments: [],
      messageComments: [],
      chatSubmitKey: "enter",
      agentCommands: [],
      isFailed: false,
      isCompleted: false,
      needsRecovery: false,
      contextItems: [],
      planContextEnabled: false,
      contextFiles: [],
      handleToggleContextFile: vi.fn(),
      handleAddContextFile: vi.fn(),
    } as never,
    composerWorkspaceId: null,
    isMoving: false,
    implementPlanHandler: undefined,
    executor: { unavailable: false },
    placeholder: "",
    handleSubmit: vi.fn(),
    handleCancelTurn: vi.fn().mockResolvedValue(undefined),
    isSending: false,
    showRequestChangesTooltip: false,
    onClarificationResolved: vi.fn(),
  };
}

describe("useComposerProps", () => {
  it("forwards working state independently of queue admission", () => {
    const { result } = renderHook(() => useComposerProps(composerArgs()));

    expect((result.current as { isWorking?: boolean }).isWorking).toBe(true);
  });
});
