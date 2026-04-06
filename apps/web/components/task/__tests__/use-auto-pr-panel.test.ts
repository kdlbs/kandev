import { describe, it, expect } from "vitest";
import { shouldAutoAddPRPanel } from "../dockview-session-tabs";

describe("shouldAutoAddPRPanel", () => {
  const base = {
    hasPR: true,
    panelExists: false,
    isRestoringLayout: false,
    isMaximized: false,
    wasOffered: false,
  };

  it("returns 'add' when task has PR and panel does not exist", () => {
    expect(shouldAutoAddPRPanel(base)).toBe("add");
  });

  it("returns 'none' when task has no PR", () => {
    expect(shouldAutoAddPRPanel({ ...base, hasPR: false })).toBe("none");
  });

  it("returns 'remove' when task has no PR but panel exists", () => {
    expect(shouldAutoAddPRPanel({ ...base, hasPR: false, panelExists: true })).toBe("remove");
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

  it("returns 'none' when panel was already offered and dismissed", () => {
    expect(shouldAutoAddPRPanel({ ...base, wasOffered: true })).toBe("none");
  });

  it("returns 'add' when all conditions are met", () => {
    expect(
      shouldAutoAddPRPanel({
        hasPR: true,
        panelExists: false,
        isRestoringLayout: false,
        isMaximized: false,
        wasOffered: false,
      }),
    ).toBe("add");
  });
});
