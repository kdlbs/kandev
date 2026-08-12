import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";
import type { TaskLspLanguageSnapshot } from "@/lib/types/http-lsp";

const mocks = vi.hoisted(() => {
  const listeners = new Set<(key: string) => void>();
  return {
    connect: vi.fn(() => vi.fn()),
    getStatus: vi.fn<() => LspStatus>(() => ({ state: "disabled" })),
    onChange: vi.fn((listener: (key: string) => void) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }),
    start: vi.fn().mockResolvedValue(undefined),
    stop: vi.fn().mockResolvedValue(undefined),
    restart: vi.fn().mockResolvedValue(undefined),
    setPolicy: vi.fn().mockResolvedValue(undefined),
    userConfigs: { typescript: { diagnostics: true } },
    storeState: {
      tasks: { activeTaskId: "task-1" as string | null },
      taskSessions: {
        items: { "session-1": { task_id: "task-1" } } as Record<string, { task_id: string }>,
      },
    },
    listeners,
  };
});

const NOW = "2026-08-05T10:00:00Z";
const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const LANGUAGE = "typescript";

function snapshot(
  phase: TaskLspLanguageSnapshot["phase"],
  overrides: Partial<TaskLspLanguageSnapshot> = {},
): TaskLspLanguageSnapshot {
  return {
    task_id: TASK_ID,
    language: LANGUAGE,
    policy: "keep_warm",
    detected: true,
    detection_state: "complete",
    detection_truncated: false,
    phase,
    generation: phase === "off" ? 0 : 2,
    revision: 3,
    last_transition_at: NOW,
    last_action: "start",
    last_initiator: "user",
    restart_required: false,
    created_at: NOW,
    updated_at: NOW,
    effective_policy: "keep_warm",
    activity: "idle",
    progress: [],
    ...overrides,
  };
}

let current = snapshot("off");
const domain = {
  get byLanguage() {
    return { [LANGUAGE]: current };
  },
  start: mocks.start,
  stop: mocks.stop,
  restart: mocks.restart,
  setPolicy: mocks.setPolicy,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector(mocks.storeState),
}));
vi.mock("@/hooks/domains/lsp/use-task-lsp", () => ({
  useTaskLsp: () => domain,
}));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    connect: mocks.connect,
    getStatus: mocks.getStatus,
    onChange: mocks.onChange,
  },
  toLspLanguage: (language: string) => (language === LANGUAGE ? language : null),
}));

import { useLsp, useLspStatus } from "./use-lsp";

beforeEach(() => {
  current = snapshot("off");
  mocks.connect.mockClear();
  mocks.getStatus.mockClear();
  mocks.getStatus.mockReturnValue({ state: "disabled" });
  mocks.start.mockClear();
  mocks.stop.mockClear();
  mocks.restart.mockClear();
  mocks.listeners.clear();
  mocks.storeState.tasks.activeTaskId = TASK_ID;
  mocks.storeState.taskSessions.items = { [SESSION_ID]: { task_id: TASK_ID } };
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("task-scoped useLsp", () => {
  it("attaches a ready task/language with task and session identity separated", async () => {
    current = snapshot("ready");
    const view = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    await waitFor(() => expect(mocks.connect).toHaveBeenCalledOnce());
    expect(mocks.connect).toHaveBeenCalledWith(TASK_ID, SESSION_ID, LANGUAGE);
    const release = mocks.connect.mock.results[0]?.value;
    view.unmount();
    expect(release).toHaveBeenCalledOnce();
  });

  it("does not attach for an unsupported active file", () => {
    current = snapshot("ready");
    const view = renderHook(() => useLsp(SESSION_ID, "plaintext"));
    expect(view.result.current.lspLanguage).toBeNull();
    expect(mocks.connect).not.toHaveBeenCalled();
  });

  it("waits for the provided session mapping instead of attaching to the previous active task", () => {
    current = snapshot("ready");
    mocks.storeState.tasks.activeTaskId = "previous-task";
    mocks.storeState.taskSessions.items = {};

    const view = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    expect(view.result.current.taskId).toBeNull();
    expect(mocks.connect).not.toHaveBeenCalled();

    mocks.storeState.taskSessions.items = { [SESSION_ID]: { task_id: TASK_ID } };
    view.rerender();
    expect(view.result.current.taskId).toBe(TASK_ID);
    expect(mocks.connect).toHaveBeenCalledWith(TASK_ID, SESSION_ID, LANGUAGE);
  });

  it("starts and stops through the task controller instead of local persistence", () => {
    const view = renderHook(() => useLspStatus(SESSION_ID, LANGUAGE));
    act(() => view.result.current.toggle());
    expect(mocks.start).toHaveBeenCalledWith(LANGUAGE);

    current = snapshot("ready");
    view.rerender();
    act(() => view.result.current.toggle());
    expect(mocks.stop).toHaveBeenCalledWith(LANGUAGE);
  });

  it("derives honest initialization and server work from task snapshots", () => {
    current = snapshot("initializing", {
      initialize_started_at: "2026-08-05T09:59:00Z",
      activity: "server_work",
      progress: [
        {
          token: "gradle",
          title: "Importing Kotlin project",
          message: "Resolving modules",
          percentage: 35,
          started_at: "2026-08-05T09:58:00Z",
        },
      ],
    });
    const view = renderHook(() => useLspStatus(SESSION_ID, LANGUAGE));
    expect(view.result.current.status).toEqual({ state: "starting" });
    expect(view.result.current.progress).toEqual({
      initializingSince: Date.parse("2026-08-05T09:59:00Z"),
      active: [
        {
          token: "gradle",
          title: "Importing Kotlin project",
          message: "Resolving modules",
          percentage: 35,
          startedAt: Date.parse("2026-08-05T09:58:00Z"),
        },
      ],
      completed: null,
      hasReportedProgress: true,
    });
  });

  it("keeps lifecycle status visible without an attachment", () => {
    current = snapshot("error", { error_message: "Gradle import failed" });
    const view = renderHook(() => useLspStatus(SESSION_ID, LANGUAGE));
    expect(view.result.current.status).toEqual({ state: "error", reason: "Gradle import failed" });
  });

  it("reattaches after a transient browser connection loss without restarting the task server", () => {
    vi.useFakeTimers();
    current = snapshot("ready");
    const firstRelease = vi.fn();
    const secondRelease = vi.fn();
    mocks.connect.mockReturnValueOnce(firstRelease).mockReturnValueOnce(secondRelease);
    const view = renderHook(() => useLsp(SESSION_ID, LANGUAGE));
    expect(mocks.connect).toHaveBeenCalledOnce();

    mocks.getStatus.mockReturnValue({ state: "error", reason: "connection dropped" });
    act(() => mocks.listeners.forEach((listener) => listener(`${TASK_ID}:${LANGUAGE}`)));
    act(() => vi.advanceTimersByTime(999));
    expect(mocks.connect).toHaveBeenCalledOnce();
    act(() => vi.advanceTimersByTime(1));
    expect(mocks.connect).toHaveBeenCalledTimes(2);
    expect(firstRelease).toHaveBeenCalledOnce();
    expect(mocks.start).not.toHaveBeenCalled();
    expect(mocks.restart).not.toHaveBeenCalled();

    view.unmount();
    expect(secondRelease).toHaveBeenCalledOnce();
  });
});
