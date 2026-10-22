import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { TabRenameInput } from "./tab-rename-input";
import { limitSessionName, useSessionRenameCommitter } from "./use-session-rename";

const { request } = vi.hoisted(() => ({ request: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request }) }));
let currentStore: ReturnType<typeof useAppStoreApi>;
const SESSION_ID = "session-unicode";
const TASK_ID = "task-unicode";
const ORIGINAL = "Original";
const RENAME_ACTION = "session.rename";
const onDone = vi.fn();
const onCancel = vi.fn();

function ActualRename({ initial }: { initial: string }) {
  currentStore = useAppStoreApi();
  const commit = useSessionRenameCommitter(SESSION_ID, TASK_ID, initial, onDone);
  return (
    <TabRenameInput
      initial={initial}
      seqBadge={null}
      limitValue={limitSessionName}
      onCommit={(next) => {
        void commit(next);
      }}
      onCancel={onCancel}
    />
  );
}

function mount(initial: string) {
  const session: TaskSession = {
    id: sessionId(SESSION_ID),
    task_id: taskId(TASK_ID),
    state: "WAITING_FOR_INPUT",
    name: initial,
    started_at: "2026-10-22T00:00:00Z",
    updated_at: "2026-10-22T00:00:00Z",
  };
  render(
    <StateProvider
      initialState={{
        taskSessions: { items: { [session.id]: session } },
      }}
    >
      <ToastProvider>
        <ActualRename initial={initial} />
      </ToastProvider>
    </StateProvider>,
  );
  return screen.getByTestId("tab-rename-input") as HTMLInputElement;
}
beforeEach(() => {
  request.mockReset();
  request.mockResolvedValue({ success: true });
  onDone.mockClear();
  onCancel.mockClear();
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// @covers AC-UI-TASK-AGENT-TAB-RECONCILIATION-002.5 and .6
// Only transport is mocked: the input, hook, API wrapper and store are real.
describe("actual session rename Unicode boundary", () => {
  it.each([
    "🧭".repeat(120),
    `${"x".repeat(119)}🧭`,
    "漢🧭".repeat(60),
    "漢".repeat(120),
    "x".repeat(120),
  ])("does not rename a legal unchanged server name", async (initial) => {
    expect(Array.from(initial)).toHaveLength(120);
    const input = mount(initial);
    expect(input.value).toBe(initial);
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(request).not.toHaveBeenCalled();
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(initial);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it.each([0, 60, 120])("preserves the ending for an overflowing insertion at %i", async (at) => {
    const initial = `${"漢".repeat(119)}🧭`;
    const points = Array.from(initial);
    const input = mount(initial);
    fireEvent.change(input, {
      target: {
        value: [...points.slice(0, at), "Z", ...points.slice(at)].join(""),
        selectionStart: points.slice(0, at).join("").length + 1,
      },
    });
    expect(input.value).toBe(initial);
    await act(async () => fireEvent.keyDown(input, { key: "Enter" }));
    expect(request).not.toHaveBeenCalled();
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(initial);
  });

  it("limits only inserted text while preserving the unselected ending", async () => {
    const initial = `${"漢".repeat(118)}🧭`;
    const input = mount(initial);
    fireEvent.change(input, { target: { value: `AB${initial}`, selectionStart: 2 } });
    expect(input.value).toBe(`A${initial}`);
    await act(async () => fireEvent.keyDown(input, { key: "Enter" }));
    expect(request).toHaveBeenCalledExactlyOnceWith(RENAME_ACTION, {
      session_id: SESSION_ID,
      name: `A${initial}`,
    });
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(`A${initial}`);
  });

  it("keeps an unchanged ASCII name without a write", async () => {
    const input = mount("Reviewer");
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(request).not.toHaveBeenCalled();
  });

  it("persists a changed trimmed ASCII name through the actual WS wrapper", async () => {
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: "  Reviewer  " } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(request).toHaveBeenCalledWith(RENAME_ACTION, {
      session_id: SESSION_ID,
      name: "Reviewer",
    });
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe("Reviewer");
  });
  it.each([
    ["non-BMP", "🧭".repeat(121), "🧭".repeat(120)],
    ["boundary pair", `${"x".repeat(119)}🧭tail`, `${"x".repeat(119)}🧭`],
    ["mixed", `${"漢🧭".repeat(60)}extra`, "漢🧭".repeat(60)],
    ["BMP", "漢".repeat(121), "漢".repeat(120)],
    ["combining code points", "e\u0301".repeat(61), "e\u0301".repeat(60)],
    ["empty reset", "   ", ""],
  ])("submits and stores the canonical %s edit once", async (_, next, expected) => {
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: next } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
      fireEvent.keyDown(input, { key: "Enter" });
      fireEvent.blur(input, { relatedTarget: document.body });
    });
    expect(request).toHaveBeenCalledExactlyOnceWith(RENAME_ACTION, {
      session_id: SESSION_ID,
      name: expected,
    });
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(expected);
    expect(onDone).toHaveBeenCalledTimes(1);
  });
});

describe("session rename completion and failure", () => {
  it("commits a Unicode edit on internal blur", async () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: "🧭".repeat(120) } });
    await act(async () => {
      fireEvent.blur(input, { relatedTarget: document.body });
    });
    expect(request).toHaveBeenCalledExactlyOnceWith(RENAME_ACTION, {
      session_id: SESSION_ID,
      name: "🧭".repeat(120),
    });
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe("🧭".repeat(120));
  });

  it("cancels a Unicode draft without a blur write", async () => {
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: "🧭".repeat(120) } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Escape" });
      fireEvent.blur(input, { relatedTarget: document.body });
    });
    expect(request).not.toHaveBeenCalled();
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(ORIGINAL);
  });

  it("leaves the original store after a failed Unicode write", async () => {
    request.mockRejectedValue(new Error("Rename unavailable"));
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: "🧭".repeat(120) } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
    });
    expect(request).toHaveBeenCalledExactlyOnceWith(RENAME_ACTION, {
      session_id: SESSION_ID,
      name: "🧭".repeat(120),
    });
    expect(currentStore.getState().taskSessions.items[SESSION_ID].name).toBe(ORIGINAL);
    expect(screen.getByText("Rename unavailable")).toBeTruthy();
  });

  it("limits the editing draft by code point without trimming it", () => {
    const input = mount(ORIGINAL);
    fireEvent.change(input, { target: { value: ` ${"🧭".repeat(120)}tail` } });
    expect(input.value).toBe(` ${"🧭".repeat(119)}`);
    expect(request).not.toHaveBeenCalled();
  });
});
