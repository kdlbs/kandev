import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useComposerProps } from "./use-composer-props";
import type { ChatPanelState } from "./use-chat-panel-state";

function composerArgs(): Parameters<typeof useComposerProps>[0] {
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
    } as unknown as ChatPanelState,
    composerWorkspaceId: null,
    workspaceResolutionFailed: false,
    onRetryWorkspaceResolution: vi.fn(),
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

  it("uses the persisted delivery recovery record before the legacy error breadcrumb", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          phase: "reconnecting",
          revision: 1,
          session_id: "session-1",
          agent_execution_id: "exec-1",
          submission_id: "submission-1",
          stream_id: "stream-1",
          incarnation_id: "inc-1",
          harness_generation: 2,
          prompt_generation: 4,
        },
      },
    } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(true);
    expect(result.current.deliveryRecoveryPhase).toBe("reconnecting");
  });

  it("does not let a settled delivery tombstone revive a stale uncertain breadcrumb", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          phase: "settled",
          revision: 3,
          session_id: "session-1",
          agent_execution_id: "exec-1",
          submission_id: "submission-1",
          stream_id: "stream-1",
          incarnation_id: "inc-1",
          harness_generation: 2,
          prompt_generation: 4,
        },
      },
    } as never;
    args.panelState.lastAgentError = { code: "DURABLE_DELIVERY_UNCERTAIN" } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(false);
    expect(result.current.deliveryRecoveryPhase).toBeUndefined();
  });

  it("does not present a recovered live turn as uncertain", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          phase: "recovered",
          revision: 2,
          session_id: "session-1",
          agent_execution_id: "exec-1",
          submission_id: "submission-1",
          stream_id: "stream-1",
          incarnation_id: "inc-1",
          harness_generation: 2,
          prompt_generation: 4,
        },
      },
    } as never;
    args.panelState.lastAgentError = { code: "DURABLE_DELIVERY_UNCERTAIN" } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(false);
    expect(result.current.deliveryRecoveryPhase).toBeUndefined();
  });
});
