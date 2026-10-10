import { type ReactNode } from "react";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { ToolCallMessage } from "@/components/task/chat/messages/tool-call-message";
import { defaultSessionState } from "@/lib/state/slices/session/session-slice";
import type { HydrationState } from "@/lib/state/store";
import { sessionId, taskId } from "@/lib/types/ids";
import type { Message } from "@/lib/types/http";
import { useSessionMessages } from "./use-session-messages";

const transport = vi.hoisted(() => ({
  readiness: Promise.resolve(),
  request: vi.fn(),
  registerCoreSessionRecovery: vi.fn(),
}));

vi.mock("@/lib/ws/connection", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/ws/connection")>()),
  getWebSocketClient: () => ({
    request: transport.request,
    getSessionSubscriptionReadiness: () => transport.readiness,
    subscribeSessionWithReady: () => ({ ready: transport.readiness, unsubscribe: () => {} }),
    registerCoreSessionRecovery: transport.registerCoreSessionRecovery,
    retryCoreSessionRecovery: () => undefined,
  }),
}));

vi.mock("@/lib/api/domains/session-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/session-api")>()),
  listSessionTurns: async () => ({ turns: [], total: 0 }),
}));

type HistoryResponse = { messages: Message[]; has_more: boolean };
type TranscriptSnapshot = {
  store: ReturnType<typeof useAppStoreApi>;
  history: ReturnType<typeof useSessionMessages>;
};

const LIVE_REVISION = "2026-10-07T05:00:01Z";
const COMPLETED_OUTPUT = "all checks passed: 42";
const pendingResponses: Array<(response: HistoryResponse) => void> = [];
let nextSession = 0;

function deferHistory() {
  let resolve!: (response: HistoryResponse) => void;
  const promise = new Promise<HistoryResponse>((done) => {
    resolve = done;
  });
  pendingResponses.push(resolve);
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
  transport.registerCoreSessionRecovery.mockReturnValue(() => {});
  window.localStorage.clear();
});

afterEach(async () => {
  await act(async () => {
    pendingResponses.splice(0).forEach((resolve) => resolve({ messages: [], has_more: false }));
  });
  cleanup();
});

function toolMessage(sid: string, output?: string, updatedAt = "2026-10-07T05:00:00Z"): Message {
  return {
    id: "build-tool",
    task_id: taskId("refresh-task"),
    session_id: sessionId(sid),
    type: "tool_call",
    author_type: "agent",
    content: output ? "build completed" : "tool is running",
    created_at: "2026-10-07T04:59:00Z",
    updated_at: updatedAt,
    metadata: {
      tool_name: "exec",
      status: output ? "complete" : "running",
      normalized: { generic: { output } },
    },
  };
}

function TranscriptProbe({
  onSnapshot,
  sid,
}: {
  onSnapshot: (value: TranscriptSnapshot) => void;
  sid: string;
}) {
  const history = useSessionMessages(sid);
  const store = useAppStoreApi();
  onSnapshot({ history, store });
  return (
    <>
      {history.messages.map((row) => (
        <ToolCallMessage key={row.id} comment={row} />
      ))}
    </>
  );
}

async function openTranscript() {
  const sid = `real-live-refresh-${++nextSession}`;
  const initial = toolMessage(sid);
  const response = deferHistory();
  transport.request.mockImplementation((action: string) => {
    if (action !== "message.list") throw new Error(`Unexpected transport action: ${action}`);
    return response.promise;
  });
  const initialState: HydrationState = {
    connection: { status: "connected", error: null, issueSeverity: "none" },
    taskSessions: {
      ...defaultSessionState.taskSessions,
      items: {
        [sid]: {
          id: sessionId(sid),
          task_id: taskId("refresh-task"),
          state: "WAITING_FOR_INPUT",
          started_at: initial.created_at,
          updated_at: initial.updated_at!,
        },
      },
    },
    messages: { ...defaultSessionState.messages, bySession: { [sid]: [initial] } },
  };
  let snapshot!: TranscriptSnapshot;
  function Providers({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={initialState}>
        <ToastProvider>{children}</ToastProvider>
      </StateProvider>
    );
  }
  const view = render(
    <Providers>
      <TranscriptProbe
        sid={sid}
        onSnapshot={(value) => {
          snapshot = value;
        }}
      />
    </Providers>,
  );
  await waitFor(() => expect(transport.request).toHaveBeenCalledTimes(1));
  expect(snapshot.history.historyRefreshPending).toBe(true);
  return { ...view, sid, initial, response, read: () => snapshot };
}

