import { describe, expect, it } from "vitest";

import {
  DRAG_ACTIVATION_DISTANCE,
  TOUCH_DRAG_ACTIVATION_DELAY_MS,
  TOUCH_DRAG_ACTIVATION_TOLERANCE_PX,
  taskSwitcherDragActivationConstraints,
} from "@/components/task/task-switcher-subtask-dnd";

describe("task switcher subtask DnD activation", () => {
  it("keeps pointer dragging responsive but delays touch dragging for scrollable mobile lists", () => {
    expect(taskSwitcherDragActivationConstraints()).toEqual({
      pointer: { distance: DRAG_ACTIVATION_DISTANCE },
      touch: {
        delay: TOUCH_DRAG_ACTIVATION_DELAY_MS,
        tolerance: TOUCH_DRAG_ACTIVATION_TOLERANCE_PX,
      },
    });
  });

  it("reuses stable activation constraint references across renders", () => {
    expect(taskSwitcherDragActivationConstraints()).toBe(taskSwitcherDragActivationConstraints());
  });
});
