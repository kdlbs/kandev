import { describe, expect, it } from "vitest";
import { createDockview, type DockviewApi } from "dockview-react";
import { filterEphemeral, fromDockviewApi, toSerializedDockview } from "./serializer";
import type { LayoutState } from "./types";
import { CENTER_GROUP, RIGHT_BOTTOM_GROUP, RIGHT_TOP_GROUP } from "./constants";

const SESSION_PANEL_ID = "session:session-a";

type CapturedGroup = {
  id: string;
  panels: Array<{ id: string; component: string; title?: string }>;
};

function makeCapturedApi(columns: CapturedGroup[][]): DockviewApi {
  const leaf = (group: CapturedGroup) => ({
    view: {
      id: group.id,
      panels: group.panels.map((item) => ({
        id: item.id,
        title: item.title ?? item.id,
        view: { contentComponent: item.component },
      })),
      activePanel: group.panels[0] ? { id: group.panels[0].id } : undefined,
    },
  });
  const rootChildren = columns.map((groups) =>
    groups.length === 1
      ? leaf(groups[0])
      : {
          children: groups.map(leaf),
          splitview: { getViewSize: (index: number) => 300 + index * 10 },
        },
  );
  return {
    component: {
      gridview: {
        root: {
          children: rootChildren,
          splitview: { getViewSize: (index: number) => 700 - index * 300 },
        },
      },
    },
  } as unknown as DockviewApi;
}

describe("fromDockviewApi — right-column ownership", () => {
  it("preserves the production root split orientation", () => {
    const api = makeCapturedApi([
      [
        {
          id: "runtime-agent",
          panels: [{ id: SESSION_PANEL_ID, component: "chat", title: "Agent" }],
        },
      ],
    ]) as unknown as {
      component: { gridview: { root: { splitview: { orientation: string } } } };
    };
    api.component.gridview.root.splitview.orientation = "VERTICAL";

    const captured = fromDockviewApi(api as unknown as DockviewApi);

    expect(captured.rootOrientation).toBe("VERTICAL");
  });

  it("captures a live Agent session as the compact center column", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [
          {
            id: "runtime-center",
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
              { id: "changes", component: "changes", title: "Changes" },
              { id: "terminal-default", component: "terminal", title: "Terminal" },
            ],
          },
        ],
      ]),
    );

    expect(captured.columns).toHaveLength(1);
    expect(captured.columns[0]?.id).toBe("center");
    expect(captured.columns[0]?.pinned).toBeUndefined();

    const serialized = toSerializedDockview(captured, 1000, 800, new Map());
    const root = (serialized as unknown as { grid: { root: { data: unknown[] } } }).grid.root;
    expect((root.data[0] as { data: { id: string } }).data.id).toBe("runtime-center");
  });

  it("keeps a mixed session center separate from a nested right column", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [
          {
            id: "runtime-agent",
            panels: [
              { id: SESSION_PANEL_ID, component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
              { id: "pr-detail", component: "pr-detail", title: "PR Details" },
            ],
          },
        ],
        [
          {
            id: RIGHT_TOP_GROUP,
            panels: [
              { id: "session:session-b", component: "chat" },
              { id: "files", component: "files" },
            ],
          },
          { id: RIGHT_BOTTOM_GROUP, panels: [{ id: "terminal-default", component: "terminal" }] },
        ],
      ]),
    );

    expect(captured.columns.map((column) => column.id)).toEqual(["center", "right"]);
    expect(captured.columns[0]?.groups[0]?.panels.map((item) => item.id)).toEqual([
      SESSION_PANEL_ID,
      "files",
      "pr-detail",
    ]);
    expect(captured.columns[1]?.groups.map((group) => group.id)).toEqual([
      RIGHT_TOP_GROUP,
      RIGHT_BOTTOM_GROUP,
    ]);
  });
});

describe("contextual pane width ownership", () => {
  it.each(["browser", "plan", "vscode"])(
    "keeps a %s pane unpinned after merging tool tabs",
    (component) => {
      const captured = fromDockviewApi(
        makeCapturedApi([
          [{ id: CENTER_GROUP, panels: [{ id: SESSION_PANEL_ID, component: "chat" }] }],
          [
            {
              id: "group-1",
              panels: [
                { id: component, component },
                { id: "files", component: "files" },
                { id: "changes", component: "changes" },
              ],
            },
          ],
        ]),
      );
      expect(captured.columns[1]).toMatchObject({ id: component, width: 400 });
      expect(captured.columns[1].pinned).toBeUndefined();
    },
  );

  it("keeps canonical right groups pinned even with a Browser tab", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [{ id: CENTER_GROUP, panels: [{ id: SESSION_PANEL_ID, component: "chat" }] }],
        [
          {
            id: RIGHT_TOP_GROUP,
            panels: [
              { id: "browser", component: "browser" },
              { id: "files", component: "files" },
            ],
          },
        ],
      ]),
    );
    expect(captured.columns[1]).toMatchObject({ id: "right", pinned: true });
  });

  it("recognizes a content pane after its tool tabs are reordered", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [{ id: CENTER_GROUP, panels: [{ id: SESSION_PANEL_ID, component: "chat" }] }],
        [
          {
            id: "group-1",
            panels: [
              { id: "files", component: "files" },
              { id: "browser:custom", component: "browser" },
            ],
          },
        ],
      ]),
    );
    expect(captured.columns[1]).toMatchObject({ id: "browser:custom" });
    expect(captured.columns[1].pinned).toBeUndefined();
  });
});

const PLUGIN_PANEL_ID = "plugin:kandev-plugin-prompt-history:prompt-history";
const PLUGIN_PANEL = {
  id: PLUGIN_PANEL_ID,
  component: "plugin-panel",
  title: "Prompt History",
  tabComponent: "pluginPanelTab",
  params: { pluginId: "kandev-plugin-prompt-history", panelKey: "prompt-history" },
};

