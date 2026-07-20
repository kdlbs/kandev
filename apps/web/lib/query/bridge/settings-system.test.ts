import { describe, expect, it } from "vitest";
import type { BackendMessageMap, BackendMessageType } from "@/lib/types/backend";
import type { BackendMessage } from "@/lib/types/backend-message";
import type { WebSocketClient } from "@/lib/ws/client";
import { makeQueryClient } from "../client";
import { qk } from "../keys";
import { registerSettingsSystemBridge } from "./settings-system";

type AnyBackendMessage = BackendMessage<string, Record<string, unknown>>;
type Handler = (message: AnyBackendMessage) => void;

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

const CREATED_AT = "2026-06-23T00:00:00Z";

describe("settings/system query bridge", () => {
  it("patches existing secrets cache without materializing a missing list", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    const registration = registerSettingsSystemBridge(
      ws as unknown as WebSocketClient,
      queryClient,
    );

    ws.emit({
      type: "notification",
      action: "secrets.created",
      payload: {
        id: "secret-1",
        name: "GITHUB_TOKEN",
        has_value: true,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    });

    expect(queryClient.getQueryData(qk.settings.secrets())).toBeUndefined();

    queryClient.setQueryData(qk.settings.secrets(), [
      {
        id: "secret-1",
        name: "OLD_TOKEN",
        has_value: true,
        created_at: CREATED_AT,
        updated_at: CREATED_AT,
      },
    ]);
    ws.emit({
      type: "notification",
      action: "secrets.updated",
      payload: {
        id: "secret-1",
        name: "GITHUB_TOKEN",
        has_value: true,
        created_at: CREATED_AT,
        updated_at: "2026-06-23T00:01:00Z",
      },
    });

    expect(queryClient.getQueryData(qk.settings.secrets())).toEqual([
      expect.objectContaining({ id: "secret-1", name: "GITHUB_TOKEN" }),
    ]);

    registration.cleanup();
  });

  it("does not materialize an empty secrets cache from delete events", () => {
    const ws = new FakeWebSocketClient();
    const queryClient = makeQueryClient();
    const registration = registerSettingsSystemBridge(
      ws as unknown as WebSocketClient,
      queryClient,
    );

    ws.emit({
      type: "notification",
      action: "secrets.deleted",
      payload: { id: "secret-1" },
    });

    expect(queryClient.getQueryData(qk.settings.secrets())).toBeUndefined();

    registration.cleanup();
  });
});
