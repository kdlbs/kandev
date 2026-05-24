import type { LayoutColumn, LayoutState } from "./types";
import {
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  panel,
} from "./constants";

const COMPACT_SIDEBAR_WIDTH = 220;
// Compact preset intentionally caps the sidebar tight (small toolbar look),
// so it overrides the runtime cap rather than inheriting it.
const COMPACT_SIDEBAR_MAX_PX = 260;

export function defaultLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "right",
        pinned: true,
        width: 350,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [panel("files"), panel("changes")] },
          { id: RIGHT_BOTTOM_GROUP, panels: [panel("terminal-default")] },
        ],
      },
    ],
  };
}

export function compactLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        width: COMPACT_SIDEBAR_WIDTH,
        maxWidth: COMPACT_SIDEBAR_MAX_PX,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [
          {
            id: CENTER_GROUP,
            panels: [panel("chat"), panel("files"), panel("changes"), panel("terminal-default")],
          },
        ],
      },
    ],
  };
}

export function planLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "plan",
        groups: [{ panels: [panel("plan")] }],
      },
    ],
  };
}

export function previewLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "preview",
        groups: [{ panels: [panel("browser")] }],
      },
    ],
  };
}

export function vscodeLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "right",
        groups: [{ panels: [panel("vscode")] }],
      },
    ],
  };
}

export type BuiltInPreset = "default" | "compact" | "plan" | "preview" | "vscode";

const PRESET_MAP: Record<BuiltInPreset, () => LayoutState> = {
  default: defaultLayout,
  compact: compactLayout,
  plan: planLayout,
  preview: previewLayout,
  vscode: vscodeLayout,
};

export function getPresetLayout(preset: BuiltInPreset): LayoutState {
  return PRESET_MAP[preset]();
}

export function getPresetSidebarColumn(preset: BuiltInPreset): LayoutColumn {
  return (
    getPresetLayout(preset).columns.find((column) => column.id === "sidebar") ??
    defaultLayout().columns[0]
  );
}
