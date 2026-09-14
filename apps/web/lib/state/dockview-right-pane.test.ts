import { describe, expect, it } from "vitest";
import type { LayoutColumn, LayoutState } from "./layout-manager/types";
import { toSerializedDockview } from "./layout-manager/serializer";
import {
  captureRightPane,
  getRightPaneToggleState,
  readHiddenRightPane,
  restoreRightPane,
  stripHiddenRightPaneMetadata,
  withHiddenRightPaneMetadata,
} from "./dockview-right-pane";

const panel = (id: string, component = id, params?: Record<string, unknown>) => ({
  id,
  component,
  title: id,
  ...(params ? { params } : {}),
});

function centerColumn(): LayoutColumn {
  return {
    id: "center",
    groups: [
      {
        id: "center-group",
        activePanel: "session:session-a",
        panels: [panel("session:session-a", "chat"), panel("changes")],
      },
    ],
  };
}

function nestedRightColumn(): LayoutColumn {
  const top = {
    id: "right-top",
    activePanel: "browser",
    panels: [panel("browser", "browser", { url: "https://example.test" }), panel("plan")],
  };
  const bottom = {
    id: "right-bottom",
    activePanel: "terminal",
    panels: [panel("terminal", "terminal", { terminalId: "terminal-a" })],
  };
  return {
    id: "browser-region",
    width: 420,
    groups: [top, bottom],
    tree: {
      type: "branch",
      size: 420,
      children: [
        { type: "leaf", size: 280, group: top },
        { type: "leaf", size: 140, group: bottom },
      ],
    },
  };
}

function visibleLayout(columns: LayoutColumn[]): LayoutState {
  return { columns, rootOrientation: "HORIZONTAL" };
}

describe("contextual right-pane selection and recovery", () => {
  it("captures the actual final region and restores its nested tree, tabs, and parameters", () => {
    // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.8
    // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.9
    const original = visibleLayout([centerColumn(), nestedRightColumn()]);
    const captured = captureRightPane(original);

    expect(captured?.layout.columns.map((column) => column.id)).toEqual(["center"]);
    expect(captured?.hiddenRightPane.column).toEqual(original.columns[1]);

    const changedRemaining = {
      ...captured!.layout,
      columns: [
        {
          ...captured!.layout.columns[0],
          groups: [
            {
              ...captured!.layout.columns[0].groups[0],
              activePanel: "changes",
            },
          ],
        },
      ],
    };
    const restored = restoreRightPane(changedRemaining, captured!.hiddenRightPane);

    expect(restored?.columns[0]?.groups[0]?.activePanel).toBe("changes");
    expect(restored?.columns[1]).toEqual(original.columns[1]);
    expect(restored?.columns[1]?.tree).toEqual(nestedRightColumn().tree);
  });

  it("selects only the outermost final region in a three-column layout", () => {
    // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.8
    const layout = visibleLayout([
      centerColumn(),
      { id: "plan", groups: [{ panels: [panel("plan")] }] },
      { id: "browser", groups: [{ panels: [panel("browser")] }] },
    ]);
    const captured = captureRightPane(layout);

    expect(captured?.hiddenRightPane.sourceColumnId).toBe("browser");
    expect(captured?.layout.columns.map((column) => column.id)).toEqual(["center", "plan"]);
  });

  it("does not expose a target for a single region, a vertical split, or an Agent region", () => {
    // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.3
    // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.8
    const single = visibleLayout([centerColumn()]);
    const vertical = {
      ...visibleLayout([centerColumn(), { id: "stack", groups: [{ panels: [panel("files")] }] }]),
      rootOrientation: "VERTICAL" as const,
    };
    const agentOnRight = visibleLayout([
      centerColumn(),
      { id: "other", groups: [{ panels: [panel("session:session-b", "chat")] }] },
    ]);

    expect(getRightPaneToggleState(single, null)).toEqual({
      available: false,
      visible: false,
      hidden: false,
    });
    expect(getRightPaneToggleState(vertical, null).available).toBe(false);
    expect(getRightPaneToggleState(agentOnRight, null).available).toBe(false);
  });
});

