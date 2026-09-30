import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RunRead } from "@/lib/api/domains/coordinator-autonomy-api";
import { resetWakeRunStore } from "@/lib/coordinator/wake-run-store";
import { sessionId, taskId, type Message } from "@/lib/types/http";

const flags = vi.hoisted(() => ({ phase3: true }));
const getRun = vi.fn();
const fetchTask = vi.fn();
let conversationTask: {
  id: string;
  workspaceId: string;
  metadata: Record<string, unknown>;
} | null = null;

vi.mock("@/hooks/domains/settings/use-coordinator-phase3-effective", () => ({
  useCoordinatorPhase3Effective: () => flags.phase3,
}));
vi.mock("@/hooks/domains/kanban/use-task-by-id", () => ({
  useTaskById: () => conversationTask,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...a: unknown[]) => fetchTask(...a),
}));
vi.mock("@/lib/api/domains/coordinator-autonomy-api", () => ({
  getRun: (...a: unknown[]) => getRun(...a),
}));
type Handler = (message: { payload: Record<string, unknown> }) => void;
let handlers: Handler[] = [];
vi.mock("@/lib/ws/connection", () => ({
  useWebSocketClient: () => ({
    on: (_event: string, handler: Handler) => {
      handlers.push(handler);
      return () => {
        handlers = handlers.filter((h) => h !== handler);
      };
    },
  }),
}));

import { WokenByEntry, wakeTurnIdOf } from "./woken-by-entry";

const BODY = "Unattended turn. No person started this turn.";

function message(
  metadata: Record<string, unknown> | undefined = { coordinator_wake_turn_id: "t1" },
) {
  return {
    id: "m1",
    session_id: sessionId("s1"),
    task_id: taskId("conv-1"),
    author_type: "user",
    content: BODY,
    type: "message",
    created_at: "2026-09-30T10:00:00Z",
    metadata,
  } as Message;
}

function run(over: Partial<RunRead> = {}): RunRead {
  return {
    id: "t1",
    coordinator_id: "c1",
    conversation_task_id: "conv-1",
    session_id: "s1",
    started_at: "2026-09-30T10:00:00Z",
    finished_at: null,
    outcome: null,
    wake_count: 2,
    denied_permissions: 0,
    cost_subcents: null,
    stop_requested_at: null,
    stop_state: null,
    wakes: [
      {
        id: "w1",
        kind: "question",
        task_id: "task-1",
        task_identifier: "KAN-1",
        task_title: "Pick a DB",
      },
      { id: "w2", kind: "stall", task_id: "task-2", task_identifier: null, task_title: null },
    ],
    ...over,
  };
}

function renderEntry(turnId = "t1") {
  return render(
    <WokenByEntry
      comment={message()}
      turnId={turnId}
      fallback={<div data-testid="ordinary">{BODY}</div>}
    />,
  );
}

const flush = () => act(async () => void (await Promise.resolve()));

beforeEach(() => {
  flags.phase3 = true;
  handlers = [];
  resetWakeRunStore();
  getRun.mockReset();
  fetchTask.mockReset();
  conversationTask = { id: "conv-1", workspaceId: "w1", metadata: { coordinator_id: "c1" } };
});
afterEach(cleanup);

const ENTRY = "woken-by-entry";
const HEADER = "woken-by-header";
const DATA_STATE = "data-state";

describe("wakeTurnIdOf", () => {
  it("accepts only a non-empty string", () => {
    expect(wakeTurnIdOf({ coordinator_wake_turn_id: "t1" })).toBe("t1");
    expect(wakeTurnIdOf({ coordinator_wake_turn_id: "" })).toBeNull();
    expect(wakeTurnIdOf({ coordinator_wake_turn_id: 7 })).toBeNull();
    expect(wakeTurnIdOf({})).toBeNull();
    expect(wakeTurnIdOf(undefined)).toBeNull();
  });
});

