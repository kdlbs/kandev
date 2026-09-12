/* eslint-disable max-lines,sonarjs/no-duplicate-string,max-lines-per-function -- This integration fixture covers the full conversation lifecycle with many fixture literals. */
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RawSessionEvent } from "@/lib/ws/client";
import type { PluginSessionMessagesState, PluginSessionTurnsState } from "./types";
import { PluginConversationScopeProvider, pluginConversationApi } from "./conversation-host";

const transport = vi.hoisted(() => ({
  request: vi.fn(),
  listener: null as ((event: RawSessionEvent) => void) | null,
  statusListener: null as ((status: "connected" | "disconnected") => void) | null,
}));

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://host" }) }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({
    request: transport.request,
    onRawSessionEvent(listener: (event: RawSessionEvent) => void) {
      transport.listener = listener;
      return () => {
        if (transport.listener === listener) transport.listener = null;
      };
    },
    onConnectionStatus(listener: (status: "connected" | "disconnected") => void) {
      transport.statusListener = listener;
      return () => {
        if (transport.statusListener === listener) transport.statusListener = null;
      };
    },
  }),
}));

let currentState: PluginSessionMessagesState | null = null;
let currentTurnsState: PluginSessionTurnsState | null = null;
const SESSION_SUBSCRIBE_ACTION = "session.subscribe";
const SESSION_ACK_ACTION = "session.ack";
const BINDING_PATH_SUFFIX = "/conversation/binding";
const FAR_FUTURE_EXPIRY = "2099-01-01T00:00:00Z";
const MESSAGE_CREATED_AT = "2026-09-07T12:00:00Z";
const SESSION_REMOVED_EVENT = "session.removed";

function Harness({ sessionId, taskId }: { sessionId: string | null; taskId?: string | null }) {
  currentState = pluginConversationApi.useSessionMessages({
    sessionId,
    taskId,
    authorTypes: ["user"],
    sort: "desc",
    pageSize: 1,
  });
  return (
    <div>
      <span data-testid="messages">
        {currentState.messages.map((message) => message.content).join("|")}
      </span>
      <span data-testid="removed">{String(currentState.removed)}</span>
      <span data-testid="error">{currentState.error?.code ?? ""}</span>
    </div>
  );
}

function TurnsHarness({ sessionId, taskId }: { sessionId: string | null; taskId?: string | null }) {
  currentTurnsState = pluginConversationApi.useSessionTurns(sessionId, taskId);
  return (
    <div>
      <span data-testid="turns">{currentTurnsState.turns.map((turn) => turn.id).join("|")}</span>
      <span data-testid="turns-removed">{String(currentTurnsState.removed)}</span>
      <span data-testid="turns-error">{currentTurnsState.error?.code ?? ""}</span>
    </div>
  );
}

function renderTurnsHarness(sessionId: string | null, taskId?: string | null) {
  return render(
    <PluginConversationScopeProvider
      pluginId="plugin-history"
      taskId="task-1"
      sessionId={sessionId}
    >
      <TurnsHarness sessionId={sessionId} taskId={taskId} />
    </PluginConversationScopeProvider>,
  );
}

function renderHarness(sessionId: string | null, taskId?: string | null) {
  return render(
    <PluginConversationScopeProvider
      pluginId="plugin-history"
      taskId="task-1"
      sessionId={sessionId}
    >
      <Harness sessionId={sessionId} taskId={taskId} />
    </PluginConversationScopeProvider>,
  );
}

