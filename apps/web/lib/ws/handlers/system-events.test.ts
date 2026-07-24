import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerSystemEventsHandlers } from "./system-events";

function makeStore() {
  const state = {
    setUpdateAvailableNotification: vi.fn(),
  } as unknown as AppState;

  return { getState: () => state } as unknown as StoreApi<AppState>;
}

function makeMessage(
  payload: BackendMessageMap["system.update_available"]["payload"],
): BackendMessageMap["system.update_available"] {
  return {
    id: "msg-1",
    type: "notification",
    action: "system.update_available",
    payload,
    timestamp: new Date().toISOString(),
  } as BackendMessageMap["system.update_available"];
}

describe("system.update_available handler", () => {
  it("stores the notification so the toast bridge hook can consume it", () => {
    const store = makeStore();
    const handler = registerSystemEventsHandlers(store)["system.update_available"]!;

    handler(makeMessage({ version: "v1.2.3", url: "https://example/v1.2.3" }));

    expect(store.getState().setUpdateAvailableNotification).toHaveBeenCalledWith({
      version: "v1.2.3",
      url: "https://example/v1.2.3",
    });
  });
});
