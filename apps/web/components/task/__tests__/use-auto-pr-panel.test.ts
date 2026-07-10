import { describe, it, expect } from "vitest";
import type { DockviewApi, AddPanelOptions } from "dockview-react";
import {
  shouldAutoAddPRPanel,
  resolvePRPanelTargetGroup,
  runAutoPRPanelEffect,
} from "../dockview-session-tabs";
import { CENTER_GROUP, RIGHT_TOP_GROUP } from "@/lib/state/layout-manager";

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

  it("falls back to the center group when the live session panel is in a right group", () => {
    // Corrupted layouts can briefly leave the session panel in the right tools
    // column. The PR panel must not follow it there.
    const api = makeApi([{ id: "session:abc", groupId: RIGHT_TOP_GROUP }]);
    expect(resolvePRPanelTargetGroup(api, "abc", "group-center")).toBe("group-center");
  });

  it("uses the well-known center group when both candidates are right groups", () => {
    const api = makeApi([{ id: "session:abc", groupId: RIGHT_TOP_GROUP }]);
    expect(resolvePRPanelTargetGroup(api, "abc", RIGHT_TOP_GROUP)).toBe(CENTER_GROUP);
  });
});

// ---------------------------------------------------------------------------
// runAutoPRPanelEffect — per-PR tab stamping/backfill
// ---------------------------------------------------------------------------

type FullMockPanel = {
  id: string;
  params: Record<string, unknown>;
  group: { id: string };
  api: {
    setActive: () => void;
    updateParameters: (p: Record<string, unknown>) => void;
    close: () => void;
  };
};

function makeFullApi(): { api: DockviewApi; panels: FullMockPanel[] } {
  const panels: FullMockPanel[] = [];
  const groups = [{ id: CENTER_GROUP }];
  const api = {
    get groups() {
      return groups;
    },
    getPanel(id: string) {
      return panels.find((p) => p.id === id);
    },
    addPanel(opts: AddPanelOptions & { id: string }) {
      const panel: FullMockPanel = {
        id: opts.id,
        params: { ...(opts.params ?? {}) },
        group: { id: CENTER_GROUP },
        api: {
          setActive() {},
          updateParameters(p: Record<string, unknown>) {
            Object.assign(panel.params, p);
          },
          close() {
            const i = panels.indexOf(panel);
            if (i >= 0) panels.splice(i, 1);
          },
        },
      };
      panels.push(panel);
      return panel;
    },
  } as unknown as DockviewApi;
  return { api, panels };
}

const LEGACY_PR_ID = "pr-detail";
const DEFAULT_PR_KEY = "org/repo/1";

const BASE_EFFECT_PARAMS = {
  isRestoringLayout: false,
  isMaximized: false,
  centerGroupId: CENTER_GROUP,
};

describe("runAutoPRPanelEffect", () => {
  it("stamps the newly auto-shown panel with the default PR's key", () => {
    const { api } = makeFullApi();

    runAutoPRPanelEffect(api, "session-add", {
      ...BASE_EFFECT_PARAMS,
      hasPR: true,
      defaultPRKey: DEFAULT_PR_KEY,
    });

    const panel = api.getPanel(LEGACY_PR_ID) as unknown as FullMockPanel;
    expect(panel).toBeDefined();
    expect(panel.params.prKey).toBe(DEFAULT_PR_KEY);
  });

  it("backfills the key on a legacy panel restored without one", () => {
    const { api } = makeFullApi();
    api.addPanel({
      id: LEGACY_PR_ID,
      component: LEGACY_PR_ID,
      title: "Pull Request",
      position: { referenceGroup: CENTER_GROUP },
    });

    runAutoPRPanelEffect(api, "session-backfill", {
      ...BASE_EFFECT_PARAMS,
      hasPR: true,
      defaultPRKey: DEFAULT_PR_KEY,
    });

    const panel = api.getPanel(LEGACY_PR_ID) as unknown as FullMockPanel;
    expect(panel.params.prKey).toBe(DEFAULT_PR_KEY);
  });

  it("resyncs a legacy panel whose stamped key no longer matches the current default", () => {
    // Regression (Greptile P1 / cubic-dev-ai on PR #1636): the legacy panel
    // must track whichever PR is CURRENTLY the task's default — reused
    // across a task switch or after the primary PR changes for the same
    // task. Nothing else ever stamps a deliberately different key onto this
    // specific panel (a manual "+" menu pick of another PR always creates
    // its own separate `pr-detail|<key>` tab instead), so resyncing here can
    // never clobber a real user choice.
    const { api } = makeFullApi();
    api.addPanel({
      id: LEGACY_PR_ID,
      component: LEGACY_PR_ID,
      title: "Pull Request",
      params: { prKey: "org/repo/2" },
      position: { referenceGroup: CENTER_GROUP },
    });

    runAutoPRPanelEffect(api, "session-resync", {
      ...BASE_EFFECT_PARAMS,
      hasPR: true,
      defaultPRKey: DEFAULT_PR_KEY,
    });

    const panel = api.getPanel(LEGACY_PR_ID) as unknown as FullMockPanel;
    expect(panel.params.prKey).toBe(DEFAULT_PR_KEY);
  });
});