function response(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
const TEST_MESSAGE_ID = "message-1";
const MESSAGE_ADDED_EVENT = "message.added";

function requiredEventPayload(eventType: string): Record<string, unknown> {
  switch (eventType) {
    case MESSAGE_ADDED_EVENT:
      return {
        message_id: TEST_MESSAGE_ID,
        author_type: "user",
        content: "",
        created_at: MESSAGE_CREATED_AT,
      };
    case "message.updated":
      return {
        message_id: TEST_MESSAGE_ID,
        author_type: "user",
        content: "",
        created_at: MESSAGE_CREATED_AT,
        updated_at: MESSAGE_CREATED_AT,
      };
    case "message.deleted":
      return { message_id: TEST_MESSAGE_ID };
    case "session.turn.started":
      return { id: "turn-1", started_at: MESSAGE_CREATED_AT };
    case "session.turn.completed":
      return {
        id: "turn-1",
        started_at: MESSAGE_CREATED_AT,
        completed_at: MESSAGE_CREATED_AT,
        updated_at: MESSAGE_CREATED_AT,
      };
    default:
      return {};
  }
}

function event(
  sequence: number,
  eventType: string,
  payload: Record<string, unknown>,
  taskId: string | null = "task-1",
): RawSessionEvent {
  const requiredPayload = requiredEventPayload(eventType);
  return {
    type: "session.event",
    protocol_version: 1,
    event_type: eventType,
    session_id: "session-1",
    task_id: taskId,
    sequence,
    event_id: `event-${sequence}`,
    payload: {
      session_id: "session-1",
      ...(taskId !== null ? { task_id: taskId } : {}),
      ...requiredPayload,
      ...payload,
      type: eventType,
    },
  };
}

beforeEach(() => {
  currentState = null;
  transport.listener = null;
  transport.statusListener = null;
  transport.request.mockReset();
  transport.request.mockImplementation((action: string) => {
    if (action === SESSION_SUBSCRIBE_ACTION) {
      return Promise.resolve({
        success: true,
        snapshot_token: "snapshot-1",
        resume_token: "resume-0",
        consumer_id: "consumer-1",
        event_watermark: 0,
        expires_at: FAR_FUTURE_EXPIRY,
        result: "fresh",
      });
    }
    if (action === SESSION_ACK_ACTION) {
      return Promise.resolve({ success: true, resume_token: "resume-next" });
    }
    return Promise.resolve({ success: true });
  });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
describe("plugin conversation Host facade", () => {
  it("keeps nullable sessions empty without transport activity", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    renderHarness(null);

    expect(screen.getByTestId("messages").textContent).toBe("");
    expect(currentState?.hasMore).toBe(false);
    expect(currentState?.hydrated).toBe(false);
    expect(await currentState?.loadMore()).toBe(0);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(transport.request).not.toHaveBeenCalled();
  });

  it("retries initial binding setup after a transient failure", async () => {
    let bindingFetches = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
        bindingFetches += 1;
        if (bindingFetches === 1) {
          return Promise.resolve(
            response(
              { error: { code: "upstream_failure", message: "temporary", retryable: true } },
              503,
            ),
          );
        }
        return Promise.resolve(
          response({ bindingToken: "binding-2", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.error?.retryable).toBe(true));

    act(() => currentState?.retry());

    await waitFor(() => expect(currentState?.hydrated).toBe(true));
    expect(bindingFetches).toBe(2);
  });

  it("retries initial ordered subscription after a transient failure", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) =>
      String(input).endsWith(BINDING_PATH_SUFFIX)
        ? Promise.resolve(
            response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
          )
        : Promise.resolve(response({ messages: [], hasMore: false, cursor: null })),
    );
    vi.stubGlobal("fetch", fetchMock);
    transport.request.mockRejectedValueOnce(new Error("temporary subscribe failure"));
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.error?.retryable).toBe(true));

    act(() => currentState?.retry());

    await waitFor(() => expect(currentState?.hydrated).toBe(true));
    expect(
      transport.request.mock.calls.filter(([action]) => action === SESSION_SUBSCRIBE_ACTION),
    ).toHaveLength(2);
  });

  it("commits the authorized snapshot before projecting and acknowledging ordered live events", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      return Promise.resolve(
        response({
          messages: [
            {
              id: "message-1",
              taskId: "task-1",
              sessionId: "session-1",
              authorType: "user",
              type: "message",
              content: "original",
              createdAt: MESSAGE_CREATED_AT,
              updatedAt: MESSAGE_CREATED_AT,
            },
          ],
          hasMore: false,
          cursor: null,
        }),
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    renderHarness("session-1");
    await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe("original"));
    expect(currentState?.hydrated).toBe(true);

    act(() => {
      transport.listener?.(
        event(1, "message.updated", {
          message_id: "message-1",
          author_type: "user",
          content: "updated",
          message_type: "message",
          created_at: MESSAGE_CREATED_AT,
          updated_at: "2026-09-07T12:01:00Z",
        }),
      );
    });
    expect(screen.getByTestId("messages").textContent).toBe("updated");
    await waitFor(() =>
      expect(transport.request).toHaveBeenCalledWith(
        SESSION_ACK_ACTION,
        expect.objectContaining({ sequence: 1 }),
      ),
    );
    await act(async () => {
      await Promise.resolve();
    });
    const initialConsumerId = "consumer-1";

    act(() => {
      transport.statusListener?.("disconnected");
    });
    act(() => {
      transport.statusListener?.("connected");
    });
    await waitFor(() =>
      expect(transport.request).toHaveBeenCalledWith(
        SESSION_SUBSCRIBE_ACTION,
        expect.objectContaining({
          consumer_id: initialConsumerId,
          last_seen_sequence: 1,
          resume_token: "resume-next",
        }),
      ),
    );
    act(() => {
      transport.listener?.(event(2, "message.deleted", { message_id: "message-1" }));
      transport.listener?.(event(3, SESSION_REMOVED_EVENT, {}, null));
    });
    expect(screen.getByTestId("messages").textContent).toBe("");
    expect(screen.getByTestId("removed").textContent).toBe("true");
    expect(currentState?.hasMore).toBe(false);
  });
});