/** Non-square VERTICAL root: a short Agent row above a taller right row split in three tabs-wide. */
function verticalLayout(): LayoutState {
  const files = { id: "group-files", panels: [panelOf("files")], activePanel: "files" };
  const plugin = { id: "group-plugin", panels: [PLUGIN_PANEL], activePanel: PLUGIN_PANEL_ID };
  const changes = { id: "group-changes", panels: [panelOf("changes")], activePanel: "changes" };
  const tree = {
    type: "branch" as const,
    size: 400,
    children: [
      { type: "leaf" as const, size: 200, group: files },
      { type: "leaf" as const, size: 400, group: changes },
      { type: "leaf" as const, size: 600, group: plugin },
    ],
  };
  return {
    rootOrientation: "VERTICAL",
    columns: [
      {
        id: "center",
        width: 300,
        groups: [{ id: "group-center", panels: [panelOf("chat")], activePanel: "chat" }],
      },
      // Historical snapshots carry horizontal pin metadata on this row.
      {
        id: "right",
        pinned: true,
        width: 400,
        maxWidth: 120,
        groups: [files, changes, plugin],
        tree,
      },
    ],
  };
}

function panelOf(id: string) {
  return { id, component: id, title: id };
}

type GridNode = { type: string; size: number; data: unknown };

describe("toSerializedDockview — VERTICAL root axis", () => {
  /** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
  it("sizes root rows by height and nested columns by width, ignoring horizontal pinning", () => {
    const serialized = toSerializedDockview(
      verticalLayout(),
      1400,
      700,
      new Map([["right", 250]]),
    ) as unknown as {
      grid: { orientation: string; root: GridNode; width: number; height: number };
    };

    const { root } = serialized.grid;
    const rows = root.data as GridNode[];
    expect(serialized.grid.orientation).toBe("VERTICAL");
    expect(root.size).toBe(1400);
    expect(rows.map((row) => row.size)).toEqual([300, 400]);
    const nested = rows[1].data as GridNode[];
    expect(rows[1].size).toBe(400);
    expect(nested.map((node) => node.size)).toEqual([233, 467, 700]);
  });

  it("scales the complete root row extents when the host height changes", () => {
    const serialized = toSerializedDockview(verticalLayout(), 1400, 350, new Map()) as unknown as {
      grid: { root: GridNode };
    };

    expect((serialized.grid.root.data as GridNode[]).map((row) => row.size)).toEqual([150, 200]);
  });

  it("keeps the horizontal pin policy for explicit HORIZONTAL and legacy roots", () => {
    for (const rootOrientation of ["HORIZONTAL", undefined] as const) {
      const layout = verticalLayout();
      if (rootOrientation) layout.rootOrientation = rootOrientation;
      else delete layout.rootOrientation;
      layout.columns[1].maxWidth = undefined;

      const serialized = toSerializedDockview(
        layout,
        1400,
        700,
        new Map([["right", 250]]),
      ) as unknown as {
        grid: { root: GridNode };
      };

      expect((serialized.grid.root.data as GridNode[]).map((column) => column.size)).toEqual([
        1150, 250,
      ]);
    }
  });
});

describe("VERTICAL root through a real Dockview", () => {
  function mountDockview(width: number, height: number): DockviewApi {
    const element = document.createElement("div");
    document.body.appendChild(element);
    const api = createDockview(element, {
      createComponent: () => ({ element: document.createElement("div"), init() {}, dispose() {} }),
    } as never);
    api.layout(width, height);
    return api;
  }

  /** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
  it("round-trips unequal root-axis proportions and nested widths without inferring a pin", () => {
    const api = mountDockview(1400, 700);
    api.fromJSON(toSerializedDockview(verticalLayout(), 1400, 700, new Map([["right", 250]])));

    const captured = fromDockviewApi(api);

    expect(captured.rootOrientation).toBe("VERTICAL");
    expect(captured.columns.map((column) => column.width)).toEqual([300, 400]);
    expect(captured.columns.map((column) => column.pinned)).toEqual([undefined, undefined]);
    expect(captured.columns[1].tree).toMatchObject({
      type: "branch",
      children: [
        { size: 233, group: { panels: [{ id: "files" }] } },
        { size: 467, group: { panels: [{ id: "changes" }] } },
        { size: 700, group: { panels: [{ id: PLUGIN_PANEL_ID }], activePanel: PLUGIN_PANEL_ID } },
      ],
    });
    api.dispose();
  });

  it("still infers the pinned right column for a HORIZONTAL root", () => {
    const api = mountDockview(1400, 700);
    const layout = verticalLayout();
    layout.rootOrientation = "HORIZONTAL";
    api.fromJSON(toSerializedDockview(layout, 1400, 700, new Map()));

    expect(fromDockviewApi(api).columns[1].pinned).toBe(true);
    api.dispose();
  });
});

describe("filterEphemeral — layout metadata", () => {
  /** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
  it.each(["VERTICAL", "HORIZONTAL"] as const)(
    "keeps an explicit %s root orientation",
    (rootOrientation) => {
      const layout = verticalLayout();
      layout.rootOrientation = rootOrientation;

      const filtered = filterEphemeral(layout);

      expect(filtered.rootOrientation).toBe(rootOrientation);
      expect(filtered.columns[1].tree).toMatchObject({
        children: [{}, {}, { group: { panels: [PLUGIN_PANEL] } }],
      });
    },
  );

  it("does not invent an orientation for a legacy snapshot", () => {
    const layout = verticalLayout();
    delete layout.rootOrientation;

    expect("rootOrientation" in filterEphemeral(layout)).toBe(false);
  });
});
