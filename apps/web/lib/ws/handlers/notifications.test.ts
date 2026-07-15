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
const MESSAGE_TITLE = "Task needs your input";
const MESSAGE_BODY = "An agent is waiting for your input.";
const notificationMock = vi.fn();

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    turns: { bySession: { [SESSION_ID]: [{ id: "turn-1" }] } },
    tasks: { activeTaskId: OTHER_TASK_ID },
    ...overrides,
  } as unknown as AppState;

  return { getState: () => state } as unknown as StoreApi<AppState>;
}

function makeMessage(id = "message-1"): BackendMessageMap["session.waiting_for_input"] {
  return {
    id,
    type: "notification",
    action: "session.waiting_for_input",
    payload: {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      title: MESSAGE_TITLE,
      body: MESSAGE_BODY,
    },
  };
}

function getHandler(store: StoreApi<AppState>) {
  return registerNotificationsHandlers(store)["session.waiting_for_input"]!;
}

describe("session.waiting_for_input handler", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      "Notification",
      Object.assign(notificationMock, { permission: "granted" as NotificationPermission }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    delete (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
  });

  it("delegates event deduplication to the native notification command", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };

    const handler = getHandler(makeStore());
    handler(makeMessage());
    handler(makeMessage());
    handler(makeMessage("message-2"));

    expect(invoke).toHaveBeenCalledTimes(3);
    expect(invoke).toHaveBeenNthCalledWith(1, "show_native_notification", {
      request: {
        eventId: "session.waiting_for_input:message-1",
        title: MESSAGE_TITLE,
        body: MESSAGE_BODY,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      },
    });
    expect(invoke).toHaveBeenNthCalledWith(2, "show_native_notification", {
      request: {
        eventId: "session.waiting_for_input:message-1",
        title: MESSAGE_TITLE,
        body: MESSAGE_BODY,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      },
    });
    expect(invoke).toHaveBeenNthCalledWith(3, "show_native_notification", {
      request: {
        eventId: "session.waiting_for_input:message-2",
        title: MESSAGE_TITLE,
        body: MESSAGE_BODY,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      },
    });
    expect(notificationMock).not.toHaveBeenCalled();
    expect(playWaitingForInputSound).toHaveBeenCalledTimes(3);
  });

  it("plays the sound and shows a notification when not suppressed", () => {
    getHandler(makeStore())(makeMessage());

    expect(playWaitingForInputSound).toHaveBeenCalledTimes(1);
    expect(notificationMock).toHaveBeenCalledWith(MESSAGE_TITLE, {
      body: MESSAGE_BODY,
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

describe("session.waiting_for_input malformed payloads", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal(
      "Notification",
      Object.assign(notificationMock, { permission: "granted" as NotificationPermission }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    delete (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__;
  });

  it("uses the browser notification fallback when an event lacks a task ID", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    const message = {
      ...makeMessage(),
      payload: { ...makeMessage().payload, task_id: undefined },
    } as unknown as BackendMessageMap["session.waiting_for_input"];

    getHandler(makeStore())(message);

    expect(invoke).not.toHaveBeenCalled();
    expect(notificationMock).toHaveBeenCalledWith(MESSAGE_TITLE, { body: MESSAGE_BODY });
  });
});