describe("strict ordered event validation poisons instead of projecting", () => {
  function stubEmptySnapshot() {
    const fetchMock = vi.fn((input: RequestInfo | URL) =>
      String(input).endsWith(BINDING_PATH_SUFFIX)
        ? Promise.resolve(
            response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
          )
        : Promise.resolve(response({ messages: [], hasMore: false, cursor: null })),
    );
    vi.stubGlobal("fetch", fetchMock);
  }

  it("does not treat a removal with a mismatched payload session_id as terminal", async () => {
    stubEmptySnapshot();
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(event(1, SESSION_REMOVED_EVENT, { session_id: "session-other" }, null));
    });
    expect(screen.getByTestId("removed").textContent).toBe("false");
    await waitFor(() =>
      expect(transport.request).toHaveBeenCalledWith(
        SESSION_SUBSCRIBE_ACTION,
        expect.objectContaining({
          replace_cursor: true,
          consumer_kind: "plugin",
          plugin_id: "plugin-history",
          session_id: "session-1",
        }),
      ),
    );
    expect(screen.getByTestId("removed").textContent).toBe("false");
  });

  it("projects attachment-only messages with empty content through add and update", async () => {
    stubEmptySnapshot();
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(
        event(1, "message.added", {
          message_id: "message-attachment-only",
          content: "",
          created_at: MESSAGE_CREATED_AT,
        }),
      );
    });
    expect(currentState?.messages).toHaveLength(1);
    expect(currentState?.messages[0]?.content).toBe("");

    act(() => {
      transport.listener?.(
        event(2, "message.updated", {
          message_id: "message-attachment-only",
          content: "",
          created_at: MESSAGE_CREATED_AT,
          updated_at: "2026-09-07T12:01:00Z",
        }),
      );
    });
    expect(currentState?.messages).toHaveLength(1);
    expect(currentState?.messages[0]).toMatchObject({
      id: "message-attachment-only",
      content: "",
      updatedAt: "2026-09-07T12:01:00Z",
    });
  });

  it("does not let a retry page overwrite a live event projected mid-fetch", async () => {
    let resolveRetryPage: ((value: Response) => void) | undefined;
    const retryPage = new Promise<Response>((resolve) => {
      resolveRetryPage = resolve;
    });
    let messageFetches = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      messageFetches += 1;
      if (messageFetches === 1) {
        return Promise.resolve(
          response({
            messages: [
              {
                id: "message-original",
                taskId: "task-1",
                sessionId: "session-1",
                authorType: "user",
                type: "message",
                content: "original",
                createdAt: MESSAGE_CREATED_AT,
                updatedAt: MESSAGE_CREATED_AT,
              },
            ],
            hasMore: true,
            cursor: "cursor-1",
          }),
        );
      }
      if (messageFetches === 2) {
        return Promise.reject(new TypeError("network down"));
      }
      return retryPage;
    });
    vi.stubGlobal("fetch", fetchMock);
    renderHarness("session-1");
    await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe("original"));

    await expect(currentState?.loadMore()).rejects.toBeTruthy();
    await waitFor(() => expect(currentState?.error?.retryable).toBe(true));

    act(() => currentState?.retry());
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));

    // A live update lands while the retry page is still in flight.
    act(() => {
      transport.listener?.(
        event(1, "message.added", {
          message_id: "message-live",
          content: "live-during-retry",
          created_at: "2026-09-07T12:00:01Z",
          updated_at: "2026-09-07T12:00:01Z",
        }),
      );
    });
    expect(screen.getByTestId("messages").textContent).toBe("");

    await act(async () => {
      resolveRetryPage?.(
        response({
          messages: [
            {
              id: "message-original",
              taskId: "task-1",
              sessionId: "session-1",
              authorType: "user",
              type: "message",
              content: "original",
              createdAt: MESSAGE_CREATED_AT,
              updatedAt: MESSAGE_CREATED_AT,
            },
          ],
          hasMore: false,
          cursor: null,
        }),
      );
      await Promise.resolve();
    });
    await waitFor(() =>
      expect(screen.getByTestId("messages").textContent).toContain("live-during-retry"),
    );
    expect(screen.getByTestId("messages").textContent).toContain("original");
  });

  it("projects live events from the subscribe watermark, not sequence one", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) =>
      String(input).endsWith(BINDING_PATH_SUFFIX)
        ? Promise.resolve(
            response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
          )
        : Promise.resolve(response({ messages: [], hasMore: false, cursor: null })),
    );
    vi.stubGlobal("fetch", fetchMock);
    // An existing non-empty session subscribes at event_watermark 3; live
    // frames therefore start at sequence 4 and must not park waiting for 1.
    transport.request.mockImplementation((action: string) => {
      if (action === SESSION_SUBSCRIBE_ACTION) {
        return Promise.resolve({
          success: true,
          snapshot_token: "snapshot-1",
          resume_token: "resume-0",
          consumer_id: "consumer-1",
          event_watermark: 3,
          expires_at: FAR_FUTURE_EXPIRY,
          result: "fresh",
        });
      }
      if (action === SESSION_ACK_ACTION) {
        return Promise.resolve({ success: true, resume_token: "resume-next" });
      }
      return Promise.resolve({ success: true });
    });
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(
        event(4, "message.added", { message_id: "message-4", content: "live-four" }),
      );
    });
    expect(screen.getByTestId("messages").textContent).toBe("live-four");
  });

  it("accepts session.removed whose task_id differs from the session task", async () => {
    stubEmptySnapshot();
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(event(1, SESSION_REMOVED_EVENT, { task_id: "task-moved" }, null));
    });
    expect(screen.getByTestId("removed").textContent).toBe("true");
  });

  it("does not resurrect rows from events delivered after terminal removal", async () => {
    stubEmptySnapshot();
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(event(1, SESSION_REMOVED_EVENT, {}, null));
      transport.listener?.(event(2, "message.added", { message_id: "late", content: "late" }));
    });
    expect(screen.getByTestId("removed").textContent).toBe("true");
    expect(screen.getByTestId("messages").textContent).toBe("");
  });

  it("recovers through a durable rebind after a message with a missing required id", async () => {
    stubEmptySnapshot();
    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(event(1, "message.added", { message_id: "", content: "no-id" }));
    });
    expect(screen.getByTestId("messages").textContent).toBe("");
    await waitFor(() =>
      expect(transport.request).toHaveBeenCalledWith(
        SESSION_SUBSCRIBE_ACTION,
        expect.objectContaining({ replace_cursor: true }),
      ),
    );
    await act(async () => {
      await Promise.resolve();
    });

    act(() => {
      transport.listener?.(event(1, SESSION_REMOVED_EVENT, {}, null));
    });
    expect(screen.getByTestId("removed").textContent).toBe("true");
  });
});
it("does not fetch after terminal removal even through a captured loadMore callback", async () => {
  const fetchMock = vi.fn((input: RequestInfo | URL) =>
    String(input).endsWith(BINDING_PATH_SUFFIX)
      ? Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        )
      : Promise.resolve(response({ messages: [], hasMore: true, cursor: "cursor-1" })),
  );
  vi.stubGlobal("fetch", fetchMock);
  renderHarness("session-1");
  await waitFor(() => expect(currentState?.hasMore).toBe(true));
  const loadMore = currentState?.loadMore;
  act(() => transport.listener?.(event(1, SESSION_REMOVED_EVENT, {}, null)));
  await expect(loadMore?.()).resolves.toBe(0);
  expect(fetchMock).toHaveBeenCalledTimes(2);
});

