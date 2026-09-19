import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { useLateClarificationMessage } from "./use-late-clarification-message";

const admissionMock = vi.hoisted(() => vi.fn());
const fetchTaskSessionMock = vi.hoisted(() => vi.fn());
const listTaskSessionMessagesMock = vi.hoisted(() => vi.fn());
const storeRef = vi.hoisted(() => ({ current: null as TestStore | null }));
const TASK_ID = "task-late-answer";
const SESSION_ID = "session-late-answer";
const QUESTION_ID = "question-1";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => storeRef.current }),
}));

vi.mock("@/hooks/use-message-handler", () => ({
  useMessageHandler: () => ({ handleSendMessageWithOutcome: admissionMock }),
}));

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchTaskSession: fetchTaskSessionMock,
  listTaskSessionMessages: listTaskSessionMessagesMock,
}));

type TestStore = {
  taskSessions: { items: Record<string, TaskSession> };
  messages: { bySession: Record<string, Message[]> };
  setTaskSession: (session: TaskSession) => void;
  mergeMessages: (sessionId: string, messages: Message[]) => void;
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function sourceMessage(pendingId: string): Message {
  return {
    id: `message-${pendingId}`,
    task_id: toTaskId(TASK_ID),
    session_id: toSessionId(SESSION_ID),
    author_type: "agent",
    type: "clarification_request",
    content: "Choose one",
    created_at: "2026-09-19T00:00:00Z",
    metadata: {
      pending_id: pendingId,
      question_id: QUESTION_ID,
      question_index: 0,
      question_total: 1,
      status: "expired",
      question: {
        id: QUESTION_ID,
        prompt: "Choose one",
        options: [{ option_id: "option-1", label: "Option one" }],
      },
    },
  } as unknown as Message;
}

function sourceSession(): TaskSession {
  return {
    id: toSessionId(SESSION_ID),
    task_id: toTaskId(TASK_ID),
    state: "IDLE",
    started_at: "2026-09-19T00:00:00Z",
    updated_at: "2026-09-19T00:00:00Z",
  } as TaskSession;
}

function resetStore() {
  const next: TestStore = {
    taskSessions: { items: {} },
    messages: { bySession: {} },
    setTaskSession: (session) => {
      next.taskSessions.items[session.id] = session;
    },
    mergeMessages: (sessionId, messages) => {
      next.messages.bySession[sessionId] = messages;
    },
  };
  storeRef.current = next;
}

beforeEach(() => {
  resetStore();
  admissionMock.mockReset();
  fetchTaskSessionMock.mockReset();
  listTaskSessionMessagesMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useLateClarificationMessage", () => {
  it("retains a failed draft and client identity across unmount and remount", async () => {
    const source = sourceMessage("pending-remount");
    const state = storeRef.current!;
    state.taskSessions.items[source.session_id] = sourceSession();
    state.messages.bySession[source.session_id] = [source];
    const firstDelivery = deferred<"sent">();
    admissionMock.mockReturnValueOnce(firstDelivery.promise).mockResolvedValueOnce("queued");
    const snapshot = {
      messages: [source],
      answers: [{ question_id: QUESTION_ID, selected_options: ["option-1"] }],
    } as const;

    const first = renderHook(() =>
      // eslint-disable-next-line react-hooks/rules-of-hooks -- renderHook callback is the hook under test.
      useLateClarificationMessage(source),
    );
    let firstSend!: Promise<"sent" | "queued">;
    await act(async () => {
      firstSend = first.result.current.send(snapshot);
      await waitFor(() => expect(admissionMock).toHaveBeenCalledTimes(1));
    });
    const firstClientMessageId = admissionMock.mock.calls[0][0].clientMessageId;
    first.unmount();
    firstDelivery.reject(new Error("transport unavailable"));
    await expect(firstSend).rejects.toThrow("transport unavailable");

    const remounted = renderHook(() =>
      // eslint-disable-next-line react-hooks/rules-of-hooks -- renderHook callback is the hook under test.
      useLateClarificationMessage(source),
    );
    expect(remounted.result.current.state).toMatchObject({
      status: "error",
      snapshot,
    });

    await act(async () => {
      await remounted.result.current.send(snapshot);
    });
    expect(admissionMock.mock.calls[1][0].clientMessageId).toBe(firstClientMessageId);
    expect(remounted.result.current.state.status).toBe("queued");
  });

  it("shares a delayed failure from the active form with the transcript form", async () => {
    const source = sourceMessage("pending-shared");
    const state = storeRef.current!;
    state.taskSessions.items[source.session_id] = sourceSession();
    state.messages.bySession[source.session_id] = [source];
    const delivery = deferred<"sent">();
    admissionMock.mockReturnValueOnce(delivery.promise);
    const snapshot = {
      messages: [source],
      answers: [{ question_id: QUESTION_ID, custom_text: "Keep this answer" }],
    } as const;

    const active = renderHook(() => useLateClarificationMessage(source));
    const transcript = renderHook(() => useLateClarificationMessage(source));
    let send!: Promise<"sent" | "queued">;
    await act(async () => {
      send = active.result.current.send(snapshot);
      await waitFor(() => expect(transcript.result.current.state.status).toBe("sending"));
    });
    delivery.reject(new Error("late delivery failed"));
    await expect(send).rejects.toThrow("late delivery failed");
    await waitFor(() => expect(transcript.result.current.state.status).toBe("error"));
    expect(transcript.result.current.state.snapshot?.answers).toEqual(snapshot.answers);
  });

  it("hydrates a source session and transcript before Inbox admission", async () => {
    const source = sourceMessage("pending-inbox");
    const state = storeRef.current!;
    const session = sourceSession();
    fetchTaskSessionMock.mockResolvedValue({ session });
    listTaskSessionMessagesMock.mockResolvedValue({
      messages: [source],
      has_more: false,
      cursor: "",
    });
    admissionMock.mockResolvedValue("sent");
    const snapshot = {
      messages: [source],
      answers: [{ question_id: QUESTION_ID, selected_options: ["option-1"] }],
    } as const;

    const hook = renderHook(() => useLateClarificationMessage(source));
    await act(async () => {
      await hook.result.current.send(snapshot);
    });

    expect(fetchTaskSessionMock).toHaveBeenCalledWith(source.session_id, { cache: "no-store" });
    expect(listTaskSessionMessagesMock).toHaveBeenCalledWith(
      source.session_id,
      { limit: 100, sort: "asc" },
      { cache: "no-store" },
    );
    expect(state.taskSessions.items[source.session_id]).toEqual(session);
    expect(state.messages.bySession[source.session_id]).toEqual([source]);
    expect(admissionMock).toHaveBeenCalledWith(
      expect.objectContaining({ clientMessageId: expect.any(String) }),
    );
    expect(hook.result.current.state.status).toBe("sent");
  });
});
