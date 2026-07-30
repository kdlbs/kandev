import { describe, expect, it } from "vitest";
import type { DockviewApi } from "dockview-react";
import { CENTER_GROUP, RIGHT_TOP_GROUP, SIDEBAR_GROUP } from "./layout-manager";
import { resolvePRPanelTargetGroup } from "./pr-panel-placement";

type TestPanel = {
  id: string;
  group: TestGroup;
};

type TestGroup = {
  id: string;
  panels: TestPanel[];
  element: {
    getBoundingClientRect: () => DOMRect;
  };
};

type GroupInput = {
  id: string;
  left: number;
  top?: number;
};

function makeRect(left: number, top: number): DOMRect {
  return {
    x: left,
    y: top,
    left,
    top,
    width: 100,
    height: 100,
    right: left + 100,
    bottom: top + 100,
    toJSON: () => ({}),
  } as DOMRect;
}

function makeApi(
  groupInputs: GroupInput[],
  panelInputs: Array<{ id: string; groupId: string }> = [],
): DockviewApi {
  const groups: TestGroup[] = groupInputs.map(({ id, left, top = 0 }) => ({
    id,
    panels: [],
    element: { getBoundingClientRect: () => makeRect(left, top) },
  }));
  const panels = panelInputs.map(({ id, groupId }) => {
    const group = groups.find((candidate) => candidate.id === groupId);
    if (!group) throw new Error(`missing test group ${groupId}`);
    const panel: TestPanel = { id, group };
    group.panels.push(panel);
    return panel;
  });
  return {
    groups,
    panels,
    getPanel: (id: string) => panels.find((panel) => panel.id === id),
  } as unknown as DockviewApi;
}

const baseOptions = {
  activeSessionId: "session-1",
  centerGroupId: CENTER_GROUP,
  rightTopGroupId: RIGHT_TOP_GROUP,
} as const;
const SESSION_PANEL_ID = `session:${baseOptions.activeSessionId}`;

describe("resolvePRPanelTargetGroup", () => {
  it("places Agent-side tabs with the live session when available", () => {
    const api = makeApi(
      [
        { id: CENTER_GROUP, left: 200 },
        { id: "live-agent", left: 300 },
      ],
      [{ id: SESSION_PANEL_ID, groupId: "live-agent" }],
    );

    expect(resolvePRPanelTargetGroup(api, { ...baseOptions, placement: "agent" })).toBe(
      "live-agent",
    );
  });

  it("falls back to the tracked center for Agent-side tabs", () => {
    const api = makeApi([{ id: CENTER_GROUP, left: 200 }]);

    expect(resolvePRPanelTargetGroup(api, { ...baseOptions, placement: "agent" })).toBe(
      CENTER_GROUP,
    );
  });

  it("prefers the designated right content group", () => {
    const api = makeApi(
      [
        { id: CENTER_GROUP, left: 200 },
        { id: RIGHT_TOP_GROUP, left: 700 },
        { id: "further-right", left: 900 },
      ],
      [{ id: SESSION_PANEL_ID, groupId: CENTER_GROUP }],
    );

    expect(resolvePRPanelTargetGroup(api, { ...baseOptions, placement: "right" })).toBe(
      RIGHT_TOP_GROUP,
    );
  });

  it("uses the topmost visually furthest-right eligible group as fallback", () => {
    const api = makeApi(
      [
        { id: CENTER_GROUP, left: 200 },
        { id: "lower-right", left: 900, top: 500 },
        { id: "top-right", left: 900, top: 100 },
      ],
      [{ id: SESSION_PANEL_ID, groupId: CENTER_GROUP }],
    );

    expect(
      resolvePRPanelTargetGroup(api, {
        ...baseOptions,
        rightTopGroupId: "missing-designated-group",
        placement: "right",
      }),
    ).toBe("top-right");
  });

  it("excludes the sidebar and Agent group from right-side fallback", () => {
    const api = makeApi(
      [
        { id: CENTER_GROUP, left: 700 },
        { id: SIDEBAR_GROUP, left: 1_000 },
        { id: "tools", left: 600 },
      ],
      [
        { id: SESSION_PANEL_ID, groupId: CENTER_GROUP },
        { id: "sidebar", groupId: SIDEBAR_GROUP },
      ],
    );

    expect(
      resolvePRPanelTargetGroup(api, {
        ...baseOptions,
        rightTopGroupId: "missing-designated-group",
        placement: "right",
      }),
    ).toBe("tools");
  });

  it("falls back beside Agent instead of creating a split", () => {
    const api = makeApi(
      [{ id: CENTER_GROUP, left: 200 }],
      [{ id: SESSION_PANEL_ID, groupId: CENTER_GROUP }],
    );

    expect(
      resolvePRPanelTargetGroup(api, {
        ...baseOptions,
        rightTopGroupId: "missing-designated-group",
        placement: "right",
      }),
    ).toBe(CENTER_GROUP);
  });
});
