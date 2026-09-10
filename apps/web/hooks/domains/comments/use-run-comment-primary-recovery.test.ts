import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { PlanComment } from "@/lib/state/slices/comments";
import { WebSocketRequestError } from "@/lib/ws/request-error";

const TASK_ID = "task-1";
const SELECTED_SESSION_ID = "sess-1";
const PRIMARY_SESSION_ID = "primary-session";
const NEW_PRIMARY_ID = "new-primary";
const WAITING_STATE = "WAITING_FOR_INPUT";
const PRIMARY_CHANGED_CODE = "primary_session_changed";
const PRIMARY_CHANGED_RUN_CODE = "primary-session-changed";

const mockQueueMessage = vi.fn();
const mockSendMessageRequest = vi.fn();
const mockMarkCommentsSent = vi.fn();
const mockAddMessage = vi.fn();
const mockSetTaskPlanComments = vi.fn();
const mockGetTaskPlanComments = vi.fn();
const mockListTaskSessions = vi.fn();
let mockStoreState: ReturnType<typeof makeStoreState>;

const mockSetTaskSession = vi.fn((next: MockSession) => {
  mockStoreState.taskSessions.items[next.id] = next;
  const sessions = mockStoreState.taskSessionsByTask.itemsByTaskId[next.task_id];
  const index = sessions.findIndex((candidate) => candidate.id === next.id);
  if (index >= 0) sessions[index] = next;
});

const mockSetTaskSessionsForTask = vi.fn((taskId: string, sessions: MockSession[]) => {
  mockStoreState.taskSessionsByTask.itemsByTaskId[taskId] = sessions;
  for (const session of sessions) mockStoreState.taskSessions.items[session.id] = session;
});

const mockSetState = vi.fn(
  (
    updater: (
      state: ReturnType<typeof makeStoreState>,
    ) => Partial<ReturnType<typeof makeStoreState>>,
  ) => {
    mockStoreState = { ...mockStoreState, ...updater(mockStoreState) };
  },
);

vi.mock("@/lib/api/domains/queue-api", () => ({
  appendToQueue: vi.fn(),
  queueMessage: (...args: unknown[]) => mockQueueMessage(...args),
}));

vi.mock("@/lib/api/domains/plan-comment-api", () => ({
  getTaskPlanComments: (...args: unknown[]) => mockGetTaskPlanComments(...args),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  listTaskSessions: (...args: unknown[]) => mockListTaskSessions(...args),
}));

vi.mock("@/hooks/use-message-handler", () => ({
  sendMessageRequest: (...args: unknown[]) => mockSendMessageRequest(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => mockStoreState, setState: mockSetState }),
}));

vi.mock("@/lib/state/slices/comments", () => ({
  useCommentsStore: (
    selector: (state: { markCommentsSent: typeof mockMarkCommentsSent }) => unknown,
  ) => selector({ markCommentsSent: mockMarkCommentsSent }),
}));

import { useRunComment } from "./use-run-comment";

type MockSession = {
  id: string;
  task_id: string;
  state: string;
  is_primary: boolean;
  queue_incarnation_id: string;
};

function makeSession(
  id: string,
  state: string,
  isPrimary: boolean,
  queueIncarnationId: string,
): MockSession {
  return {
    id,
    task_id: TASK_ID,
    state,
    is_primary: isPrimary,
    queue_incarnation_id: queueIncarnationId,
  };
}

function makeStoreState() {
  const selected = makeSession(SELECTED_SESSION_ID, WAITING_STATE, false, "inc-selected");
  const primary = makeSession(PRIMARY_SESSION_ID, WAITING_STATE, true, "inc-primary");
  return {
    kanban: { tasks: [{ id: TASK_ID, primarySessionId: PRIMARY_SESSION_ID as string | null }] },
    kanbanMulti: { snapshots: {} },
    taskSessions: {
      items: { [SELECTED_SESSION_ID]: selected, [PRIMARY_SESSION_ID]: primary } as Record<
        string,
        MockSession
      >,
      activityEpochBySession: {},
    },
    taskSessionsByTask: {
      itemsByTaskId: { [TASK_ID]: [primary, selected] } as Record<string, MockSession[]>,
    },
    taskPlans: { commentsMigrationStatusByTaskId: { [TASK_ID]: "complete" } },
    queue: { metaBySessionId: {} },
    chatInput: { planModeBySessionId: {} },
    addMessage: mockAddMessage,
    setTaskPlanComments: mockSetTaskPlanComments,
    setTaskSession: mockSetTaskSession,
    setTaskSessionsForTask: mockSetTaskSessionsForTask,
  };
}

