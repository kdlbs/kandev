import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RawSessionEvent } from "@/lib/ws/client";
import type { PluginSessionMessagesState } from "./types";
import { PluginConversationScopeProvider, pluginConversationApi } from "./conversation-host";

const transport = vi.hoisted(() => ({
  request: vi.fn(),
  listener: null as ((event: RawSessionEvent) => void) | null,
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
    onConnectionStatus(_listener: (status: "connected" | "disconnected") => void) {
      return () => undefined;
    },
  }),
}));
const BINDING_PATH_SUFFIX = "/conversation/binding";
const FAR_FUTURE = "2099-01-01T00:00:00Z";

function ScopedHarness({ sessionId, testId }: { sessionId: string; testId: string }) {
  const state = pluginConversationApi.useSessionMessages({ sessionId });
  return <span data-testid={testId}>{String(state.hydrated)}</span>;
}

let userQueryState: PluginSessionMessagesState | null = null;
let agentQueryState: PluginSessionMessagesState | null = null;
let ascendingState: PluginSessionMessagesState | null = null;

function AscendingHarness() {
  ascendingState = pluginConversationApi.useSessionMessages({
    sessionId: "session-1",
    authorTypes: ["agent"],
    sort: "asc",
    pageSize: 1,
  });
  return (
    <span data-testid="ascending-query">
      {ascendingState.messages.map((item) => item.content).join("|")}
    </span>
  );
}

function TwoQueryHarness() {
  userQueryState = pluginConversationApi.useSessionMessages({
    sessionId: "session-1",
    authorTypes: ["user"],
    sort: "desc",
    pageSize: 1,
  });
  agentQueryState = pluginConversationApi.useSessionMessages({
    sessionId: "session-1",
    authorTypes: ["agent"],
    sort: "asc",
    pageSize: 2,
  });
  return (
    <>
      <span data-testid="user-query">
        {userQueryState.messages.map((message) => message.content).join("|")}
      </span>
      <span data-testid="agent-query">
        {agentQueryState.messages.map((message) => message.content).join("|")}
      </span>
    </>
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

function message(id: string, content: string, createdAt: string, authorType: "user" | "agent") {
  return {
    id,
    taskId: "task-1",
    sessionId: "session-1",
    authorType,
    type: "message",
    content,
    createdAt,
    updatedAt: createdAt,
  };
}

function liveAgentEvent(sequence: number): RawSessionEvent {
  return {
    type: "session.event",
    protocol_version: 1,
    event_type: "message.added",
    session_id: "session-1",
    task_id: "task-1",
    sequence,
    event_id: `event-${sequence}`,
    payload: {
      type: "message.added",
      session_id: "session-1",
      task_id: "task-1",
      message_id: "message-live",
      author_type: "agent",
      content: "live",
      message_type: "message",
      created_at: "2026-09-07T12:02:00Z",
    },
  };
}

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function renderScope(taskId: string, sessionId: string, testId: string) {
  return (
    <PluginConversationScopeProvider
      pluginId="plugin-history"
      taskId={taskId}
      sessionId={sessionId}
    >
      <ScopedHarness sessionId={sessionId} testId={testId} />
    </PluginConversationScopeProvider>
  );
}

function resetHarness() {
  transport.request.mockReset();
  transport.listener = null;
  userQueryState = null;
  agentQueryState = null;
  ascendingState = null;
  transport.request.mockResolvedValue({
    success: true,
    snapshot_token: "snapshot-1",
    resume_token: "resume-1",
    consumer_id: "consumer-1",
    event_watermark: 0,
    expires_at: FAR_FUTURE,
    result: "fresh",
  });
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL) => {
      if (String(input).endsWith(BINDING_PATH_SUFFIX)) {
        return Promise.resolve(
          response({
            bindingToken: "binding-1",
            generation: 7,
            expiresAt: FAR_FUTURE,
          }),
        );
      }
      return Promise.resolve(response({ messages: [], hasMore: false, cursor: null }));
    }),
  );
}

function registerIsolationLifecycle() {
  beforeEach(resetHarness);
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });
}

describe("plugin conversation panel isolation", () => {
  registerIsolationLifecycle();

  it("gives concurrent panels independent consumer identities and snapshots", async () => {
    render(
      <>
        {renderScope("task-1", "session-1", "panel-one")}
        {renderScope("task-2", "session-2", "panel-two")}
      </>,
    );

    await waitFor(() => expect(screen.getByTestId("panel-one").textContent).toBe("true"));
    await waitFor(() => expect(screen.getByTestId("panel-two").textContent).toBe("true"));
    const subscriptions = transport.request.mock.calls.filter(
      ([action]) => action === "session.subscribe",
    );
    expect(subscriptions).toHaveLength(2);
    expect(subscriptions[0][1].consumer_id).not.toBe(subscriptions[1][1].consumer_id);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("session-1"))).toBe(true);
    expect(fetchMock.mock.calls.some(([input]) => String(input).includes("session-2"))).toBe(true);
  });
});

