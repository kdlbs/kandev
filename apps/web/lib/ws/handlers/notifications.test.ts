import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
  NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
  NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE,
} from "@/lib/notifications/events";
import type { BackendMessageMap } from "@/lib/types/backend";
import { registerNotificationsHandlers } from "./notifications";

vi.mock("@/lib/notifications/sound", () => ({
  playWaitingForInputSound: vi.fn(),
}));

import { playWaitingForInputSound } from "@/lib/notifications/sound";

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const OTHER_TASK_ID = "task-2";
const MESSAGE_TITLE = "Agent needs your answer";
const MESSAGE_BODY = 'The agent asked a question on "Task one".';
const notificationMock = vi.fn();
type SemanticNotificationMessage =
  | BackendMessageMap["session.clarification_requested"]
  | BackendMessageMap["session.turn_finished"]
  | BackendMessageMap["office.inbox_item"];

function makeStore(overrides: Partial<AppState> = {}) {
  const state = {
    turns: { bySession: { [SESSION_ID]: [{ id: "turn-1" }] } },
    tasks: { activeTaskId: OTHER_TASK_ID },
    ...overrides,
  } as unknown as AppState;

  return { getState: () => state } as unknown as StoreApi<AppState>;
}

function makeMessage(
  id = "message-1",
  action:
    | "session.turn_finished"
    | "session.clarification_requested"
    | "office.inbox_item" = "session.clarification_requested",
  title = MESSAGE_TITLE,
  body = MESSAGE_BODY,
):
  | BackendMessageMap["session.clarification_requested"]
  | BackendMessageMap["session.turn_finished"]
  | BackendMessageMap["office.inbox_item"] {
  return {
    id,
    type: "notification",
    action,
    payload: {
      task_id: TASK_ID,
      session_id: SESSION_ID,
      occurrence_id: "pending-1",
      title,
      body,
    },
  } as SemanticNotificationMessage;
}

function getHandler(
  store: StoreApi<AppState>,
  action:
    | "session.turn_finished"
    | "session.clarification_requested"
    | "office.inbox_item" = "session.clarification_requested",
) {
  return (
    registerNotificationsHandlers(store) as unknown as Record<
      string,
      (message: SemanticNotificationMessage) => void
    >
  )[action]!;
}

describe("notification handler registration", () => {
  it("registers handlers for both semantic notification events", () => {
    const handlers = registerNotificationsHandlers(makeStore()) as Record<string, unknown>;

    expect(handlers[NOTIFICATION_EVENT_SESSION_TURN_FINISHED]).toEqual(expect.any(Function));
    expect(handlers[NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]).toEqual(
      expect.any(Function),
    );
    expect(handlers["office.inbox_item"]).toEqual(expect.any(Function));
    expect(handlers[NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]).toEqual(expect.any(Function));
  });

  it("routes a Local update occurrence into the in-app toast bridge", () => {
    const setUpdateAvailableNotification = vi.fn();
    const store = makeStore({ setUpdateAvailableNotification } as unknown as Partial<AppState>);
    const handler = (
      registerNotificationsHandlers(store) as Record<string, (message: unknown) => void>
    )[NOTIFICATION_EVENT_SYSTEM_UPDATE_AVAILABLE]!;
    const payload = {
      version: "v1.2.3",
      url: "https://example.test/releases/v1.2.3",
      title: "Kandev update available",
      body: "Kandev v1.2.3 is available.",
      occurrence_id: "v1.2.3",
    };

    handler({ payload });

    expect(setUpdateAvailableNotification).toHaveBeenCalledWith(payload);
  });
});

describe("semantic session notification handlers", () => {
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
        eventId: "session.clarification_requested:message-1",
        title: MESSAGE_TITLE,
        body: MESSAGE_BODY,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      },
    });
    expect(invoke).toHaveBeenNthCalledWith(2, "show_native_notification", {
      request: {
        eventId: "session.clarification_requested:message-1",
        title: MESSAGE_TITLE,
        body: MESSAGE_BODY,
        taskId: TASK_ID,
        sessionId: SESSION_ID,
      },
    });
    expect(invoke).toHaveBeenNthCalledWith(3, "show_native_notification", {
      request: {
        eventId: "session.clarification_requested:message-2",
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

  it("suppresses both channels while the user is viewing the task", () => {
    const store = makeStore({ tasks: { activeTaskId: TASK_ID } } as unknown as Partial<AppState>);

    getHandler(store)(makeMessage());

    expect(playWaitingForInputSound).not.toHaveBeenCalled();
    expect(notificationMock).not.toHaveBeenCalled();
  });
});

describe("semantic session notification malformed payloads", () => {
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
    } as unknown as BackendMessageMap["session.clarification_requested"];

    getHandler(makeStore())(message);

    expect(invoke).not.toHaveBeenCalled();
    expect(notificationMock).toHaveBeenCalledWith(MESSAGE_TITLE, { body: MESSAGE_BODY });
  });

  it("uses the browser notification fallback when a semantic event lacks every occurrence identity", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    const message = {
      ...makeMessage(),
      id: undefined,
      payload: { ...makeMessage().payload, occurrence_id: undefined },
    } as unknown as BackendMessageMap["session.clarification_requested"];

    getHandler(makeStore())(message);

    expect(invoke).not.toHaveBeenCalled();
    expect(notificationMock).toHaveBeenCalledWith(MESSAGE_TITLE, { body: MESSAGE_BODY });
  });

  it("uses the semantic occurrence as the native identity when the envelope ID is absent", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };
    const message = {
      ...makeMessage(),
      id: undefined,
      payload: { ...makeMessage().payload, occurrence_id: "pending-1" },
    } as unknown as BackendMessageMap["session.clarification_requested"];

    getHandler(makeStore())(message);

    expect(invoke).toHaveBeenCalledWith("show_native_notification", {
      request: expect.objectContaining({
        eventId: `${NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED}:pending-1`,
      }),
    });
  });

  it("keeps turn-finished copy and identity through native delivery", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };

    getHandler(
      makeStore(),
      "session.turn_finished",
    )(
      makeMessage(
        "turn-1",
        "session.turn_finished",
        "Agent turn finished",
        'The agent finished a turn on "Task one".',
      ),
    );

    expect(invoke).toHaveBeenCalledWith("show_native_notification", {
      request: expect.objectContaining({
        eventId: "session.turn_finished:turn-1",
        title: "Agent turn finished",
        body: 'The agent finished a turn on "Task one".',
      }),
    });
  });
});

describe("office inbox notification delivery", () => {
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

  it("delivers office inbox items through the browser fallback", () => {
    const invoke = vi.fn().mockResolvedValue("shown");
    (window as Window & { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__ = {
      invoke,
      transformCallback: vi.fn(),
    };

    const message = {
      ...makeMessage("inbox-1", "office.inbox_item", "", ""),
      payload: { session_id: undefined, title: "", body: "" },
    } as BackendMessageMap["office.inbox_item"];

    getHandler(makeStore(), "office.inbox_item")(message);

    expect(invoke).not.toHaveBeenCalled();
    expect(notificationMock).toHaveBeenCalledWith("New inbox item", {
      body: "A new item needs your attention.",
    });
  });
});