function makePlanComment(): PlanComment {
  return {
    id: "comment-1",
    source: "plan",
    sessionId: "",
    taskId: TASK_ID,
    planId: "plan-1",
    version: 2,
    text: "split step 2",
    selectedText: "step 2",
    createdAt: new Date().toISOString(),
    status: "pending",
  };
}

function primaryChangedError(primarySessionId: string | null, state: string | null) {
  return new WebSocketRequestError("Primary session changed", PRIMARY_CHANGED_CODE, {
    primary_session_id: primarySessionId,
    primary_session_state: state,
  });
}

function renderCommentHook() {
  return renderHook(() => useRunComment({ sessionId: SELECTED_SESSION_ID, taskId: TASK_ID }));
}

function setup() {
  vi.clearAllMocks();
  mockStoreState = makeStoreState();
  mockListTaskSessions.mockResolvedValue({
    sessions: mockStoreState.taskSessionsByTask.itemsByTaskId[TASK_ID],
  });
  mockGetTaskPlanComments.mockResolvedValue({
    task_id: TASK_ID,
    plan_id: "plan-1",
    revision: 3,
    comments: [],
  });
}

describe("useRunComment primary-session recovery", () => {
  beforeEach(setup);

  it("applies refreshed primary details before retry", async () => {
    const replacement = makeSession(NEW_PRIMARY_ID, "STARTING", false, "inc-new-primary");
    mockStoreState.taskSessions.items[NEW_PRIMARY_ID] = replacement;
    mockStoreState.taskSessionsByTask.itemsByTaskId[TASK_ID].push(replacement);
    mockListTaskSessions.mockResolvedValueOnce({
      sessions: [
        { ...mockStoreState.taskSessions.items[PRIMARY_SESSION_ID], is_primary: false },
        { ...replacement, state: WAITING_STATE, is_primary: true },
        mockStoreState.taskSessions.items[SELECTED_SESSION_ID],
      ],
    });
    mockSendMessageRequest.mockRejectedValueOnce(
      primaryChangedError(NEW_PRIMARY_ID, WAITING_STATE),
    );
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: PRIMARY_CHANGED_RUN_CODE,
    });
    const firstAdmissionID = mockSendMessageRequest.mock.calls[0]?.[0]?.clientMessageId;
    await result.current.runComment(makePlanComment());

    expect(mockSendMessageRequest).toHaveBeenLastCalledWith(
      expect.objectContaining({
        resolvedSessionId: NEW_PRIMARY_ID,
        clientMessageId: firstAdmissionID,
      }),
    );
  });

  it("clears the primary projection when refresh finds none", async () => {
    mockListTaskSessions.mockResolvedValueOnce({
      sessions: [
        { ...mockStoreState.taskSessions.items[PRIMARY_SESSION_ID], is_primary: false },
        mockStoreState.taskSessions.items[SELECTED_SESSION_ID],
      ],
    });
    mockSendMessageRequest.mockRejectedValueOnce(primaryChangedError("", ""));
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: PRIMARY_CHANGED_RUN_CODE,
    });

    expect(mockStoreState.kanban.tasks[0].primarySessionId).toBeNull();
    expect(mockSetTaskSessionsForTask).toHaveBeenCalled();
  });

  it("clears cached primary flags when refresh fails with no primary", async () => {
    mockListTaskSessions.mockRejectedValueOnce(new Error("session refresh unavailable"));
    mockSendMessageRequest.mockRejectedValueOnce(primaryChangedError(null, null));
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: PRIMARY_CHANGED_RUN_CODE,
    });

    expect(mockStoreState.kanban.tasks[0].primarySessionId).toBeNull();
    expect(mockStoreState.taskSessions.items[PRIMARY_SESSION_ID].is_primary).toBe(false);
  });

  it("hydrates an uncached replacement primary before retry", async () => {
    const replacement = makeSession(NEW_PRIMARY_ID, WAITING_STATE, true, "inc-new-primary");
    mockListTaskSessions.mockResolvedValueOnce({
      sessions: [
        { ...mockStoreState.taskSessions.items[PRIMARY_SESSION_ID], is_primary: false },
        replacement,
      ],
    });
    mockSendMessageRequest.mockRejectedValueOnce(
      primaryChangedError(NEW_PRIMARY_ID, WAITING_STATE),
    );
    const { result } = renderCommentHook();

    await expect(result.current.runComment(makePlanComment())).rejects.toMatchObject({
      code: PRIMARY_CHANGED_RUN_CODE,
    });
    await act(async () => {
      await result.current.runComment(makePlanComment());
    });

    expect(mockSendMessageRequest).toHaveBeenLastCalledWith(
      expect.objectContaining({ resolvedSessionId: NEW_PRIMARY_ID }),
    );
    expect(mockStoreState.taskSessions.items[NEW_PRIMARY_ID]).toMatchObject(replacement);
  });
});
