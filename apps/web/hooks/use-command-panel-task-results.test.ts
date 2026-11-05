import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const listTasksByWorkspace = vi.fn();
const searchTaskConversations = vi.fn().mockResolvedValue({ tasks: [], state: "ready" });
vi.mock("@/lib/api", () => ({
  searchTaskConversations: (...args: unknown[]) => searchTaskConversations(...args),
  listTasksByWorkspace: (...args: unknown[]) => listTasksByWorkspace(...args),
}));

import { useInlineTaskSearchEffect } from "./use-command-panel-task-results";
import type { Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";

const WORKSPACE_ID = "workspace-1";
const WORKFLOW_ID = "workflow-1";

function task(id: string, title: string, overrides: Partial<Task> = {}): Task {
  return {
    id: taskId(id),
    workspace_id: workspaceId(WORKSPACE_ID),
    workflow_id: workflowId(WORKFLOW_ID),
    workflow_step_id: "step-1",
    position: 0,
    title,
    description: "",
    state: "IN_PROGRESS",
    priority: "medium",
    created_at: "2026-08-24T09:00:00Z",
    updated_at: "2026-08-24T09:00:00Z",
    ...overrides,
  };
}

const STEPS = [{ id: "step-1", position: 0, show_in_command_panel: true }];

type HarnessProps = {
  mode: "commands" | "search-tasks";
  search?: string;
  open?: boolean;
  workspace?: string;
};

function harness(
  setTaskResults: (t: Task[]) => void,
  setIsSearching: (s: boolean) => void,
  search = "",
) {
  return renderHook(
    ({ mode, search = "", open = true, workspace = WORKSPACE_ID }: HarnessProps) =>
      useInlineTaskSearchEffect({
        mode,
        search,
        open,
        workspaceId: workspace,
        steps: STEPS,
        setTaskResults,
        setIsSearching,
      }),
    {
      initialProps: { mode: "commands", search } as HarnessProps,
    },
  );
}

afterEach(() => {
  listTasksByWorkspace.mockReset();
  searchTaskConversations.mockReset().mockResolvedValue({ tasks: [], state: "ready" });
});

// @covers AC-TASKS-WORKSPACE-SEARCH-002.2
it("requests the eligible window before ordering and falls back when steps are absent", async () => {
  const later = task("later", "Later", { workflow_step_id: "step-later" });
  const earlier = task("earlier", "Earlier", { workflow_step_id: "step-earlier" });
  listTasksByWorkspace.mockResolvedValue({ tasks: [later, earlier], total: 2 });
  const setTaskResults = vi.fn();
  const setIsSearching = vi.fn();
  const visibleSteps = [
    { id: "step-earlier", position: 0, show_in_command_panel: true },
    { id: "step-later", position: 1, show_in_command_panel: true },
  ];
  const { rerender } = renderHook(
    ({ steps }) =>
      useInlineTaskSearchEffect({
        mode: "search-tasks",
        search: "",
        open: true,
        workspaceId: WORKSPACE_ID,
        workflowId: WORKFLOW_ID,
        steps,
        setTaskResults,
        setIsSearching,
      }),
    { initialProps: { steps: visibleSteps } },
  );
  await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith([earlier, later]));
  expect(listTasksByWorkspace.mock.calls[0][1]).toEqual({
    page: 1,
    pageSize: 20,
    workflowId: WORKFLOW_ID,
    commandPanelVisible: true,
  });
  listTasksByWorkspace.mockClear();
  rerender({ steps: [] });
  await waitFor(() => expect(listTasksByWorkspace).toHaveBeenCalled());
  expect(listTasksByWorkspace.mock.calls[0][1]).toEqual({ page: 1, pageSize: 20 });
});

it("aborts an obsolete idle request", async () => {
  let finish!: (value: unknown) => void;
  let signal!: AbortSignal;
  listTasksByWorkspace.mockImplementationOnce(
    (_workspace, _params, options) =>
      new Promise((resolve) => {
        signal = options.init.signal;
        finish = resolve;
      }),
  );
  const setTaskResults = vi.fn();
  const setIsSearching = vi.fn();
  const hiddenSteps = [{ id: "hidden", position: 0, show_in_command_panel: false }];
  const { rerender } = renderHook(
    ({ open }) =>
      useInlineTaskSearchEffect({
        mode: "commands",
        search: "",
        open,
        workspaceId: WORKSPACE_ID,
        workflowId: WORKFLOW_ID,
        steps: hiddenSteps,
        setTaskResults,
        setIsSearching,
      }),
    { initialProps: { open: true } },
  );
  await waitFor(() => expect(listTasksByWorkspace).toHaveBeenCalledOnce());
  expect(listTasksByWorkspace.mock.calls[0][1].commandPanelVisible).toBe(true);
  rerender({ open: false });
  expect(signal.aborted).toBe(true);
  finish({ tasks: [task("late", "Late", { workflow_step_id: "hidden" })], total: 1 });
  await waitFor(() => expect(setTaskResults).toHaveBeenLastCalledWith([]));
  expect(setIsSearching).toHaveBeenLastCalledWith(false);
});

