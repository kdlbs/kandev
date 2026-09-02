import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WebSocketClient, WebSocketRequestError } from "./client";

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

function taskSubscribeRequest(socket: FakeWebSocket, index = 0) {
  const request = socket.sent.filter((message) => message.action === "task.subscribe")[index];
  if (!request) throw new Error("No task.subscribe request was sent");
  return request;
}

function acknowledge(socket: FakeWebSocket, request: SentRequest) {
  socket.receive({
    id: request.id,
    type: "response",
    payload: { success: true },
  });
}

beforeEach(() => {
  vi.stubGlobal("WebSocket", FakeWebSocket);
  FakeWebSocket.reset();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe("session subscription readiness", () => {
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

    await expect(subscription.ready).resolves.toBeUndefined();
    expect(ready).toBe(true);
    subscription.unsubscribe();
  });

  it("shares one in-flight acknowledgement between ref-counted consumers", async () => {
    const { client, socket } = connectClient();
    const first = client.subscribeSessionWithReady("sess-1");
    const second = client.subscribeSessionWithReady("sess-1");

    expect(second.ready).toBe(first.ready);
    expect(socket.sent.filter((message) => message.action === "session.subscribe")).toHaveLength(1);

    acknowledge(socket, sessionSubscribeRequest(socket));
    await expect(second.ready).resolves.toBeUndefined();

    first.unsubscribe();
    second.unsubscribe();
  });

  it("allows a failed registration to be retried with fresh readiness", async () => {
    const { client, socket } = connectClient();
    const subscription = client.subscribeSessionWithReady("sess-1");
    const firstRequest = sessionSubscribeRequest(socket);

    socket.receive({
      id: firstRequest.id,
      type: "error",
      payload: { message: "session is not ready" },
    });
    await expect(subscription.ready).rejects.toThrow("session is not ready");

    const retry = client.resubscribeSession("sess-1");
    const retryRequest = sessionSubscribeRequest(socket, 1);
    expect(retry).not.toBe(subscription.ready);

    acknowledge(socket, retryRequest);
    await expect(retry).resolves.toBeUndefined();
    subscription.unsubscribe();
  });

  it("tracks the re-registration after reconnect", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const initial = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
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
    await expect(reconnected.ready).resolves.toBeUndefined();
    initial.unsubscribe();
    reconnected.unsubscribe();
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

describe("task subscription readiness", () => {
  it("resolves only after the server acknowledges the registration", async () => {
    const { client, socket } = connectClient();
    const subscription = client.subscribeTaskWithReady("task-1");
    const request = taskSubscribeRequest(socket);
    let ready = false;

    void subscription.ready.then(() => {
      ready = true;
    });
    await Promise.resolve();
    expect(ready).toBe(false);

    acknowledge(socket, request);

    await expect(subscription.ready).resolves.toBeUndefined();
    expect(ready).toBe(true);
    subscription.unsubscribe();
  });

  it("tracks a fresh acknowledgement after reconnect", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const initial = client.subscribeTaskWithReady("task-1");
    acknowledge(socket, taskSubscribeRequest(socket));
    await expect(initial.ready).resolves.toBeUndefined();

    socket.close();
    vi.advanceTimersByTime(0);
    const reconnectedSocket = FakeWebSocket.latest();
    reconnectedSocket.open();

    const reconnected = client.subscribeTaskWithReady("task-1");
    const reconnectRequest = taskSubscribeRequest(reconnectedSocket);
    expect(reconnected.ready).not.toBe(initial.ready);

    acknowledge(reconnectedSocket, reconnectRequest);
    await expect(reconnected.ready).resolves.toBeUndefined();
    initial.unsubscribe();
    reconnected.unsubscribe();
  });

  it("exposes reconnect readiness without acquiring another task lease", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const lease = client.subscribeTaskWithReady("task-1");
    acknowledge(socket, taskSubscribeRequest(socket));
    await expect(lease.ready).resolves.toBeUndefined();

    socket.close();
    vi.advanceTimersByTime(0);
    const reconnectedSocket = FakeWebSocket.latest();
    reconnectedSocket.open();

    const readiness = client.getTaskSubscriptionReadiness("task-1");
    const reconnectRequests = reconnectedSocket.sent.filter(
      (message) => message.action === "task.subscribe",
    );
    expect(reconnectRequests).toHaveLength(1);
    acknowledge(reconnectedSocket, reconnectRequests[0]);
    await expect(readiness).resolves.toBeUndefined();

    lease.unsubscribe();
    expect(
      reconnectedSocket.sent.filter((message) => message.action === "task.unsubscribe"),
    ).toHaveLength(1);
  });
});

describe("session subscription reconnect recovery", () => {
  it("keeps queued hydration behind the reconnect subscription acknowledgement", async () => {
    vi.useFakeTimers();
    const { client, socket } = connectClient({ enabled: true, initialDelay: 0, maxAttempts: 1 });
    const initial = client.subscribeSessionWithReady("sess-1");
    acknowledge(socket, sessionSubscribeRequest(socket));
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
      "message.list",
    ]);
    const reconnectRequest = sessionSubscribeRequest(reconnectedSocket);
    const hydrationRequest = reconnectedSocket.sent.find(
      (message) => message.action === "message.list",
    );
    if (!hydrationRequest) throw new Error("No queued message.list request was sent");

    acknowledge(reconnectedSocket, reconnectRequest);
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
