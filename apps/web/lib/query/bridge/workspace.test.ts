import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerWorkspaceBridge } from "./workspace";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;
const WORKFLOW_ID = "workflow-1";

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

describe("workspace query bridge", () => {
  it("invalidates workflow step query data for workflow step events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    queryClient.setQueryData(qk.workflows.steps(WORKFLOW_ID), [
      { id: "step-1", workflow_id: WORKFLOW_ID, name: "Todo", position: 1 },
    ]);
    queryClient.setQueryData(qk.workflows.snapshot(WORKFLOW_ID), {
      workflow: { id: WORKFLOW_ID },
      steps: [],
      tasks: [],
    });

    const registration = registerWorkspaceBridge(ws as unknown as WebSocketClient, queryClient);
    ws.emit({
      type: "notification",
      action: "workflow.step.updated",
      payload: {
        step: {
          id: "step-1",
          workflow_id: WORKFLOW_ID,
          name: "Doing",
          position: 1,
          state: "active",
          color: "#00f",
        },
      },
    });

    expect(queryClient.getQueryState(qk.workflows.steps(WORKFLOW_ID))?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(qk.workflows.snapshot(WORKFLOW_ID))?.isInvalidated).toBe(true);

    registration.cleanup();
  });
});
