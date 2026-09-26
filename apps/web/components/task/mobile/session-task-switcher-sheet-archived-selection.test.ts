import { describe, expect, it, vi } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import {
  createTaskSheetSelectionController,
  selectTaskFromSheet,
} from "./session-task-switcher-sheet-selection";

const ARCHIVED_TASK_ID = "archived-task";
const REMEMBERED_SESSION_ID = "remembered-session";
const PRIMARY_SESSION_ID = "primary-session";

async function flushSelection(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("selectTaskFromSheet archived tasks", () => {
  it("selects an existing remembered conversation without launching", async () => {
    const taskId = ARCHIVED_TASK_ID;
    const loadTaskSessionsForTask = vi.fn(async () => [
      {
        id: REMEMBERED_SESSION_ID,
        task_id: taskId,
        state: "COMPLETED",
      } as TaskSession,
      {
        id: PRIMARY_SESSION_ID,
        task_id: taskId,
        state: "COMPLETED",
        is_primary: true,
      } as TaskSession,
    ]);
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      selectionController: createTaskSheetSelectionController(),
      taskId,
      task: { isArchived: true, primarySessionId: PRIMARY_SESSION_ID },
      state: {
        lastSessionByTaskId: { [taskId]: REMEMBERED_SESSION_ID },
        environmentIdBySessionId: { [REMEMBERED_SESSION_ID]: "env-remembered" },
        taskSessionsById: {
          [REMEMBERED_SESSION_ID]: {
            id: REMEMBERED_SESSION_ID,
            task_id: taskId,
          } as TaskSession,
        },
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
    });

    await flushSelection();

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(taskId);
    expect(setActiveSession).toHaveBeenCalledWith(taskId, REMEMBERED_SESSION_ID);
    expect(setActiveTask).not.toHaveBeenCalled();
    expect(navigate).toHaveBeenCalledWith(taskId, REMEMBERED_SESSION_ID);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("opens a sessionless archived task without preparing a replacement", async () => {
    const taskId = "archived-empty-task";
    const loadTaskSessionsForTask = vi.fn(async () => []);
    const setActiveSession = vi.fn();
    const setActiveTask = vi.fn();
    const navigate = vi.fn();
    const onOpenChange = vi.fn();

    selectTaskFromSheet({
      selectionController: createTaskSheetSelectionController(),
      taskId,
      task: { isArchived: true },
      state: {
        lastSessionByTaskId: {},
        environmentIdBySessionId: {},
        taskSessionsById: {},
      },
      loadTaskSessionsForTask,
      setActiveSession,
      setActiveTask,
      navigate,
      onOpenChange,
    });

    await flushSelection();

    expect(loadTaskSessionsForTask).toHaveBeenCalledWith(taskId);
    expect(setActiveSession).not.toHaveBeenCalled();
    expect(setActiveTask).toHaveBeenCalledWith(taskId);
    expect(navigate).toHaveBeenCalledWith(taskId, undefined);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});