async function settleHistory(
  transcript: Awaited<ReturnType<typeof openTranscript>>,
  messages: Message[],
) {
  await act(async () => {
    transcript.response.resolve({ messages, has_more: false });
  });
  await waitFor(() => expect(transcript.read().history.historyRefreshPending).toBe(false));
}

// @covers AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.1, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.2, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.6
describe("production transcript during a held history refresh", () => {
  it("keeps the completed result rendered after older same-ID history settles", async () => {
    const transcript = await openTranscript();
    const store = transcript.read().store;
    const completed = toolMessage(transcript.sid, COMPLETED_OUTPUT, LIVE_REVISION);
    act(() => store.getState().updateMessage(completed));
    expect(screen.getByText(COMPLETED_OUTPUT)).toBeTruthy();
    expect(transport.request).toHaveBeenCalledTimes(1);

    await settleHistory(transcript, [transcript.initial]);

    // Assert the visible outcome before a store assertion can terminate this regression.
    expect(screen.getByText(COMPLETED_OUTPUT)).toBeTruthy();
    expect(screen.queryByText("tool is running")).toBeNull();
    expect(transcript.read().store).toBe(store);
    expect(transcript.read().history.messages[0]).toMatchObject(completed);
  });

  it.each([false, true])(
    "renders a newer server result (intermediate live change=%s)",
    async (liveChange) => {
      const transcript = await openTranscript();
      if (liveChange) {
        act(() =>
          transcript
            .read()
            .store.getState()
            .updateMessage(toolMessage(transcript.sid, "intermediate live result", LIVE_REVISION)),
        );
        expect(screen.getByText("intermediate live result")).toBeTruthy();
      }
      const server = toolMessage(
        transcript.sid,
        "server verified 43 checks",
        "2026-10-07T05:00:02Z",
      );
      await settleHistory(transcript, [server]);
      expect(screen.getByText("server verified 43 checks")).toBeTruthy();
      expect(screen.queryByText("intermediate live result")).toBeNull();
      expect(transcript.read().history.messages[0]).toMatchObject(server);
    },
  );

  it("retains a distinct live tool added during the read without leaving refresh pending", async () => {
    const transcript = await openTranscript();
    const addition = {
      ...toolMessage(transcript.sid, "new live tool result", LIVE_REVISION),
      id: "second-tool",
      created_at: LIVE_REVISION,
    };
    act(() => transcript.read().store.getState().addMessage(addition));
    expect(screen.getByText("new live tool result")).toBeTruthy();
    expect(transport.request).toHaveBeenCalledTimes(1);
    await settleHistory(transcript, [transcript.initial]);
    expect(screen.getByText("new live tool result")).toBeTruthy();
    expect(transcript.read().history.messages.map((row) => row.id)).toEqual([
      transcript.initial.id,
      addition.id,
    ]);
  });
});

// @covers AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.1, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.5, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.6
it("keeps the rendered completion through the registered authoritative recovery callback", async () => {
  const transcript = await openTranscript();
  await settleHistory(transcript, [transcript.initial]);
  const recovery = transport.registerCoreSessionRecovery.mock.calls.at(
    -1,
  )?.[1] as () => Promise<boolean>;
  expect(recovery).toBeTypeOf("function");
  const response = deferHistory();
  transport.request.mockReturnValue(response.promise);
  let recovered!: Promise<boolean>;
  act(() => {
    recovered = recovery();
  });
  await waitFor(() => expect(transport.request).toHaveBeenCalledTimes(2));
  const completed = toolMessage(transcript.sid, "recovery retained all 42 checks", LIVE_REVISION);
  act(() => transcript.read().store.getState().updateMessage(completed));
  expect(screen.getByText("recovery retained all 42 checks")).toBeTruthy();
  await act(async () => {
    response.resolve({ messages: [transcript.initial], has_more: false });
    await recovered;
  });
  await waitFor(() => expect(transcript.read().history.historyRefreshPending).toBe(false));
  expect(screen.getByText("recovery retained all 42 checks")).toBeTruthy();
  expect(screen.queryByText("tool is running")).toBeNull();
  expect(await recovered).toBe(true);
  expect(transcript.read().history.messages[0]).toMatchObject(completed);
});