it("settles an all-hidden workflow with no task results", async () => {
  listTasksByWorkspace.mockResolvedValue({ tasks: [], total: 0 });
  const setTaskResults = vi.fn();
  const setIsSearching = vi.fn();
  renderHook(() =>
    useInlineTaskSearchEffect({
      mode: "commands",
      search: "",
      open: true,
      workspaceId: WORKSPACE_ID,
      workflowId: WORKFLOW_ID,
      steps: [{ id: "hidden", position: 0, show_in_command_panel: false }],
      setTaskResults,
      setIsSearching,
    }),
  );
  await waitFor(() => expect(setIsSearching).toHaveBeenLastCalledWith(false));
  expect(setTaskResults).toHaveBeenLastCalledWith([]);
  expect(listTasksByWorkspace.mock.calls[0][1].commandPanelVisible).toBe(true);
});

describe("useInlineTaskSearchEffect", () => {
  it("caps the commands scope at a five-row preview and the tasks scope at twenty", async () => {
    const many = Array.from({ length: 20 }, (_, i) => task(`task-${i}`, `Task ${i}`));
    listTasksByWorkspace.mockResolvedValue({ tasks: many });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());

    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith(many.slice(0, 5)));

    setTaskResults.mockClear();
    rerender({ mode: "search-tasks" });
    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith(many));
  });

  it("drops the previous scope's results when the scope changes", async () => {
    listTasksByWorkspace.mockResolvedValue({ tasks: [task("task-1", "Only task")] });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());
    await waitFor(() => expect(setTaskResults).toHaveBeenCalled());

    setTaskResults.mockClear();
    rerender({ mode: "search-tasks" });

    // The five-row preview must not stand in as the tasks scope's full result
    // set (nor suppress its loading state) while the wider request is in flight.
    expect(setTaskResults).toHaveBeenNthCalledWith(1, []);
  });

  it("keeps results when the scope is unchanged", async () => {
    listTasksByWorkspace.mockResolvedValue({ tasks: [task("task-1", "Only task")] });
    const setTaskResults = vi.fn();
    const { rerender } = harness(setTaskResults, vi.fn());
    await waitFor(() => expect(setTaskResults).toHaveBeenCalled());

    setTaskResults.mockClear();
    rerender({ mode: "commands" });
    expect(setTaskResults).not.toHaveBeenCalledWith([]);
  });

  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.5
  it("uses archived_at instead of terminal state for the active task preview", async () => {
    const archivedInProgress = task("archived-in-progress", "Archived in progress", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const unarchivedCompleted = task("unarchived-completed", "Unarchived completed", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockResolvedValue({ tasks: [archivedInProgress, unarchivedCompleted] });
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn());

    await waitFor(() => expect(setTaskResults).toHaveBeenCalledWith([unarchivedCompleted]));
  });

  // @covers AC-TASKS-COMMAND-PANEL-ARCHIVED-TASKS-001.6
  it("ranks archived matches after non-archived matches by archived_at", async () => {
    const archivedInProgress = task("archived-in-progress", "Archived in progress", {
      state: "IN_PROGRESS",
      archived_at: "2026-08-24T09:05:00Z",
    });
    const unarchivedCompleted = task("unarchived-completed", "Unarchived completed", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockResolvedValue({ tasks: [archivedInProgress, unarchivedCompleted] });
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn(), "archived");

    await waitFor(() =>
      expect(setTaskResults).toHaveBeenCalledWith([unarchivedCompleted, archivedInProgress]),
    );
  });

  it("queries active matches before a bounded archived fallback page", async () => {
    const archivedTasks = Array.from({ length: 20 }, (_, index) =>
      task(`archived-${index}`, `Archived ${index}`, {
        archived_at: `2026-08-24T09:${String(index).padStart(2, "0")}:00Z`,
      }),
    );
    const unarchivedTask = task("unarchived-match", "Unarchived match", {
      state: "COMPLETED",
    });
    listTasksByWorkspace.mockImplementation(
      (_workspaceId: string, params: { onlyArchived?: boolean }) =>
        Promise.resolve(
          params.onlyArchived
            ? { tasks: archivedTasks, total: 10_000 }
            : { tasks: [unarchivedTask], total: 1 },
        ),
    );
    const setTaskResults = vi.fn();

    harness(setTaskResults, vi.fn(), "match");

    await waitFor(() =>
      expect(setTaskResults).toHaveBeenCalledWith([unarchivedTask, ...archivedTasks.slice(0, 4)]),
    );
    expect(listTasksByWorkspace).toHaveBeenCalledTimes(2);
    expect(listTasksByWorkspace.mock.calls[0][1]).toEqual({
      query: "match",
      page: 1,
      pageSize: 5,
    });
    expect(listTasksByWorkspace.mock.calls[1][1]).toEqual({
      query: "match",
      page: 1,
      pageSize: 4,
      onlyArchived: true,
    });
  });
});

