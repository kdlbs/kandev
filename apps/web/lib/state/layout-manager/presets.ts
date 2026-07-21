import type { LayoutState } from "./types";
import { CENTER_GROUP, RIGHT_TOP_GROUP, RIGHT_BOTTOM_GROUP, panel } from "./constants";

export function defaultLayout(): LayoutState {
  return {
    columns: [
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
