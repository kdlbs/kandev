import { describe, it, expect, vi } from "vitest";
import { dispatchSidebarRowClick } from "./task-switcher";

function fakeEvent(mods: Partial<MouseEvent> = {}) {
  return {
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    preventDefault: vi.fn(),
    ...mods,
  } as unknown as React.MouseEvent;
}

function handlers() {
  return {
    onSelectTask: vi.fn(),
    onToggleSelectTask: vi.fn(),
    onSelectTaskRange: vi.fn(),
  };
}

describe("dispatchSidebarRowClick", () => {
  it("cmd/meta-click toggles and prevents default", () => {
    const h = handlers();
    const e = fakeEvent({ metaKey: true });
    dispatchSidebarRowClick(e, "t1", false, h);
    expect(e.preventDefault).toHaveBeenCalled();
    expect(h.onToggleSelectTask).toHaveBeenCalledWith("t1");
    expect(h.onSelectTask).not.toHaveBeenCalled();
  });

  it("shift-click range-selects and prevents default", () => {
    const h = handlers();
    const e = fakeEvent({ shiftKey: true });
    dispatchSidebarRowClick(e, "t1", false, h);
    expect(e.preventDefault).toHaveBeenCalled();
    expect(h.onSelectTaskRange).toHaveBeenCalledWith("t1");
  });

  it("plain click toggles while a selection is active", () => {
    const h = handlers();
    dispatchSidebarRowClick(fakeEvent(), "t1", true, h);
    expect(h.onToggleSelectTask).toHaveBeenCalledWith("t1");
    expect(h.onSelectTask).not.toHaveBeenCalled();
  });

  it("plain click navigates when nothing is selected", () => {
    const h = handlers();
    dispatchSidebarRowClick(fakeEvent(), "t1", false, h);
    expect(h.onSelectTask).toHaveBeenCalledWith("t1");
    expect(h.onToggleSelectTask).not.toHaveBeenCalled();
  });
});