describe("message query snapshot isolation", () => {
  registerIsolationLifecycle();

  it("buffers live events until every distinct message query commits its snapshot", async () => {
    const userPage = deferred<Response>();
    const agentPage = deferred<Response>();
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith(BINDING_PATH_SUFFIX)) {
          return Promise.resolve(
            response({
              bindingToken: "binding-1",
              generation: 7,
              expiresAt: FAR_FUTURE,
            }),
          );
        }
        return url.includes("author_type=user") ? userPage.promise : agentPage.promise;
      }),
    );
    render(
      <PluginConversationScopeProvider
        pluginId="plugin-history"
        taskId="task-1"
        sessionId="session-1"
      >
        <TwoQueryHarness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalledTimes(3));

    await act(async () => {
      userPage.resolve(
        response({
          messages: [message("message-user", "user snapshot", "2026-09-07T12:00:00Z", "user")],
          hasMore: false,
          cursor: null,
        }),
      );
      await userPage.promise;
    });
    act(() => transport.listener?.(liveAgentEvent(1)));

    expect(screen.getByTestId("agent-query").textContent).toBe("");
    expect(transport.request.mock.calls.filter(([action]) => action === "session.ack")).toEqual([]);

    await act(async () => {
      agentPage.resolve(
        response({
          messages: [message("message-agent", "agent snapshot", "2026-09-07T12:01:00Z", "agent")],
          hasMore: false,
          cursor: null,
        }),
      );
      await agentPage.promise;
    });
    expect(screen.getByTestId("agent-query").textContent).toBe("agent snapshot|live");
    await waitFor(() =>
      expect(transport.request.mock.calls.some(([action]) => action === "session.ack")).toBe(true),
    );
  });
});

describe("message continuation isolation", () => {
  registerIsolationLifecycle();

  it("renews concurrent continuations independently for distinct message queries", async () => {
    transport.request.mockImplementation((action: string) =>
      Promise.resolve(
        action === "session.subscribe"
          ? {
              success: true,
              snapshot_token: "snapshot-expiring",
              resume_token: "resume-1",
              consumer_id: "consumer-1",
              event_watermark: 0,
              expires_at: new Date(Date.now() + 30_000).toISOString(),
              result: "fresh",
            }
          : { success: true },
      ),
    );
    const renewalCursors: string[] = [];
    const requestedPageCursors: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        if (url.endsWith(BINDING_PATH_SUFFIX)) {
          return Promise.resolve(
            response({
              bindingToken: "binding-1",
              generation: 7,
              expiresAt: FAR_FUTURE,
            }),
          );
        }
        if (url.endsWith("/conversation/continuation/renew")) {
          const body = JSON.parse(String(init?.body)) as { cursor: string };
          renewalCursors.push(body.cursor);
          return Promise.resolve(
            response({
              cursor: `renewed-${body.cursor}`,
              snapshot_token: `snapshot-${body.cursor}`,
              expires_at: FAR_FUTURE,
            }),
          );
        }
        const parsed = new URL(url);
        const author = parsed.searchParams.get("author_type");
        const cursor = parsed.searchParams.get("cursor");
        if (cursor) requestedPageCursors.push(cursor);
        return Promise.resolve(
          response({
            messages: cursor
              ? []
              : [
                  message(
                    `message-${author}`,
                    `${author} snapshot`,
                    "2026-09-07T12:00:00Z",
                    author === "agent" ? "agent" : "user",
                  ),
                ],
            hasMore: !cursor,
            cursor: cursor ? null : `cursor-${author}`,
          }),
        );
      }),
    );
    render(
      <PluginConversationScopeProvider
        pluginId="plugin-history"
        taskId="task-1"
        sessionId="session-1"
      >
        <TwoQueryHarness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => {
      expect(userQueryState?.hasMore).toBe(true);
      expect(agentQueryState?.hasMore).toBe(true);
    });

    await act(async () => {
      await Promise.all([userQueryState!.loadMore(), agentQueryState!.loadMore()]);
    });

    expect(renewalCursors.sort()).toEqual(["cursor-agent", "cursor-user"]);
    expect(requestedPageCursors.sort()).toEqual(["renewed-cursor-agent", "renewed-cursor-user"]);
  });
});

describe("ascending message pagination", () => {
  registerIsolationLifecycle();

  it("deduplicates and re-sorts an ascending list after live delivery then loadMore", async () => {
    let messagePage = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith(BINDING_PATH_SUFFIX)) {
          return Promise.resolve(
            response({
              bindingToken: "binding-1",
              generation: 7,
              expiresAt: FAR_FUTURE,
            }),
          );
        }
        messagePage += 1;
        return Promise.resolve(
          response(
            messagePage === 1
              ? {
                  messages: [message("message-base", "base", "2026-09-07T12:00:00Z", "agent")],
                  hasMore: true,
                  cursor: "cursor-1",
                }
              : {
                  messages: [
                    message("message-live", "stale page", "2026-09-07T12:02:00Z", "agent"),
                    message("message-middle", "middle", "2026-09-07T12:01:00Z", "agent"),
                  ],
                  hasMore: false,
                  cursor: null,
                },
          ),
        );
      }),
    );
    render(
      <PluginConversationScopeProvider
        pluginId="plugin-history"
        taskId="task-1"
        sessionId="session-1"
      >
        <AscendingHarness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => expect(ascendingState?.hasMore).toBe(true));
    act(() => transport.listener?.(liveAgentEvent(1)));
    expect(screen.getByTestId("ascending-query").textContent).toBe("base|live");

    await act(async () => {
      await ascendingState!.loadMore();
    });

    expect(screen.getByTestId("ascending-query").textContent).toBe("base|middle|live");
  });
});
