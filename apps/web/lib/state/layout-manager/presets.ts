import type { LayoutState } from "./types";
import {
  LAYOUT_SIDEBAR_MAX_PX,
  LAYOUT_RIGHT_MAX_PX,
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  panel,
} from "./constants";

export function defaultLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
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
        maxWidth: LAYOUT_RIGHT_MAX_PX,
        groups: [
          { id: RIGHT_TOP_GROUP, panels: [panel("files"), panel("changes")] },
          { id: RIGHT_BOTTOM_GROUP, panels: [panel("terminal-default")] },
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
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
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
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
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
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
        groups: [{ id: SIDEBAR_GROUP, panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ id: CENTER_GROUP, panels: [panel("chat")] }],
      },
      {
        id: "right",
        maxWidth: LAYOUT_RIGHT_MAX_PX,
        groups: [{ panels: [panel("vscode")] }],
      },
    ],
  };
}

export type BuiltInPreset = "default" | "plan" | "preview" | "vscode";

const PRESET_MAP: Record<BuiltInPreset, () => LayoutState> = {
  default: defaultLayout,
  plan: planLayout,
  preview: previewLayout,
  vscode: vscodeLayout,
};

export function getPresetLayout(preset: BuiltInPreset): LayoutState {
  return PRESET_MAP[preset]();
}