it("deduplicates panels reopened in the remaining layout without losing the retained subtree", () => {
  // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.10
  const original = visibleLayout([
    centerColumn(),
    {
      id: "right",
      groups: [{ id: "right-group", panels: [panel("files"), panel("shared")] }],
    },
  ]);
  const captured = captureRightPane(original)!;
  const current = visibleLayout([
    {
      ...centerColumn(),
      groups: [
        {
          ...centerColumn().groups[0],
          panels: [...centerColumn().groups[0].panels, panel("shared")],
        },
      ],
    },
  ]);

  const restored = restoreRightPane(current, captured.hiddenRightPane);

  expect(restored?.columns[1]?.groups[0]?.panels.map((item) => item.id)).toEqual(["files"]);
  const ids = restored!.columns.flatMap((column) =>
    column.groups.flatMap((group) => group.panels.map((item) => item.id)),
  );
  expect(new Set(ids).size).toBe(ids.length);
});

it("rejects recovery metadata that cannot produce a safe serialized layout", () => {
  const captured = captureRightPane(visibleLayout([centerColumn(), nestedRightColumn()]))!;
  const malformedGroupId = JSON.parse(JSON.stringify(captured.hiddenRightPane)) as {
    column: { groups: Array<{ id?: unknown }> };
  };
  malformedGroupId.column.groups[0].id = 42;

  const malformedTree = {
    ...captured.hiddenRightPane,
    column: {
      ...captured.hiddenRightPane.column,
      tree: {
        ...captured.hiddenRightPane.column.tree!,
        size: Number.NaN,
      },
    },
  };

  expect(readHiddenRightPane({ kandevHiddenRightPane: malformedGroupId })).toBeNull();
  expect(readHiddenRightPane({ kandevHiddenRightPane: malformedTree })).toBeNull();

  const current = captured.layout;
  expect(restoreRightPane(current, malformedTree)).toBeNull();
  expect(current.columns.map((column) => column.id)).toEqual(["center"]);
});

it("keeps the split axis when deduplication removes a nested sibling", () => {
  const browserGroup = { id: "browser-group", panels: [panel("browser")] };
  const planGroup = { id: "plan-group", panels: [panel("plan")] };
  const terminalGroup = {
    id: "terminal-group",
    activePanel: "terminal",
    panels: [panel("terminal", "terminal")],
  };
  const original = visibleLayout([
    {
      ...centerColumn(),
      groups: [
        ...centerColumn().groups,
        { id: "reopened-terminal", panels: [panel("terminal", "terminal")] },
      ],
    },
    {
      id: "browser-region",
      width: 420,
      groups: [browserGroup, planGroup, terminalGroup],
      tree: {
        type: "branch",
        size: 420,
        children: [
          {
            type: "branch",
            size: 280,
            children: [
              { type: "leaf", size: 140, group: browserGroup },
              { type: "leaf", size: 140, group: planGroup },
            ],
          },
          { type: "leaf", size: 140, group: terminalGroup },
        ],
      },
    },
  ]);
  const captured = captureRightPane(original)!;
  const restored = restoreRightPane(captured.layout, captured.hiddenRightPane);

  expect(restored?.columns[1]?.tree).toMatchObject({ type: "branch" });
  const tree = restored?.columns[1]?.tree;
  expect(tree?.type).toBe("branch");
  if (!tree || tree.type !== "branch") return;
  expect(tree.children).toHaveLength(1);
  expect(tree.children[0]?.type).toBe("branch");

  const serialized = toSerializedDockview(restored!, 1200, 800, new Map());
  const root = serialized as unknown as {
    grid: {
      root: {
        data: Array<
          { type: "leaf"; data: { views: string[] } } | { type: "branch"; data: unknown[] }
        >;
      };
    };
  };
  const rightNode = root.grid.root.data[1];
  expect(rightNode?.type).toBe("branch");
  if (!rightNode || rightNode.type !== "branch") return;
  expect(rightNode.data).toHaveLength(1);
  const survivingSplit = rightNode.data[0] as {
    type: "branch";
    data: Array<{ type: "leaf"; data: { views: string[] } }>;
  };
  expect(survivingSplit.type).toBe("branch");
  expect(survivingSplit.data.map((child) => child.data.views)).toEqual([["browser"], ["plan"]]);
  expect(survivingSplit.data).not.toContainEqual({ type: "leaf", data: { views: [] } });
});

