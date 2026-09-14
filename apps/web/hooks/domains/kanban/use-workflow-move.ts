import { useCallback, useRef, useState } from "react";
import {
  moveTask,
  type MoveTaskPayload,
  type WorkflowMoveResponse,
} from "@/lib/api/domains/kanban-api";

export type WorkflowMoveResult =
  | { disposition: "committed"; response: WorkflowMoveResponse }
  | { disposition: "failed"; error: unknown };

/** Coordinates a single task move without mutating live session state. */
export function useWorkflowMove() {
  const [isMoving, setIsMoving] = useState(false);
  const inFlightCount = useRef(0);

  const move = useCallback(async (taskId: string, payload: MoveTaskPayload) => {
    inFlightCount.current += 1;
    setIsMoving(true);
    try {
      const response = await moveTask(taskId, payload);
      return {
        disposition: "committed",
        response,
      } satisfies WorkflowMoveResult;
    } catch (error) {
      return { disposition: "failed", error } satisfies WorkflowMoveResult;
    } finally {
      inFlightCount.current -= 1;
      if (inFlightCount.current === 0) setIsMoving(false);
    }
  }, []);

  return { move, isMoving };
}
