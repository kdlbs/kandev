import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useComposerProps } from "./use-composer-props";
import type { ChatPanelState } from "./use-chat-panel-state";
import { sessionId, taskId } from "@/lib/types/http";

const recovery = vi.hoisted(() => ({ hasCard: false, requestLocal: false }));
vi.mock("./session-recovery-context", () => ({
  useSessionComposerRecovery: () => ({
    model: recovery.hasCard ? { kind: "generic", requestLocal: recovery.requestLocal } : null,
  }),
}));
afterEach(() => {
  recovery.hasCard = false;
  recovery.requestLocal = false;
});

vi.mock("./use-composer-prompt-suggestion", () => ({
  useComposerPromptSuggestion: (_panel: unknown, blocked: boolean) => ({
    promptSuggestion: blocked ? null : "suggested reply",
    onPromptSuggestionDismiss: () => {},
  }),
}));

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

const FIXTURE_TIME = "2026-10-15T10:00:00Z";

const deliveryIdentity = {
  session_id: "session-1",
  agent_execution_id: "exec-1",
  submission_id: "submission-1",
  stream_id: "stream-1",
  incarnation_id: "inc-1",
  prompt_generation: 4,
};

describe("prompt suggestion recovery", () => {
  it("hides suggestions during delivery recovery and restores them after recovery", () => {
    const args = composerArgs();
    const { result, rerender } = renderHook(() => useComposerProps(args));
    expect(result.current.promptSuggestion).toBe("suggested reply");
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          ...deliveryIdentity,
          phase: "reconnecting",
          revision: 1,
          harness_generation: 2,
        },
      },
    } as never;
    rerender();
    expect(result.current.uncertainDelivery).toBe(true);
    expect(result.current.promptSuggestion).toBeNull();
    args.panelState.session = null;
    rerender();
    expect(result.current.promptSuggestion).toBe("suggested reply");
  });

  it("hides suggestions when a session recovery block requires attention", () => {
    const args = composerArgs();
    args.panelState.session = {
      session_recovery_blocks: [{ id: "block-1", consumer_reference: "agent_delivery" }],
    } as never;
    const { result } = renderHook(() => useComposerProps(args));
    expect(result.current.needsRecovery).toBe(true);
    expect(result.current.promptSuggestion).toBeNull();
  });
});

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
          ...deliveryIdentity,
          phase: "reconnecting",
          revision: 1,
          harness_generation: 2,
        },
      },
    } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(true);
    expect(result.current.deliveryRecoveryPhase).toBe("reconnecting");
  });

  it("shows delivery recovery for an open block before a recovery record exists", () => {
    const args = composerArgs();
    args.panelState.session = {
      session_recovery_blocks: [
        {
          id: "block-1",
          incarnation_id: "inc-1",
          expected_generation: 2,
          reason: "unresolved_durable_work",
          consumer_reference: "agent_delivery",
          delivery_submission_id: "",
          delivery_stream_id: "",
          updated_at: "2026-10-10T10:00:00Z",
        },
      ],
    } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(true);
    expect(result.current.deliveryRecoveryPhase).toBe("uncertain");
  });

  it("does not let a settled delivery tombstone revive a stale uncertain breadcrumb", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          ...deliveryIdentity,
          phase: "settled",
          revision: 3,
          harness_generation: 2,
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
          ...deliveryIdentity,
          phase: "recovered",
          revision: 2,
          harness_generation: 2,
        },
      },
    } as never;
    args.panelState.lastAgentError = { code: "DURABLE_DELIVERY_UNCERTAIN" } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(false);
    expect(result.current.deliveryRecoveryPhase).toBeUndefined();
  });

  it("does not revive an uncertain recovery card after silent restoration", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          ...deliveryIdentity,
          phase: "restored",
          revision: 4,
          harness_generation: 2,
        },
      },
    } as never;
    args.panelState.lastAgentError = { code: "DURABLE_DELIVERY_UNCERTAIN" } as never;

    const { result } = renderHook(() => useComposerProps(args));

    expect(result.current.uncertainDelivery).toBe(false);
    expect(result.current.deliveryRecoveryPhase).toBeUndefined();
  });
});