it("keeps generated group IDs distinct from live explicit IDs", () => {
  const original = visibleLayout([
    {
      ...centerColumn(),
      groups: [{ ...centerColumn().groups[0], id: "group-1" }],
    },
    { id: "plan", groups: [{ panels: [panel("plan")] }] },
  ]);
  const captured = captureRightPane(original)!;
  const restored = restoreRightPane(captured.layout, captured.hiddenRightPane)!;
  const serialized = toSerializedDockview(restored, 1200, 800, new Map()) as unknown as {
    grid: {
      root: {
        data: Array<
          | { type: "leaf"; data: { id: string } }
          | { type: "branch"; data: Array<{ type: "leaf"; data: { id: string } }> }
        >;
      };
    };
  };
  const ids = serialized.grid.root.data.flatMap((node) =>
    node.type === "leaf" ? [node.data.id] : node.data.map((child) => child.data.id),
  );

  expect(ids).toEqual(["group-1", "group-2"]);
  expect(new Set(ids).size).toBe(ids.length);
});

it("restores non-pinned pane width across repeated hide and show cycles", () => {
  let current = visibleLayout([
    { ...centerColumn(), width: 600 },
    { id: "plan", width: 600, groups: [{ id: "plan-group", panels: [panel("plan")] }] },
  ]);

  for (let cycle = 0; cycle < 3; cycle++) {
    const captured = captureRightPane(current)!;
    current = restoreRightPane(captured.layout, captured.hiddenRightPane, {
      totalWidth: 1200,
      pinnedWidths: new Map(),
    })!;
    expect(current.columns.map((column) => column.width)).toEqual([600, 600]);

    const serialized = toSerializedDockview(current, 1200, 800, new Map());
    const root = serialized as unknown as {
      grid: { root: { data: Array<{ size: number }> } };
    };
    expect(root.grid.root.data.map((node) => node.size)).toEqual([600, 600]);
  }
});

it("clamps a retained non-pinned pane to viewport space while preserving live proportions", () => {
  const original = visibleLayout([
    { ...centerColumn(), width: 400 },
    { id: "preview", width: 200, groups: [{ panels: [panel("preview")] }] },
    { id: "plan", width: 600, groups: [{ panels: [panel("plan")] }] },
  ]);
  const captured = captureRightPane(original)!;
  const edited = {
    ...captured.layout,
    columns: captured.layout.columns.map((column, index) => ({
      ...column,
      width: index === 0 ? 800 : 200,
    })),
  };

  const narrow = restoreRightPane(edited, captured.hiddenRightPane, {
    totalWidth: 900,
    pinnedWidths: new Map(),
  })!;
  expect(narrow.columns.map((column) => column.width)).toEqual([288, 72, 540]);

  const wide = restoreRightPane(edited, captured.hiddenRightPane, {
    totalWidth: 1600,
    pinnedWidths: new Map(),
  })!;
  expect(wide.columns.map((column) => column.width)).toEqual([800, 200, 600]);
});

it("keeps a retained pane authoritative until its layout context is invalid", () => {
  // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.5
  // @covers AC-UI-RIGHT-PANEL-VISIBILITY-001.10
  const captured = captureRightPane(
    visibleLayout([centerColumn(), { id: "plan", groups: [{ panels: [panel("plan")] }] }]),
  )!;

  expect(getRightPaneToggleState(captured.layout, captured.hiddenRightPane)).toEqual({
    available: true,
    visible: false,
    hidden: true,
  });
  expect(
    getRightPaneToggleState(
      visibleLayout([{ id: "unrelated", groups: [{ panels: [panel("files")] }] }]),
      captured.hiddenRightPane,
    ),
  ).toEqual({ available: false, visible: false, hidden: false });

  const reopened = visibleLayout([
    centerColumn(),
    { id: "reopened", groups: [{ panels: [panel("plan")] }] },
  ]);
  expect(getRightPaneToggleState(reopened, captured.hiddenRightPane)).toEqual({
    available: true,
    visible: true,
    hidden: false,
  });
});

it("round-trips and strips environment recovery metadata", () => {
  const captured = captureRightPane(
    visibleLayout([centerColumn(), { id: "plan", groups: [{ panels: [panel("plan")] }] }]),
  )!;
  const serialized = withHiddenRightPaneMetadata(
    { grid: { root: { type: "branch" } }, panels: {} },
    captured.hiddenRightPane,
  );

  expect(readHiddenRightPane(serialized)).toEqual(captured.hiddenRightPane);
  expect(stripHiddenRightPaneMetadata(serialized)).toEqual({
    grid: { root: { type: "branch" } },
    panels: {},
  });
  expect(readHiddenRightPane({ kandevHiddenRightPane: { version: 99 } })).toBeNull();
});
