import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { deriveSessionInputMode } from "./domains/session/session-input-mode";
import {
  useLateClarificationMessage,
  type LateClarificationSnapshot,
} from "./use-late-clarification-message";

const admissionMock = vi.hoisted(() => vi.fn());
const fetchTaskSessionMock = vi.hoisted(() => vi.fn());
const listTaskSessionMessagesMock = vi.hoisted(() => vi.fn());
const storeRef = vi.hoisted(() => ({ current: null as ReturnType<typeof createAppStore> | null }));
const TASK_ID = toTaskId("task-late-refresh");
const SESSION_ID = toSessionId("session-late-refresh");
const TIMESTAMP = "2026-09-19T00:00:00Z";

vi.mock("react-i18next", async (importOriginal) => ({
  ...(await importOriginal<typeof import("react-i18next")>()),
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => storeRef.current,
}));

vi.mock("@/hooks/use-message-handler", () => ({
  useMessageHandler: () => ({ handleSendMessageWithOutcome: admissionMock }),
}));

vi.mock("@/lib/api/domains/session-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/session-api")>()),
  fetchTaskSession: fetchTaskSessionMock,
  listTaskSessionMessages: listTaskSessionMessagesMock,
}));

function sourceSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: SESSION_ID,
    task_id: TASK_ID,
    state: "RUNNING",
    foreground_activity: "background",
    supports_steering: false,
    started_at: TIMESTAMP,
    updated_at: TIMESTAMP,
    ...overrides,
  } as TaskSession;
}

function sourceSnapshot(pendingId: string): LateClarificationSnapshot {
  const message: Message = {
    id: `message-${pendingId}`,
    task_id: TASK_ID,
    session_id: SESSION_ID,
    author_type: "agent",
    type: "clarification_request",
    content: "Choose one",
    created_at: TIMESTAMP,
    metadata: {
      pending_id: pendingId,
      question_id: "question-1",
      status: "expired",
      question: { id: "question-1", prompt: "Choose one", options: [] },
    },
  };
  storeRef.current!.getState().mergeMessages(SESSION_ID, [message]);
  return {
    messages: [message],
    answers: [{ question_id: "question-1", custom_text: "Keep this answer" }],
  };
}

beforeEach(() => {
  storeRef.current = createAppStore();
  fetchTaskSessionMock.mockReset();
  listTaskSessionMessagesMock.mockReset();
  admissionMock.mockReset().mockImplementation(async () => {
    const session = storeRef.current!.getState().taskSessions.items[SESSION_ID];
    const mode = deriveSessionInputMode(session);
    if (mode === "unavailable") throw new Error("Session unavailable");
    return mode === "queue" ? "queued" : "sent";
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("late clarification refresh with live session updates", () => {
  it.each([
    { name: "newer state", state: "IDLE" as const, liveUpdatedAt: "2026-09-19T00:00:01Z" },
    { name: "same-timestamp activity", state: "RUNNING" as const, liveUpdatedAt: TIMESTAMP },
  ])(
    "preserves a $name event that arrives during the refresh",
    async ({ name, state, liveUpdatedAt }) => {
      const store = storeRef.current!;
      const fetched = sourceSession({ state });
      store.getState().setTaskSession(fetched);
      const snapshot = sourceSnapshot(`pending-${name}`);
      let finishRefresh!: (response: { session: TaskSession }) => void;
      fetchTaskSessionMock.mockReturnValue(
        new Promise<{ session: TaskSession }>((resolve) => {
          finishRefresh = resolve;
        }),
      );
      const hook = renderHook(() => useLateClarificationMessage(snapshot.messages[0]));
      let send!: Promise<"sent" | "queued">;
      await act(async () => {
        send = hook.result.current.send(snapshot);
      });
      expect(fetchTaskSessionMock).toHaveBeenCalledTimes(1);
      expect(admissionMock).not.toHaveBeenCalled();

      await act(async () => {
        store.getState().upsertTaskSessionFromEvent(
          TASK_ID,
          sourceSession({
            foreground_activity: "generating",
            updated_at: liveUpdatedAt,
          }),
        );
        finishRefresh({ session: fetched });
        await expect(send).resolves.toBe("queued");
      });

      expect(store.getState().taskSessions.items[SESSION_ID]).toMatchObject({
        state: "RUNNING",
        foreground_activity: "generating",
        updated_at: liveUpdatedAt,
      });
      expect(listTaskSessionMessagesMock).not.toHaveBeenCalled();
      expect(hook.result.current.state.status).toBe("queued");
    },
  );

  it("accepts fresh activity when the live event preceded the refresh", async () => {
    const store = storeRef.current!;
    store
      .getState()
      .upsertTaskSessionFromEvent(TASK_ID, sourceSession({ foreground_activity: "generating" }));
    const snapshot = sourceSnapshot("pending-before-refresh");
    fetchTaskSessionMock.mockResolvedValue({ session: sourceSession() });
    const hook = renderHook(() => useLateClarificationMessage(snapshot.messages[0]));

    await act(async () => {
      await expect(hook.result.current.send(snapshot)).resolves.toBe("sent");
    });

    expect(store.getState().taskSessions.items[SESSION_ID].foreground_activity).toBe("background");
    expect(hook.result.current.state.status).toBe("sent");
  });

  it("retains the answer after a refresh error and permits retry", async () => {
    storeRef.current!.getState().setTaskSession(sourceSession());
    const snapshot = sourceSnapshot("pending-refresh-error");
    const error = new Error("Refresh unavailable");
    fetchTaskSessionMock
      .mockRejectedValueOnce(error)
      .mockResolvedValueOnce({ session: sourceSession() });
    const hook = renderHook(() => useLateClarificationMessage(snapshot.messages[0]));

    await act(async () => {
      await expect(hook.result.current.send(snapshot)).rejects.toThrow(error);
    });

    expect(admissionMock).not.toHaveBeenCalled();
    expect(hook.result.current.state).toMatchObject({ status: "error", error, snapshot });

    await act(async () => {
      await expect(hook.result.current.send(snapshot)).resolves.toBe("sent");
    });

    expect(fetchTaskSessionMock).toHaveBeenCalledTimes(2);
    expect(admissionMock).toHaveBeenCalledTimes(1);
    expect(hook.result.current.state.status).toBe("sent");
  });
});