it("commits terminal removal while the initial snapshot is still pending", async () => {
  let resolveSnapshot: ((value: Response) => void) | undefined;
  const snapshot = new Promise<Response>((resolve) => {
    resolveSnapshot = resolve;
  });
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
      return Promise.resolve(
        response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
      );
    }
    return snapshot;
  });
  vi.stubGlobal("fetch", fetchMock);
  renderHarness("session-1");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

  act(() => {
    transport.listener?.(event(1, SESSION_REMOVED_EVENT, {}, null));
  });
  expect(screen.getByTestId("removed").textContent).toBe("true");

  await act(async () => {
    resolveSnapshot?.(
      response({
        messages: [
          {
            id: "message-late",
            taskId: "task-1",
            sessionId: "session-1",
            authorType: "user",
            type: "message",
            content: "must not resurrect",
            createdAt: MESSAGE_CREATED_AT,
            updatedAt: MESSAGE_CREATED_AT,
          },
        ],
        hasMore: true,
        cursor: "late-cursor",
      }),
    );
    await snapshot;
  });
  expect(screen.getByTestId("removed").textContent).toBe("true");
  expect(screen.getByTestId("error").textContent).toBe("");
  expect(screen.getByTestId("messages").textContent).toBe("");
});
it("replays add, update, and delete events after a blocked snapshot without resurrection", async () => {
  let resolveSnapshot: ((value: Response) => void) | undefined;
  const snapshot = new Promise<Response>((resolve) => {
    resolveSnapshot = resolve;
  });
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
      return Promise.resolve(
        response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
      );
    }
    return snapshot;
  });
  vi.stubGlobal("fetch", fetchMock);
  renderHarness("session-1");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

  act(() => {
    transport.listener?.(
      event(1, MESSAGE_ADDED_EVENT, {
        message_id: "message-new",
        author_type: "user",
        content: "temporary",
        message_type: "message",
        created_at: "2026-09-07T12:02:00Z",
      }),
    );
    transport.listener?.(
      event(2, "message.updated", {
        message_id: "message-base",
        author_type: "user",
        content: "updated",
        message_type: "message",
        created_at: MESSAGE_CREATED_AT,
        updated_at: "2026-09-07T12:03:00Z",
      }),
    );
    transport.listener?.(event(3, "message.deleted", { message_id: "message-new" }));
  });
  expect(screen.getByTestId("messages").textContent).toBe("");

  await act(async () => {
    resolveSnapshot?.(
      response({
        messages: [
          {
            id: "message-base",
            taskId: "task-1",
            sessionId: "session-1",
            authorType: "user",
            type: "message",
            content: "original",
            createdAt: MESSAGE_CREATED_AT,
            updatedAt: MESSAGE_CREATED_AT,
          },
        ],
        hasMore: false,
        cursor: null,
      }),
    );
    await snapshot;
  });

  expect(screen.getByTestId("messages").textContent).toBe("updated");
  await waitFor(() =>
    expect(transport.request).toHaveBeenCalledWith(
      SESSION_ACK_ACTION,
      expect.objectContaining({ sequence: 3 }),
    ),
  );
});
it("keeps turns terminal when removal wins the initial snapshot race", async () => {
  let resolveSnapshot: ((value: Response) => void) | undefined;
  const snapshot = new Promise<Response>((resolve) => {
    resolveSnapshot = resolve;
  });
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
      return Promise.resolve(
        response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
      );
    }
    return snapshot;
  });
  vi.stubGlobal("fetch", fetchMock);
  renderTurnsHarness("session-1");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

  act(() => {
    transport.listener?.(event(1, SESSION_REMOVED_EVENT, {}, null));
  });
  expect(screen.getByTestId("turns-removed").textContent).toBe("true");

  await act(async () => {
    resolveSnapshot?.(
      response({
        turns: [
          {
            id: "turn-late",
            taskId: "task-1",
            sessionId: "session-1",
            startedAt: MESSAGE_CREATED_AT,
            updatedAt: MESSAGE_CREATED_AT,
          },
        ],
      }),
    );
    await snapshot;
  });
  expect(screen.getByTestId("turns-removed").textContent).toBe("true");
  expect(screen.getByTestId("turns-error").textContent).toBe("");
  expect(screen.getByTestId("turns").textContent).toBe("");
});
describe("plugin conversation ordering and binding lifecycle", () => {
  it("holds a forward sequence gap and drains events when the missing sequence arrives", async () => {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
    });
    vi.stubGlobal("fetch", fetchMock);
    renderHarness("session-1");
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

    act(() => {
      transport.listener?.(
        event(2, MESSAGE_ADDED_EVENT, {
          message_id: "message-two",
          author_type: "user",
          content: "second",
          message_type: "message",
          created_at: "2026-09-07T12:02:00Z",
        }),
      );
    });
    expect(screen.getByTestId("messages").textContent).toBe("");
    expect(transport.request.mock.calls.some(([action]) => action === SESSION_ACK_ACTION)).toBe(
      false,
    );

    act(() => {
      transport.listener?.(
        event(1, MESSAGE_ADDED_EVENT, {
          message_id: "message-one",
          author_type: "user",
          content: "first",
          message_type: "message",
          created_at: "2026-09-07T12:01:00Z",
        }),
      );
    });

    expect(screen.getByTestId("messages").textContent).toBe("second|first");
    await waitFor(() =>
      expect(transport.request).toHaveBeenCalledWith(
        SESSION_ACK_ACTION,
        expect.objectContaining({ sequence: 2 }),
      ),
    );
  });

  it("refreshes a loader binding and atomically replaces a superseded generation", async () => {
    let bindingReads = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
        bindingReads += 1;
        const expiresAt =
          bindingReads === 1 ? new Date(Date.now() + 60_000).toISOString() : FAR_FUTURE_EXPIRY;
        return Promise.resolve(
          response({
            bindingToken: `binding-${bindingReads}`,
            generation: bindingReads + 6,
            expiresAt,
          }),
        );
      }
      expect(new Headers(init?.headers).get("X-Kandev-Plugin-Binding")).toBe("binding-2");
      return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
    });
    vi.stubGlobal("fetch", fetchMock);

    renderHarness("session-1");
    await waitFor(() => expect(currentState?.loading).toBe(false));

    expect(bindingReads).toBe(2);
    const subscribeGenerations = transport.request.mock.calls
      .filter(([action]) => action === SESSION_SUBSCRIBE_ACTION)
      .map(([, request]) => request.generation);
    expect(subscribeGenerations).toEqual([7, 8]);
    expect(transport.request).toHaveBeenCalledWith(
      "session.unsubscribe",
      expect.objectContaining({ generation: 7 }),
    );
  });
});

