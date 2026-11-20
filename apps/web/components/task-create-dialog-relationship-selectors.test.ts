import { describe, expect, it } from "vitest";
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import {
  filterCreateDialogCandidates,
  filterCreateParentCandidates,
  filterCreateRelatedCandidates,
  reconcileRelatedTaskIds,
  toggleRelatedTaskSelection,
} from "./task-create-dialog-relationship-selectors";

type Task = KanbanState["tasks"][number];
const task = (id: string, overrides: Partial<Task> = {}): Task => ({
  id,
  title: id,
  workflowId: "workflow-1",
  workflowStepId: "step-1",
  position: 0,
  isArchived: false,
  ...overrides,
});

describe("relationship selector candidates", () => {
  it("excludes the current task, its descendants, and archived parent candidates", () => {
    const tasks = [
      task("parent", { parentTaskId: "current" }),
      task("current"),
      task("descendant", { parentTaskId: "current" }),
      task("archived", { isArchived: true }),
      task("eligible"),
    ];
    expect(filterCreateParentCandidates(tasks, "current").map(({ id }) => id)).toEqual([
      "eligible",
    ]);
  });

  it("filters related choices by workspace, self, dependencies, parent, and archive state", () => {
    const tasks = [
      task("self"),
      task("blocked"),
      task("parent"),
      task("archived", { isArchived: true }),
      task("other-workspace", { workspaceId: "ws-2" }),
      task("eligible", { workspaceId: "ws-1" }),
    ];
    const workspaceTasks = filterCreateDialogCandidates(tasks, "ws-1");
    expect(
      filterCreateRelatedCandidates(workspaceTasks, "self", ["blocked"], "parent").map(
        ({ id }) => id,
      ),
    ).toEqual(["eligible"]);
  });

  it("toggles multi-selection without duplicates", () => {
    expect(toggleRelatedTaskSelection(["a"], "b")).toEqual(["a", "b"]);
    expect(toggleRelatedTaskSelection(["a", "b"], "a")).toEqual(["b"]);
  });

  it("excludes ordinary subtasks from parent candidates but keeps Office subtasks", () => {
    const tasks = [
      task("current"),
      task("ordinary-subtask", { parentTaskId: "some-parent" }),
      task("office-subtask", { parentTaskId: "some-parent", isFromOffice: true }),
      task("top-level"),
    ];
    expect(filterCreateParentCandidates(tasks, "current").map(({ id }) => id)).toEqual([
      "office-subtask",
      "top-level",
    ]);
  });

  it("drops related selections made mutually exclusive by a new conflict", () => {
    expect(reconcileRelatedTaskIds(["a", "b", "c"], ["b"])).toEqual(["a", "c"]);
    expect(reconcileRelatedTaskIds(["a", "b"], [])).toEqual(["a", "b"]);
    expect(reconcileRelatedTaskIds(["a", "b"], ["c"])).toEqual(["a", "b"]);
  });
});
