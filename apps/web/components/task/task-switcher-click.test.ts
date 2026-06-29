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

function fakeKeyEvent(mods: Partial<KeyboardEvent> = {}) {
  return {
    metaKey: false,
    ctrlKey: false,
    shiftKey: false,
    preventDefault: vi.fn(),
    ...mods,
  } as unknown as React.KeyboardEvent;
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

  // Keyboard activation (Enter/Space) routes through the same dispatcher.
  it("cmd/meta keypress toggles", () => {
    const h = handlers();
    const e = fakeKeyEvent({ metaKey: true });
    dispatchSidebarRowClick(e, "t1", false, h);
    expect(e.preventDefault).toHaveBeenCalled();
    expect(h.onToggleSelectTask).toHaveBeenCalledWith("t1");
  });

  it("shift keypress range-selects", () => {
    const h = handlers();
    dispatchSidebarRowClick(fakeKeyEvent({ shiftKey: true }), "t1", false, h);
    expect(h.onSelectTaskRange).toHaveBeenCalledWith("t1");
  });

  it("plain keypress toggles while a selection is active", () => {
    const h = handlers();
    dispatchSidebarRowClick(fakeKeyEvent(), "t1", true, h);
    expect(h.onToggleSelectTask).toHaveBeenCalledWith("t1");
    expect(h.onSelectTask).not.toHaveBeenCalled();
  });

  it("plain keypress navigates when nothing is selected", () => {
    const h = handlers();
    dispatchSidebarRowClick(fakeKeyEvent(), "t1", false, h);
    expect(h.onSelectTask).toHaveBeenCalledWith("t1");
  });
});
