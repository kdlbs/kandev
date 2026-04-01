import { describe, it, expect } from "vitest";
import { shouldAutoAddPRPanel } from "../dockview-session-tabs";

describe("shouldAutoAddPRPanel", () => {
  const base = {
    hasPR: true,
    panelExists: false,
    isRestoringLayout: false,
    isMaximized: false,
    wasClosedByUser: false,
  };

  it("returns 'add' when task has PR and panel does not exist", () => {
    expect(shouldAutoAddPRPanel(base)).toBe("add");
  });

  it("returns 'none' when task has no PR", () => {
    expect(shouldAutoAddPRPanel({ ...base, hasPR: false })).toBe("none");
  });

  it("returns 'none' when panel already exists", () => {
    expect(shouldAutoAddPRPanel({ ...base, panelExists: true })).toBe("none");
  });

  it("returns 'none' during layout restoration", () => {
    expect(shouldAutoAddPRPanel({ ...base, isRestoringLayout: true })).toBe("none");
  });

  it("returns 'none' during maximize state", () => {
    expect(shouldAutoAddPRPanel({ ...base, isMaximized: true })).toBe("none");
  });

  it("returns 'none' when user previously closed the panel for this task", () => {
    expect(shouldAutoAddPRPanel({ ...base, wasClosedByUser: true })).toBe("none");
  });

  it("returns 'add' when all conditions are met", () => {
    expect(
      shouldAutoAddPRPanel({
        hasPR: true,
        panelExists: false,
        isRestoringLayout: false,
        isMaximized: false,
        wasClosedByUser: false,
      }),
    ).toBe("add");
  });
});
