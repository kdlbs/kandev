import { describe, it, expect, vi } from "vitest";
import { dispatchKanbanCardClick, type Task } from "./kanban-card";

function fakeEvent(mods: Partial<MouseEvent> = {}) {
  return {
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    preventDefault: vi.fn(),
    ...mods,
  } as unknown as React.MouseEvent;
}

const task = { id: "t1", title: "T", workflowStepId: "s1" } as Task;

function handlers() {
  return {
    onToggleSelect: vi.fn(),
    onRangeSelect: vi.fn(),
    onClick: vi.fn(),
    isMultiSelectMode: false,
  };
}

describe("dispatchKanbanCardClick", () => {
  it("cmd/meta-click toggles and prevents default", () => {
    const h = handlers();
    const e = fakeEvent({ metaKey: true });
    dispatchKanbanCardClick(e, task.id, task, h);
    expect(e.preventDefault).toHaveBeenCalled();
    expect(h.onToggleSelect).toHaveBeenCalledWith("t1");
    expect(h.onClick).not.toHaveBeenCalled();
    expect(h.onRangeSelect).not.toHaveBeenCalled();
  });

  it("ctrl-click toggles (Windows/Linux)", () => {
    const h = handlers();
    dispatchKanbanCardClick(fakeEvent({ ctrlKey: true }), task.id, task, h);
    expect(h.onToggleSelect).toHaveBeenCalledWith("t1");
  });

  it("shift-click range-selects and prevents default", () => {
    const h = handlers();
    const e = fakeEvent({ shiftKey: true });
    dispatchKanbanCardClick(e, task.id, task, h);
    expect(e.preventDefault).toHaveBeenCalled();
    expect(h.onRangeSelect).toHaveBeenCalledWith("t1");
    expect(h.onToggleSelect).not.toHaveBeenCalled();
  });

  it("plain click while in multi-select mode toggles", () => {
    const h = { ...handlers(), isMultiSelectMode: true };
    dispatchKanbanCardClick(fakeEvent(), task.id, task, h);
    expect(h.onToggleSelect).toHaveBeenCalledWith("t1");
    expect(h.onClick).not.toHaveBeenCalled();
  });

  it("plain click outside multi-select mode previews/opens", () => {
    const h = handlers();
    dispatchKanbanCardClick(fakeEvent(), task.id, task, h);
    expect(h.onClick).toHaveBeenCalledWith(task);
    expect(h.onToggleSelect).not.toHaveBeenCalled();
  });
});
