/* eslint-disable max-lines-per-function -- the fixture owns one complete source-scope lifecycle. */
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { SourceConversationScope } from "./conversation-source-scope";
import type { ConversationChangedPayload } from "@/lib/types/session-events";
import { PluginConversationScopeProvider, pluginConversationApi } from "./conversation-host";

const transport = vi.hoisted(() => ({
  request: vi.fn(),
  on: vi.fn(),
  onConnectionStatus: vi.fn(() => () => undefined),
  changeListener: null as ((message: { payload: ConversationChangedPayload }) => void) | null,
  removalListener: null as ((message: { payload: { session_id: string } }) => void) | null,
}));
const SESSION_ID = "session-1";
const CREATED_AT = "2026-09-16T12:00:00Z";
const PLUGIN_ID = "plugin-history";
const CONVERSATION_SUBSCRIBE_ACTION = "session.conversation.subscribe";

vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://host" }) }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => transport,
}));

function response(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200 });
}

function Harness() {
  const state = pluginConversationApi.useSessionMessages({ sessionId: SESSION_ID, sort: "asc" });
  return (
    <>
      <span data-testid="messages">{state.messages.map((item) => item.content).join("|")}</span>
      <span data-testid="removed">{String(state.removed)}</span>
    </>
  );
}

