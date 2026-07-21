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
  SIDEBAR_GROUP,
  CENTER_GROUP,
  RIGHT_TOP_GROUP,
  RIGHT_BOTTOM_GROUP,
  TERMINAL_DEFAULT_ID,
  REUSABLE_PANEL_IDS,
  SIDEBAR_LOCK,
  KNOWN_PANEL_IDS,
  PANEL_REGISTRY,
  panel,
} from "./constants";
export type { ReusablePanelId } from "./constants";

// Runtime caps for pinned columns
export {
  computeSidebarMaxPx,
  computeRightMaxPx,
  computePinnedMaxPxFor,
  LAYOUT_PINNED_MIN_PX,
} from "./caps";

// Pinned-column target widths (enforced after every layout-change)
export {
  setPinnedTarget,
  getPinnedTarget,
  clearPinnedTarget,
  clearAllPinnedTargets,
} from "./pinned-targets";

// Presets
export {
  defaultLayout,
  compactLayout,
  planLayout,
  previewLayout,
  getPresetLayout,
} from "./presets";
export type { BuiltInPreset } from "./presets";

// Sizing
export { computeColumnWidths, computeGroupHeights, getPinnedWidth } from "./sizing";

// Serializer
export { toSerializedDockview, fromDockviewApi, filterEphemeral } from "./serializer";

// Session panel normalization for reusable saved layouts
export {
  isSessionChatPanel,
  normalizeReusableSessionPanels,
  materializeReusableChatPanel,
} from "./session-panels";

// Applier
export {
  applyLayout,
  getRootSplitview,
  resolveGroupIds,
  isCenterCandidateGroupId,
} from "./applier";
export type { LayoutGroupIds } from "./applier";

// Merger
export { mergeCurrentPanelsIntoPreset } from "./merger";

// Comparator
export { layoutStructuresMatch, savedLayoutMatchesLive } from "./comparator";

// Intent
export {
  INTENT_COMPACT,
  INTENT_PLAN,
  INTENT_PR_REVIEW,
  injectIntentPanels,
  applyActivePanelOverrides,
  resolveNamedIntent,
} from "./intent";