describe("conversation matches", () => {
  it("keeps metadata usable while conversations load, then orders and deduplicates", async () => {
    const metadata = task("metadata", "Metadata");
    const archived = task("archive", "Archived", { archived_at: "2026-01-01" });
    const conversation = task("conversation", "Conversation");
    listTasksByWorkspace.mockImplementation((_id, params) =>
      Promise.resolve({ tasks: params.onlyArchived ? [archived] : [metadata] }),
    );
    let resolve!: (value: unknown) => void;
    searchTaskConversations.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const setResults = vi.fn();
    harness(setResults, vi.fn(), "needle");
    await waitFor(() => expect(setResults).toHaveBeenLastCalledWith([metadata, archived]));
    resolve({ tasks: [conversation, metadata], state: "ready", has_more: false });
    await waitFor(() =>
      expect(setResults).toHaveBeenLastCalledWith([metadata, conversation, archived]),
    );
  });

  it("keeps metadata when conversation search fails", async () => {
    const metadata = task("metadata", "Metadata");
    listTasksByWorkspace.mockResolvedValue({ tasks: [metadata] });
    searchTaskConversations.mockRejectedValue(new Error("unavailable"));
    const setResults = vi.fn();
    harness(setResults, vi.fn(), "needle");
    await waitFor(() => expect(setResults).toHaveBeenLastCalledWith([metadata]));
  });
});

it("fences late conversations when the workspace changes or the palette closes", async () => {
  const pending: Array<{ resolve: (value: unknown) => void; signal: AbortSignal }> = [];
  searchTaskConversations.mockImplementation(
    (_workspace, _query, _limit, options) =>
      new Promise((resolve) => {
        pending.push({ resolve, signal: options.init.signal });
      }),
  );
  listTasksByWorkspace.mockResolvedValue({ tasks: [] });
  const setResults = vi.fn();
  const { rerender } = harness(setResults, vi.fn(), "needle");
  await waitFor(() => expect(pending).toHaveLength(1));
  rerender({ mode: "commands", search: "needle", workspace: "workspace-2" });
  expect(pending[0].signal.aborted).toBe(true);
  await waitFor(() => expect(pending).toHaveLength(2));
  rerender({ mode: "commands", search: "needle", workspace: "workspace-2", open: false });
  expect(pending[1].signal.aborted).toBe(true);
  pending[0].resolve({ tasks: [task("foreign", "Foreign")], state: "ready" });
  pending[1].resolve({ tasks: [task("late", "Late")], state: "ready" });
  await new Promise((resolve) => setTimeout(resolve, 10));
  expect(setResults).toHaveBeenLastCalledWith([]);
});

it("keeps the empty state loading until both search sources settle", async () => {
  listTasksByWorkspace.mockResolvedValue({ tasks: [] });
  let finish!: (value: unknown) => void;
  searchTaskConversations.mockReturnValue(
    new Promise((resolve) => {
      finish = resolve;
    }),
  );
  const searching = vi.fn();
  harness(vi.fn(), searching, "needle");
  await waitFor(() => expect(searchTaskConversations).toHaveBeenCalled());
  expect(searching).not.toHaveBeenCalledWith(false);
  finish({ tasks: [], state: "ready" });
  await waitFor(() => expect(searching).toHaveBeenLastCalledWith(false));
});
