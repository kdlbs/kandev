import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import { sessionId, taskId, type Message, type Turn } from "@/lib/types/http";
import { MessageActions } from "./message-actions";

const transport = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => transport,
}));

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const SOURCE_TASK = "native-fork-task";
const SOURCE_SESSION = "native-fork-source";
const TURN_ID = "completed-native-turn";
const TIMESTAMP = "2026-10-18T10:00:00Z";
const message: Message = {
  id: "native-fork-message",
  task_id: taskId(SOURCE_TASK),
  session_id: sessionId(SOURCE_SESSION),
  turn_id: TURN_ID,
  author_type: "agent",
  type: "message",
  content: "Completed native response",
  created_at: TIMESTAMP,
};
const completedTurn: Turn = {
  id: TURN_ID,
  task_id: taskId(SOURCE_TASK),
  session_id: sessionId(SOURCE_SESSION),
  created_at: TIMESTAMP,
  started_at: TIMESTAMP,
  updated_at: TIMESTAMP,
  completed_at: TIMESTAMP,
  metadata: { agent_type: "codex-app-server" },
};
const destination = {
  task_id: SOURCE_TASK,
  session_id: "native-fork-destination",
  state: "STARTING",
};
let store: StoreApi<AppState>;
let uuidDescriptor: PropertyDescriptor | undefined;

function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

function renderNativeFork(touch = false, enabled = true) {
  const matchMedia = window.matchMedia.bind(window);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => {
    const result = matchMedia(query);
    if (query === "(pointer: fine)") {
      Object.defineProperty(result, "matches", { configurable: true, value: !touch });
    }
    return result;
  });
  render(
    <StateProvider>
      <CaptureStore />
      <MessageActions message={message} />
    </StateProvider>,
  );
  act(() => {
    const state = store.getState();
    state.setActiveSession(SOURCE_TASK, SOURCE_SESSION);
    state.setFeatures({ ...state.features, codexAppServer: enabled });
    state.addTurn(completedTurn);
  });
}

function openConfirmation(touch = false) {
  fireEvent.click(screen.getByTestId("fork-conversation-trigger"));
  const dialog = screen.getByRole("dialog", { name: "Fork conversation" });
  expect(dialog.hasAttribute("data-vaul-drawer")).toBe(touch);
  expect(screen.getByText(/files remain shared/i)).toBeTruthy();
  fireEvent.click(screen.getByTestId("fork-conversation-confirm"));
}

function expectRequest(index = 0) {
  expect(transport.request.mock.calls[index]).toEqual([
    "session.fork",
    {
      task_id: SOURCE_TASK,
      session_id: SOURCE_SESSION,
      turn_id: TURN_ID,
      request_id: expect.stringMatching(UUID),
    },
    60_000,
  ]);
  return transport.request.mock.calls[index][1].request_id as string;
}

async function expectDestination() {
  await waitFor(() => {
    expect(store.getState().tasks.activeTaskId).toBe(destination.task_id);
    expect(store.getState().tasks.activeSessionId).toBe(destination.session_id);
  });
}

beforeEach(() => {
  uuidDescriptor = Object.getOwnPropertyDescriptor(crypto, "randomUUID");
  transport.request.mockReset();
  transport.request.mockResolvedValue(destination);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (uuidDescriptor) Object.defineProperty(crypto, "randomUUID", uuidDescriptor);
  else Reflect.deleteProperty(crypto, "randomUUID");
});

// @covers AC-AGENTS-CODEX-NATIVE-004.1, AC-AGENTS-CODEX-NATIVE-006.1
describe("native fork browser capability", () => {
  it.each([false, true])(
    "forks without randomUUID and selects the returned session on touch=%s",
    async (touch) => {
      Object.defineProperty(crypto, "randomUUID", { configurable: true, value: undefined });
      renderNativeFork(touch);
      openConfirmation(touch);

      await waitFor(() => expect(transport.request).toHaveBeenCalledTimes(1));
      expectRequest();
      await expectDestination();
    },
  );

  it("preserves the native fork outcome with randomUUID available", async () => {
    expect(typeof crypto.randomUUID).toBe("function");
    renderNativeFork();
    openConfirmation();

    await expectDestination();
    expect(transport.request).toHaveBeenCalledTimes(1);
    expectRequest();
  });

  it("hides native fork when the feature is disabled", () => {
    renderNativeFork(false, false);

    expect(screen.queryByTestId("fork-conversation-trigger")).toBeNull();
    expect(transport.request).not.toHaveBeenCalled();
    expect(store.getState().tasks.activeSessionId).toBe(SOURCE_SESSION);
  });

  it("retains the request ID and error feedback through a failed request and retry", async () => {
    transport.request.mockRejectedValueOnce(new Error("fork transport rejected"));
    renderNativeFork();
    openConfirmation();

    expect((await screen.findByRole("alert")).textContent).toBe(
      "Could not fork the conversation. The source session is unchanged.",
    );
    expect(store.getState().tasks.activeSessionId).toBe(SOURCE_SESSION);
    const firstId = expectRequest();
    expect(screen.getByTestId("fork-conversation-confirm").hasAttribute("disabled")).toBe(false);
    fireEvent.click(screen.getByTestId("fork-conversation-confirm"));

    await expectDestination();
    expect(transport.request).toHaveBeenCalledTimes(2);
    expect(expectRequest(1)).toBe(firstId);
    expect(screen.queryByRole("alert")).toBeNull();

    openConfirmation();
    await waitFor(() => expect(transport.request).toHaveBeenCalledTimes(3));
    expect(expectRequest(2)).not.toBe(firstId);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});
