import type { LayoutState } from "./types";
import { LAYOUT_SIDEBAR_MAX_PX, LAYOUT_RIGHT_MAX_PX, panel } from "./constants";

export function defaultLayout(): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        pinned: true,
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
        groups: [{ panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ panels: [panel("chat")] }],
      },
      {
        id: "right",
        pinned: true,
        maxWidth: LAYOUT_SIDEBAR_MAX_PX,
        groups: [
          { panels: [panel("changes"), panel("files")] },
          { panels: [panel("terminal-default")] },
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
        groups: [{ panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ panels: [panel("chat")] }],
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
        groups: [{ panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ panels: [panel("chat")] }],
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
        groups: [{ panels: [panel("sidebar")] }],
      },
      {
        id: "center",
        groups: [{ panels: [panel("chat")] }],
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
