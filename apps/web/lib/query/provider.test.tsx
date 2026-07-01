import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { WebSocketClient } from "@/lib/ws/client";
import { setWebSocketClient } from "@/lib/ws/connection";
import { makeQueryClient } from "./client";
import { qk } from "./keys";
import { QueryProvider } from "./provider";

type E2EWindow = Window & {
  __KANDEV_E2E_EXPOSE_STORE__?: boolean;
  __KANDEV_E2E_QUERY_CLIENT__?: ReturnType<typeof makeQueryClient>;
};

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;

class FakeWebSocketClient {
  private envelopeHandlers = new Set<Handler>();
  private handlers = new Map<string, Set<Handler>>();

  on<T extends BackendMessageType>(type: T, handler: (message: BackendMessageMap[T]) => void) {
    const bucket = this.handlers.get(type) ?? new Set<Handler>();
    bucket.add(handler as Handler);
    this.handlers.set(type, bucket);
    return () => {
      bucket.delete(handler as Handler);
    };
  }

  onEnvelope(handler: (message: BackendMessageMap[BackendMessageType]) => void) {
    this.envelopeHandlers.add(handler as Handler);
    return () => {
      this.envelopeHandlers.delete(handler as Handler);
    };
  }

  emit(message: AnyBackendMessage) {
    this.envelopeHandlers.forEach((handler) => handler(message));
    this.handlers.get(message.action)?.forEach((handler) => handler(message));
  }
}

describe("QueryProvider", () => {
  beforeEach(() => {
    delete (window as E2EWindow).__KANDEV_E2E_EXPOSE_STORE__;
    delete (window as E2EWindow).__KANDEV_E2E_QUERY_CLIENT__;
    setWebSocketClient(null);
  });

  it("renders children inside the query client provider", () => {
    render(
      <QueryProvider client={makeQueryClient()}>
        <div>child</div>
      </QueryProvider>,
    );

    expect(screen.getByText("child")).toBeTruthy();
  });

  it("exposes the query client only when E2E store exposure is enabled", async () => {
    const client = makeQueryClient();
    (window as E2EWindow).__KANDEV_E2E_EXPOSE_STORE__ = true;

    render(
      <QueryProvider client={client}>
        <div>child</div>
      </QueryProvider>,
    );

    await waitFor(() => {
      expect((window as E2EWindow).__KANDEV_E2E_QUERY_CLIENT__).toBe(client);
    });
  });

  it("registers the query bridge when the WebSocket client is installed later", async () => {
    const client = makeQueryClient();
    const ws = new FakeWebSocketClient();
    client.setQueryData(qk.tasks.detail("task-1"), { id: "task-1", title: "Old title" });

    render(
      <QueryProvider client={client}>
        <div>child</div>
      </QueryProvider>,
    );

    setWebSocketClient(ws as unknown as WebSocketClient);
    ws.emit({
      type: "notification",
      action: "task.updated",
      payload: {
        task_id: "task-1",
        workflow_id: "workflow-1",
        workflow_step_id: "step-1",
        title: "Updated title",
        is_ephemeral: false,
      },
    });

    await waitFor(() => {
      expect(client.getQueryData(qk.tasks.detail("task-1"))).toMatchObject({
        title: "Updated title",
      });
    });
  });
});
