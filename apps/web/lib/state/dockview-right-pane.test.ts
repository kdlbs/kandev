import { describe, expect, it } from "vitest";
import type { LayoutColumn, LayoutState } from "./layout-manager/types";
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

describe("contextual right-pane recovery", () => {
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
});
