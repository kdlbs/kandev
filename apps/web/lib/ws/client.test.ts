/* eslint-disable max-lines, max-lines-per-function, sonarjs/no-duplicate-string -- Ordered transport fixtures keep the full recovery handshake together. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WebSocketClient, WebSocketRequestError, WebSocketRequestTimeoutError } from "./client";

type SentRequest = {
  id: string;
  type: string;
  action: string;
  payload: unknown;
};

class FakeWebSocket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readonly sent: SentRequest[] = [];
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }

  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  send(data: string) {
    this.sent.push(JSON.parse(data) as SentRequest);
  }

  receive(message: unknown) {
    this.onmessage?.({ data: JSON.stringify(message) });
  }

  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.({ code: 1006, reason: "network lost" } as CloseEvent);
  }

  static latest() {
    const socket = FakeWebSocket.instances.at(-1);
    if (!socket) throw new Error("No fake websocket exists");
    return socket;
  }

  static reset() {
    FakeWebSocket.instances = [];
  }
}

function connectClient(options?: ConstructorParameters<typeof WebSocketClient>[2]) {
  const client = new WebSocketClient("ws://test", undefined, {
    enabled: false,
    conversationProtocol: "v1",
    ...options,
  });
  client.connect();
  const socket = FakeWebSocket.latest();
  socket.open();
  return { client, socket };
}

function sessionSubscribeRequest(socket: FakeWebSocket, index = 0) {
  const request = socket.sent.filter((message) => message.action === "session.subscribe")[index];
  if (!request) throw new Error("No session.subscribe request was sent");
  return request;
}

function coreRequestPayload(request: SentRequest): Record<string, unknown> | null {
  if (typeof request.payload !== "object" || request.payload === null) return null;
  const payload = request.payload as Record<string, unknown>;
  return payload.consumer_kind === "core" ? payload : null;
}

function acknowledge(socket: FakeWebSocket, request: SentRequest) {
  const payload: Record<string, unknown> = { success: true };
  const requestPayload = coreRequestPayload(request);
  if (requestPayload) {
    payload.session_id = requestPayload.session_id;
    payload.wire_id = requestPayload.wire_id;
    if (request.action === "session.ack") {
      payload.acknowledged_sequence = requestPayload.sequence;
      payload.resume_token = "resume-core";
    } else {
      payload.result = "fresh";
      payload.event_watermark = 0;
      payload.snapshot_cutoff = 0;
      payload.snapshot_token = "snapshot-core";
      payload.resume_token = "resume-core";
      payload.expires_at = "2026-09-07T12:00:00Z";
    }
  }
  socket.receive({ id: request.id, type: "response", payload });
}
function acknowledgeSessionRegistration(socket: FakeWebSocket, startIndex = 0) {
  acknowledge(socket, sessionSubscribeRequest(socket, startIndex));
  acknowledge(socket, sessionSubscribeRequest(socket, startIndex + 1));
}

function acknowledgeWithResumeToken(socket: FakeWebSocket, request: SentRequest, token: string) {
  const payload: Record<string, unknown> = { success: true, resume_token: token };
  const requestPayload = coreRequestPayload(request);
  if (requestPayload) {
    payload.session_id = requestPayload.session_id;
    payload.wire_id = requestPayload.wire_id;
    if (request.action === "session.ack") {
      payload.acknowledged_sequence = requestPayload.sequence;
    } else {
      payload.result = "fresh";
      payload.event_watermark = 0;
      payload.snapshot_cutoff = 0;
      payload.snapshot_token = "snapshot-core";
      payload.expires_at = "2026-09-07T12:00:00Z";
    }
  }
  socket.receive({ id: request.id, type: "response", payload });
}

beforeEach(() => {
  vi.stubGlobal("WebSocket", FakeWebSocket);
  FakeWebSocket.reset();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

// eslint-disable-next-line max-lines-per-function -- readiness tests cover registration and retry lifecycle.
describe("session subscription readiness", () => {
  it("accepts a registration acknowledgement that arrives after seven seconds", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");

    await vi.advanceTimersByTimeAsync(7000);
    acknowledgeSessionRegistration(socket);

    await expect(subscription.ready).resolves.toBeUndefined();
    subscription.unsubscribe();
  });

  it("recovers timed out registration without a visibility change", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");

    await vi.advanceTimersByTimeAsync(11000);
    acknowledgeSessionRegistration(socket, 2);

    await expect(subscription.ready).resolves.toBeUndefined();
    subscription.unsubscribe();
  });

  it("stops after the bounded registration retry budget", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");

    await vi.advanceTimersByTimeAsync(21000);

    await expect(subscription.ready).rejects.toBeInstanceOf(WebSocketRequestTimeoutError);
    expect(socket.sent.filter((message) => message.action === "session.subscribe")).toHaveLength(4);
    subscription.unsubscribe();
  });

  it("cancels a scheduled registration retry when the last consumer unsubscribes", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");

    await vi.advanceTimersByTimeAsync(10000);
    subscription.unsubscribe();
    await expect(subscription.ready).rejects.toThrow("Session subscription released");

    await vi.advanceTimersByTimeAsync(1000);
    expect(socket.sent.filter((message) => message.action === "session.subscribe")).toHaveLength(2);
  });
  it("resolves only after the server acknowledges the registration", async () => {
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");
    const request = sessionSubscribeRequest(socket);
    let ready = false;

    void subscription.ready.then(() => {
      ready = true;
    });
    await Promise.resolve();
    expect(ready).toBe(false);

    acknowledge(socket, request);
    await Promise.resolve();
    expect(ready).toBe(false);
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await expect(subscription.ready).resolves.toBeUndefined();
    expect(ready).toBe(true);
    subscription.unsubscribe();
  });

  it("shares one in-flight acknowledgement between ref-counted consumers", async () => {
    const { client, socket } = connectClient();
    const first = client.subscribeSessionWithReady("sess-1");
    const second = client.subscribeSessionWithReady("sess-1");

    expect(second.ready).toBe(first.ready);
    expect(socket.sent.filter((message) => message.action === "session.subscribe")).toHaveLength(2);

    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await expect(second.ready).resolves.toBeUndefined();

    first.unsubscribe();
    second.unsubscribe();
  });

  it("allows a failed registration to be retried with fresh readiness", async () => {
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");

    const firstRequest = sessionSubscribeRequest(socket);
    const orderedFirstRequest = sessionSubscribeRequest(socket, 1);

    socket.receive({
      id: firstRequest.id,
      type: "error",
      payload: { message: "session is not ready" },
    });
    acknowledge(socket, orderedFirstRequest);
    await expect(subscription.ready).rejects.toThrow("session is not ready");

    const retry = client.resubscribeSession("sess-1");
    const retryRequest = sessionSubscribeRequest(socket, 2);
    const orderedRetryRequest = sessionSubscribeRequest(socket, 3);
    expect(retry).not.toBe(subscription.ready);

    acknowledge(socket, retryRequest);
    acknowledge(socket, orderedRetryRequest);
    await expect(retry).resolves.toBeUndefined();
    subscription.unsubscribe();
  });

  it("tracks the re-registration after reconnect", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const initial = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await expect(initial.ready).resolves.toBeUndefined();

    socket.close();
    vi.advanceTimersByTime(0);
    const reconnectedSocket = FakeWebSocket.latest();
    reconnectedSocket.open();

    const reconnected = client.subscribeSessionWithReady("sess-1");
    const reconnectRequest = sessionSubscribeRequest(reconnectedSocket, 0);
    expect(reconnectRequest.payload).toEqual({ session_id: "sess-1" });
    expect(reconnected.ready).not.toBe(initial.ready);

    acknowledge(reconnectedSocket, reconnectRequest);
    await Promise.resolve();
    acknowledge(reconnectedSocket, sessionSubscribeRequest(reconnectedSocket, 1));
    await expect(reconnected.ready).resolves.toBeUndefined();
    initial.unsubscribe();
    reconnected.unsubscribe();
  });
});

describe("ordered core session compatibility", () => {
  it("registers a distinct core wire and dispatches each sequence before acknowledging it", async () => {
    const { client, socket } = connectClient();
    const handler = vi.fn();
    client.on("session.message.added", handler);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    const ordered = sessionSubscribeRequest(socket, 1);
    expect(ordered.payload).toMatchObject({
      session_id: "sess-1",
      consumer_kind: "core",
    });
    expect(ordered.payload).not.toHaveProperty("last_seen_sequence");
    expect((ordered.payload as { wire_id: string }).wire_id).toMatch(/^core:web:/);
    acknowledgeWithResumeToken(socket, ordered, "resume-core");
    await subscription.ready;

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "event-1",
      payload: {
        type: "message.added",
        session_id: "sess-1",
        message_id: "message-1",
        task_id: "task-1",
        author_type: "user",
        content: "hello",
        created_at: "2026-09-07T12:00:00Z",
      },
    });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler.mock.calls[0]?.[0].payload).toMatchObject({
      message_id: "message-1",
      type: "message",
    });
    const ack = socket.sent.at(-1);
    expect(ack).toMatchObject({
      action: "session.ack",
      payload: {
        session_id: "sess-1",
        consumer_kind: "core",
        sequence: 1,
        resume_token: "resume-core",
      },
    });
    if (!ack) throw new Error("No session.ack request was sent");
    acknowledge(socket, ack);
    subscription.unsubscribe();
  });
});
it("buffers ordered events until the core subscription watermark is known", async () => {
  const { client, socket } = connectClient();
  const handler = vi.fn();
  client.on("session.message.added", handler);
  const subscription = client.subscribeSessionWithReady("sess-1");
  const legacy = sessionSubscribeRequest(socket);
  const ordered = sessionSubscribeRequest(socket, 1);
  acknowledge(socket, legacy);

  socket.receive({
    type: "session.event",
    protocol_version: 1,
    event_type: "message.added",
    session_id: "sess-1",
    task_id: "task-1",
    sequence: 4,
    event_id: "event-4",
    payload: {
      type: "message.added",
      session_id: "sess-1",
      message_id: "message-4",
      task_id: "task-1",
      author_type: "user",
      content: "hello",
      created_at: "2026-09-07T12:00:00Z",
    },
  });

  expect(handler).not.toHaveBeenCalled();
  expect(socket.sent.some((message) => message.action === "session.ack")).toBe(false);

  const requestPayload = coreRequestPayload(ordered);
  socket.receive({
    id: ordered.id,
    type: "response",
    payload: {
      success: true,
      session_id: "sess-1",
      wire_id: requestPayload?.wire_id,
      result: "fresh",
      event_watermark: 3,
      snapshot_cutoff: 3,
      snapshot_token: "snapshot-core",
      resume_token: "resume-core",
      expires_at: "2026-09-07T12:00:00Z",
    },
  });

  await subscription.ready;
  expect(handler).toHaveBeenCalledTimes(1);
  expect(socket.sent.at(-1)).toMatchObject({
    action: "session.ack",
    payload: { session_id: "sess-1", consumer_kind: "core", sequence: 4 },
  });
  subscription.unsubscribe();
});

describe("ordered core session validation", () => {
  it("does not project or acknowledge a mismatched payload type", async () => {
    const { client, socket } = connectClient();
    const handler = vi.fn();
    client.on("session.message.added", handler);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await subscription.ready;
    const sentBefore = socket.sent.length;

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "event-poison",
      payload: { type: "message.updated", message_id: "message-1" },
    });

    expect(handler).not.toHaveBeenCalled();
    expect(socket.sent).toHaveLength(sentBefore + 1);
    subscription.unsubscribe();
  });

  it("delivers valid poison and turn removal envelopes to raw session consumers", async () => {
    const { client, socket } = connectClient();
    const rawHandler = vi.fn();
    client.onRawSessionEvent(rawHandler);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await subscription.ready;

    const poison = {
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "event-poison",
      payload: { type: "message.updated", message_id: "message-1" },
    };
    const turnRemoval = {
      type: "session.event",
      protocol_version: 1,
      event_type: "session.turn.removed",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 2,
      event_id: "event-turn-removed",
      payload: { type: "session.turn.removed", id: "turn-1" },
    };

    socket.receive(poison);
    socket.receive(turnRemoval);

    expect(rawHandler.mock.calls.map(([event]) => event)).toEqual([poison, turnRemoval]);
    subscription.unsubscribe();
  });

  it("acknowledges a registry-approved ignorable event without projection", async () => {
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await subscription.ready;

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "session.workspace_sources.updated",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "event-ignored",
      payload: {
        type: "session.workspace_sources.updated",
        session_id: "sess-1",
        task_id: "task-1",
      },
    });

    expect(socket.sent.at(-1)).toMatchObject({
      action: "session.ack",
      payload: { session_id: "sess-1", consumer_kind: "core", sequence: 1 },
    });
    subscription.unsubscribe();
  });
});

describe("connection generations", () => {
  it("ignores notifications delivered by a replaced socket", () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const handler = vi.fn();
    client.on("message.queue.status_changed", handler);

    socket.close();
    vi.advanceTimersByTime(0);
    const reconnectedSocket = FakeWebSocket.latest();
    reconnectedSocket.open();

    socket.receive({
      id: "stale-status",
      type: "notification",
      action: "message.queue.status_changed",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        session_incarnation_id: "incarnation-1",
        status_epoch: "old-backend",
        status_generation: 99,
        count: 1,
        max: 10,
        merge_enabled: true,
      },
    });
    expect(handler).not.toHaveBeenCalled();

    reconnectedSocket.receive({
      id: "current-status",
      type: "notification",
      action: "message.queue.status_changed",
      payload: {
        task_id: "task-1",
        session_id: "session-1",
        session_incarnation_id: "incarnation-1",
        status_epoch: "current-backend",
        status_generation: 1,
        count: 0,
        max: 10,
        merge_enabled: true,
      },
    });
    expect(handler).toHaveBeenCalledOnce();
  });
});

describe("request errors", () => {
  it("retains the backend code and details when a request fails", async () => {
    const { client, socket } = connectClient();
    const request = client.request("session.recover", { action: "resume" });
    const sent = socket.sent.at(-1);
    if (!sent) throw new Error("No request was sent");

    socket.receive({
      id: sent.id,
      type: "error",
      payload: {
        code: "CONFLICT",
        message: "The saved branch is no longer available.",
        details: {
          kind: "branch_unrecoverable",
          recovery_action: "resume_new_branch",
          original_branch: "feature/lost",
        },
      },
    });

    await expect(request).rejects.toBeInstanceOf(WebSocketRequestError);
    await expect(request).rejects.toMatchObject({
      message: "The saved branch is no longer available.",
      code: "CONFLICT",
      details: {
        kind: "branch_unrecoverable",
        recovery_action: "resume_new_branch",
        original_branch: "feature/lost",
      },
    });
  });
});

describe("session subscription reconnect recovery", () => {
  it("keeps queued hydration behind the reconnect subscription acknowledgement", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const initial = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledge(socket, sessionSubscribeRequest(socket, 1));
    await expect(initial.ready).resolves.toBeUndefined();

    socket.close();
    const reconnectReadiness = client.getSessionSubscriptionReadiness("sess-1");
    let readinessResolved = false;
    void reconnectReadiness.then(() => {
      readinessResolved = true;
    });
    client.send({
      id: "hydration-1",
      type: "request",
      action: "message.list",
      payload: { session_id: "sess-1" },
    });
    await Promise.resolve();
    expect(readinessResolved).toBe(false);

    vi.runOnlyPendingTimers();
    const reconnectedSocket = FakeWebSocket.latest();
    reconnectedSocket.open();

    expect(reconnectedSocket.sent.map((message) => message.action)).toEqual([
      "session.subscribe",
      "session.subscribe",
      "message.list",
    ]);
    const reconnectRequest = sessionSubscribeRequest(reconnectedSocket);
    const hydrationRequest = reconnectedSocket.sent.find(
      (message) => message.action === "message.list",
    );
    if (!hydrationRequest) throw new Error("No queued message.list request was sent");

    acknowledge(reconnectedSocket, reconnectRequest);
    await Promise.resolve();
    acknowledge(reconnectedSocket, sessionSubscribeRequest(reconnectedSocket, 1));
    await expect(reconnectReadiness).resolves.toBeUndefined();
    expect(hydrationRequest.id).toBe("hydration-1");
    initial.unsubscribe();
  });

  it("rejects an active readiness when reconnect recovery is disabled", async () => {
    const { client, socket } = connectClient({ enabled: false });
    const subscription = client.subscribeSessionWithReady("sess-1");
    socket.close();

    await expect(subscription.ready).rejects.toThrow("WebSocket connection closed");
    subscription.unsubscribe();
  });
});

describe("ordered core session validation", () => {
  it("runs poison recovery for a session.event-shaped frame that fails the strict envelope check", async () => {
    const { client, socket } = connectClient();
    const handler = vi.fn();
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    const ordered = sessionSubscribeRequest(socket, 1);
    acknowledgeWithResumeToken(socket, ordered, "resume-core");
    await subscription.ready;
    const sentBefore = socket.sent.length;

    // protocol_version 2 (unsupported) with an otherwise contiguous shape.
    socket.receive({
      type: "session.event",
      protocol_version: 2,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "event-1",
      payload: { type: "message.added", message_id: "message-2" },
    });

    expect(handler).not.toHaveBeenCalled();
    // One recovery re-subscribe with replace_cursor, no ACK of the malformed
    // frame.
    const recovery = socket.sent
      .slice(sentBefore)
      .filter((frame) => frame.action === "session.subscribe");
    expect(recovery).toHaveLength(1);
    expect(recovery[0]?.payload).toMatchObject({ replace_cursor: true, session_id: "sess-1" });
    expect(socket.sent.slice(sentBefore).some((frame) => frame.action === "session.ack")).toBe(
      false,
    );
    subscription.unsubscribe();
  });

  it("keeps core projection paused until recovery hydration completes", async () => {
    const { client, socket } = connectClient();
    const handler = vi.fn();
    client.on("session.message.added", handler);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledgeWithResumeToken(socket, sessionSubscribeRequest(socket, 1), "resume-core");
    await subscription.ready;

    let resolveRecovery!: (repaired: boolean) => void;
    let recoveryCompleted = false;
    const recovery = new Promise<boolean>((resolve) => {
      resolveRecovery = resolve;
    });
    const registerRecovery = (
      client as unknown as {
        registerCoreSessionRecovery: (
          sessionId: string,
          handler: () => Promise<boolean>,
        ) => () => void;
      }
    ).registerCoreSessionRecovery;
    const unregisterRecovery = registerRecovery.call(client, "sess-1", () =>
      recovery.then((repaired) => {
        recoveryCompleted = true;
        return repaired;
      }),
    );

    socket.receive({
      type: "session.event",
      protocol_version: 2,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "poison-1",
      payload: { type: "message.added", session_id: "sess-1", message_id: "poison" },
    });
    const recoveryRequest = sessionSubscribeRequest(socket, 2);
    socket.receive({
      id: recoveryRequest.id,
      type: "response",
      payload: {
        success: true,
        session_id: "sess-1",
        wire_id: coreRequestPayload(recoveryRequest)?.wire_id,
        result: "invalid_resume",
        event_watermark: 3,
        snapshot_cutoff: 3,
        snapshot_token: "snapshot-recovery",
        resume_token: "resume-recovery",
        expires_at: "2099-01-01T00:00:00Z",
      },
    });

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 3,
      event_id: "event-3",
      payload: {
        type: "message.added",
        session_id: "sess-1",
        task_id: "task-1",
        message_id: "message-3",
        author_type: "user",
        content: "after snapshot cutoff",
        created_at: "2026-09-07T12:00:00Z",
      },
    });
    expect(handler).not.toHaveBeenCalled();

    resolveRecovery(true);
    await vi.waitFor(() => expect(recoveryCompleted).toBe(true));

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 4,
      event_id: "event-4",
      payload: {
        type: "message.added",
        session_id: "sess-1",
        task_id: "task-1",
        message_id: "message-4",
        author_type: "user",
        content: "after recovery",
        created_at: "2026-09-07T12:01:00Z",
      },
    });
    expect(handler).toHaveBeenCalledTimes(1);
    unregisterRecovery();
    subscription.unsubscribe();
  });

  it("keeps failed core recovery paused until an explicit retry succeeds", async () => {
    const { client, socket } = connectClient();
    const projected = vi.fn();
    client.on("session.message.added", projected);
    const recovery = vi
      .fn<() => Promise<boolean>>()
      .mockResolvedValueOnce(false)
      .mockResolvedValueOnce(true);
    const unregister = client.registerCoreSessionRecovery("sess-1", recovery);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledgeWithResumeToken(socket, sessionSubscribeRequest(socket, 1), "resume-core");
    await subscription.ready;

    socket.receive({
      type: "session.event",
      protocol_version: 2,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "poison-1",
      payload: { type: "message.added", session_id: "sess-1", message_id: "poison" },
    });
    const recoveryRequest = sessionSubscribeRequest(socket, 2);
    socket.receive({
      id: recoveryRequest.id,
      type: "response",
      payload: {
        success: true,
        session_id: "sess-1",
        wire_id: coreRequestPayload(recoveryRequest)?.wire_id,
        result: "invalid_resume",
        event_watermark: 1,
        snapshot_cutoff: 1,
        snapshot_token: "snapshot-recovery",
        resume_token: "resume-recovery",
        expires_at: "2099-01-01T00:00:00Z",
      },
    });
    await vi.waitFor(() => expect(recovery).toHaveBeenCalledTimes(1));

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 2,
      event_id: "event-2",
      payload: {
        type: "message.added",
        session_id: "sess-1",
        task_id: "task-1",
        message_id: "message-2",
        author_type: "user",
        content: "held until repair",
        created_at: "2026-09-07T12:00:00Z",
      },
    });
    expect(projected).not.toHaveBeenCalled();

    const retry = client.retryCoreSessionRecovery("sess-1");
    expect(retry).toBeDefined();
    await expect(retry).resolves.toBe(true);
    expect(projected).toHaveBeenCalledTimes(1);
    unregister();
    subscription.unsubscribe();
  });

  it("keeps a recovery generation dirty until a later core owner hydrates it", async () => {
    const { client, socket } = connectClient();
    const projected = vi.fn();
    client.on("session.message.added", projected);
    const subscription = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
    await Promise.resolve();
    acknowledgeWithResumeToken(socket, sessionSubscribeRequest(socket, 1), "resume-core");
    await subscription.ready;

    socket.receive({
      type: "session.event",
      protocol_version: 2,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 1,
      event_id: "poison-1",
      payload: { type: "message.added", session_id: "sess-1", message_id: "poison" },
    });
    const recoveryRequest = sessionSubscribeRequest(socket, 2);
    socket.receive({
      id: recoveryRequest.id,
      type: "response",
      payload: {
        success: true,
        session_id: "sess-1",
        wire_id: coreRequestPayload(recoveryRequest)?.wire_id,
        result: "invalid_resume",
        event_watermark: 1,
        snapshot_cutoff: 1,
        snapshot_token: "snapshot-recovery",
        resume_token: "resume-recovery",
        expires_at: "2099-01-01T00:00:00Z",
      },
    });
    await Promise.resolve();

    socket.receive({
      type: "session.event",
      protocol_version: 1,
      event_type: "message.added",
      session_id: "sess-1",
      task_id: "task-1",
      sequence: 2,
      event_id: "event-2",
      payload: {
        type: "message.added",
        session_id: "sess-1",
        task_id: "task-1",
        message_id: "message-2",
        author_type: "user",
        content: "wait for owner",
        created_at: "2026-09-07T12:00:00Z",
      },
    });
    expect(projected).not.toHaveBeenCalled();

    const unregister = client.registerCoreSessionRecovery("sess-1", async () => true);
    await vi.waitFor(() => expect(projected).toHaveBeenCalledTimes(1));
    unregister();
    subscription.unsubscribe();
  });

  it("projects source conversation batches and ignores legacy duplicates", async () => {
    const { client, socket } = connectClient({ conversationProtocol: "v2" });
    const projected = vi.fn();
    const changed = vi.fn();
    client.on("session.message.added", projected);
    client.on("session.conversation.changed", changed);
    const subscription = client.subscribeSessionWithReady("sess-1");
    const v2Request = socket.sent.find(
      (message) => message.action === "session.conversation.subscribe",
    );
    if (!v2Request) throw new Error("No source conversation subscribe request was sent");
    const scopeID = (v2Request.payload as { scope_id: string }).scope_id;
    socket.receive({
      id: v2Request.id,
      type: "response",
      payload: {
        success: true,
        protocol_version: 2,
        scope_id: scopeID,
        session_id: "sess-1",
        epoch: "epoch-1",
        revision: "0",
      },
    });
    const legacyRequest = socket.sent.find((message) => message.action === "session.subscribe");
    if (!legacyRequest) throw new Error("No session subscribe request was sent");
    socket.receive({ id: legacyRequest.id, type: "response", payload: { success: true } });
    await subscription.ready;

    socket.receive({
      type: "notification",
      action: "session.conversation.changed",
      payload: {
        protocol_version: 2,
        scope_id: scopeID,
        session_id: "sess-1",
        epoch: "epoch-1",
        base_revision: "0",
        revision: "1",
        operations: [
          {
            kind: "upsert",
            entity: "message",
            id: "message-1",
            message: {
              task_id: "task-1",
              author_type: "user",
              content: "source",
              type: "message",
              created_at: "2026-09-16T12:00:00Z",
              updated_at: "2026-09-16T12:00:00Z",
            },
          },
        ],
      },
    });
    socket.receive({
      type: "notification",
      action: "session.message.added",
      payload: { session_id: "sess-1", message_id: "message-1" },
    });
    expect(projected).toHaveBeenCalledTimes(1);
    expect(changed).toHaveBeenCalledTimes(1);
    subscription.unsubscribe();
  });

  it("repairs malformed source operations before advancing the applied revision", async () => {
    const { client, socket } = connectClient({ conversationProtocol: "v2" });
    const projected = vi.fn();
    const recover = vi.fn(async () => true);
    client.on("session.message.added", projected);
    const subscription = client.subscribeSessionWithReady("sess-1");
    const v2Request = socket.sent.find(
      (message) => message.action === "session.conversation.subscribe",
    );
    if (!v2Request) throw new Error("No source conversation subscribe request was sent");
    const scopeID = (v2Request.payload as { scope_id: string }).scope_id;
    socket.receive({
      id: v2Request.id,
      type: "response",
      payload: {
        success: true,
        protocol_version: 2,
        scope_id: scopeID,
        session_id: "sess-1",
        epoch: "epoch-1",
        revision: "0",
      },
    });
    const legacyRequest = socket.sent.find((message) => message.action === "session.subscribe");
    if (!legacyRequest) throw new Error("No session subscribe request was sent");
    socket.receive({ id: legacyRequest.id, type: "response", payload: { success: true } });
    await subscription.ready;
    client.registerCoreSessionRecovery("sess-1", recover);

    socket.receive({
      type: "notification",
      action: "session.conversation.changed",
      payload: {
        protocol_version: 2,
        scope_id: scopeID,
        session_id: "sess-1",
        epoch: "epoch-1",
        base_revision: "0",
        revision: "1",
        operations: [{ kind: "upsert", entity: "message", id: "message-1" }],
      },
    });

    await vi.waitFor(() => expect(recover).toHaveBeenCalledTimes(1));
    expect(projected).not.toHaveBeenCalled();
    subscription.unsubscribe();
  });
  it.each(["missed", "delivered", "closed"])(
    "checks idle source revisions with %s updates",
    async (outcome) => {
      vi.useFakeTimers();
      const { client, socket } = connectClient({ conversationProtocol: "v2" });
      const projected = vi.fn();
      const recover = vi.fn(async () => true);
      client.on("session.message.added", projected);
      const subscription = client.subscribeSessionWithReady("sess-1");
      const v2Request = socket.sent.find(
        (message) => message.action === "session.conversation.subscribe",
      );
      if (!v2Request) throw new Error("No source conversation subscribe request was sent");
      const scopeID = (v2Request.payload as { scope_id: string }).scope_id;
      socket.receive({
        id: v2Request.id,
        type: "response",
        payload: {
          success: true,
          protocol_version: 2,
          scope_id: scopeID,
          session_id: "sess-1",
          epoch: "epoch-1",
          revision: "0",
        },
      });
      const legacyRequest = socket.sent.find((message) => message.action === "session.subscribe");
      if (!legacyRequest) throw new Error("No session subscribe request was sent");
      socket.receive({ id: legacyRequest.id, type: "response", payload: { success: true } });
      await subscription.ready;
      client.registerCoreSessionRecovery("sess-1", recover);

      const check = (revision: string) =>
        socket.receive({
          type: "notification",
          action: "session.conversation.changed",
          payload: {
            protocol_version: 2,
            scope_id: scopeID,
            session_id: "sess-1",
            epoch: "epoch-1",
            base_revision: revision,
            revision,
            check: true,
            operations: [],
          },
        });
      check("0");
      await vi.advanceTimersByTimeAsync(1000);
      expect(recover).not.toHaveBeenCalled();
      check("1");
      await vi.advanceTimersByTimeAsync(999);
      expect(recover).not.toHaveBeenCalled();
      if (outcome === "closed") subscription.unsubscribe();
      if (outcome === "delivered")
        socket.receive({
          type: "notification",
          action: "session.conversation.changed",
          payload: {
            protocol_version: 2,
            scope_id: scopeID,
            session_id: "sess-1",
            epoch: "epoch-1",
            base_revision: "0",
            revision: "1",
            operations: [],
          },
        });
      await vi.advanceTimersByTimeAsync(1);
      expect(recover).toHaveBeenCalledTimes(outcome === "missed" ? 1 : 0);

      subscription.unsubscribe();
    },
  );
});