describe("recovery card ownership", () => {
  it.each([undefined, "restored", "settled"])(
    "preserves the existing recovery card for a bound crash with %s metadata",
    (phase) => {
      recovery.hasCard = true;
      const args = composerArgs();
      args.panelState.session = {
        session_recovery_blocks: [
          {
            id: "block-1",
            reason: "unknown_prompt_outcome",
            consumer_reference: "agent_delivery",
            delivery_submission_id: "canonical-submission",
            delivery_stream_id: "stream-1",
          },
        ],
        metadata: phase
          ? {
              agent_delivery_recovery: {
                ...deliveryIdentity,
                phase,
                revision: 4,
                harness_generation: 2,
              },
            }
          : {},
      } as NonNullable<ChatPanelState["session"]>;
      const { result } = renderHook(() => useComposerProps(args));
      expect(result.current.uncertainDelivery).toBe(false);
      expect(result.current.needsRecovery).toBe(true);
    },
  );

  it("keeps active delivery recovery in control even with an existing error card", () => {
    recovery.hasCard = true;
    const args = composerArgs();
    args.panelState.session = {
      id: sessionId("session-1"),
      task_id: taskId("task-1"),
      state: "WAITING_FOR_INPUT",
      started_at: FIXTURE_TIME,
      updated_at: FIXTURE_TIME,
      metadata: {
        agent_delivery_recovery: {
          ...deliveryIdentity,
          phase: "uncertain",
          revision: 1,
          harness_generation: 2,
        },
      },
    };
    const { result } = renderHook(() => useComposerProps(args));
    expect(result.current.uncertainDelivery).toBe(true);
  });
});

describe("canonical interrupted prompt recovery", () => {
  it("keeps a new generic fence visible despite an earlier restored snapshot", () => {
    const args = composerArgs();
    args.panelState.session = {
      metadata: {
        agent_delivery_recovery: {
          ...deliveryIdentity,
          phase: "restored",
          revision: 4,
          harness_generation: 2,
        },
      },
      session_recovery_blocks: [
        {
          id: "new-block",
          reason: "unresolved_durable_work",
          consumer_reference: "agent_delivery",
          delivery_submission_id: "",
          delivery_stream_id: "",
        },
      ],
    } as never;
    const { result } = renderHook(() => useComposerProps(args));
    expect(result.current.uncertainDelivery).toBe(true);
    expect(result.current.needsRecovery).toBe(true);
  });

  it.each([undefined, "restored", "settled"])(
    "shows interrupted delivery recovery after cancel with %s historical metadata",
    (phase) => {
      const args = composerArgs();
      args.panelState.needsRecovery = false;
      args.panelState.session = {
        session_recovery_blocks: [
          {
            id: "block-1",
            reason: "unknown_prompt_outcome",
            consumer_reference: "agent_delivery",
            delivery_submission_id: "canonical-submission",
            delivery_stream_id: "",
          },
        ],
      } as NonNullable<ChatPanelState["session"]>;
      args.panelState.isWorking = false;
      args.panelState.session.metadata = phase
        ? {
            agent_delivery_recovery: {
              ...deliveryIdentity,
              phase,
              revision: 4,
              harness_generation: 2,
            },
          }
        : undefined;
      const { result, rerender } = renderHook(() => useComposerProps(args));
      expect(result.current.uncertainDelivery).toBe(true);
      expect(result.current.deliveryRecoveryPhase).toBe("uncertain");
      expect(result.current.needsRecovery).toBe(true);
      args.panelState.session.session_recovery_blocks = [];
      rerender();
      expect(result.current.uncertainDelivery).toBe(false);
      expect(result.current.needsRecovery).toBe(false);
    },
  );
});

it("keeps bound delivery controls when automatic launch reports a request-local failure", () => {
  const args = composerArgs();
  args.panelState.session = {
    id: sessionId("session-1"),
    task_id: taskId("task-1"),
    state: "WAITING_FOR_INPUT",
    started_at: FIXTURE_TIME,
    updated_at: FIXTURE_TIME,
    session_recovery_blocks: [
      {
        id: "block-1",
        reason: "unknown_prompt_outcome",
        consumer_reference: "agent_delivery",
        delivery_submission_id: "canonical-submission",
        incarnation_id: "inc-1",
        expected_generation: 1,
        updated_at: FIXTURE_TIME,
      },
    ],
  };
  const { result, rerender } = renderHook(() => useComposerProps(args));
  expect(result.current.uncertainDelivery).toBe(true);
  recovery.hasCard = true;
  recovery.requestLocal = true;
  rerender();
  expect(result.current.uncertainDelivery).toBe(true);
  expect(result.current.needsRecovery).toBe(true);
  args.panelState.session.session_recovery_blocks = [];
  rerender();
  expect(result.current.uncertainDelivery).toBe(false);
});