describe("source conversation scope", () => {
  beforeEach(() => {
    let pageRevision = "0";
    transport.changeListener = null;
    transport.removalListener = null;
    transport.on.mockReset();
    transport.on.mockImplementation((action: string, listener: typeof transport.changeListener) => {
      if (action === "session.conversation.changed") transport.changeListener = listener;
      if (action === "session.removed") {
        transport.removalListener = listener as typeof transport.removalListener;
      }
      return () => {
        if (action === "session.conversation.changed") transport.changeListener = null;
        if (action === "session.removed") transport.removalListener = null;
      };
    });
    transport.request.mockReset();
    transport.request.mockImplementation((action: string, payload: { scope_id?: string }) => {
      if (action === CONVERSATION_SUBSCRIBE_ACTION) {
        return Promise.resolve({
          success: true,
          protocol_version: 2,
          scope_id: payload.scope_id,
          session_id: SESSION_ID,
          epoch: "epoch-1",
          revision: pageRevision,
        });
      }
      return Promise.resolve({ success: true });
    });
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL) => {
        const url = String(input);
        if (url.endsWith("/conversation/binding")) {
          return Promise.resolve(
            response({
              bindingToken: "binding-1",
              generation: 7,
              expiresAt: "2099-01-01T00:00:00Z",
            }),
          );
        }
        return Promise.resolve(
          response({
            messages: [],
            hasMore: false,
            cursor: null,
            epoch: "epoch-1",
            revision: pageRevision,
          }),
        );
      }),
    );
    Object.defineProperty(transport, "setPageRevision", {
      configurable: true,
      value: (revision: string) => {
        pageRevision = revision;
      },
    });
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("uses source reads and applies a matching ID update without rereading", async () => {
    const fetchMock = globalThis.fetch as unknown as ReturnType<typeof vi.fn>;
    render(
      <PluginConversationScopeProvider pluginId={PLUGIN_ID} taskId="task-1" sessionId={SESSION_ID}>
        <Harness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe(""));
    expect(transport.request).toHaveBeenCalledWith(
      CONVERSATION_SUBSCRIBE_ACTION,
      expect.objectContaining({ consumer_kind: "plugin", session_id: SESSION_ID }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining(`/conversation/v2/task-sessions/${SESSION_ID}/messages`),
      expect.anything(),
    );

    const subscribePayload = transport.request.mock.calls.find(
      ([action]) => action === CONVERSATION_SUBSCRIBE_ACTION,
    )?.[1] as { scope_id: string };
    act(() => {
      transport.changeListener?.({
        payload: {
          protocol_version: 2,
          scope_id: subscribePayload.scope_id,
          session_id: SESSION_ID,
          epoch: "epoch-1",
          base_revision: "0",
          revision: "1",
          operations: [
            {
              kind: "upsert",
              entity: "message",
              id: "message-1",
              message: {
                id: "message-1",
                taskId: "task-1",
                sessionId: SESSION_ID,
                authorType: "user",
                type: "message",
                content: "from source",
                createdAt: CREATED_AT,
                updatedAt: CREATED_AT,
              },
            },
          ],
        },
      });
    });
    await waitFor(() => expect(screen.getByTestId("messages").textContent).toBe("from source"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("repairs a revision gap through a fresh source page", async () => {
    const setPageRevision = (
      transport as typeof transport & { setPageRevision: (value: string) => void }
    ).setPageRevision;
    render(
      <PluginConversationScopeProvider pluginId={PLUGIN_ID} taskId="task-1" sessionId={SESSION_ID}>
        <Harness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => expect(transport.changeListener).not.toBeNull());
    const scopeID = transport.request.mock.calls.find(
      ([action]) => action === CONVERSATION_SUBSCRIBE_ACTION,
    )?.[1].scope_id as string;
    setPageRevision("3");
    act(() => {
      transport.changeListener?.({
        payload: {
          protocol_version: 2,
          scope_id: scopeID,
          session_id: SESSION_ID,
          epoch: "epoch-1",
          base_revision: "2",
          revision: "3",
          operations: [],
        },
      });
    });
    await waitFor(() =>
      expect((globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls.length).toBe(3),
    );
    expect(
      transport.request.mock.calls.filter(([action]) => action === CONVERSATION_SUBSCRIBE_ACTION),
    ).toHaveLength(2);
  });

  it("projects terminal session removal from the core notification", async () => {
    render(
      <PluginConversationScopeProvider pluginId={PLUGIN_ID} taskId="task-1" sessionId={SESSION_ID}>
        <Harness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("removed").textContent).toBe("false"));

    act(() => {
      transport.removalListener?.({ payload: { session_id: SESSION_ID } });
    });

    await waitFor(() => expect(screen.getByTestId("removed").textContent).toBe("true"));
  });
  it("renews an expired binding for reads and reconnects", async () => {
    const now = Date.now();
    const clock = vi.spyOn(Date, "now").mockReturnValue(now);
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockImplementation(async () =>
      response({
        bindingToken: Date.now() === now ? "old" : "renewed",
        generation: 7,
        expiresAt: new Date(Date.now() + 600_000).toISOString(),
      }),
    );
    const scope = new SourceConversationScope(
      PLUGIN_ID,
      "task-1",
      SESSION_ID,
      new AbortController(),
    );
    try {
      expect((await scope.ready()).bindingToken).toBe("old");
      clock.mockReturnValue(now + 601_000);
      const [first, second] = await Promise.all([scope.ready(), scope.renewContinuation("cursor")]);
      expect(first.bindingToken).toBe("renewed");
      expect(second.binding.bindingToken).toBe("renewed");
      scope.reconnect();
      await waitFor(() =>
        expect(transport.request).toHaveBeenLastCalledWith(
          CONVERSATION_SUBSCRIBE_ACTION,
          expect.objectContaining({ binding_token: "renewed" }),
        ),
      );
      expect(fetchMock).toHaveBeenCalledTimes(2);
    } finally {
      scope.close();
      clock.mockRestore();
    }
  });

  it("loads every turn page before publishing hydrated history", async () => {
    function TurnsHarness() {
      const state = pluginConversationApi.useSessionTurns(SESSION_ID);
      return <span data-testid="turns">{state.turns.map((turn) => turn.id).join("|")}</span>;
    }
    const original = vi.mocked(fetch).getMockImplementation()!;
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      if (!url.pathname.endsWith("/turns")) return original(input, init);
      const second = url.searchParams.has("cursor");
      const turns = Array.from({ length: second ? 1 : 100 }, (_, index) => ({
        id: `turn-${second ? 100 : index}`,
        sessionId: SESSION_ID,
        taskId: "task-1",
        startedAt: CREATED_AT,
        completedAt: "2026-09-16T12:01:00Z",
      }));
      return response({
        turns,
        hasMore: !second,
        cursor: second ? null : "next",
        epoch: "epoch-1",
        revision: "0",
      });
    });
    render(
      <PluginConversationScopeProvider pluginId={PLUGIN_ID} taskId="task-1" sessionId={SESSION_ID}>
        <TurnsHarness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("turns").textContent?.split("|")).toHaveLength(101),
    );
    expect(screen.getByTestId("turns").textContent).toContain("turn-100");
  });

  it("checks observed revisions without certifying missed updates", async () => {
    vi.useFakeTimers();
    const scope = new SourceConversationScope(
      PLUGIN_ID,
      "task-1",
      SESSION_ID,
      new AbortController(),
    );
    try {
      await scope.ready();
      const scopeID = transport.request.mock.calls[0][1].scope_id;
      const recover = vi.fn();
      scope.subscribeRebind(recover);
      const check = (revision: string) =>
        scope.acceptChange({
          protocol_version: 2,
          scope_id: scopeID,
          session_id: SESSION_ID,
          epoch: "epoch-1",
          base_revision: revision,
          revision,
          check: true,
          operations: [],
        });
      check("0");
      await vi.advanceTimersByTimeAsync(1000);
      expect(recover).not.toHaveBeenCalled();
      check("1");
      await vi.advanceTimersByTimeAsync(999);
      expect(recover).not.toHaveBeenCalled();
      await vi.advanceTimersByTimeAsync(1);
      expect(recover).toHaveBeenCalledOnce();
    } finally {
      scope.close();
      vi.useRealTimers();
    }
  });

  it("terminates the plugin scope on server authorization removal", async () => {
    const scope = new SourceConversationScope(
      PLUGIN_ID,
      "task-1",
      SESSION_ID,
      new AbortController(),
    );
    await scope.ready();
    scope.acceptChange({
      scope_id: transport.request.mock.calls[0][1].scope_id,
      session_id: SESSION_ID,
      terminal: true,
    });
    expect(scope.isTerminal()).toBe(true);
    scope.close();
  });

  it("retries binding acquisition after a failed request", async () => {
    vi.mocked(fetch).mockRejectedValueOnce(new Error("offline"));
    const scope = new SourceConversationScope(
      PLUGIN_ID,
      "task-1",
      SESSION_ID,
      new AbortController(),
    );
    try {
      await expect(scope.ready()).rejects.toThrow("offline");
      await expect(scope.ready()).resolves.toMatchObject({ bindingToken: "binding-1" });
    } finally {
      scope.close();
    }
  });

  it.each(["delivered", "closed"])(
    "does not recover a revision check after %s",
    async (outcome) => {
      vi.useFakeTimers();
      const scope = new SourceConversationScope(
        PLUGIN_ID,
        "task-1",
        SESSION_ID,
        new AbortController(),
      );
      try {
        await scope.ready();
        const recover = vi.fn();
        scope.subscribeRebind(recover);
        const frame = {
          protocol_version: 2,
          scope_id: transport.request.mock.calls[0][1].scope_id,
          session_id: SESSION_ID,
          epoch: "epoch-1",
          revision: "1",
          operations: [],
        };
        scope.acceptChange({ ...frame, base_revision: "1", check: true });
        if (outcome === "closed") scope.close();
        else scope.acceptChange({ ...frame, base_revision: "0" });
        await vi.advanceTimersByTimeAsync(1000);
        expect(recover).not.toHaveBeenCalled();
      } finally {
        scope.close();
        vi.useRealTimers();
      }
    },
  );
  it("does not publish partial turns when a continuation fails", async () => {
    function FailedTurnsHarness() {
      const state = pluginConversationApi.useSessionTurns(SESSION_ID);
      return (
        <span data-testid="turn-result">{`${state.turns.length}:${state.hydrated}:${state.error?.code ?? "none"}`}</span>
      );
    }
    const original = vi.mocked(fetch).getMockImplementation()!;
    vi.mocked(fetch).mockImplementation(async (input, init) => {
      const url = new URL(String(input));
      if (!url.pathname.endsWith("/turns")) return original(input, init);
      if (url.searchParams.has("cursor"))
        return new Response(
          JSON.stringify({
            error: { code: "upstream_failure", retryable: true },
          }),
          { status: 500 },
        );
      return response({
        turns: [
          {
            id: "turn-1",
            sessionId: SESSION_ID,
            taskId: "task-1",
            startedAt: CREATED_AT,
          },
        ],
        hasMore: true,
        cursor: "next",
        epoch: "epoch-1",
        revision: "0",
      });
    });
    render(
      <PluginConversationScopeProvider pluginId={PLUGIN_ID} taskId="task-1" sessionId={SESSION_ID}>
        <FailedTurnsHarness />
      </PluginConversationScopeProvider>,
    );
    await waitFor(() =>
      expect(screen.getByTestId("turn-result").textContent).toBe("0:false:upstream_failure"),
    );
  });
});