it("atomically rebinds and replaces snapshots past a poison event", async () => {
  let snapshotReads = 0;
  let subscribes = 0;
  transport.request.mockImplementation((action: string) => {
    if (action === SESSION_SUBSCRIBE_ACTION) {
      subscribes += 1;
      return Promise.resolve({
        success: true,
        snapshot_token: `snapshot-${subscribes}`,
        resume_token: `resume-${subscribes}`,
        consumer_id: "consumer-1",
        event_watermark: subscribes - 1,
        expires_at: FAR_FUTURE_EXPIRY,
        result: "fresh",
      });
    }
    if (action === SESSION_ACK_ACTION) {
      return Promise.resolve({ success: true, resume_token: "resume-next" });
    }
    return Promise.resolve({ success: true });
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      snapshotReads += 1;
      return Promise.resolve(
        response({
          messages: [
            {
              id: "message-1",
              taskId: "task-1",
              sessionId: "session-1",
              authorType: "user",
              type: "message",
              content: snapshotReads === 1 ? "before poison" : "after poison",
              createdAt: MESSAGE_CREATED_AT,
              updatedAt: MESSAGE_CREATED_AT,
            },
          ],
          hasMore: false,
          cursor: null,
        }),
      );
    }),
  );

  renderHarness("session-1");
  await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe("before poison"));

  act(() => transport.listener?.(event(1, "unknown.event", {})));

  await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe("after poison"));
  expect(subscribes).toBe(2);
  expect(transport.request.mock.calls.some(([action]) => action === SESSION_ACK_ACTION)).toBe(
    false,
  );
});

