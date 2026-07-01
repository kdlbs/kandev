import { expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerSessionRuntimeBridge } from "./session-runtime";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;
type BridgeHarness = {
  ws: FakeWebSocketClient;
  queryClient: ReturnType<typeof makeQueryClient>;
  cleanup: () => void;
};

class FakeWebSocketClient {
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

function setupBridge(): BridgeHarness {
  const ws = new FakeWebSocketClient();
  const queryClient = makeQueryClient();
  const registration = registerSessionRuntimeBridge(ws as unknown as WebSocketClient, queryClient);
  return { ws, queryClient, cleanup: registration.cleanup };
}

it("patches prompt usage and todo caches from runtime events", () => {
  const { ws, queryClient, cleanup } = setupBridge();

  ws.emit({
    type: "notification",
    action: "session.prompt_usage",
    payload: {
      task_id: "task-1",
      session_id: "session-1",
      agent_id: "agent-1",
      usage: {
        input_tokens: 100,
        output_tokens: 25,
        cached_read_tokens: 10,
        total_tokens: 125,
      },
      timestamp: "2026-06-23T00:00:05Z",
    },
  });
  ws.emit({
    type: "notification",
    action: "session.todos_updated",
    payload: {
      task_id: "task-1",
      session_id: "session-1",
      agent_id: "agent-1",
      entries: [
        {
          description: "Run verification",
          status: "completed",
          priority: "high",
        },
      ],
      timestamp: "2026-06-23T00:00:07Z",
    },
  });

  expect(queryClient.getQueryData(qk.sessionRuntime.promptUsage("session-1"))).toMatchObject({
    inputTokens: 100,
    outputTokens: 25,
    cachedReadTokens: 10,
    totalTokens: 125,
  });
  expect(queryClient.getQueryData(qk.sessionRuntime.todos("session-1"))).toEqual([
    {
      description: "Run verification",
      status: "completed",
      priority: "high",
    },
  ]);

  cleanup();
});

it("patches agent capabilities and poll mode caches from runtime events", () => {
  const { ws, queryClient, cleanup } = setupBridge();

  ws.emit({
    type: "notification",
    action: "session.agent_capabilities",
    payload: {
      task_id: "task-1",
      session_id: "session-1",
      agent_id: "agent-1",
      supports_image: true,
      supports_audio: false,
      supports_embedded_context: true,
      auth_methods: [
        {
          id: "login",
          name: "Login",
          description: "Authenticate in terminal",
          terminal_auth: {
            command: "agent",
            args: ["login"],
            label: "Run login",
          },
        },
      ],
      timestamp: "2026-06-23T00:00:06Z",
    },
  });
  ws.emit({
    type: "notification",
    action: "session.poll_mode_changed",
    payload: {
      task_id: "task-1",
      session_id: "session-1",
      poll_mode: "slow",
      reason: "subscribed",
      timestamp: "2026-06-23T00:00:08Z",
    },
  });

  expect(queryClient.getQueryData(qk.sessionRuntime.agentCapabilities("session-1"))).toEqual({
    supportsImage: true,
    supportsAudio: false,
    supportsEmbeddedContext: true,
    authMethods: [
      {
        id: "login",
        name: "Login",
        description: "Authenticate in terminal",
        terminalAuth: {
          command: "agent",
          args: ["login"],
          label: "Run login",
        },
        meta: undefined,
      },
    ],
  });
  expect(queryClient.getQueryData(qk.sessionRuntime.pollMode("session-1"))).toBe("slow");

  cleanup();
});
