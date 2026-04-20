import { describe, it, expect } from "vitest";
import type { DockviewApi } from "dockview-react";
import { shouldAutoAddPRPanel, resolvePRPanelTargetGroup } from "../dockview-session-tabs";

function makeApi(panels: Array<{ id: string; groupId: string }>): DockviewApi {
  return {
    getPanel(id: string) {
      const p = panels.find((x) => x.id === id);
      return p ? { id: p.id, group: { id: p.groupId } } : undefined;
    },
  } as unknown as DockviewApi;
}

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

describe("resolvePRPanelTargetGroup", () => {
  it("returns the session chat panel's live group when it exists", () => {
    // Regression: previously the PR panel was anchored to the store's
    // centerGroupId, which could lag behind layout transitions and drop the
    // PR panel in a split instead of as a tab next to the session.
    const api = makeApi([{ id: "session:abc", groupId: "group-live-center" }]);
    expect(resolvePRPanelTargetGroup(api, "abc", "stale-center-id")).toBe("group-live-center");
  });

  it("falls back to centerGroupId when the session panel is missing", () => {
    const api = makeApi([]);
    expect(resolvePRPanelTargetGroup(api, "abc", "center-id")).toBe("center-id");
  });

  it("prefers the session panel even when its group differs from centerGroupId", () => {
    // centerGroupId still points at the old session's group during a switch;
    // the new session's chat panel is the authoritative anchor.
    const api = makeApi([{ id: "session:new", groupId: "group-new" }]);
    expect(resolvePRPanelTargetGroup(api, "new", "group-old")).toBe("group-new");
  });
});