it("surfaces terminal removal after an earlier poison event without advancing ACK", async () => {
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
      return Promise.resolve(
        response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
      );
    }
    return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
  });
  vi.stubGlobal("fetch", fetchMock);
  renderHarness("session-1");
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

  act(() => {
    transport.listener?.(event(1, "unknown.event", {}));
    transport.listener?.(event(2, SESSION_REMOVED_EVENT, {}, null));
  });

  expect(screen.getByTestId("removed").textContent).toBe("true");
  expect(transport.request.mock.calls.some(([action]) => action === SESSION_ACK_ACTION)).toBe(
    false,
  );
});

describe("plugin conversation pagination", () => {
  it("joins concurrent requests for one older-page continuation", async () => {
    let page = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      page += 1;
      return Promise.resolve(
        response({
          messages: [
            {
              id: `message-${page}`,
              taskId: "task-1",
              sessionId: "session-1",
              authorType: "user",
              type: "message",
              content: `page-${page}`,
              createdAt: `2026-09-07T12:0${page}:00Z`,
              updatedAt: `2026-09-07T12:0${page}:00Z`,
            },
          ],
          hasMore: page === 1,
          cursor: page === 1 ? "cursor-1" : null,
        }),
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    renderHarness("session-1");
    await waitFor(() => expect(currentState?.hasMore).toBe(true));
    let counts: number[] = [];
    const loadMore = currentState!.loadMore;
    await act(async () => {
      counts = await Promise.all([loadMore(), loadMore()]);
    });

    expect(page).toBe(2);
    expect(counts).toEqual([1, 1]);
    expect(screen.getByTestId("messages").textContent).toContain("page-2");
  });
});

describe("ordered turns convergence", () => {
  function stubTurnsFetch() {
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      return Promise.resolve(response({ turns: [], hasMore: false, cursor: null }));
    });
    vi.stubGlobal("fetch", fetchMock);
    return fetchMock;
  }

  it("drops a turn from live state when session.turn.removed arrives", async () => {
    stubTurnsFetch();
    renderTurnsHarness("session-1");
    await waitFor(() => expect(currentTurnsState?.hydrated).toBe(true));

    act(() => {
      transport.listener?.(
        event(1, "session.turn.started", {
          id: "turn-1",
          started_at: MESSAGE_CREATED_AT,
          completed_at: null,
          created_at: MESSAGE_CREATED_AT,
          updated_at: MESSAGE_CREATED_AT,
        }),
      );
      transport.listener?.(
        event(2, "session.turn.completed", {
          id: "turn-1",
          started_at: MESSAGE_CREATED_AT,
          completed_at: MESSAGE_CREATED_AT,
          created_at: MESSAGE_CREATED_AT,
          updated_at: MESSAGE_CREATED_AT,
        }),
      );
    });
    await waitFor(() => expect(currentTurnsState?.turns).toHaveLength(1));

    act(() => {
      transport.listener?.(event(3, "session.turn.removed", { id: "turn-1" }));
    });
    expect(currentTurnsState?.turns).toHaveLength(0);
    expect(currentTurnsState?.hydrated).toBe(true);
    expect(screen.getByTestId("turns-removed").textContent).toBe("false");
  });

  it.each([
    { name: "inherited task", selectedTaskId: undefined, removalTaskId: "task-2", removed: false },
    { name: "explicit task", selectedTaskId: "task-1", removalTaskId: "task-1", removed: true },
    { name: "session-wide scope", selectedTaskId: null, removalTaskId: "task-2", removed: true },
  ])(
    "applies turn removals only within $name",
    async ({ selectedTaskId, removalTaskId, removed }) => {
      stubTurnsFetch();
      renderTurnsHarness("session-1", selectedTaskId);
      await waitFor(() => expect(currentTurnsState?.hydrated).toBe(true));
      act(() => {
        transport.listener?.(
          event(1, "session.turn.started", {
            id: "turn-1",
            started_at: MESSAGE_CREATED_AT,
            completed_at: null,
            created_at: MESSAGE_CREATED_AT,
            updated_at: MESSAGE_CREATED_AT,
          }),
        );
      });
      await waitFor(() => expect(currentTurnsState?.turns).toHaveLength(1));

      act(() => {
        transport.listener?.(event(2, "session.turn.removed", { id: "turn-1" }, removalTaskId));
      });

      expect(currentTurnsState?.turns).toHaveLength(removed ? 0 : 1);
    },
  );

  it("does not let a retry turns page overwrite a live completion mid-fetch", async () => {
    let resolveRetry: ((value: Response) => void) | undefined;
    const retryPage = new Promise<Response>((resolve) => {
      resolveRetry = resolve;
    });
    let turnFetches = 0;
    const fetchMock = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({ bindingToken: "binding-1", generation: 7, expiresAt: FAR_FUTURE_EXPIRY }),
        );
      }
      turnFetches += 1;
      if (turnFetches === 1) {
        return Promise.reject(new TypeError("network down"));
      }
      return retryPage;
    });
    vi.stubGlobal("fetch", fetchMock);
    renderTurnsHarness("session-1");
    await waitFor(() => expect(currentTurnsState?.error?.retryable).toBe(true));

    // Retry starts a fresh page that is still in flight when live events land.
    act(() => currentTurnsState?.retry());
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

    act(() => {
      transport.listener?.(
        event(1, "session.turn.started", {
          id: "turn-1",
          started_at: MESSAGE_CREATED_AT,
          completed_at: null,
          created_at: MESSAGE_CREATED_AT,
          updated_at: MESSAGE_CREATED_AT,
        }),
      );
      transport.listener?.(
        event(2, "session.turn.completed", {
          id: "turn-1",
          started_at: MESSAGE_CREATED_AT,
          completed_at: MESSAGE_CREATED_AT,
          created_at: MESSAGE_CREATED_AT,
          updated_at: MESSAGE_CREATED_AT,
        }),
      );
    });

    await act(async () => {
      resolveRetry?.(response({ turns: [], hasMore: false, cursor: null }));
      await Promise.resolve();
    });
    // The buffered live events drain on top of the (empty) retry page instead
    // of being lost, and the completion survives the page assignment.
    await waitFor(() =>
      expect(currentTurnsState?.turns.find((turn) => turn.id === "turn-1")?.completedAt).toBe(
        MESSAGE_CREATED_AT,
      ),
    );
  });
});
