import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Task } from "@/components/kanban-card";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}));

const reorderBand = vi.fn();
let bandPending = false;
vi.mock("./use-step-reorder", () => ({
  useStepReorder: () => ({
    reorderBand: (...args: unknown[]) => reorderBand(...args),
    isBandPending: () => bandPending,
  }),
}));

import { useKeyboardReorder } from "./use-keyboard-reorder";

const WORKFLOW_ID = "wf1";
const STEP_ID = "step-1";

function admittedTask(id: string, position: number): Task {
  return { id, title: id, workflowStepId: STEP_ID, position, wipAdmitted: true } as Task;
}

function keyEvent(key: string) {
  return { key, preventDefault: vi.fn() } as unknown as React.KeyboardEvent;
}

function bubbledKeyEvent(key: string) {
  // Simulates a key event bubbling up from a focused descendant (e.g. a
  // card action button), where target !== currentTarget.
  return {
    key,
    preventDefault: vi.fn(),
    target: {},
    currentTarget: {},
  } as unknown as React.KeyboardEvent;
}

beforeEach(() => {
  reorderBand.mockReset();
  bandPending = false;
});

describe("useKeyboardReorder", () => {
  const tasks = [admittedTask("a", 0), admittedTask("b", 1), admittedTask("c", 2)];

  it("picks a card up on Space and announces its position", () => {
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });

    expect(result.current.pickedUpTaskId).toBe("a");
    expect(result.current.draftOrder).toEqual(["a", "b", "c"]);
    expect(result.current.announcement).toContain("kanban:reorderPositionAnnouncement");
    expect(result.current.announcement).toContain('"position":1');
    expect(result.current.announcement).toContain('"total":3');
  });

  it("builds the draft order from position, not from the tasks array's own order (AC.15)", () => {
    // tasks arrives in creation order while positions reflect a prior
    // reorder: true step order by position is c(0), a(1), b(2).
    const outOfOrderTasks = [admittedTask("a", 1), admittedTask("b", 2), admittedTask("c", 0)];
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, outOfOrderTasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), outOfOrderTasks[0]);
    });

    expect(result.current.draftOrder).toEqual(["c", "a", "b"]);
  });

  it("moves the picked-up card down one place on ArrowDown and re-announces", () => {
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });
    act(() => {
      result.current.handleKeyDown(keyEvent("ArrowDown"), tasks[0]);
    });

    expect(result.current.draftOrder).toEqual(["b", "a", "c"]);
    expect(result.current.announcement).toContain('"position":2');
  });

  it("ignores an arrow press that would move the card past the band edge", () => {
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });
    act(() => {
      result.current.handleKeyDown(keyEvent("ArrowUp"), tasks[0]);
    });

    expect(result.current.draftOrder).toEqual(["a", "b", "c"]);
  });

  it("commits the draft order on Enter and clears the picked-up state", async () => {
    reorderBand.mockResolvedValue(undefined);
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });
    act(() => {
      result.current.handleKeyDown(keyEvent("ArrowDown"), tasks[0]);
    });
    await act(async () => {
      result.current.handleKeyDown(keyEvent("Enter"), tasks[0]);
    });

    expect(reorderBand).toHaveBeenCalledWith({
      workflowId: WORKFLOW_ID,
      stepId: STEP_ID,
      band: "admitted",
      draggedId: "a",
      visibleOrderAfterMove: ["b", "a", "c"],
    });
    expect(result.current.pickedUpTaskId).toBeNull();
  });

  it("cancels on Escape without calling reorderBand", () => {
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });
    act(() => {
      result.current.handleKeyDown(keyEvent("Escape"), tasks[0]);
    });

    expect(result.current.pickedUpTaskId).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
  });

  it("refuses to pick up a card while its band has a reorder in flight (AC.27)", () => {
    bandPending = true;
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), tasks[0]);
    });

    expect(result.current.pickedUpTaskId).toBeNull();
  });

  it("refuses to pick up a card whose band has fewer than two members (AC.32)", () => {
    const soleTask = [admittedTask("solo", 0)];
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, soleTask));

    act(() => {
      result.current.handleKeyDown(keyEvent(" "), soleTask[0]);
    });
    expect(result.current.pickedUpTaskId).toBeNull();

    // A second Space/Enter with nothing picked up calls pickUp again, not
    // commit - it must keep refusing rather than issue a no-op reorder
    // request for the single-member band.
    act(() => {
      result.current.handleKeyDown(keyEvent("Enter"), soleTask[0]);
    });
    expect(result.current.pickedUpTaskId).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
  });
});

describe("useKeyboardReorder — bubbled key events", () => {
  const tasks = [admittedTask("a", 0), admittedTask("b", 1), admittedTask("c", 2)];

  it("ignores Space/Enter bubbling from a focused descendant (e.g. a card action button)", () => {
    const { result } = renderHook(() => useKeyboardReorder(WORKFLOW_ID, tasks));

    act(() => {
      result.current.handleKeyDown(bubbledKeyEvent(" "), tasks[0]);
    });
    expect(result.current.pickedUpTaskId).toBeNull();

    act(() => {
      result.current.handleKeyDown(bubbledKeyEvent("Enter"), tasks[0]);
    });
    expect(result.current.pickedUpTaskId).toBeNull();
    expect(reorderBand).not.toHaveBeenCalled();
  });
});
