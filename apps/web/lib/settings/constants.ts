export const STORAGE_KEYS = {
  BACKEND_URL: "kandev.settings.backendUrl",
  ONBOARDING_COMPLETED: "kandev.onboarding.completed",
  /** Last settings page opened on this device; where bare `/settings` resolves. */
  LAST_SETTINGS_PATH: "kandev.settings.lastPath",
  /** How the settings menu renders on this device: flat, accordion, persistent. */
  SETTINGS_MENU_MODE: "kandev.settings.menuMode",
  /** Branch keys left open in the persistent tree on this device. */
  SETTINGS_MENU_EXPANDED: "kandev.settings.menuExpanded",
  /** Whether chat text, new items, and scrolling animate on this device. */
  CHAT_ANIMATIONS: "kandev.settings.chatAnimations",
  /** Whether agent rich-output line and bar charts animate on this device. */
  RICH_OUTPUT_ANIMATIONS: "kandev.settings.richOutputAnimations",
} as const;

export const DEFAULT_BACKEND_URL = "http://localhost:38429";

// Kanban Preview Panel Settings
export const PREVIEW_PANEL = {
  // Minimum width of the preview panel in pixels at a fine pointer (mouse/trackpad).
  // Also the storage floor: the chosen width is never persisted below this.
  MIN_WIDTH_PX: 320,

  // Minimum rendered width of the preview panel in pixels at a coarse pointer (touch),
  // where the header's fixed-width controls and step indicator need a larger hit area.
  COARSE_MIN_WIDTH_PX: 380,

  // Default width of the preview panel in pixels when first opened
  DEFAULT_WIDTH_PX: 500,

  // Maximum width of the preview panel in viewport width percentage (prevents covering entire screen)
  MAX_WIDTH_VW: 95,

  // Minimum width of the kanban board as a percentage of viewport before the panel switches to floating mode
  MIN_KANBAN_WIDTH_PERCENT: 50,
} as const;

// Preview header step indicator minimum width: a content floor for the
// marker and step count (and, at a coarse pointer, the disclosure cue),
// independent of the step name — which truncates to 0 first. `min-width:
// min-content` cannot express this floor here: the name span's `white-space:
// nowrap` (from `truncate`) makes its own min-content size its full,
// unbroken text width, which then dominates the indicator's computed
// min-content. These are the rounded worst-case (two-digit count) values for
// the in-scope 10-99 step budget; see the system design's Header layout
// section.
export const PREVIEW_HEADER_INDICATOR = {
  MIN_WIDTH_PX: 68,
  COARSE_MIN_WIDTH_PX: 88,
} as const;
