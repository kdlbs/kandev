import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type TaskSession,
} from "@/lib/types/http";
import { useLateClarificationMessage } from "./use-late-clarification-message";

const admissionMock = vi.hoisted(() => vi.fn());
const markMock = vi.hoisted(() => vi.fn());
const bumpMock = vi.hoisted(() => vi.fn());
const storeRef = vi.hoisted(() => ({ current: null as Record<string, unknown> | null }));
const TASK_ID = "task-unanswered";
const ORIGINAL_SESSION = "session-original";
const PRIMARY_SESSION = "session-primary";

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
  fetchTaskSession: vi.fn(),
  listTaskSessionMessages: vi.fn(),
}));

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  markClarificationInboxLateAnswer: (...args: unknown[]) => markMock(...args),
}));

function sourceMessage(pendingId: string): Message {
  return {
    id: `message-${pendingId}`,
    task_id: toTaskId(TASK_ID),
    session_id: toSessionId(ORIGINAL_SESSION),
    author_type: "agent",
    type: "clarification_request",
    content: "Choose one",
    created_at: "2026-10-01T00:00:00Z",
    metadata: {
      pending_id: pendingId,
      question_id: "q-db",
      status: "pending",
      question: { id: "q-db", prompt: "Choose one", options: [{ option_id: "a", label: "A" }] },
    },
  } as unknown as Message;
}

function session(id: string): TaskSession {
  return { id: toSessionId(id), task_id: toTaskId(TASK_ID), state: "IDLE" } as TaskSession;
}

function resetStore(enabled: boolean) {
  storeRef.current = {
    taskSessions: {
      items: {
        [ORIGINAL_SESSION]: session(ORIGINAL_SESSION),
        [PRIMARY_SESSION]: session(PRIMARY_SESSION),
      },
    },
    messages: { bySession: { [ORIGINAL_SESSION]: [], [PRIMARY_SESSION]: [] } },
    setTaskSession: vi.fn(),
    mergeMessages: vi.fn(),
    workspaces: {
      activeId: "w1",
      items: [{ id: "w1", clarification_unanswered_inbox_enabled: enabled }],
    },
    bumpNeedsYouInboxRefreshTick: bumpMock,
  };
}

async function sendLateAnswer(pendingId: string, targetSessionId?: string) {
  const source = sourceMessage(pendingId);
  const { result } = renderHook(() =>
    // eslint-disable-next-line react-hooks/rules-of-hooks -- renderHook callback is the hook under test.
    useLateClarificationMessage(source, { targetSessionId }),
  );
  let outcome: unknown;
  await act(async () => {
    outcome = await result.current.send({
      messages: [source],
      answers: [{ question_id: "q-db", selected_options: ["a"] }],
    });
  });
  return { outcome, sessionId: result.current.sessionId };
}

beforeEach(() => {
  admissionMock.mockReset().mockResolvedValue("sent");
  markMock.mockReset().mockResolvedValue(undefined);
  bumpMock.mockReset();
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useLateClarificationMessage unanswered close", () => {
  it("closes only the answered question with the admitted message identity", async () => {
    resetStore(true);
    const { outcome } = await sendLateAnswer("pending-close");
    expect(outcome).toBe("sent");
    const clientMessageId = admissionMock.mock.calls[0][0].clientMessageId;
    expect(markMock).toHaveBeenCalledTimes(1);
    expect(markMock).toHaveBeenCalledWith("pending-close", {
      messageId: clientMessageId,
      sessionId: ORIGINAL_SESSION,
    });
    expect(bumpMock).toHaveBeenCalledTimes(1);
  });

  it("sends nothing extra while the workspace setting is off", async () => {
    resetStore(false);
    await sendLateAnswer("pending-off");
    expect(admissionMock).toHaveBeenCalledTimes(1);
    expect(markMock).not.toHaveBeenCalled();
    expect(bumpMock).not.toHaveBeenCalled();
  });

  it("delivers to an explicitly chosen task session and records that target", async () => {
    resetStore(true);
    const { sessionId } = await sendLateAnswer("pending-target", PRIMARY_SESSION);
    expect(sessionId).toBe(PRIMARY_SESSION);
    expect(markMock.mock.calls[0][1].sessionId).toBe(PRIMARY_SESSION);
  });

  it("keeps the delivered outcome when recording the close fails", async () => {
    resetStore(true);
    markMock.mockRejectedValue(new Error("409"));
    const { outcome } = await sendLateAnswer("pending-conflict");
    expect(outcome).toBe("sent");
    expect(bumpMock).not.toHaveBeenCalled();
  });
});