describe("WokenByEntry", () => {
  it("is the ordinary message while phase 3 is not effective, with no read", () => {
    flags.phase3 = false;
    renderEntry();
    expect(screen.getByTestId("ordinary")).toBeTruthy();
    expect(screen.queryByTestId(ENTRY)).toBeNull();
    expect(getRun).not.toHaveBeenCalled();
  });

  it("shows a skeleton until the first read settles", async () => {
    getRun.mockReturnValue(new Promise(() => undefined));
    renderEntry();
    expect(screen.getByTestId(ENTRY).getAttribute(DATA_STATE)).toBe("loading");
  });

  it("renders the loaded header from the run, with the list collapsed by default", async () => {
    getRun.mockResolvedValue(run());
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(ENTRY).getAttribute(DATA_STATE)).toBe("loaded"));
    expect(getRun).toHaveBeenCalledWith("w1", "c1", "t1");
    expect(screen.getByTestId(HEADER).textContent).toBe("Woken by 2 events");
    expect(screen.queryByTestId("woken-by-denied")).toBeNull();
    expect(screen.queryByText(BODY)).toBeNull();
    const toggle = screen.getByTestId("woken-by-toggle");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByTestId("woken-by-list")).toBeNull();
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    const rows = screen.getAllByTestId("woken-by-row").map((r) => r.textContent);
    expect(rows).toEqual(["questionKAN-1Pick a DB", "stalltask-2"]);
  });

  it("uses the singular header and shows the denied count only above zero", async () => {
    getRun.mockResolvedValue(run({ wake_count: 1, denied_permissions: 3 }));
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(HEADER)).toBeTruthy());
    expect(screen.getByTestId(HEADER).textContent).toBe("Woken by 1 event");
    expect(screen.getByTestId("woken-by-denied").textContent).toBe("3 permissions denied");
  });

  it("uses wake_count for the header even when wakes holds fewer rows", async () => {
    getRun.mockResolvedValue(run({ wake_count: 9, wakes: [] }));
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(HEADER)).toBeTruthy());
    expect(screen.getByTestId(HEADER).textContent).toBe("Woken by 9 events");
  });
});

describe("WokenByEntry failure and refresh", () => {
  it("falls to Unavailable on a failed first read, revealing the message text behind the toggle with no retry", async () => {
    getRun.mockRejectedValue(new Error("404"));
    renderEntry();
    await waitFor(() =>
      expect(screen.getByTestId(ENTRY).getAttribute(DATA_STATE)).toBe("unavailable"),
    );
    expect(screen.getByTestId(HEADER).textContent).toBe("Woken by events");
    expect(screen.queryByText(BODY)).toBeNull();
    fireEvent.click(screen.getByTestId("woken-by-toggle"));
    expect(screen.getByTestId("woken-by-body").textContent).toBe(BODY);
    expect(screen.queryByRole("button", { name: /try again|retry/i })).toBeNull();
  });

  it("is Unavailable without a read when the conversation task carries no coordinator id", async () => {
    conversationTask = { id: "conv-1", workspaceId: "w1", metadata: {} };
    renderEntry();
    expect(screen.getByTestId(ENTRY).getAttribute(DATA_STATE)).toBe("unavailable");
    expect(getRun).not.toHaveBeenCalled();
  });

  it("fetches the conversation task when it is not loaded", async () => {
    conversationTask = null;
    fetchTask.mockResolvedValue({ workspace_id: "w1", metadata: { coordinator_id: "c1" } });
    getRun.mockResolvedValue(run());
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(ENTRY).getAttribute(DATA_STATE)).toBe("loaded"));
    expect(fetchTask).toHaveBeenCalledWith("conv-1");
    expect(getRun).toHaveBeenCalledWith("w1", "c1", "t1");
  });

  it("re-reads an open run on autonomy_changed and keeps loaded data when the re-read fails", async () => {
    getRun.mockResolvedValueOnce(run());
    renderEntry("t-open");
    await waitFor(() => expect(screen.getByTestId(HEADER).textContent).toContain("2"));
    getRun.mockResolvedValueOnce(run({ wake_count: 4, outcome: "completed" }));
    act(() =>
      handlers.forEach((h) => h({ payload: { coordinator_id: "c1", autonomy_changed: true } })),
    );
    await waitFor(() => expect(screen.getByTestId(HEADER).textContent).toBe("Woken by 4 events"));
    getRun.mockClear();
    act(() =>
      handlers.forEach((h) => h({ payload: { coordinator_id: "c1", autonomy_changed: true } })),
    );
    await flush();
    expect(getRun).not.toHaveBeenCalled();
  });

  it("ignores updates without autonomy_changed or for another coordinator", async () => {
    getRun.mockResolvedValue(run());
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(HEADER)).toBeTruthy());
    getRun.mockClear();
    act(() => handlers.forEach((h) => h({ payload: { coordinator_id: "c1" } })));
    act(() =>
      handlers.forEach((h) => h({ payload: { coordinator_id: "c2", autonomy_changed: true } })),
    );
    await flush();
    expect(getRun).not.toHaveBeenCalled();
  });

  it("keeps the toggle and rows inside the bubble", async () => {
    getRun.mockResolvedValue(run());
    renderEntry();
    await waitFor(() => expect(screen.getByTestId(ENTRY)).toBeTruthy());
    const entry = screen.getByTestId(ENTRY);
    expect(entry.className).toContain("overflow-hidden");
    expect(entry.className).toContain("max-w-full");
    expect(screen.getByTestId("woken-by-toggle").className).toContain("min-h-11");
  });
});
