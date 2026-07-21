import { describe, it, expect, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { reconcileSessionPanelOrder } from "./dockview-session-tab-order";

const GROUP_ID = "group-center";
const OTHER_GROUP_ID = "group-other";
const SPEC_PANEL_ID = "session:spec-1";
const WORK_PANEL_ID = "session:work-2";

type MoveToOptions = {
  group: unknown;
  position: "center";
  index: number;
  skipSetActive: boolean;
};

type MoveToCall = {
  panelId: string;
  options: MoveToOptions;
};

/**
 * Builds a fake DockviewApi with panels distributed across one or more
 * groups, tracking every `moveTo` call so tests can assert exact reorder
 * sequences. Mirrors `makeTabOrderApi` in dockview-session-tabs.test.ts.
 */
function makeOrderApi(panelSpecs: Array<{ id: string; groupId: string }>): {
  api: DockviewApi;
  moveToCalls: MoveToCall[];
  panelsByGroup: Record<string, Array<{ id: string }>>;
} {
  const moveToCalls: MoveToCall[] = [];
  const groups: Record<string, { id: string; panels: Array<{ id: string }> }> = {};
  const panels = panelSpecs.map(({ id, groupId }) => {
    const group = (groups[groupId] ??= { id: groupId, panels: [] });
    const panel = {
      id,
      group,
      api: {
        moveTo: vi.fn((options: MoveToOptions) => {
          moveToCalls.push({ panelId: id, options });
        }),
      },
    };
    group.panels.push(panel);
    return panel;
  });
  return {
    api: {
      getPanel: (id: string) => panels.find((p) => p.id === id) ?? null,
    } as unknown as DockviewApi,
    moveToCalls,
    panelsByGroup: Object.fromEntries(Object.entries(groups).map(([id, g]) => [id, g.panels])),
  };
}

describe("reconcileSessionPanelOrder", () => {
  it("moves an active session panel outside the first workflow step back into step-flow order", () => {
    // Dockview restored/left "Work" (step 2, rank 2) as the first tab even
    // though the current step-flow order says "Spec" (rank 1) leads.
    const { api, moveToCalls } = makeOrderApi([
      { id: WORK_PANEL_ID, groupId: GROUP_ID },
      { id: SPEC_PANEL_ID, groupId: GROUP_ID },
    ]);
    const activePanel = api.getPanel(WORK_PANEL_ID);

    reconcileSessionPanelOrder(api, ["spec-1", "work-2"], activePanel);

    expect(moveToCalls).toEqual([
      {
        panelId: SPEC_PANEL_ID,
        options: {
          group: expect.objectContaining({ id: GROUP_ID }),
          position: "center",
          index: 0,
          skipSetActive: true,
        },
      },
      {
        panelId: WORK_PANEL_ID,
        options: {
          group: expect.objectContaining({ id: GROUP_ID }),
          position: "center",
          index: 1,
          skipSetActive: true,
        },
      },
    ]);
  });

  it("does not move panels that already match the step-flow order", () => {
    const { api, moveToCalls } = makeOrderApi([
      { id: SPEC_PANEL_ID, groupId: GROUP_ID },
      { id: WORK_PANEL_ID, groupId: GROUP_ID },
    ]);
    const activePanel = api.getPanel(SPEC_PANEL_ID);

    reconcileSessionPanelOrder(api, ["spec-1", "work-2"], activePanel);

    expect(moveToCalls).toEqual([]);
  });

  it("leaves panels in a different group untouched", () => {
    const { api, moveToCalls } = makeOrderApi([
      { id: SPEC_PANEL_ID, groupId: GROUP_ID },
      { id: WORK_PANEL_ID, groupId: OTHER_GROUP_ID },
    ]);
    const activePanel = api.getPanel(SPEC_PANEL_ID);

    reconcileSessionPanelOrder(api, ["work-2", "spec-1"], activePanel);

    expect(moveToCalls).toEqual([]);
  });

  it("is a no-op when there is no active panel", () => {
    const { api, moveToCalls } = makeOrderApi([{ id: SPEC_PANEL_ID, groupId: GROUP_ID }]);

    reconcileSessionPanelOrder(api, ["spec-1"], undefined);

    expect(moveToCalls).toEqual([]);
  });
});
