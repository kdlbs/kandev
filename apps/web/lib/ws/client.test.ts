/**
 * Focused tests for WebSocketClient.refreshSessionData — the deterministic
 * git-status refresh used when a poll-backed panel becomes the active tab.
 * It has two guards: it no-ops unless the socket is connected AND the session
 * is currently focused; otherwise it re-sends a `session.focus` frame WITHOUT
 * touching the focus ref-count.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { WebSocketClient } from "./client";

type SentFrame = { id: string; type: string; action: string; payload?: { session_id?: string } };

// Minimal WebSocket stand-in: records sends and exposes the lifecycle hooks the
// client wires up, so a test can drive the connection open synchronously.
class MockWebSocket {
  static instances: MockWebSocket[] = [];
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((event: unknown) => void) | null = null;
  sent: string[] = [];

  constructor(public url: string) {
    MockWebSocket.instances.push(this);
  }

  send(data: string) {
    this.sent.push(data);
  }

  close() {
    this.onclose?.({});
  }
}

const SESSION_ID = "sess-1";
const ACTION_FOCUS = "session.focus";
const ACTION_UNFOCUS = "session.unfocus";

function sentFrames(socket: MockWebSocket): SentFrame[] {
  return socket.sent.map((raw) => JSON.parse(raw) as SentFrame);
}

function framesFor(socket: MockWebSocket, action: string): SentFrame[] {
  return sentFrames(socket).filter((f) => f.action === action);
}

describe("WebSocketClient.refreshSessionData", () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal("WebSocket", MockWebSocket as unknown as typeof WebSocket);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function connectedClient(): { client: WebSocketClient; socket: MockWebSocket } {
    const client = new WebSocketClient("ws://test");
    client.connect();
    const socket = MockWebSocket.instances[0];
    socket.onopen?.();
    return { client, socket };
  }

  it("no-ops when the connection is absent (never connected)", () => {
    const client = new WebSocketClient("ws://test");
    // No connect() → status is "disconnected". The focus count is also empty,
    // but the connection guard must short-circuit first.
    client.refreshSessionData(SESSION_ID);
    expect(MockWebSocket.instances).toHaveLength(0);
  });

  it("no-ops when the session is not currently focused", () => {
    const { client, socket } = connectedClient();
    const before = socket.sent.length;
    // Connected, but no focusSession() call → focus count is 0.
    client.refreshSessionData(SESSION_ID);
    expect(socket.sent.length).toBe(before);
    expect(framesFor(socket, ACTION_FOCUS)).toHaveLength(0);
  });

  it("re-sends a session.focus frame for a focused session without changing the ref-count", () => {
    const { client, socket } = connectedClient();

    // Focus once → sends the initial focus frame (0→1 transition).
    client.focusSession(SESSION_ID);
    expect(framesFor(socket, ACTION_FOCUS)).toHaveLength(1);

    // Refresh → re-sends the focus frame.
    client.refreshSessionData(SESSION_ID);
    const focusFrames = framesFor(socket, ACTION_FOCUS);
    expect(focusFrames).toHaveLength(2);
    expect(focusFrames[1].payload?.session_id).toBe(SESSION_ID);
    expect(focusFrames[1].type).toBe("request");

    // The ref-count was untouched: a single unfocus drops to 0 and emits one
    // unfocus frame (proving refresh did not bump the count to 2).
    client.unfocusSession(SESSION_ID);
    expect(framesFor(socket, ACTION_UNFOCUS)).toHaveLength(1);
    expect(framesFor(socket, ACTION_FOCUS)).toHaveLength(2);
  });
});
