import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useTaskMoveChoices } from "./task-command-choices";

afterEach(cleanup);

it("omits workflows without destinations while preserving both destination actions", () => {
  const openMoveOptions = vi.fn();
  const moveImmediately = vi.fn();
  const target = { id: "review", title: "Review", color: "bg-blue-500" };
  const { result } = renderHook(() =>
    useTaskMoveChoices({
      task: { id: "task", title: "Task", workflowId: "current" },
      workflows: [
        { id: "current", name: "Current" },
        { id: "empty", name: "Empty" },
        { id: "missing", name: "Missing" },
        { id: "target", name: "Target" },
      ],
      stepsByWorkflowId: { empty: [], target: [target] },
      openMoveOptions,
      moveImmediately,
    }),
  );
  expect(result.current.workflows.map((item) => item.label)).toEqual(["Target"]);
  const destination = result.current.workflows[0].children![0];
  destination.action?.();
  destination.immediateAction?.();
  expect(openMoveOptions).toHaveBeenCalledWith({ ...target, workflow_id: "target" });
  expect(moveImmediately).toHaveBeenCalledWith({ ...target, workflow_id: "target" });
});
