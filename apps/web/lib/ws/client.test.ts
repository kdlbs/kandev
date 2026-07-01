import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketClient } from "./client";

type MockMessageHandler = (event: MessageEvent<string>) => void;

class MockWebSocket {
  static instances: MockWebSocket[] = [];

  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: MockMessageHandler | null = null;
  onopen: (() => void) | null = null;
  send = vi.fn();

  constructor(readonly url: string) {
    MockWebSocket.instances.push(this);
  }

  close() {
    this.onclose?.({ code: 1000, reason: "test" } as CloseEvent);
  }

  emit(data: unknown) {
    this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent<string>);
  }
}

describe("WebSocketClient envelope subscriptions", () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("notifies envelope subscribers for response and notification frames", () => {
    const client = new WebSocketClient("ws://example.test/socket", undefined, {
      enabled: false,
    });
    const seen: string[] = [];

    const unsubscribe = client.onEnvelope((message) => {
      seen.push(message.action);
    });
    client.connect();
    const socket = MockWebSocket.instances[0];

    socket.emit({
      id: "response-1",
      type: "response",
      action: "session.subscribe",
      payload: { session_id: "session-1" },
    });
    socket.emit({
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: "task-1",
        workflow_id: "workflow-1",
        workflow_step_id: "step-1",
        title: "Updated",
        is_ephemeral: false,
      },
    });

    unsubscribe();
    socket.emit({
      type: "notification",
      action: "workspace.updated",
      payload: { id: "workspace-1", name: "Workspace" },
    });

    expect(seen).toEqual(["session.subscribe", "task.updated"]);
  });
});
