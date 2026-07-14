import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerNotificationsHandlers } from "./notifications";

vi.mock("@/lib/notifications/sound", () => ({
  playWaitingForInputSound: vi.fn(),
}));

import { playWaitingForInputSound } from "@/lib/notifications/sound";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const OTHER_TASK_ID = "task-2";

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    turns: { bySession: { [SESSION_ID]: [{ id: "turn-1" }] } },
    tasks: { activeTaskId: OTHER_TASK_ID },
    ...overrides,
  } as unknown as AppState;

  return { getState: () => state } as unknown as StoreApi<AppState>;
}

function makeMessage(): BackendMessageMap["session.waiting_for_input"] {
  return {
    id: "message-1",
    type: "notification",
    action: "session.waiting_for_input",
    payload: {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      title: "Task needs your input",
      body: "An agent is waiting for your input.",
    },
  };
}

function getHandler(store: StoreApi<AppState>) {
  return registerNotificationsHandlers(store)["session.waiting_for_input"]!;
}

describe("session.waiting_for_input handler", () => {
  const notificationMock = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      "Notification",
      Object.assign(notificationMock, { permission: "granted" as NotificationPermission }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("plays the sound and shows a notification when not suppressed", () => {
    getHandler(makeStore())(makeMessage());

    expect(playWaitingForInputSound).toHaveBeenCalledTimes(1);
    expect(notificationMock).toHaveBeenCalledWith("Task needs your input", {
      body: "An agent is waiting for your input.",
    });
  });

  it("plays the sound even when notification permission is not granted", () => {
    vi.stubGlobal(
      "Notification",
      Object.assign(notificationMock, { permission: "denied" as NotificationPermission }),
    );

    getHandler(makeStore())(makeMessage());

    expect(playWaitingForInputSound).toHaveBeenCalledTimes(1);
    expect(notificationMock).not.toHaveBeenCalled();
  });

  it("plays the sound when the Notification API is unavailable", () => {
    vi.unstubAllGlobals();
    vi.stubGlobal("Notification", undefined);

    getHandler(makeStore())(makeMessage());

    expect(playWaitingForInputSound).toHaveBeenCalledTimes(1);
  });

  it("suppresses both channels while the session has no completed turns", () => {
    const store = makeStore({ turns: { bySession: {} } } as unknown as Partial<AppState>);

    getHandler(store)(makeMessage());

    expect(playWaitingForInputSound).not.toHaveBeenCalled();
    expect(notificationMock).not.toHaveBeenCalled();
  });

  it("suppresses both channels while the user is viewing the task", () => {
    const store = makeStore({ tasks: { activeTaskId: TASK_ID } } as unknown as Partial<AppState>);

    getHandler(store)(makeMessage());

    expect(playWaitingForInputSound).not.toHaveBeenCalled();
    expect(notificationMock).not.toHaveBeenCalled();
  });
});
