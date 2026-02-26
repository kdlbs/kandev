// Types
export type {
  LayoutState,
  LayoutColumn,
  LayoutGroup,
  LayoutPanel,
  LayoutNode,
  LayoutLeafNode,
  LayoutBranchNode,
  LayoutIntent,
  LayoutIntentPanel,
} from "./types";

// Constants
export {
  LAYOUT_SIDEBAR_RATIO,
  LAYOUT_RIGHT_RATIO,
  LAYOUT_SIDEBAR_MAX_PX,
  LAYOUT_RIGHT_MAX_PX,
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  TERMINAL_DEFAULT_ID,
  SIDEBAR_LOCK,
  KNOWN_PANEL_IDS,
  PANEL_REGISTRY,
  panel,
} from "./constants";

// Presets
export { defaultLayout, planLayout, previewLayout, getPresetLayout } from "./presets";
export type { BuiltInPreset } from "./presets";

// Sizing
export { computeColumnWidths, computeGroupHeights, getPinnedWidth } from "./sizing";

// Serializer
export { toSerializedDockview, fromDockviewApi, filterEphemeral } from "./serializer";

// Applier
export { applyLayout, getRootSplitview, resolveGroupIds } from "./applier";
export type { LayoutGroupIds } from "./applier";

// Merger
export { mergeCurrentPanelsIntoPreset } from "./merger";

// Intent
export { injectIntentPanels, applyActivePanelOverrides } from "./intent";
